package apihttp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

// A regular user transferring their own site used to panic on a nil role
// assertion; with the old goroutine-based timeout middleware that panic killed
// the whole API process.
func TestTransfer_RegularUser_NoPanic(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)
	u, _ := s.Users.GetByUsername(ctx, "regular")
	site := &store.Site{ServerID: srvID, Name: "own", Domain: "own.example.com", Runtime: "static", OwnerID: nullInt(u.ID)}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	other := &store.User{Username: "other", Email: "other@example.com", PasswordHash: "password123", Role: auth.RoleUser}
	if err := s.Users.Create(ctx, other); err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/transfer",
		map[string]string{"new_owner_email": "other@example.com"}, token)
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("transfer by owner returned 500: %s", rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("owner should be able to transfer their site, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Any handler panic must become a 500 for that request only.
func TestRouter_RecoversFromHandlerPanic(t *testing.T) {
	e, _, _ := newTestApp(t)
	e.GET("/api/v1/__panic", func(c echo.Context) error { panic("boom") })
	rec := doRequest(t, e, "GET", "/api/v1/__panic", nil, "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 from recovered panic, got %d", rec.Code)
	}
	rec = doRequest(t, e, "GET", "/api/v1/health", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("router unusable after panic: %d", rec.Code)
	}
}

func TestTwoFASetup_ServiceToken_Forbidden(t *testing.T) {
	e, s, ctx := newTestApp(t)
	raw, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ServiceTokens.Create(ctx, &store.ServiceToken{Name: "ci", KeyHash: auth.HashToken(raw), Scopes: "*"}); err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, e, "POST", "/api/v1/auth/2fa/setup", nil, raw)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for service token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTwoFASetup_RefusedWhileEnabled(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)
	u, _ := s.Users.GetByUsername(ctx, "admin")
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "test", AccountName: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TwoFactor.SetTOTPSecret(ctx, u.ID, key.Secret()); err != nil {
		t.Fatal(err)
	}
	if err := s.TwoFactor.EnableTOTP(ctx, u.ID); err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, e, "POST", "/api/v1/auth/2fa/setup", nil, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 while 2FA enabled, got %d", rec.Code)
	}
	if got, _ := s.TwoFactor.GetTOTPSecret(ctx, u.ID); got != key.Secret() {
		t.Fatal("live TOTP secret was overwritten")
	}

	// With 2FA off, setup still works.
	if err := s.TwoFactor.DisableTOTP(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	rec = doRequest(t, e, "POST", "/api/v1/auth/2fa/setup", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when 2FA disabled, got %d", rec.Code)
	}
}

// A user assigned with "admin" permission may manage a site but not delete it.
func TestDelete_AdminAssignee_Forbidden(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)
	assignee, _ := s.Users.GetByUsername(ctx, "regular")
	owner := &store.User{Username: "owner", Email: "owner@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	if err := s.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	site := &store.Site{ServerID: srvID, Name: "shared", Domain: "shared.example.com", Runtime: "static", OwnerID: nullInt(owner.ID)}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	if err := s.UserSites.Assign(ctx, assignee.ID, site.ID, "admin"); err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, e, "DELETE", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10), nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for admin assignee, got %d", rec.Code)
	}
	if got, _ := s.Sites.GetByID(ctx, site.ID); got == nil {
		t.Fatal("site was deleted by an assignee")
	}
}

// The deploy job payload must carry plain values the agent can bind
// (sql.Null* used to marshal as {"String":…,"Valid":…}).
func TestDeploy_PayloadHasPlainValues(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)
	admin, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "app", Domain: "app.example.com", Runtime: "node", RootPath: "/home/admin/htdocs/app.example.com",
		OwnerID: nullInt(admin.ID), AppPort: sql.NullInt64{Int64: 3001, Valid: true}, AppCommand: sql.NullString{String: "npm start", Valid: true}}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	if err := s.Sites.UpdateGitConfig(ctx, site.ID, "https://github.com/example/app.git", "main"); err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/deploy", nil, token)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			JobID int64 `json:"job_id"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	job, err := s.Jobs.GetByID(ctx, resp.Data.JobID)
	if err != nil || job == nil {
		t.Fatalf("job not found: %v", err)
	}
	var p struct {
		RootPath   string `json:"root_path"`
		Repo       string `json:"repo"`
		AppPort    int64  `json:"app_port"`
		AppCommand string `json:"app_command"`
	}
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		t.Fatalf("payload does not decode into agent shape: %v\n%s", err, job.Payload)
	}
	if p.AppPort != 3001 || p.AppCommand != "npm start" || p.RootPath == "" || p.Repo == "" {
		t.Fatalf("unexpected payload: %s", job.Payload)
	}
}

// Owned and assigned sites are listed together, honour filters and pagination,
// and total_count reports the whole accessible set.
func TestList_AssignedSitesPaginatedAndCounted(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)
	me, _ := s.Users.GetByUsername(ctx, "regular")
	other := &store.User{Username: "other", Email: "other@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	if err := s.Users.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	mine := &store.Site{ServerID: srvID, Name: "mine", Domain: "mine.example.com", Runtime: "static", OwnerID: nullInt(me.ID)}
	shared := &store.Site{ServerID: srvID, Name: "shared", Domain: "shared.example.com", Runtime: "static", OwnerID: nullInt(other.ID)}
	hidden := &store.Site{ServerID: srvID, Name: "hidden", Domain: "hidden.example.com", Runtime: "static", OwnerID: nullInt(other.ID)}
	for _, st := range []*store.Site{mine, shared, hidden} {
		if err := s.Sites.Create(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UserSites.Assign(ctx, me.ID, shared.ID, "read"); err != nil {
		t.Fatal(err)
	}

	list := func(q string) (domains []string, total int) {
		rec := doRequest(t, e, "GET", "/api/v1/sites"+q, nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d", q, rec.Code)
		}
		var r struct {
			Data struct {
				Sites      []map[string]any `json:"sites"`
				TotalCount int              `json:"total_count"`
			} `json:"data"`
		}
		json.Unmarshal(rec.Body.Bytes(), &r)
		for _, st := range r.Data.Sites {
			domains = append(domains, st["domain"].(string))
		}
		return domains, r.Data.TotalCount
	}

	if d, total := list(""); len(d) != 2 || total != 2 {
		t.Fatalf("expected owned+assigned (2), got %v total=%d", d, total)
	}
	if d, total := list("?limit=1"); len(d) != 1 || total != 2 {
		t.Fatalf("expected 1 row of 2, got %v total=%d", d, total)
	}
	if d, _ := list("?search=shared"); len(d) != 1 || d[0] != "shared.example.com" {
		t.Fatalf("search must apply to assigned sites, got %v", d)
	}
	if d, _ := list("?search=hidden"); len(d) != 0 {
		t.Fatalf("unassigned site leaked: %v", d)
	}
}
