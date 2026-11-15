package gitprovider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

type gitea struct{ *client }

type gtRepo struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	HTMLURL       string `json:"html_url"`
	SSHURL        string `json:"ssh_url"`
	CloneURL      string `json:"clone_url"`
	UpdatedAt     string `json:"updated_at"`
	Permissions   struct {
		Admin bool `json:"admin"`
	} `json:"permissions"`
}

func (r gtRepo) repo() Repo {
	return Repo{ID: strconv.FormatInt(r.ID, 10), FullName: r.FullName, Description: r.Description, DefaultBranch: r.DefaultBranch,
		Private: r.Private, HTMLURL: r.HTMLURL, SSHURL: r.SSHURL, CloneURL: r.CloneURL, CanAdmin: r.Permissions.Admin, UpdatedAt: r.UpdatedAt}
}

func (g *gitea) CurrentUser(ctx context.Context) (*User, error) {
	var u struct {
		Login     string `json:"login"`
		FullName  string `json:"full_name"`
		AvatarURL string `json:"avatar_url"`
	}
	if _, err := g.do(ctx, "GET", "/user", nil, &u); err != nil {
		return nil, err
	}
	return &User{Login: u.Login, Name: u.FullName, AvatarURL: u.AvatarURL}, nil
}

func (g *gitea) ListRepos(ctx context.Context, query string) ([]Repo, error) {
	var out []Repo
	for page := 1; page <= 10; page++ {
		var rs []gtRepo
		if _, err := g.do(ctx, "GET", fmt.Sprintf("/user/repos?limit=50&page=%d", page), nil, &rs); err != nil {
			return nil, err
		}
		for _, r := range rs {
			if matches(r.FullName, query) {
				out = append(out, r.repo())
			}
		}
		if len(rs) < 50 {
			break
		}
	}
	return out, nil
}

func (g *gitea) GetRepo(ctx context.Context, full string) (*Repo, error) {
	p, err := repoPath(full)
	if err != nil {
		return nil, err
	}
	var r gtRepo
	if _, err := g.do(ctx, "GET", "/repos/"+p, nil, &r); err != nil {
		return nil, err
	}
	repo := r.repo()
	return &repo, nil
}

func (g *gitea) ListBranches(ctx context.Context, full string) ([]Branch, error) {
	p, err := repoPath(full)
	if err != nil {
		return nil, err
	}
	var out []Branch
	for page := 1; page <= 10; page++ {
		var bs []struct {
			Name   string `json:"name"`
			Commit struct {
				ID string `json:"id"`
			} `json:"commit"`
		}
		if _, err := g.do(ctx, "GET", fmt.Sprintf("/repos/%s/branches?limit=50&page=%d", p, page), nil, &bs); err != nil {
			return nil, err
		}
		for _, b := range bs {
			out = append(out, Branch{Name: b.Name, SHA: b.Commit.ID})
		}
		if len(bs) < 50 {
			break
		}
	}
	return out, nil
}

func (g *gitea) GetCommit(ctx context.Context, full, ref string) (*Commit, error) {
	p, err := repoPath(full)
	if err != nil {
		return nil, err
	}
	var c struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"commit"`
	}
	if _, err := g.do(ctx, "GET", "/repos/"+p+"/git/commits/"+url.PathEscape(ref), nil, &c); err != nil {
		return nil, err
	}
	return &Commit{SHA: c.SHA, Message: firstLine(c.Commit.Message), Author: c.Commit.Author.Name, URL: c.HTMLURL}, nil
}

func (g *gitea) AddDeployKey(ctx context.Context, full, title, key string) (string, error) {
	p, err := repoPath(full)
	if err != nil {
		return "", err
	}
	var k struct {
		ID int64 `json:"id"`
	}
	if _, err := g.do(ctx, "POST", "/repos/"+p+"/keys", map[string]any{"title": title, "key": key, "read_only": true}, &k); err != nil {
		return "", err
	}
	return strconv.FormatInt(k.ID, 10), nil
}

func (g *gitea) DeleteDeployKey(ctx context.Context, full, id string) error {
	p, err := repoPath(full)
	if err != nil {
		return err
	}
	_, err = g.do(ctx, "DELETE", "/repos/"+p+"/keys/"+url.PathEscape(id), nil, nil)
	return ignoreNotFound(err)
}

func (g *gitea) CreateWebhook(ctx context.Context, full, hookURL, secret string, verifySSL bool) (string, error) {
	p, err := repoPath(full)
	if err != nil {
		return "", err
	}
	var h struct {
		ID int64 `json:"id"`
	}
	body := map[string]any{"type": "gitea", "active": true, "events": []string{"push"},
		"config": map[string]string{"url": hookURL, "content_type": "json", "secret": secret}}
	if _, err := g.do(ctx, "POST", "/repos/"+p+"/hooks", body, &h); err != nil {
		return "", err
	}
	return strconv.FormatInt(h.ID, 10), nil
}

func (g *gitea) DeleteWebhook(ctx context.Context, full, id string) error {
	p, err := repoPath(full)
	if err != nil {
		return err
	}
	_, err = g.do(ctx, "DELETE", "/repos/"+p+"/hooks/"+url.PathEscape(id), nil, nil)
	return ignoreNotFound(err)
}

func (g *gitea) WebhookExists(ctx context.Context, full, id string) (bool, error) {
	p, err := repoPath(full)
	if err != nil {
		return false, err
	}
	_, err = g.do(ctx, "GET", "/repos/"+p+"/hooks/"+url.PathEscape(id), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (g *gitea) StartDeployment(ctx context.Context, full, sha, ref, description string) (string, error) {
	return sha, g.UpdateDeployment(ctx, full, sha, sha, StatePending, "", description)
}

func (g *gitea) UpdateDeployment(ctx context.Context, full, handle, sha string, state State, targetURL, description string) error {
	p, err := repoPath(full)
	if err != nil {
		return err
	}
	st := map[State]string{StatePending: "pending", StateInProgress: "pending", StateSuccess: "success", StateFailure: "failure", StateError: "error"}[state]
	body := map[string]any{"state": st, "context": "SigardaPanel deploy", "description": truncate(description, 140)}
	if targetURL != "" {
		body["target_url"] = targetURL
	}
	_, err = g.do(ctx, "POST", "/repos/"+p+"/statuses/"+url.PathEscape(sha), body, nil)
	return err
}

func (g *gitea) KnownHosts(context.Context) []string { return nil }
