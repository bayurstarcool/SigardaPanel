package apihttp

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

func TestUpdateSecurityValidatesAndQueuesVhostJob(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)
	owner, _ := s.Users.GetByUsername(ctx, "regular")
	site := &store.Site{ServerID: srvID, Name: "Sec", Domain: "sec.example.com", Runtime: "static",
		RootPath: "/home/regular/htdocs/sec.example.com", Status: "active", OwnerID: nullInt(owner.ID)}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/sites/" + strconv.FormatInt(site.ID, 10) + "/security"

	evil := map[string]any{"blocked_ips": []string{"1.2.3.4; } location /leak/ { alias /etc/; autoindex on; } location /z/ { deny 5.6.7.8"}}
	if rec := doRequest(t, e, http.MethodPut, path, evil, userToken); rec.Code != http.StatusBadRequest {
		t.Fatalf("injection = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(t, e, http.MethodPut, path, map[string]any{"basic_auth_enabled": true, "basic_auth_username": "a:b", "basic_auth_password": "secret1"}, userToken); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad username = %d", rec.Code)
	}
	if rec := doRequest(t, e, http.MethodPut, path, map[string]any{"basic_auth_enabled": true, "basic_auth_username": "admin"}, userToken); rec.Code != http.StatusBadRequest {
		t.Fatalf("basic auth without any password = %d", rec.Code)
	}

	rec := doRequest(t, e, http.MethodPut, path, map[string]any{"basic_auth_enabled": true, "basic_auth_username": "admin", "basic_auth_password": "secret1", "blocked_ips": []string{"1.2.3.4"}}, userToken)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"job_id"`) {
		t.Fatalf("valid update = %d %s", rec.Code, rec.Body.String())
	}
	first, _ := s.Security.GetBySiteID(ctx, site.ID)
	if !strings.HasPrefix(first.BasicAuthPassword, "$2") {
		t.Fatalf("password not hashed: %q", first.BasicAuthPassword)
	}
	// Saving again without a password keeps the hash.
	rec = doRequest(t, e, http.MethodPut, path, map[string]any{"basic_auth_enabled": true, "basic_auth_username": "admin", "blocked_ips": []string{"1.2.3.4", "5.6.7.0/24"}}, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("second update = %d %s", rec.Code, rec.Body.String())
	}
	second, _ := s.Security.GetBySiteID(ctx, site.ID)
	if second.BasicAuthPassword != first.BasicAuthPassword || len(second.BlockedIPs) != 2 {
		t.Fatalf("hash or IPs lost: %+v", second)
	}
	jobs, _ := s.Jobs.List(ctx, store.JobListFilter{})
	regens := 0
	for _, j := range jobs {
		if j.Type == jobmodel.TypeSiteCreate && j.SiteID.Int64 == site.ID {
			regens++
		}
	}
	if regens != 2 {
		t.Fatalf("expected 2 vhost jobs, got %d", regens)
	}

	s.Sites.UpdateStatus(ctx, site.ID, "suspended")
	if rec := doRequest(t, e, http.MethodPut, path, map[string]any{"blocked_ips": []string{}}, userToken); rec.Code != http.StatusConflict {
		t.Fatalf("suspended site = %d", rec.Code)
	}
}
