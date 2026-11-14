package gitprovider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type gitlab struct{ *client }

// project is the URL-encoded project path (group/subgroup/name).
func project(full string) (string, error) {
	if _, _, err := splitFull(full); err != nil {
		return "", err
	}
	return strings.ReplaceAll(url.PathEscape(full), "/", "%2F"), nil
}

type glProject struct {
	ID                int64  `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
	Description       string `json:"description"`
	DefaultBranch     string `json:"default_branch"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"web_url"`
	SSHURL            string `json:"ssh_url_to_repo"`
	HTTPURL           string `json:"http_url_to_repo"`
	LastActivityAt    string `json:"last_activity_at"`
	Permissions       struct {
		ProjectAccess *struct {
			AccessLevel int `json:"access_level"`
		} `json:"project_access"`
		GroupAccess *struct {
			AccessLevel int `json:"access_level"`
		} `json:"group_access"`
	} `json:"permissions"`
}

func (p glProject) repo() Repo {
	level := 0
	if p.Permissions.ProjectAccess != nil {
		level = p.Permissions.ProjectAccess.AccessLevel
	}
	if p.Permissions.GroupAccess != nil && p.Permissions.GroupAccess.AccessLevel > level {
		level = p.Permissions.GroupAccess.AccessLevel
	}
	return Repo{ID: strconv.FormatInt(p.ID, 10), FullName: p.PathWithNamespace, Description: p.Description, DefaultBranch: p.DefaultBranch,
		Private: p.Visibility != "public", HTMLURL: p.WebURL, SSHURL: p.SSHURL, CloneURL: p.HTTPURL,
		CanAdmin: level >= 40, UpdatedAt: p.LastActivityAt} // 40 = Maintainer
}

func (g *gitlab) CurrentUser(ctx context.Context) (*User, error) {
	var u struct {
		Username  string `json:"username"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if _, err := g.do(ctx, "GET", "/user", nil, &u); err != nil {
		return nil, err
	}
	return &User{Login: u.Username, Name: u.Name, AvatarURL: u.AvatarURL}, nil
}

func (g *gitlab) ListRepos(ctx context.Context, query string) ([]Repo, error) {
	var out []Repo
	for page := 1; page <= 5; page++ {
		var ps []glProject
		path := fmt.Sprintf("/projects?membership=true&per_page=100&page=%d&order_by=last_activity_at&min_access_level=30", page)
		if q := strings.TrimSpace(query); q != "" {
			path += "&search=" + url.QueryEscape(q)
		}
		if _, err := g.do(ctx, "GET", path, nil, &ps); err != nil {
			return nil, err
		}
		for _, p := range ps {
			out = append(out, p.repo())
		}
		if len(ps) < 100 {
			break
		}
	}
	return out, nil
}

func (g *gitlab) GetRepo(ctx context.Context, full string) (*Repo, error) {
	id, err := project(full)
	if err != nil {
		return nil, err
	}
	var p glProject
	if _, err := g.do(ctx, "GET", "/projects/"+id, nil, &p); err != nil {
		return nil, err
	}
	r := p.repo()
	return &r, nil
}

func (g *gitlab) ListBranches(ctx context.Context, full string) ([]Branch, error) {
	id, err := project(full)
	if err != nil {
		return nil, err
	}
	var out []Branch
	for page := 1; page <= 5; page++ {
		var bs []struct {
			Name   string `json:"name"`
			Commit struct {
				ID string `json:"id"`
			} `json:"commit"`
		}
		if _, err := g.do(ctx, "GET", fmt.Sprintf("/projects/%s/repository/branches?per_page=100&page=%d", id, page), nil, &bs); err != nil {
			return nil, err
		}
		for _, b := range bs {
			out = append(out, Branch{Name: b.Name, SHA: b.Commit.ID})
		}
		if len(bs) < 100 {
			break
		}
	}
	return out, nil
}

func (g *gitlab) GetCommit(ctx context.Context, full, ref string) (*Commit, error) {
	id, err := project(full)
	if err != nil {
		return nil, err
	}
	var c struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		AuthorName string `json:"author_name"`
		WebURL     string `json:"web_url"`
	}
	if _, err := g.do(ctx, "GET", "/projects/"+id+"/repository/commits/"+url.PathEscape(ref), nil, &c); err != nil {
		return nil, err
	}
	return &Commit{SHA: c.ID, Message: firstLine(c.Title), Author: c.AuthorName, URL: c.WebURL}, nil
}

func (g *gitlab) AddDeployKey(ctx context.Context, full, title, key string) (string, error) {
	id, err := project(full)
	if err != nil {
		return "", err
	}
	var k struct {
		ID int64 `json:"id"`
	}
	if _, err := g.do(ctx, "POST", "/projects/"+id+"/deploy_keys", map[string]any{"title": title, "key": key, "can_push": false}, &k); err != nil {
		return "", err
	}
	return strconv.FormatInt(k.ID, 10), nil
}

func (g *gitlab) DeleteDeployKey(ctx context.Context, full, keyID string) error {
	id, err := project(full)
	if err != nil {
		return err
	}
	_, err = g.do(ctx, "DELETE", "/projects/"+id+"/deploy_keys/"+url.PathEscape(keyID), nil, nil)
	return ignoreNotFound(err)
}

func (g *gitlab) CreateWebhook(ctx context.Context, full, hookURL, secret string, verifySSL bool) (string, error) {
	id, err := project(full)
	if err != nil {
		return "", err
	}
	var h struct {
		ID int64 `json:"id"`
	}
	body := map[string]any{"url": hookURL, "push_events": true, "token": secret, "enable_ssl_verification": verifySSL}
	if _, err := g.do(ctx, "POST", "/projects/"+id+"/hooks", body, &h); err != nil {
		return "", err
	}
	return strconv.FormatInt(h.ID, 10), nil
}

func (g *gitlab) DeleteWebhook(ctx context.Context, full, hookID string) error {
	id, err := project(full)
	if err != nil {
		return err
	}
	_, err = g.do(ctx, "DELETE", "/projects/"+id+"/hooks/"+url.PathEscape(hookID), nil, nil)
	return ignoreNotFound(err)
}

func (g *gitlab) WebhookExists(ctx context.Context, full, hookID string) (bool, error) {
	id, err := project(full)
	if err != nil {
		return false, err
	}
	_, err = g.do(ctx, "GET", "/projects/"+id+"/hooks/"+url.PathEscape(hookID), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// GitLab has no deployments API for external tools that shows on commits
// the same way; a commit status does.
func (g *gitlab) StartDeployment(ctx context.Context, full, sha, ref, description string) (string, error) {
	return sha, g.UpdateDeployment(ctx, full, sha, sha, StatePending, "", description)
}

func (g *gitlab) UpdateDeployment(ctx context.Context, full, handle, sha string, state State, targetURL, description string) error {
	id, err := project(full)
	if err != nil {
		return err
	}
	st := map[State]string{StatePending: "pending", StateInProgress: "running", StateSuccess: "success", StateFailure: "failed", StateError: "failed"}[state]
	body := map[string]any{"state": st, "name": "SigardaPanel deploy", "description": truncate(description, 140)}
	if targetURL != "" {
		body["target_url"] = targetURL
	}
	_, err = g.do(ctx, "POST", "/projects/"+id+"/statuses/"+url.PathEscape(sha), body, nil)
	return err
}

func (g *gitlab) KnownHosts(context.Context) []string { return nil }
