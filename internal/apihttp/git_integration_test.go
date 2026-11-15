package apihttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/gitprovider"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// fakeGit is an in-memory git provider account.
type fakeGit struct {
	mu       sync.Mutex
	login    string
	admin    bool
	keys     map[string]string // id -> pubkey
	hooks    map[string]string // id -> url
	verify   map[string]bool
	statuses []string
	next     int
}

func newFakeGit(login string) *fakeGit {
	return &fakeGit{login: login, admin: true, keys: map[string]string{}, hooks: map[string]string{}, verify: map[string]bool{}}
}
func (f *fakeGit) id() string             { f.next++; return strconv.Itoa(f.next) }
func (f *fakeGit) Kind() gitprovider.Kind { return gitprovider.GitHub }
func (f *fakeGit) BaseURL() string        { return "https://github.com" }
func (f *fakeGit) KnownHosts(context.Context) []string {
	return []string{"github.com ssh-ed25519 AAAAHOSTKEY"}
}
func (f *fakeGit) CurrentUser(context.Context) (*gitprovider.User, error) {
	if f.login == "" {
		return nil, fmt.Errorf("github.com rejected the token")
	}
	return &gitprovider.User{Login: f.login}, nil
}
func (f *fakeGit) repo(full string) gitprovider.Repo {
	return gitprovider.Repo{FullName: full, DefaultBranch: "main", Private: true, SSHURL: "git@github.com:" + full + ".git", HTMLURL: "https://github.com/" + full, CanAdmin: f.admin}
}
func (f *fakeGit) ListRepos(_ context.Context, q string) ([]gitprovider.Repo, error) {
	return []gitprovider.Repo{f.repo("octo/shop"), f.repo("octo/blog")}, nil
}
func (f *fakeGit) GetRepo(_ context.Context, full string) (*gitprovider.Repo, error) {
	r := f.repo(full)
	return &r, nil
}
func (f *fakeGit) ListBranches(context.Context, string) ([]gitprovider.Branch, error) {
	return []gitprovider.Branch{{Name: "main", SHA: "abc"}}, nil
}
func (f *fakeGit) GetCommit(context.Context, string, string) (*gitprovider.Commit, error) {
	return nil, nil
}
func (f *fakeGit) AddDeployKey(_ context.Context, _, _, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.id()
	f.keys[id] = key
	return id, nil
}
func (f *fakeGit) DeleteDeployKey(_ context.Context, _, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.keys, id)
	return nil
}
func (f *fakeGit) CreateWebhook(_ context.Context, _, u, _ string, verify bool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.id()
	f.hooks[id], f.verify[id] = u, verify
	return id, nil
}
func (f *fakeGit) DeleteWebhook(_ context.Context, _, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.hooks, id)
	return nil
}
func (f *fakeGit) WebhookExists(_ context.Context, _, id string) (bool, error) {
	_, ok := f.hooks[id]
	return ok, nil
}
func (f *fakeGit) StartDeployment(_ context.Context, _, sha, _, _ string) (string, error) {
	f.statuses = append(f.statuses, "start:"+sha)
	return "dep-" + sha, nil
}
func (f *fakeGit) UpdateDeployment(_ context.Context, _, h, _ string, st gitprovider.State, _, _ string) error {
	f.statuses = append(f.statuses, string(st))
	return nil
}

func useFakeGit(t *testing.T, f *fakeGit) {
	prev := gitprovider.Open
	gitprovider.Open = func(cfg gitprovider.Config) (gitprovider.Provider, error) {
		if cfg.Token == "" {
			return nil, fmt.Errorf("an access token is required")
		}
		return f, nil
	}
	t.Cleanup(func() { gitprovider.Open = prev })
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return out.Data
}

func TestGitIntegrationConnectDeployDisconnect(t *testing.T) {
	t.Setenv("SIGARDAPANEL_PUBLIC_URL", "https://panel.example.com")
	fg := newFakeGit("octo")
	useFakeGit(t, fg)
	f := newAuthzFixture(t)
	owner, tok := f.user("owner", auth.RoleUser)
	writer, wtok := f.user("writer", auth.RoleUser)
	_, otherTok := f.user("other", auth.RoleUser)
	st := f.site(owner, "shop", "php")
	f.s.DB().Exec(`UPDATE sites SET system_user = 'owner' WHERE id = ?`, st.ID)
	f.assign(writer, st, "write")
	var keyCalls []string
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		json.NewDecoder(r.Body).Decode(&p)
		keyCalls = append(keyCalls, fmt.Sprint(p["action"]))
		w.Write([]byte(`{"ok":true,"public_key":"ssh-ed25519 AAAASITEKEY sigardapanel-site"}`))
	}))
	defer agent.Close()
	f.s.DB().Exec(`UPDATE servers SET agent_url = ?, agent_token = 't' WHERE id = ?`, agent.URL, f.serverID)

	// Connections: a token that does not verify is refused.
	if rec := f.do("POST", "/api/v1/git/connections", map[string]any{"provider": "github", "token": ""}, tok); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty token = %d", rec.Code)
	}
	if rec := f.do("POST", "/api/v1/git/connections", map[string]any{"provider": "gitlab", "token": "x", "base_url": "http://10.0.0.5", "allow_private": true}, tok); rec.Code != http.StatusForbidden {
		t.Fatalf("private network for a user = %d", rec.Code)
	}
	rec := f.do("POST", "/api/v1/git/connections", map[string]any{"provider": "github", "token": "ghp_secret"}, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("connect = %d %s", rec.Code, rec.Body.String())
	}
	connID := int64(decodeData(t, rec)["id"].(float64))
	if strings.Contains(rec.Body.String(), "ghp_secret") {
		t.Fatal("token echoed back")
	}
	var stored string
	f.s.DB().QueryRow(`SELECT token FROM git_connections WHERE id = ?`, connID).Scan(&stored)
	if stored == "ghp_secret" {
		t.Fatal("token stored in plain text")
	}
	cid := strconv.FormatInt(connID, 10)
	if rec := f.do("GET", "/api/v1/git/connections/"+cid+"/repos", nil, otherTok); rec.Code != http.StatusNotFound {
		t.Fatalf("another user's connection = %d", rec.Code)
	}
	if rec := f.do("GET", "/api/v1/git/connections/"+cid+"/repos", nil, tok); !strings.Contains(rec.Body.String(), "octo/shop") {
		t.Fatalf("repos = %s", rec.Body.String())
	}

	// Connect the site: only the owner, only with their own connection.
	sid := sitePath(st, "/git")
	connect := map[string]any{"connection_id": connID, "repo": "octo/shop", "branch": "main", "deploy_script": "php artisan migrate --force"}
	if rec := f.do("POST", sid+"/connect", connect, wtok); rec.Code != http.StatusForbidden {
		t.Fatalf("writer connects = %d", rec.Code)
	}
	rec = f.do("POST", sid+"/connect", connect, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("connect site = %d %s", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	if data["webhook"] != true || data["deployment_id"] == nil {
		t.Fatalf("connect result = %v", data)
	}
	if len(fg.keys) != 1 || len(fg.hooks) != 1 {
		t.Fatalf("provider keys=%v hooks=%v", fg.keys, fg.hooks)
	}
	for id, u := range fg.hooks {
		if u != fmt.Sprintf("https://panel.example.com/api/v1/webhooks/deploy/%d", st.ID) || !fg.verify[id] {
			t.Fatalf("hook = %s verify=%v", u, fg.verify[id])
		}
	}
	site, _ := f.s.Sites.GetByID(f.ctx, st.ID)
	if site.GitRepo.String != "git@github.com:octo/shop.git" {
		t.Fatalf("repo = %s", site.GitRepo.String)
	}
	jobs, _ := f.s.Jobs.List(f.ctx, store.JobListFilter{})
	var deployJob *jobmodel.Job
	for i := range jobs {
		if jobs[i].Type == jobmodel.TypeSiteDeploy {
			deployJob = &jobs[i]
		}
	}
	if deployJob == nil || deployJob.MaxAttempts != 1 || !strings.Contains(deployJob.Payload, `"use_deploy_key":true`) ||
		!strings.Contains(deployJob.Payload, "AAAAHOSTKEY") || !strings.Contains(deployJob.Payload, "artisan migrate") {
		t.Fatalf("deploy job = %+v", deployJob)
	}

	// Settings: admin-level; a writer can read and deploy but not configure.
	if rec := f.do("PATCH", sid, map[string]any{"deploy_script": "rm -rf ~"}, wtok); rec.Code == http.StatusOK {
		t.Fatal("writer changed the deploy script")
	}
	if rec := f.do("GET", sid, nil, wtok); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"can_configure":false`) {
		t.Fatalf("writer view = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do("PATCH", sid, map[string]any{"auto_deploy": false}, tok); rec.Code != http.StatusOK {
		t.Fatalf("owner settings = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do("PATCH", sid, map[string]any{"repo_url": "https://example.com/x.git"}, tok); rec.Code != http.StatusConflict {
		t.Fatalf("manual URL on a connected site = %d", rec.Code)
	}

	// Push webhook: auto deploy off → nothing queued; on → a push deployment.
	secret, _ := f.s.Sites.GenerateWebhookSecret(f.ctx, st.ID)
	push := []byte(`{"ref":"refs/heads/main","after":"0123456789abcdef0123456789abcdef01234567","head_commit":{"message":"Fix checkout\n\nbody","author":{"name":"Ana"}}}`)
	hook := func() *httptest.ResponseRecorder {
		return doRequestWithHeaders(t, f.e, "POST", "/api/v1/webhooks/deploy/"+strconv.FormatInt(st.ID, 10), push, "", map[string]string{"X-Hub-Signature-256": computeHMAC(push, secret)})
	}
	if rec := hook(); !strings.Contains(rec.Body.String(), "auto deploy is off") {
		t.Fatalf("push with auto deploy off = %s", rec.Body.String())
	}
	f.do("PATCH", sid, map[string]any{"auto_deploy": true}, tok)
	if rec := hook(); !strings.Contains(rec.Body.String(), "deploy queued") {
		t.Fatalf("push = %s", rec.Body.String())
	}
	deps, _ := f.s.Git.ListDeployments(f.ctx, st.ID, 10)
	if len(deps) != 2 || deps[0].TriggeredBy != "push" || deps[0].CommitMessage != "Fix checkout" || deps[0].CommitAuthor != "Ana" {
		t.Fatalf("deployments = %+v", deps)
	}

	// Redeploy / rollback a recorded commit (writer may deploy).
	f.s.DB().Exec(`UPDATE deployments SET status = 'succeeded' WHERE id = ?`, deps[0].ID)
	rec = f.do("POST", sitePath(st, "/deployments/"+strconv.FormatInt(deps[0].ID, 10)+"/rollback"), nil, wtok)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("rollback = %d %s", rec.Code, rec.Body.String())
	}
	if d, _ := f.s.Git.GetDeployment(f.ctx, int64(decodeData(t, rec)["deployment_id"].(float64))); d == nil || d.TriggeredBy != "rollback" || d.TargetCommit != deps[0].CommitSHA {
		t.Fatalf("rollback deployment = %+v", d)
	}
	if rec := f.do("GET", sitePath(st, "/deployments/"+strconv.FormatInt(deps[0].ID, 10)), nil, otherTok); rec.Code != http.StatusNotFound {
		t.Fatalf("other user reads deployment = %d", rec.Code)
	}

	// The connection cannot be removed while the site uses it.
	if rec := f.do("DELETE", "/api/v1/git/connections/"+cid, nil, tok); rec.Code != http.StatusConflict {
		t.Fatalf("delete used connection = %d", rec.Code)
	}
	// Disconnect: key and hook removed at the provider and on the server.
	if rec := f.do("DELETE", sid, nil, wtok); rec.Code != http.StatusForbidden {
		t.Fatalf("writer disconnects = %d", rec.Code)
	}
	if rec := f.do("DELETE", sid, nil, tok); rec.Code != http.StatusOK {
		t.Fatalf("disconnect = %d %s", rec.Code, rec.Body.String())
	}
	if len(fg.keys) != 0 || len(fg.hooks) != 0 || keyCalls[len(keyCalls)-1] != "delete" {
		t.Fatalf("after disconnect keys=%v hooks=%v agent=%v", fg.keys, fg.hooks, keyCalls)
	}
	if site, _ := f.s.Sites.GetByID(f.ctx, st.ID); site.GitRepo.Valid {
		t.Fatal("repository still configured")
	}
	if rec := f.do("DELETE", "/api/v1/git/connections/"+cid, nil, tok); rec.Code != http.StatusOK {
		t.Fatalf("delete connection = %d", rec.Code)
	}
}

func TestGitConnectNeedsRepoAdminAndIPWebhookSkipsVerify(t *testing.T) {
	t.Setenv("SIGARDAPANEL_PUBLIC_URL", "https://203.0.114.9")
	fg := newFakeGit("octo")
	useFakeGit(t, fg)
	f := newAuthzFixture(t)
	owner, tok := f.user("owner", auth.RoleUser)
	st := f.site(owner, "blog", "static")
	f.s.DB().Exec(`UPDATE sites SET system_user = 'owner' WHERE id = ?`, st.ID)
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"public_key":"ssh-ed25519 AAAAK"}`))
	}))
	defer agent.Close()
	f.s.DB().Exec(`UPDATE servers SET agent_url = ?, agent_token = 't' WHERE id = ?`, agent.URL, f.serverID)
	rec := f.do("POST", "/api/v1/git/connections", map[string]any{"provider": "github", "token": "t"}, tok)
	connID := decodeData(t, rec)["id"]
	fg.admin = false
	if rec := f.do("POST", sitePath(st, "/git/connect"), map[string]any{"connection_id": connID, "repo": "octo/blog"}, tok); rec.Code != http.StatusForbidden {
		t.Fatalf("no admin access = %d", rec.Code)
	}
	fg.admin = true
	if rec := f.do("POST", sitePath(st, "/git/connect"), map[string]any{"connection_id": connID, "repo": "octo/blog", "deploy_now": false}, tok); rec.Code != http.StatusOK {
		t.Fatalf("connect = %d %s", rec.Code, rec.Body.String())
	}
	for id := range fg.hooks {
		if fg.verify[id] {
			t.Fatal("webhook to an IP address asks the provider to verify a certificate it cannot have")
		}
	}
	if deps, _ := f.s.Git.ListDeployments(f.ctx, st.ID, 10); len(deps) != 0 {
		t.Fatal("deploy_now=false still deployed")
	}
}

func TestParsePushPayloads(t *testing.T) {
	gl := parsePush([]byte(`{"ref":"refs/heads/main","after":"aaa","checkout_sha":"bbb","commits":[{"id":"bbb","message":"Add page\n","author":{"name":"Bo"}}]}`))
	if gl.After != "bbb" || gl.Message != "Add page" || gl.Author != "Bo" {
		t.Errorf("gitlab = %+v", gl)
	}
	del := parsePush([]byte(`{"ref":"refs/heads/main","after":"0000000000000000000000000000000000000000"}`))
	if !del.Deleted {
		t.Error("branch deletion not detected")
	}
}
