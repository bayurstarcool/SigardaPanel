package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/db"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

func newTestAppWithAllMigrations(t *testing.T) (*echo.Echo, *store.Store, context.Context) {
	t.Helper()
	f, _ := os.CreateTemp("", "api_test_*.db")
	t.Cleanup(func() { os.Remove(f.Name()) })
	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	migrations.Up(d, migrations.AllMigrations)
	s := store.New(d)
	e := NewRouter(s, d, nil)
	return e, s, t.Context()
}

func createServer(t *testing.T, s *store.Store, ctx context.Context) int64 {
	t.Helper()
	server := &store.Server{
		Name:       "test-server",
		Hostname:   "srv.example.com",
		Status:     "active",
		AgentToken: "token-abc",
	}
	if err := s.Servers.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}
	return server.ID
}

func TestList_Empty(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/sites", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Sites []map[string]any `json:"sites"`
			Count int              `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data.Count != 0 {
		t.Fatalf("expected count 0, got %d", result.Data.Count)
	}
}

func TestList_Admin_SeesAll(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	user1, _ := s.Users.GetByUsername(ctx, "admin")

	s.Sites.Create(ctx, &store.Site{ServerID: srvID, Name: "A", Domain: "a1.example.com", Runtime: "static", OwnerID: nullInt(user1.ID)})
	s.Sites.Create(ctx, &store.Site{ServerID: srvID, Name: "B", Domain: "b1.example.com", Runtime: "static", OwnerID: nullInt(user1.ID)})

	rec := doRequest(t, e, "GET", "/api/v1/sites", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Sites []map[string]any `json:"sites"`
			Count int              `json:"count"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.Count != 2 {
		t.Fatalf("expected count 2, got %d", result.Data.Count)
	}
}

// The SSL dashboard reads certificate state from the sites list; it must be present.
func TestList_IncludesSSLFields(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)
	admin, _ := s.Users.GetByUsername(ctx, "admin")

	site := &store.Site{ServerID: srvID, Name: "S", Domain: "ssl.example.com", Runtime: "static", OwnerID: nullInt(admin.ID)}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	expires := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	if err := s.Sites.UpdateSSL(ctx, site.ID, "active", "Let's Encrypt", expires); err != nil {
		t.Fatalf("update ssl: %v", err)
	}

	rec := doRequest(t, e, "GET", "/api/v1/sites", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result struct {
		Data struct {
			Sites []map[string]any `json:"sites"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Data.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(result.Data.Sites))
	}
	got := result.Data.Sites[0]
	if got["ssl_status"] != "active" || got["ssl_issuer"] != "Let's Encrypt" {
		t.Fatalf("missing ssl status/issuer: %v", got)
	}
	if ts, _ := got["ssl_expires_at"].(string); ts == "" {
		t.Fatalf("missing ssl_expires_at: %v", got)
	}
}

func TestList_RegularUser_SeesOnlyOwn(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	adminToken := newAdminToken(t, s, ctx)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	// Create another regular user
	otherUser := &store.User{Username: "other", Email: "other@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	s.Users.Create(ctx, otherUser)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	regularUser, _ := s.Users.GetByUsername(ctx, "regular")

	// Admin creates two sites: one for admin, one for other user
	s.Sites.Create(ctx, &store.Site{ServerID: srvID, Name: "AdminSite", Domain: "admin.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)})
	s.Sites.Create(ctx, &store.Site{ServerID: srvID, Name: "OtherSite", Domain: "other.example.com", Runtime: "static", OwnerID: nullInt(otherUser.ID)})

	// Regular user creates one site
	s.Sites.Create(ctx, &store.Site{ServerID: srvID, Name: "UserSite", Domain: "user.example.com", Runtime: "static", OwnerID: nullInt(regularUser.ID)})

	// Regular user lists - should only see UserSite
	rec := doRequest(t, e, "GET", "/api/v1/sites", nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Sites []map[string]any `json:"sites"`
			Count int              `json:"count"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.Count != 1 {
		t.Fatalf("expected count 1, got %d", result.Data.Count)
	}
	if result.Data.Sites[0]["domain"] != "user.example.com" {
		t.Fatalf("expected user.example.com, got %v", result.Data.Sites[0]["domain"])
	}

	// Admin sees all 3
	rec2 := doRequest(t, e, "GET", "/api/v1/sites", nil, adminToken)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
	var result2 struct {
		Data struct {
			Sites []map[string]any `json:"sites"`
			Count int              `json:"count"`
		} `json:"data"`
	}
	json.NewDecoder(rec2.Body).Decode(&result2)
	if result2.Data.Count != 1 {
		t.Fatalf("expected admin count 1 (owned site only), got %d", result2.Data.Count)
	}
}

func TestCreate_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	body := map[string]any{
		"server_id": srvID,
		"name":      "My Site",
		"domain":    "mysite.example.com",
		"runtime":   "static",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			Site map[string]any `json:"site"`
			Job  map[string]any `json:"job"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data.Site["domain"] != "mysite.example.com" {
		t.Fatalf("expected mysite.example.com, got %v", result.Data.Site["domain"])
	}
	if result.Data.Site["status"] != "pending" {
		t.Fatalf("expected status pending, got %v", result.Data.Site["status"])
	}
	if result.Data.Job["type"] != jobmodel.TypeSiteCreate {
		t.Fatalf("expected job type %s, got %v", jobmodel.TypeSiteCreate, result.Data.Job["type"])
	}
}

func TestCreate_InvalidDomain(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	body := map[string]any{
		"server_id": srvID,
		"name":      "Bad",
		"domain":    "not a domain!",
		"runtime":   "static",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreate_InvalidRuntime(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	body := map[string]any{
		"server_id": srvID,
		"name":      "Bad",
		"domain":    "bad.example.com",
		"runtime":   "ruby",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreate_InvalidRootPath_PathTraversal(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	body := map[string]any{
		"server_id": srvID,
		"name":      "Bad",
		"domain":    "bad.example.com",
		"runtime":   "static",
		"root_path": "/var/www/../../etc/passwd",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreate_ServerNotFound(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	body := map[string]any{
		"server_id": 9999,
		"name":      "Orphan",
		"domain":    "orphan.example.com",
		"runtime":   "static",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreate_DuplicateDomain(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	body := map[string]any{
		"server_id": srvID,
		"name":      "First",
		"domain":    "dup.example.com",
		"runtime":   "static",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for first, got %d", rec.Code)
	}

	rec2 := doRequest(t, e, "POST", "/api/v1/sites", body, token)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate, got %d", rec2.Code)
	}
}

func TestCreate_SetsOwnerFromSession(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	body := map[string]any{
		"server_id": srvID,
		"name":      "Owned",
		"domain":    "owned.example.com",
		"runtime":   "static",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites", body, userToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			Site map[string]any `json:"site"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	regularUser, _ := s.Users.GetByUsername(ctx, "regular")
	if result.Data.Site["owner_id"] != float64(regularUser.ID) {
		t.Fatalf("expected owner_id %d, got %v", regularUser.ID, result.Data.Site["owner_id"])
	}
}

func TestGet_Found_Admin(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "X", Domain: "x.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "GET", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result struct {
		Data map[string]any `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data["domain"] != "x.example.com" {
		t.Fatalf("expected domain x.example.com, got %v", result.Data["domain"])
	}
}

func TestGet_Found_Owner(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	regularUser, _ := s.Users.GetByUsername(ctx, "regular")
	site := &store.Site{ServerID: srvID, Name: "Mine", Domain: "mine.example.com", Runtime: "static", OwnerID: nullInt(regularUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "GET", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10), nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result struct {
		Data map[string]any `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data["domain"] != "mine.example.com" {
		t.Fatalf("expected domain mine.example.com, got %v", result.Data["domain"])
	}
}

func TestGet_OtherUsersSite_AsRegularUser_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	// Create another user and site owned by them
	otherUser := &store.User{Username: "other", Email: "other@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	s.Users.Create(ctx, otherUser)
	site := &store.Site{ServerID: srvID, Name: "Other", Domain: "other.example.com", Runtime: "static", OwnerID: nullInt(otherUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "GET", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10), nil, userToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDelete_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Del", Domain: "del.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "DELETE", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			SiteID float64 `json:"site_id"`
			JobID  float64 `json:"job_id"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.SiteID != float64(site.ID) {
		t.Fatalf("expected site_id %d, got %v", site.ID, result.Data.SiteID)
	}

	// Verify site deleted
	got, _ := s.Sites.GetByID(ctx, site.ID)
	if got != nil {
		t.Fatal("expected site to be deleted")
	}
}

func TestDelete_RegularUser_NotOwner_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	// Create another user who owns the site
	otherUser := &store.User{Username: "owner", Email: "owner@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	s.Users.Create(ctx, otherUser)

	site := &store.Site{ServerID: srvID, Name: "AdminSite", Domain: "adminonly.example.com", Runtime: "static", OwnerID: nullInt(otherUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "DELETE", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10), nil, userToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeploy_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Dep", Domain: "dep.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/deploy", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without git repository, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestReloadNginx_AsOwner(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	regularUser, _ := s.Users.GetByUsername(ctx, "regular")
	site := &store.Site{ServerID: srvID, Name: "Reload", Domain: "reload.example.com", Runtime: "static", OwnerID: nullInt(regularUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/reload-nginx", nil, userToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			JobID  float64 `json:"job_id"`
			Status string  `json:"status"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.JobID == 0 {
		t.Fatal("expected non-zero job_id")
	}
	if result.Data.Status != jobmodel.StatusQueued {
		t.Fatalf("expected status %s, got %s", jobmodel.StatusQueued, result.Data.Status)
	}
}

func TestReloadNginx_AsAdmin(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	adminToken := newAdminToken(t, s, ctx)
	_ = newUserToken(t, s, ctx) // create regular user
	srvID := createServer(t, s, ctx)

	regularUser, _ := s.Users.GetByUsername(ctx, "regular")
	site := &store.Site{ServerID: srvID, Name: "Reload", Domain: "reload-admin.example.com", Runtime: "static", OwnerID: nullInt(regularUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/reload-nginx", nil, adminToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-owned site, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestReloadNginx_OtherUser_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	// Create another user who owns the site
	otherUser := &store.User{Username: "owner", Email: "owner@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	s.Users.Create(ctx, otherUser)

	site := &store.Site{ServerID: srvID, Name: "Reload", Domain: "reload-other.example.com", Runtime: "static", OwnerID: nullInt(otherUser.ID)}
	s.Sites.Create(ctx, site)

	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/reload-nginx", nil, userToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestReloadNginx_InvalidID_400(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)

	rec := doRequest(t, e, "POST", "/api/v1/sites/abc/reload-nginx", nil, userToken)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestReloadNginx_NotFound_404(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)

	rec := doRequest(t, e, "POST", "/api/v1/sites/9999/reload-nginx", nil, userToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateConfig_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Cfg", Domain: "cfg.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	body := map[string]any{
		"git_repo":   "https://github.com/user/repo.git",
		"git_branch": "main",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/config", body, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data map[string]any `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data["ok"] != true {
		t.Fatalf("expected ok true, got %v", result.Data["ok"])
	}
	if _, ok := result.Data["webhook_secret"]; !ok {
		t.Fatal("expected webhook_secret in response")
	}

	got, _ := s.Sites.GetByID(ctx, site.ID)
	if got == nil || !got.GitRepo.Valid || got.GitRepo.String != "https://github.com/user/repo.git" {
		t.Fatalf("expected git repo updated, got %v", got.GitRepo)
	}
	if !got.WebhookSecret.Valid || len(got.WebhookSecret.String) != 64 {
		t.Fatalf("expected webhook secret generated, got %v", got.WebhookSecret)
	}
}

func TestUpdateConfig_InvalidRepo(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Cfg", Domain: "cfg2.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	body := map[string]any{
		"git_repo": "git@github.com:user/repo.git",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(site.ID, 10)+"/config", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func nullInt(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: true}
}
