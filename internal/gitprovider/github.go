package gitprovider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type github struct{ *client }

type ghRepo struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	HTMLURL       string `json:"html_url"`
	SSHURL        string `json:"ssh_url"`
	CloneURL      string `json:"clone_url"`
	PushedAt      string `json:"pushed_at"`
	Permissions   struct {
		Admin bool `json:"admin"`
	} `json:"permissions"`
}

func (r ghRepo) repo() Repo {
	return Repo{ID: strconv.FormatInt(r.ID, 10), FullName: r.FullName, Description: r.Description, DefaultBranch: r.DefaultBranch,
		Private: r.Private, HTMLURL: r.HTMLURL, SSHURL: r.SSHURL, CloneURL: r.CloneURL, CanAdmin: r.Permissions.Admin, UpdatedAt: r.PushedAt}
}

func (g *github) CurrentUser(ctx context.Context) (*User, error) {
	var u struct {
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if _, err := g.do(ctx, "GET", "/user", nil, &u); err != nil {
		return nil, err
	}
	return &User{Login: u.Login, Name: u.Name, AvatarURL: u.AvatarURL}, nil
}

func (g *github) ListRepos(ctx context.Context, query string) ([]Repo, error) {
	var out []Repo
	for page := 1; page <= 5; page++ {
		var rs []ghRepo
		if _, err := g.do(ctx, "GET", fmt.Sprintf("/user/repos?per_page=100&page=%d&sort=pushed&affiliation=owner,collaborator,organization_member", page), nil, &rs); err != nil {
			return nil, err
		}
		for _, r := range rs {
			if matches(r.FullName, query) {
				out = append(out, r.repo())
			}
		}
		if len(rs) < 100 {
			break
		}
	}
	return out, nil
}

func (g *github) GetRepo(ctx context.Context, full string) (*Repo, error) {
	p, err := repoPath(full)
	if err != nil {
		return nil, err
	}
	var r ghRepo
	if _, err := g.do(ctx, "GET", "/repos/"+p, nil, &r); err != nil {
		return nil, err
	}
	repo := r.repo()
	return &repo, nil
}

func (g *github) ListBranches(ctx context.Context, full string) ([]Branch, error) {
	p, err := repoPath(full)
	if err != nil {
		return nil, err
	}
	var out []Branch
	for page := 1; page <= 5; page++ {
		var bs []struct {
			Name   string `json:"name"`
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		if _, err := g.do(ctx, "GET", fmt.Sprintf("/repos/%s/branches?per_page=100&page=%d", p, page), nil, &bs); err != nil {
			return nil, err
		}
		for _, b := range bs {
			out = append(out, Branch{Name: b.Name, SHA: b.Commit.SHA})
		}
		if len(bs) < 100 {
			break
		}
	}
	return out, nil
}

func (g *github) GetCommit(ctx context.Context, full, ref string) (*Commit, error) {
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
	if _, err := g.do(ctx, "GET", "/repos/"+p+"/commits/"+url.PathEscape(ref), nil, &c); err != nil {
		return nil, err
	}
	return &Commit{SHA: c.SHA, Message: firstLine(c.Commit.Message), Author: c.Commit.Author.Name, URL: c.HTMLURL}, nil
}

func (g *github) AddDeployKey(ctx context.Context, full, title, key string) (string, error) {
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

func (g *github) DeleteDeployKey(ctx context.Context, full, id string) error {
	p, err := repoPath(full)
	if err != nil {
		return err
	}
	_, err = g.do(ctx, "DELETE", "/repos/"+p+"/keys/"+url.PathEscape(id), nil, nil)
	return ignoreNotFound(err)
}

func (g *github) CreateWebhook(ctx context.Context, full, hookURL, secret string, verifySSL bool) (string, error) {
	p, err := repoPath(full)
	if err != nil {
		return "", err
	}
	var h struct {
		ID int64 `json:"id"`
	}
	body := map[string]any{"name": "web", "active": true, "events": []string{"push"},
		"config": map[string]any{"url": hookURL, "content_type": "json", "secret": secret, "insecure_ssl": map[bool]string{true: "0", false: "1"}[verifySSL]}}
	if _, err := g.do(ctx, "POST", "/repos/"+p+"/hooks", body, &h); err != nil {
		return "", err
	}
	return strconv.FormatInt(h.ID, 10), nil
}

func (g *github) DeleteWebhook(ctx context.Context, full, id string) error {
	p, err := repoPath(full)
	if err != nil {
		return err
	}
	_, err = g.do(ctx, "DELETE", "/repos/"+p+"/hooks/"+url.PathEscape(id), nil, nil)
	return ignoreNotFound(err)
}

func (g *github) WebhookExists(ctx context.Context, full, id string) (bool, error) {
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

func (g *github) StartDeployment(ctx context.Context, full, sha, ref, description string) (string, error) {
	p, err := repoPath(full)
	if err != nil {
		return "", err
	}
	var d struct {
		ID int64 `json:"id"`
	}
	body := map[string]any{"ref": sha, "environment": "production", "description": description,
		"auto_merge": false, "required_contexts": []string{}, "production_environment": true}
	if _, err := g.do(ctx, "POST", "/repos/"+p+"/deployments", body, &d); err != nil {
		return "", err
	}
	return strconv.FormatInt(d.ID, 10), nil
}

func (g *github) UpdateDeployment(ctx context.Context, full, handle, sha string, state State, targetURL, description string) error {
	p, err := repoPath(full)
	if err != nil {
		return err
	}
	body := map[string]any{"state": string(state), "description": truncate(description, 140), "auto_inactive": true}
	if targetURL != "" && state == StateSuccess {
		body["environment_url"] = targetURL
	}
	_, err = g.do(ctx, "POST", "/repos/"+p+"/deployments/"+url.PathEscape(handle)+"/statuses", body, nil)
	return err
}

func (g *github) KnownHosts(ctx context.Context) []string {
	var meta struct {
		SSHKeys []string `json:"ssh_keys"`
	}
	if _, err := g.do(ctx, "GET", "/meta", nil, &meta); err != nil {
		return nil
	}
	host := g.host
	var out []string
	for _, k := range meta.SSHKeys {
		if f := strings.Fields(k); len(f) >= 2 && (strings.HasPrefix(f[0], "ssh-") || strings.HasPrefix(f[0], "ecdsa-")) {
			out = append(out, host+" "+strings.TrimSpace(k))
		}
	}
	return out
}

func ignoreNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
