package gitprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type call struct {
	Method, Path, RawPath, Auth string
	Body                        map[string]any
}

func fakeServer(t *testing.T, routes map[string]any) (*httptest.Server, *[]call) {
	t.Helper()
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := call{Method: r.Method, Path: r.URL.Path, RawPath: r.URL.EscapedPath(), Auth: r.Header.Get("Authorization") + r.Header.Get("PRIVATE-TOKEN")}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &c.Body)
		}
		calls = append(calls, c)
		key := r.Method + " " + r.URL.EscapedPath()
		v, ok := routes[key]
		if !ok {
			w.WriteHeader(404)
			w.Write([]byte(`{"message":"Not Found"}`))
			return
		}
		if code, ok := v.(int); ok {
			w.WriteHeader(code)
			w.Write([]byte(`{"message":"nope"}`))
			return
		}
		json.NewEncoder(w).Encode(v)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func newTest(t *testing.T, k Kind, srv *httptest.Server) Provider {
	t.Helper()
	p, err := New(Config{Kind: k, BaseURL: srv.URL, Token: "tok", AllowPrivate: true, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGitHubEnterpriseFlow(t *testing.T) {
	srv, calls := fakeServer(t, map[string]any{
		"GET /api/v3/user":                                     map[string]any{"login": "octo", "name": "Octo"},
		"GET /api/v3/user/repos":                               []map[string]any{{"id": 1, "full_name": "octo/shop", "default_branch": "main", "private": true, "ssh_url": "git@ghe.example:octo/shop.git", "permissions": map[string]any{"admin": true}}, {"id": 2, "full_name": "octo/blog"}},
		"POST /api/v3/repos/octo/shop/keys":                    map[string]any{"id": 77},
		"POST /api/v3/repos/octo/shop/hooks":                   map[string]any{"id": 88},
		"POST /api/v3/repos/octo/shop/deployments":             map[string]any{"id": 99},
		"POST /api/v3/repos/octo/shop/deployments/99/statuses": map[string]any{},
		"DELETE /api/v3/repos/octo/shop/keys/77":               map[string]any{},
	})
	p := newTest(t, GitHub, srv)
	ctx := context.Background()
	if u, err := p.CurrentUser(ctx); err != nil || u.Login != "octo" {
		t.Fatalf("user %v %v", u, err)
	}
	repos, err := p.ListRepos(ctx, "SHO")
	if err != nil || len(repos) != 1 || !repos[0].Private || !repos[0].CanAdmin {
		t.Fatalf("repos %+v %v", repos, err)
	}
	if id, err := p.AddDeployKey(ctx, "octo/shop", "t", "ssh-ed25519 AAA"); err != nil || id != "77" {
		t.Fatal(id, err)
	}
	if id, err := p.CreateWebhook(ctx, "octo/shop", "https://panel/x", "s3cret", true); err != nil || id != "88" {
		t.Fatal(id, err)
	}
	h, err := p.StartDeployment(ctx, "octo/shop", "abc", "main", "deploy")
	if err != nil || h != "99" {
		t.Fatal(h, err)
	}
	if err := p.UpdateDeployment(ctx, "octo/shop", h, "abc", StateSuccess, "https://shop.example", "ok"); err != nil {
		t.Fatal(err)
	}
	if err := p.DeleteDeployKey(ctx, "octo/shop", "77"); err != nil {
		t.Fatal(err)
	}
	// Deleting something already gone is fine.
	if err := p.DeleteWebhook(ctx, "octo/shop", "1"); err != nil {
		t.Fatal(err)
	}
	for _, c := range *calls {
		if c.Auth != "Bearer tok" {
			t.Fatalf("auth header %q", c.Auth)
		}
		if c.Path == "/api/v3/repos/octo/shop/keys" && c.Body["read_only"] != true {
			t.Fatalf("deploy key not read-only: %v", c.Body)
		}
		if c.Path == "/api/v3/repos/octo/shop/hooks" {
			cfg := c.Body["config"].(map[string]any)
			if cfg["secret"] != "s3cret" || cfg["content_type"] != "json" {
				t.Fatalf("hook %v", c.Body)
			}
		}
	}
}

func TestGitLabProjectPathEncodingAndStatus(t *testing.T) {
	srv, calls := fakeServer(t, map[string]any{
		"GET /api/v4/projects/grp%2Fsub%2Fapp":               map[string]any{"id": 5, "path_with_namespace": "grp/sub/app", "visibility": "private", "permissions": map[string]any{"project_access": map[string]any{"access_level": 40}}},
		"POST /api/v4/projects/grp%2Fsub%2Fapp/deploy_keys":  map[string]any{"id": 3},
		"POST /api/v4/projects/grp%2Fsub%2Fapp/statuses/abc": map[string]any{},
	})
	p := newTest(t, GitLab, srv)
	ctx := context.Background()
	r, err := p.GetRepo(ctx, "grp/sub/app")
	if err != nil || !r.CanAdmin || !r.Private {
		t.Fatalf("repo %+v %v", r, err)
	}
	if _, err := p.AddDeployKey(ctx, "grp/sub/app", "t", "ssh-ed25519 AAA"); err != nil {
		t.Fatal(err)
	}
	if err := p.UpdateDeployment(ctx, "grp/sub/app", "abc", "abc", StateFailure, "", "build failed"); err != nil {
		t.Fatal(err)
	}
	last := (*calls)[len(*calls)-1]
	if last.Body["state"] != "failed" || (*calls)[1].Body["can_push"] != false || last.Auth != "tok" {
		t.Fatalf("calls %+v", *calls)
	}
}

func TestGiteaWebhookAndErrors(t *testing.T) {
	srv, calls := fakeServer(t, map[string]any{
		"POST /api/v1/repos/me/site/hooks": map[string]any{"id": 4},
		"GET /api/v1/user":                 401,
		"POST /api/v1/repos/me/site/keys":  403,
	})
	p := newTest(t, Gitea, srv)
	ctx := context.Background()
	if _, err := p.CreateWebhook(ctx, "me/site", "https://panel/h", "sec", true); err != nil {
		t.Fatal(err)
	}
	if b := (*calls)[0].Body; b["type"] != "gitea" || (*calls)[0].Auth != "token tok" {
		t.Fatalf("hook body %v", b)
	}
	if _, err := p.CurrentUser(ctx); err == nil || !strings.Contains(err.Error(), "rejected the token") {
		t.Fatalf("401 = %v", err)
	}
	if _, err := p.AddDeployKey(ctx, "me/site", "t", "k"); err == nil || !strings.Contains(err.Error(), "lacks a permission") {
		t.Fatalf("403 = %v", err)
	}
	if _, err := p.GetRepo(ctx, "me/missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("404 = %v", err)
	}
	if _, err := p.GetRepo(ctx, "../etc"); err == nil {
		t.Fatal("bad repo name accepted")
	}
}

func TestGuardedClientRefusesPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"login":"x"}`)) }))
	defer srv.Close()
	p, err := New(Config{Kind: Gitea, BaseURL: "https://" + strings.TrimPrefix(srv.URL, "http://"), Token: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.CurrentUser(context.Background()); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("loopback reached: %v", err)
	}
	if _, err := New(Config{Kind: Gitea, BaseURL: "http://git.example.com", Token: "t"}); err == nil {
		t.Fatal("plain http accepted")
	}
	if _, err := New(Config{Kind: GitHub, BaseURL: "https://user:pw@github.com", Token: "t"}); err == nil {
		t.Fatal("credentials in URL accepted")
	}
}

func TestSSHHost(t *testing.T) {
	cases := map[string]string{
		"git@github.com:o/r.git":                 "github.com",
		"ssh://git@git.example.com:2222/o/r.git": "[git.example.com]:2222",
		"ssh://git@gitlab.com/o/r.git":           "gitlab.com",
	}
	for in, want := range cases {
		if got := SSHHost(in); got != want {
			t.Errorf("%s = %s, want %s", in, got, want)
		}
	}
}
