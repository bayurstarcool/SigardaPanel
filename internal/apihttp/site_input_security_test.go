package apihttp

import (
	"net/http"
	"strings"
	"testing"
)

func TestCreateSite_RejectsRootEscalationInputs(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx) // role=user, username "regular"
	srvID := createServer(t, s, ctx)

	base := func() map[string]any {
		return map[string]any{"server_id": srvID, "name": "x", "domain": "x.example.com", "runtime": "proxy",
			"proxy_pass_url": "http://127.0.0.1:3000"}
	}
	cases := map[string]struct {
		mutate func(map[string]any)
		want   int
	}{
		"nginx injection via proxy_pass_url": {func(b map[string]any) {
			b["proxy_pass_url"] = "http://x;\n}\nlocation /p/ {alias /home/;autoindex on;\n"
		}, http.StatusBadRequest},
		"root_path with newline": {func(b map[string]any) { b["root_path"] = "/home/regular/x\n}\nserver {" }, http.StatusBadRequest},
		"other tenant's home":    {func(b map[string]any) { b["root_path"] = "/home/someone/htdocs/x" }, http.StatusForbidden},
		"system tree":            {func(b map[string]any) { b["root_path"] = "/etc/nginx" }, http.StatusBadRequest},
		"docker host bind mount": {func(b map[string]any) {
			b["runtime"] = "docker"
			b["docker_image"] = "nginx"
			b["docker_volume"] = "/:/host"
		}, http.StatusBadRequest},
		"docker option-like image": {func(b map[string]any) { b["runtime"] = "docker"; b["docker_image"] = "--privileged" }, http.StatusBadRequest},
		"bad php version":          {func(b map[string]any) { b["runtime"] = "php"; b["php_version"] = "8.3;x" }, http.StatusBadRequest},
	}
	for name, tc := range cases {
		b := base()
		tc.mutate(b)
		rec := doRequest(t, e, "POST", "/api/v1/sites", b, userToken)
		if rec.Code != tc.want {
			t.Errorf("%s: got %d want %d (%s)", name, rec.Code, tc.want, strings.TrimSpace(rec.Body.String()))
		}
	}

	// A non-admin cannot choose the unix account: system_user is pinned to their own.
	b := base()
	b["system_user"] = "root"
	rec := doRequest(t, e, "POST", "/api/v1/sites", b, userToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("valid site rejected: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"system_user":"regular"`) {
		t.Errorf("system_user not pinned to owner: %s", rec.Body.String())
	}
}

func TestCreateSite_AdminSystemUserMustNotBeRoot(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)
	rec := doRequest(t, e, "POST", "/api/v1/sites", map[string]any{
		"server_id": srvID, "name": "x", "domain": "y.example.com", "runtime": "static", "system_user": "root",
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("admin system_user=root: got %d", rec.Code)
	}
}
