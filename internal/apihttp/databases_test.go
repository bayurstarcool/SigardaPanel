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
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

// newTestAppDB returns a fully wired Echo router backed by an in-memory SQLite
// database with all schemas (including DatabasesSchema and JobsSchema) applied.
func newTestAppDB(t *testing.T) (*echo.Echo, *store.Store, context.Context) {
	t.Helper()
	f, _ := os.CreateTemp("", "db_api_test_*.db")
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

// createSiteWithOwner creates a server, a user with the given role, a site
// owned by that user, and returns the site ID plus a session token for the user.
func createSiteWithOwner(t *testing.T, s *store.Store, ctx context.Context, username, role string) (int64, string) {
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

	owner := &store.User{
		Username:     username,
		Email:        username + "@test.com",
		PasswordHash: "password123",
		Role:         role,
	}
	if err := s.Users.Create(ctx, owner); err != nil {
		t.Fatalf("create owner: %v", err)
	}

	site := &store.Site{
		ServerID: server.ID,
		Name:     "test-site",
		Domain:   "test.example.com",
		Runtime:  "static",
		RootPath: "/var/www/test",
		OwnerID:  sql.NullInt64{Int64: owner.ID, Valid: true},
	}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatalf("create site: %v", err)
	}

	token, err := auth.NewToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	if err := s.Sessions.Create(ctx, &store.Session{
		UserID:    owner.ID,
		TokenHash: auth.HashToken(token),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	return site.ID, token
}

func TestDatabaseCreate_Success(t *testing.T) {
	e, s, ctx := newTestAppDB(t)
	siteID, token := createSiteWithOwner(t, s, ctx, "owner1", auth.RoleAdmin)

	body := map[string]string{
		"name": "appdb",
		"type": "mysql",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(siteID, 10)+"/databases", body, token)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			DatabaseID float64 `json:"database_id"`
			JobID      float64 `json:"job_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data.DatabaseID == 0 {
		t.Fatal("expected non-zero database_id")
	}
	if result.Data.JobID == 0 {
		t.Fatal("expected non-zero job_id")
	}

	// Verify database persisted with expected fields
	got, err := s.Databases.GetByID(ctx, int64(result.Data.DatabaseID))
	if err != nil {
		t.Fatalf("get database: %v", err)
	}
	if got == nil {
		t.Fatal("expected database in store")
	}
	if got.Name != "sp_appdb" || got.Type != "mysql" {
		t.Fatalf("unexpected database: %+v", got)
	}
}

func TestDatabaseCreate_InvalidName_400(t *testing.T) {
	e, s, ctx := newTestAppDB(t)
	siteID, token := createSiteWithOwner(t, s, ctx, "owner2", auth.RoleAdmin)

	body := map[string]string{
		"name": "invalid-name-with-dash",
		"type": "mysql",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(siteID, 10)+"/databases", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestDatabaseCreate_InvalidType_400(t *testing.T) {
	e, s, ctx := newTestAppDB(t)
	siteID, token := createSiteWithOwner(t, s, ctx, "owner3", auth.RoleAdmin)

	body := map[string]string{
		"name": "appdb",
		"type": "mongodb",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(siteID, 10)+"/databases", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestDatabaseCreate_SiteNotFound_404(t *testing.T) {
	e, s, ctx := newTestAppDB(t)
	_, token := createSiteWithOwner(t, s, ctx, "owner4", auth.RoleAdmin)

	body := map[string]string{
		"name": "appdb",
		"type": "mysql",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites/99999/databases", body, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestDatabaseCreate_OtherUser_404(t *testing.T) {
	e, s, ctx := newTestAppDB(t)
	// Site owned by "owner_a", token belongs to "user_b" (different user)
	siteID, _ := createSiteWithOwner(t, s, ctx, "owner_a", auth.RoleUser)

	// Create a separate user that is not the site owner
	otherUser := &store.User{
		Username:     "user_b",
		Email:        "user_b@test.com",
		PasswordHash: "password123",
		Role:         auth.RoleUser,
	}
	if err := s.Users.Create(ctx, otherUser); err != nil {
		t.Fatalf("create other user: %v", err)
	}
	otherToken, err := auth.NewToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	if err := s.Sessions.Create(ctx, &store.Session{
		UserID:    otherUser.ID,
		TokenHash: auth.HashToken(otherToken),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	body := map[string]string{
		"name": "appdb",
		"type": "mysql",
	}
	rec := doRequest(t, e, "POST", "/api/v1/sites/"+strconv.FormatInt(siteID, 10)+"/databases", body, otherToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestDatabaseList_Success(t *testing.T) {
	e, s, ctx := newTestAppDB(t)
	siteID, token := createSiteWithOwner(t, s, ctx, "owner5", auth.RoleAdmin)

	if err := s.Databases.Create(ctx, &store.Database{SiteID: siteID, Name: "alpha", Type: "mysql"}); err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if err := s.Databases.Create(ctx, &store.Database{SiteID: siteID, Name: "beta", Type: "mysql"}); err != nil {
		t.Fatalf("create beta: %v", err)
	}

	rec := doRequest(t, e, "GET", "/api/v1/sites/"+strconv.FormatInt(siteID, 10)+"/databases", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			Databases []map[string]any `json:"databases"`
			Count     int              `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data.Count != 2 {
		t.Fatalf("expected count 2, got %d", result.Data.Count)
	}
	if len(result.Data.Databases) != 2 {
		t.Fatalf("expected 2 databases in list, got %d", len(result.Data.Databases))
	}
}

func TestDatabaseGet_Found(t *testing.T) {
	e, s, ctx := newTestAppDB(t)
	siteID, token := createSiteWithOwner(t, s, ctx, "owner6", auth.RoleAdmin)

	database := &store.Database{SiteID: siteID, Name: "getdb", Type: "mysql"}
	if err := s.Databases.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}

	rec := doRequest(t, e, "GET", "/api/v1/databases/"+strconv.FormatInt(database.ID, 10), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data["name"] != "getdb" {
		t.Fatalf("expected name getdb, got %v", result.Data["name"])
	}
	if result.Data["type"] != "mysql" {
		t.Fatalf("expected type mysql, got %v", result.Data["type"])
	}
}

func TestDatabaseGet_NotFound_404(t *testing.T) {
	e, s, ctx := newTestAppDB(t)
	_, token := createSiteWithOwner(t, s, ctx, "owner7", auth.RoleAdmin)

	rec := doRequest(t, e, "GET", "/api/v1/databases/99999", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestDatabaseDelete_Success(t *testing.T) {
	e, s, ctx := newTestAppDB(t)
	siteID, token := createSiteWithOwner(t, s, ctx, "owner8", auth.RoleAdmin)

	database := &store.Database{SiteID: siteID, Name: "deldb", Type: "mysql"}
	if err := s.Databases.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}

	rec := doRequest(t, e, "DELETE", "/api/v1/databases/"+strconv.FormatInt(database.ID, 10), nil, token)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			DatabaseID float64 `json:"database_id"`
			JobID      float64 `json:"job_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if int64(result.Data.DatabaseID) != database.ID {
		t.Fatalf("expected database_id %d, got %v", database.ID, result.Data.DatabaseID)
	}
	if result.Data.JobID == 0 {
		t.Fatal("expected non-zero job_id")
	}

	// Database should be hard-deleted from store (per deleteDatabase handler).
	got, _ := s.Databases.GetByID(ctx, database.ID)
	if got != nil {
		t.Fatal("expected database to be deleted")
	}
}

func TestDBUserCreate_Success(t *testing.T) {
	e, s, ctx := newTestAppDB(t)
	siteID, token := createSiteWithOwner(t, s, ctx, "owner9", auth.RoleAdmin)

	database := &store.Database{SiteID: siteID, Name: "userdb", Type: "mysql"}
	if err := s.Databases.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}

	body := map[string]string{
		"username": "appuser",
		"password": "supersecret",
	}
	rec := doRequest(t, e, "POST", "/api/v1/databases/"+strconv.FormatInt(database.ID, 10)+"/users", body, token)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			UserID   float64 `json:"user_id"`
			Username string  `json:"username"`
			JobID    float64 `json:"job_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data.UserID == 0 {
		t.Fatal("expected non-zero user_id")
	}
	if result.Data.Username != "appuser" {
		t.Fatalf("expected username appuser, got %s", result.Data.Username)
	}
	if result.Data.JobID == 0 {
		t.Fatal("expected non-zero job_id")
	}

	// Verify password was hashed (not stored in plaintext)
	got, err := s.DBUsers.GetByID(ctx, int64(result.Data.UserID))
	if err != nil {
		t.Fatalf("get db user: %v", err)
	}
	if got == nil {
		t.Fatal("expected db user in store")
	}
	if got.PasswordHash == "supersecret" {
		t.Fatal("expected password to be hashed, got plaintext")
	}
}

func TestDBUserCreate_ShortPassword_400(t *testing.T) {
	e, s, ctx := newTestAppDB(t)
	siteID, token := createSiteWithOwner(t, s, ctx, "owner10", auth.RoleAdmin)

	database := &store.Database{SiteID: siteID, Name: "shortpwddb", Type: "mysql"}
	if err := s.Databases.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}

	body := map[string]string{
		"username": "appuser",
		"password": "short",
	}
	rec := doRequest(t, e, "POST", "/api/v1/databases/"+strconv.FormatInt(database.ID, 10)+"/users", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", rec.Code, rec.Body.String())
	}
}
