package store

import (
	"database/sql"
	"testing"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/jobmodel"
)

func TestSiteCreate_Basic(t *testing.T) {
	db := testDB(t)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	// Create a server first
	srv := &Server{Name: "srv", Hostname: "srv.example.com", Status: "active", AgentToken: "tok"}
	if err := servers.Create(ctx, srv); err != nil {
		t.Fatalf("create server: %v", err)
	}

	site := &Site{
		ServerID: srv.ID,
		Name:     "Test Site",
		Domain:   "test.example.com",
		Runtime:  "static",
	}
	if err := sites.Create(ctx, site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	if site.ID == 0 {
		t.Fatal("site ID not set after create")
	}
	if site.Status != "pending" {
		t.Fatalf("expected status pending, got %s", site.Status)
	}
	if site.RootPath != "/home/test.example.com/htdocs/test.example.com" {
		t.Fatalf("expected root path /home/test.example.com/htdocs/test.example.com, got %s", site.RootPath)
	}
}

func TestSiteGetByID_Found(t *testing.T) {
	db := testDB(t)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	srv := &Server{Name: "srv", Hostname: "srv.example.com", Status: "active", AgentToken: "tok"}
	servers.Create(ctx, srv)

	site := &Site{ServerID: srv.ID, Name: "A", Domain: "a.example.com", Runtime: "static"}
	sites.Create(ctx, site)

	got, err := sites.GetByID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got == nil {
		t.Fatal("expected site, got nil")
	}
	if got.Domain != "a.example.com" {
		t.Fatalf("expected domain a.example.com, got %s", got.Domain)
	}
}

func TestSiteGetByID_NotFound(t *testing.T) {
	db := testDB(t)
	sites := NewSiteStore(db)

	got, err := sites.GetByID(ctx, 9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil, got site")
	}
}

func TestSiteGetByDomain(t *testing.T) {
	db := testDB(t)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	srv := &Server{Name: "srv", Hostname: "srv.example.com", Status: "active", AgentToken: "tok"}
	servers.Create(ctx, srv)

	site := &Site{ServerID: srv.ID, Name: "A", Domain: "find.example.com", Runtime: "static"}
	sites.Create(ctx, site)

	got, err := sites.GetByDomain(ctx, "find.example.com")
	if err != nil {
		t.Fatalf("get by domain: %v", err)
	}
	if got == nil {
		t.Fatal("expected site, got nil")
	}
	if got.ID != site.ID {
		t.Fatalf("expected ID %d, got %d", site.ID, got.ID)
	}
}

func TestSiteList_FilteredByServer(t *testing.T) {
	db := testDB(t)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	srv1 := &Server{Name: "s1", Hostname: "s1.example.com", Status: "active", AgentToken: "t1"}
	srv2 := &Server{Name: "s2", Hostname: "s2.example.com", Status: "active", AgentToken: "t2"}
	servers.Create(ctx, srv1)
	servers.Create(ctx, srv2)

	sites.Create(ctx, &Site{ServerID: srv1.ID, Name: "A", Domain: "a.example.com", Runtime: "static"})
	sites.Create(ctx, &Site{ServerID: srv2.ID, Name: "B", Domain: "b.example.com", Runtime: "static"})
	sites.Create(ctx, &Site{ServerID: srv1.ID, Name: "C", Domain: "c.example.com", Runtime: "static"})

	list, err := sites.List(ctx, SiteListFilter{ServerID: srv1.ID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 sites for server %d, got %d", srv1.ID, len(list))
	}
}

func TestSiteList_FilteredByOwner(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	u1 := &User{Username: "alice", Email: "a@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	u2 := &User{Username: "bob", Email: "b@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	users.Create(ctx, u1)
	users.Create(ctx, u2)

	srv := &Server{Name: "srv", Hostname: "srv.example.com", Status: "active", AgentToken: "tok"}
	servers.Create(ctx, srv)

	sites.Create(ctx, &Site{ServerID: srv.ID, Name: "A", Domain: "a.example.com", Runtime: "static", OwnerID: sql.NullInt64{Int64: u1.ID, Valid: true}})
	sites.Create(ctx, &Site{ServerID: srv.ID, Name: "B", Domain: "b.example.com", Runtime: "static", OwnerID: sql.NullInt64{Int64: u2.ID, Valid: true}})

	list, err := sites.List(ctx, SiteListFilter{OwnerID: u1.ID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 site for owner %d, got %d", u1.ID, len(list))
	}
	if list[0].Domain != "a.example.com" {
		t.Fatalf("expected domain a.example.com, got %s", list[0].Domain)
	}
}

func TestSiteUpdate_Status(t *testing.T) {
	db := testDB(t)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	srv := &Server{Name: "srv", Hostname: "srv.example.com", Status: "active", AgentToken: "tok"}
	servers.Create(ctx, srv)

	site := &Site{ServerID: srv.ID, Name: "A", Domain: "a.example.com", Runtime: "static"}
	sites.Create(ctx, site)

	if err := sites.UpdateStatus(ctx, site.ID, "active"); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, err := sites.GetByID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Status != "active" {
		t.Fatalf("expected status active, got %s", got.Status)
	}
}

func TestSiteDelete_Cascade(t *testing.T) {
	db := testDB(t)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)
	jobStore := NewJobStore(db)

	srv := &Server{Name: "srv", Hostname: "srv.example.com", Status: "active", AgentToken: "tok"}
	servers.Create(ctx, srv)

	site := &Site{ServerID: srv.ID, Name: "A", Domain: "del.example.com", Runtime: "static"}
	sites.Create(ctx, site)

	job := &jobmodel.Job{
		ServerID: sql.NullInt64{Int64: srv.ID, Valid: true},
		SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
		Type:     jobmodel.TypeSiteCreate,
		Payload:  `{}`,
	}
	if err := jobStore.Create(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := sites.Delete(ctx, site.ID); err != nil {
		t.Fatalf("delete site: %v", err)
	}

	got, err := sites.GetByID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if got != nil {
		t.Fatal("expected site to be deleted")
	}

	gotJob, err := jobStore.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job after delete: %v", err)
	}
	if gotJob != nil {
		t.Fatal("expected job to be cascade deleted")
	}
}

func TestSiteCreate_DefaultSSLStatus(t *testing.T) {
	db := testDB(t)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	srv := &Server{Name: "srv", Hostname: "srv.example.com", Status: "active", AgentToken: "tok"}
	if err := servers.Create(ctx, srv); err != nil {
		t.Fatalf("create server: %v", err)
	}

	site := &Site{ServerID: srv.ID, Name: "Test", Domain: "ssl.example.com", Runtime: "static"}
	if err := sites.Create(ctx, site); err != nil {
		t.Fatalf("create site: %v", err)
	}

	got, err := sites.GetByID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got == nil {
		t.Fatal("expected site, got nil")
	}
	if !got.SSLStatus.Valid || got.SSLStatus.String != "none" {
		t.Fatalf("expected ssl_status none, got %v", got.SSLStatus)
	}
}

func TestSiteUpdateSSL(t *testing.T) {
	db := testDB(t)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	srv := &Server{Name: "srv", Hostname: "srv.example.com", Status: "active", AgentToken: "tok"}
	if err := servers.Create(ctx, srv); err != nil {
		t.Fatalf("create server: %v", err)
	}

	site := &Site{ServerID: srv.ID, Name: "Test", Domain: "ssl2.example.com", Runtime: "static"}
	if err := sites.Create(ctx, site); err != nil {
		t.Fatalf("create site: %v", err)
	}

	expiry := time.Now().Add(90 * 24 * time.Hour)
	if err := sites.UpdateSSL(ctx, site.ID, "active", "Let's Encrypt", expiry); err != nil {
		t.Fatalf("update ssl: %v", err)
	}

	got, err := sites.GetByID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.SSLStatus.String != "active" {
		t.Fatalf("expected ssl_status active, got %s", got.SSLStatus.String)
	}
	if got.SSLIssuer.String != "Let's Encrypt" {
		t.Fatalf("expected ssl_issuer Let's Encrypt, got %s", got.SSLIssuer.String)
	}
	if !got.SSLExpiresAt.Valid {
		t.Fatal("expected ssl_expires_at to be valid")
	}
	if !got.SSLLastCheck.Valid {
		t.Fatal("expected ssl_last_check to be valid")
	}
}

func TestSiteCountByOwner(t *testing.T) {
	db := testDB(t)
	users := NewUserStore(db)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	u := &User{Username: "carol", Email: "c@example.com", PasswordHash: "pass", Role: auth.RoleUser}
	users.Create(ctx, u)

	srv := &Server{Name: "srv", Hostname: "srv.example.com", Status: "active", AgentToken: "tok"}
	servers.Create(ctx, srv)

	sites.Create(ctx, &Site{ServerID: srv.ID, Name: "A", Domain: "a.example.com", Runtime: "static", OwnerID: sql.NullInt64{Int64: u.ID, Valid: true}})
	sites.Create(ctx, &Site{ServerID: srv.ID, Name: "B", Domain: "b.example.com", Runtime: "static", OwnerID: sql.NullInt64{Int64: u.ID, Valid: true}})

	count, err := sites.CountByOwner(ctx, u.ID)
	if err != nil {
		t.Fatalf("count by owner: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}
}

func TestSiteUpdateGitConfig(t *testing.T) {
	db := testDB(t)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	srv := &Server{Name: "srv", Hostname: "srv.example.com", Status: "active", AgentToken: "tok"}
	servers.Create(ctx, srv)

	site := &Site{ServerID: srv.ID, Name: "A", Domain: "a.example.com", Runtime: "static"}
	sites.Create(ctx, site)

	if err := sites.UpdateGitConfig(ctx, site.ID, "https://github.com/user/repo.git", "develop"); err != nil {
		t.Fatalf("update git config: %v", err)
	}

	got, err := sites.GetByID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if !got.GitRepo.Valid || got.GitRepo.String != "https://github.com/user/repo.git" {
		t.Fatalf("expected git repo, got %v", got.GitRepo)
	}
	if !got.GitBranch.Valid || got.GitBranch.String != "develop" {
		t.Fatalf("expected git branch develop, got %v", got.GitBranch)
	}
}

func TestSiteGenerateWebhookSecret(t *testing.T) {
	db := testDB(t)
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	srv := &Server{Name: "srv", Hostname: "srv.example.com", Status: "active", AgentToken: "tok"}
	servers.Create(ctx, srv)

	site := &Site{ServerID: srv.ID, Name: "A", Domain: "a.example.com", Runtime: "static"}
	sites.Create(ctx, site)

	secret, err := sites.GenerateWebhookSecret(ctx, site.ID)
	if err != nil {
		t.Fatalf("generate webhook secret: %v", err)
	}
	if len(secret) != 64 {
		t.Fatalf("expected secret length 64, got %d", len(secret))
	}

	got, err := sites.GetByID(ctx, site.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if !got.WebhookSecret.Valid || got.WebhookSecret.String != secret {
		t.Fatalf("expected webhook secret %s, got %v", secret, got.WebhookSecret)
	}
}
