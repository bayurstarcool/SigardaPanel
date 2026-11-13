package apihttp

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

func TestProxyURLChangeRegeneratesVhost(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	root := roleToken(t, s, ctx, "root", auth.RoleSuperAdmin)
	srv := &store.Server{Name: "s", Hostname: "h", Status: "active", AgentToken: "t"}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "p", Domain: "p.example.com", Runtime: "proxy", RootPath: "/home/u/htdocs/p.example.com", Status: "active"}
	s.Sites.Create(ctx, site)
	s.Sites.UpdateStatus(ctx, site.ID, "active")
	s.Sites.UpdateProxyPassURL(ctx, site.ID, "http://127.0.0.1:3000")
	url := "/api/v1/sites/" + strconv.FormatInt(site.ID, 10)

	if rec := doRequest(t, e, http.MethodPost, url+"/config", map[string]any{"proxy_pass_url": ""}, root); rec.Code != http.StatusBadRequest {
		t.Fatalf("clearing the upstream of a proxy site = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, e, http.MethodPost, url+"/config", map[string]any{"proxy_pass_url": "http://127.0.0.1:4000"}, root); rec.Code != http.StatusOK {
		t.Fatalf("update = %d %s", rec.Code, rec.Body.String())
	}
	jobs, _ := s.Jobs.List(ctx, store.JobListFilter{})
	found := false
	for _, j := range jobs {
		if j.Type == jobmodel.TypeSiteCreate && strings.Contains(j.Payload, `"vhost_regen":true`) {
			found = true
		}
	}
	if !found {
		t.Fatal("no vhost regeneration queued")
	}
}

func TestWebhookSecretShowAndRotate(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	root := roleToken(t, s, ctx, "root", auth.RoleSuperAdmin)
	stranger := roleToken(t, s, ctx, "stranger", auth.RoleUser)
	srv := &store.Server{Name: "s", Hostname: "h", Status: "active", AgentToken: "t"}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "w", Domain: "w.example.com", Runtime: "static", RootPath: "/home/u/htdocs/w.example.com", Status: "active"}
	s.Sites.Create(ctx, site)
	url := "/api/v1/sites/" + strconv.FormatInt(site.ID, 10) + "/webhook"

	if rec := doRequest(t, e, http.MethodGet, url, nil, stranger); rec.Code != http.StatusNotFound {
		t.Fatalf("stranger = %d", rec.Code)
	}
	first := doRequest(t, e, http.MethodGet, url, nil, root)
	again := doRequest(t, e, http.MethodGet, url, nil, root)
	rotated := doRequest(t, e, http.MethodPost, url+"/rotate", nil, root)
	if first.Code != 200 || first.Body.String() == "" || !strings.Contains(first.Body.String(), "/api/v1/webhooks/deploy/") {
		t.Fatalf("show = %d %s", first.Code, first.Body.String())
	}
	secret := func(b string) string { i := strings.Index(b, `"secret":"`); return b[i+10 : i+30] }
	if secret(first.Body.String()) != secret(again.Body.String()) || secret(first.Body.String()) == secret(rotated.Body.String()) {
		t.Fatal("show must be stable and rotate must change the secret")
	}
}
