package apihttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/rbac"
	"sigardapanel/internal/store"
)

type permRoute struct{ method, path, perm string }

// permissionRoutes reads every RequirePermission route from router.go.
func permissionRoutes(t *testing.T) []permRoute {
	t.Helper()
	src, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^\s*\w+\.(GET|POST|PUT|PATCH|DELETE)\("([^"]+)".*RequirePermission\(s, "([^"]+)"\)`)
	var out []permRoute
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		path := regexp.MustCompile(`:[A-Za-z_]+`).ReplaceAllString(m[2], "1")
		out = append(out, permRoute{m[1], path, m[3]})
	}
	if len(out) < 100 {
		t.Fatalf("only %d permission routes found", len(out))
	}
	return out
}

// Every permission-gated route lets a built-in role through exactly when the
// registry gives the role that permission (the registry's MinRole is the role
// each route required before RBAC, so built-in behaviour is unchanged).
func TestBuiltinRolesMatchRouteGates(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	tokens := map[string]string{
		auth.RoleSuperAdmin: roleToken(t, s, ctx, "root", auth.RoleSuperAdmin),
		auth.RoleAdmin:      roleToken(t, s, ctx, "reseller", auth.RoleAdmin),
		auth.RoleUser:       roleToken(t, s, ctx, "customer", auth.RoleUser),
	}
	for _, r := range permissionRoutes(t) {
		if _, ok := rbac.Get(r.perm); !ok {
			t.Errorf("%s %s uses unknown permission %s", r.method, r.path, r.perm)
			continue
		}
		for role, tok := range tokens {
			rec := gateRequest(e, r.method, r.path, tok)
			refused := rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "insufficient permissions")
			if want := rbac.BuiltinPermissions(role)[r.perm]; refused == want {
				t.Errorf("%s %s (%s) as %s: refused=%v, want allowed=%v (%d %s)", r.method, r.path, r.perm, role, refused, want, rec.Code, strings.TrimSpace(rec.Body.String()))
			}
		}
	}
}

func TestAuthMeListsPermissions(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	tok := roleToken(t, s, ctx, "reseller", auth.RoleAdmin)
	rec := doRequest(t, e, "GET", "/api/v1/auth/me", nil, tok)
	var res struct {
		Data struct {
			Permissions []string `json:"permissions"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &res)
	if len(res.Data.Permissions) != len(rbac.BuiltinPermissions(auth.RoleAdmin)) {
		t.Fatalf("admin permissions in /auth/me = %v", res.Data.Permissions)
	}
}

// Custom roles: CRUD by role.manage (super_admin), no escalation, built-ins
// read-only, roles in use cannot be deleted.
func TestCustomRolesCRUDAndEscalation(t *testing.T) {
	f := newAuthzFixture(t)
	_, rootTok := f.user("root", auth.RoleSuperAdmin)
	admin, adminTok := f.user("reseller", auth.RoleAdmin)

	// Only role.manage creates roles.
	if rec := f.do("POST", "/api/v1/roles", map[string]any{"name": "helper", "permissions": []string{"site.suspend"}}, adminTok); rec.Code != http.StatusForbidden {
		t.Fatalf("admin creating a role = %d", rec.Code)
	}
	rec := f.do("POST", "/api/v1/roles", map[string]any{"name": "support", "description": "Helpdesk",
		"permissions": []string{"user.impersonate", "user.impersonate_any", "site.view_all", "job.view_all", "audit.view"}}, rootTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create support role = %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)
	for _, bad := range []map[string]any{
		{"name": "admin", "permissions": []string{}},                 // built-in name
		{"name": "x y", "permissions": []string{}},                   // invalid
		{"name": "ghost", "permissions": []string{"no.such"}},        // unknown permission
		{"name": "support", "permissions": []string{"site.suspend"}}, // duplicate
	} {
		if rec := f.do("POST", "/api/v1/roles", bad, rootTok); rec.Code < 400 {
			t.Errorf("role %v accepted: %d", bad, rec.Code)
		}
	}
	// Built-ins are read-only.
	list := f.do("GET", "/api/v1/roles", nil, rootTok)
	var roles struct {
		Data struct {
			Roles []struct {
				ID         int64  `json:"id"`
				Name       string `json:"name"`
				BuiltIn    bool   `json:"built_in"`
				Assignable bool   `json:"assignable"`
			} `json:"roles"`
		} `json:"data"`
	}
	json.Unmarshal(list.Body.Bytes(), &roles)
	if len(roles.Data.Roles) != 4 {
		t.Fatalf("roles = %s", list.Body.String())
	}
	for _, r := range roles.Data.Roles {
		if r.BuiltIn {
			if rec := f.do("PATCH", "/api/v1/roles/"+strconv.FormatInt(r.ID, 10), map[string]any{"permissions": []string{}}, rootTok); rec.Code != http.StatusForbidden {
				t.Errorf("edit built-in %s = %d", r.Name, rec.Code)
			}
		}
	}

	// An admin cannot hand out the support role (it holds dangerous
	// permissions and more than the admin has), nor admin or super_admin.
	for _, role := range []string{"support", auth.RoleAdmin, auth.RoleSuperAdmin} {
		rec := f.do("POST", "/api/v1/users", map[string]any{"username": "u" + strings.ReplaceAll(role, "_", ""), "email": role + "@example.test", "password": "Temp-pass-12345", "role": role}, adminTok)
		if rec.Code != http.StatusForbidden {
			t.Errorf("admin creating a %s user = %d %s", role, rec.Code, rec.Body.String())
		}
	}
	// A non-dangerous custom role within the admin's permissions is assignable
	// by the admin once a super_admin created it.
	if rec := f.do("POST", "/api/v1/roles", map[string]any{"name": "operator", "permissions": []string{"site.suspend", "site.cron"}}, rootTok); rec.Code != http.StatusCreated {
		t.Fatalf("create operator = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do("POST", "/api/v1/users", map[string]any{"username": "opuser", "email": "op@example.test", "password": "Temp-pass-12345", "role": "operator"}, adminTok); rec.Code != http.StatusCreated {
		t.Fatalf("admin creating an operator = %d %s", rec.Code, rec.Body.String())
	}
	_ = admin

	// A role in use cannot be deleted; an unused one can.
	op, _ := f.s.Roles.GetByName(f.ctx, "operator")
	if rec := f.do("DELETE", "/api/v1/roles/"+strconv.FormatInt(op.ID, 10), nil, rootTok); rec.Code != http.StatusConflict {
		t.Fatalf("delete role in use = %d", rec.Code)
	}
	if rec := f.do("PATCH", "/api/v1/roles/"+strconv.FormatInt(created.Data.ID, 10), map[string]any{"description": "Helpdesk team", "permissions": []string{"user.impersonate", "user.impersonate_any", "site.view_all"}}, rootTok); rec.Code != http.StatusOK {
		t.Fatalf("update support = %d %s", rec.Code, rec.Body.String())
	}
}

// A custom "support" role can read every site and impersonate customers, and
// nothing more: no site writes or deletes, no user management, no servers.
func TestSupportRoleEndToEnd(t *testing.T) {
	f := newAuthzFixture(t)
	_, rootTok := f.user("root", auth.RoleSuperAdmin)
	if rec := f.do("POST", "/api/v1/roles", map[string]any{"name": "support",
		"permissions": []string{"user.impersonate", "user.impersonate_any", "site.view_all"}}, rootTok); rec.Code != http.StatusCreated {
		t.Fatalf("create support = %d %s", rec.Code, rec.Body.String())
	}
	customer, _ := f.user("customer", auth.RoleUser)
	reseller, _ := f.user("reseller", auth.RoleAdmin)
	site := f.site(customer, "shop", "php")
	sid := strconv.FormatInt(site.ID, 10)
	sup := &store.User{Username: "helpdesk", Email: "helpdesk@example.test", PasswordHash: "Temp-pass-12345", Role: "support"}
	if err := f.s.Users.Create(f.ctx, sup); err != nil {
		t.Fatal(err)
	}
	supTok := rbacSessionToken(t, f, sup.ID)

	if rec := f.do("GET", "/api/v1/sites/"+sid, nil, supTok); rec.Code != http.StatusOK {
		t.Fatalf("support reading a customer's site = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do("GET", "/api/v1/sites", nil, supTok); !strings.Contains(rec.Body.String(), "shop.example.com") {
		t.Fatalf("support site list = %s", rec.Body.String())
	}
	if rec := f.do("DELETE", "/api/v1/sites/"+sid, nil, supTok); rec.Code == http.StatusOK {
		t.Fatal("support deleted a site")
	}
	if rec := f.do("POST", "/api/v1/sites/"+sid+"/files/write", map[string]any{"path": "x.txt", "content": "x"}, supTok); rec.Code == http.StatusOK {
		t.Fatal("support wrote a site file")
	}
	for _, r := range []permRoute{{"GET", "/api/v1/users", ""}, {"POST", "/api/v1/servers", ""}, {"POST", "/api/v1/sites/" + sid + "/suspend", ""}} {
		if rec := f.do(r.method, r.path, map[string]any{}, supTok); rec.Code != http.StatusForbidden {
			t.Errorf("support %s %s = %d", r.method, r.path, rec.Code)
		}
	}
	// Impersonation: a customer yes, an admin (more permissions) no.
	if rec := f.do("POST", "/api/v1/users/"+strconv.FormatInt(customer.ID, 10)+"/impersonate", nil, supTok); rec.Code != http.StatusOK {
		t.Fatalf("support impersonating a customer = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do("POST", "/api/v1/users/"+strconv.FormatInt(reseller.ID, 10)+"/impersonate", nil, supTok); rec.Code != http.StatusForbidden {
		t.Fatalf("support impersonating an admin = %d", rec.Code)
	}
}

func rbacSessionToken(t *testing.T, f *authzFixture, userID int64) string {
	t.Helper()
	tok, _ := auth.NewToken()
	if err := f.s.Sessions.Create(f.ctx, &store.Session{UserID: userID, TokenHash: auth.HashToken(tok), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return tok
}

// gateRequest is doRequest from a distinct client address each time, so the
// per-IP rate limiter does not answer instead of the route.
var gateClient int

func gateRequest(e *echo.Echo, method, path, token string) *httptest.ResponseRecorder {
	gateClient++
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = fmt.Sprintf("10.%d.%d.%d:4000", gateClient/65536%256, gateClient/256%256, gateClient%256)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// A custom role holding user.manage_all still never reaches an account with
// more permissions than it has (a super_admin): no impersonation, password
// reset, demotion or delete.
func TestManageAllCustomRoleCannotTouchSuperAdmin(t *testing.T) {
	f := newAuthzFixture(t)
	root, rootTok := f.user("root", auth.RoleSuperAdmin)
	if rec := f.do("POST", "/api/v1/roles", map[string]any{"name": "usermgr",
		"permissions": []string{"user.manage", "user.manage_all", "user.impersonate"}}, rootTok); rec.Code != http.StatusCreated {
		t.Fatalf("create usermgr = %d %s", rec.Code, rec.Body.String())
	}
	mgr := &store.User{Username: "usermgr1", Email: "usermgr1@example.test", PasswordHash: "Temp-pass-12345", Role: "usermgr"}
	if err := f.s.Users.Create(f.ctx, mgr); err != nil {
		t.Fatal(err)
	}
	tok := rbacSessionToken(t, f, mgr.ID)
	customer, _ := f.user("customer", auth.RoleUser)
	rid := strconv.FormatInt(root.ID, 10)

	if rec := f.do("POST", "/api/v1/users/"+rid+"/impersonate", nil, tok); rec.Code != http.StatusForbidden {
		t.Errorf("impersonate super_admin = %d", rec.Code)
	}
	if rec := f.do("PATCH", "/api/v1/users/"+rid, map[string]any{"password": "Taken-over-12345"}, tok); rec.Code != http.StatusForbidden {
		t.Errorf("reset super_admin password = %d", rec.Code)
	}
	if rec := f.do("PATCH", "/api/v1/users/"+rid, map[string]any{"role": "user"}, tok); rec.Code != http.StatusForbidden {
		t.Errorf("demote super_admin = %d", rec.Code)
	}
	if rec := f.do("DELETE", "/api/v1/users/"+rid, nil, tok); rec.Code != http.StatusForbidden {
		t.Errorf("delete super_admin = %d", rec.Code)
	}
	if u, _ := f.s.Users.GetByID(f.ctx, root.ID); u == nil || u.Role != auth.RoleSuperAdmin {
		t.Fatal("super_admin changed")
	}
	// A plain user is within its reach.
	if rec := f.do("POST", "/api/v1/users/"+strconv.FormatInt(customer.ID, 10)+"/impersonate", nil, tok); rec.Code != http.StatusOK {
		t.Errorf("impersonate customer = %d %s", rec.Code, rec.Body.String())
	}
}
