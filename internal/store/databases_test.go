package store

import (
	"database/sql"
	"testing"
)

// createTestSite creates a server and a site so database inserts satisfy the
// databases.site_id foreign key. Returns the new site ID.
func createTestSite(t *testing.T, db *sql.DB, domain string) int64 {
	t.Helper()
	servers := NewServerStore(db)
	sites := NewSiteStore(db)

	srv := &Server{
		Name:       "test-server",
		Hostname:   "srv.example.com",
		Status:     "active",
		AgentToken: "token-abc",
	}
	if err := servers.Create(ctx, srv); err != nil {
		t.Fatalf("create server: %v", err)
	}

	site := &Site{
		ServerID: srv.ID,
		Name:     "Test Site",
		Domain:   domain,
		Runtime:  "static",
	}
	if err := sites.Create(ctx, site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	return site.ID
}

func TestDatabaseCreate_Basic(t *testing.T) {
	db := testDB(t)
	s := NewDatabaseStore(db)

	siteID := createTestSite(t, db, "basic.example.com")

	database := &Database{
		SiteID:    siteID,
		Name:      "testdb",
		Type:      "mysql",
		Charset:   "utf8mb4",
		Collation: "utf8mb4_unicode_ci",
	}
	if err := s.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}
	if database.ID == 0 {
		t.Fatal("expected database ID to be set after create")
	}
	if database.Charset != "utf8mb4" {
		t.Fatalf("expected charset utf8mb4, got %s", database.Charset)
	}
	if database.Collation != "utf8mb4_unicode_ci" {
		t.Fatalf("expected collation utf8mb4_unicode_ci, got %s", database.Collation)
	}
}

func TestDatabaseCreate_AppliesDefaults(t *testing.T) {
	db := testDB(t)
	s := NewDatabaseStore(db)

	siteID := createTestSite(t, db, "defaults.example.com")

	database := &Database{
		SiteID: siteID,
		Name:   "defaultsdb",
		Type:   "postgres",
	}
	if err := s.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}
	if database.Charset != "utf8mb4" {
		t.Fatalf("expected default charset utf8mb4, got %s", database.Charset)
	}
	if database.Collation != "utf8mb4_unicode_ci" {
		t.Fatalf("expected default collation utf8mb4_unicode_ci, got %s", database.Collation)
	}

	got, err := s.GetByID(ctx, database.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got == nil {
		t.Fatal("expected database to be found")
	}
	if got.Charset != "utf8mb4" || got.Collation != "utf8mb4_unicode_ci" {
		t.Fatalf("expected defaults persisted, got charset=%s collation=%s", got.Charset, got.Collation)
	}
}

func TestDatabaseGetByID_Found(t *testing.T) {
	db := testDB(t)
	s := NewDatabaseStore(db)

	siteID := createTestSite(t, db, "getid.example.com")

	database := &Database{SiteID: siteID, Name: "getdb", Type: "mysql"}
	if err := s.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}

	got, err := s.GetByID(ctx, database.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got == nil {
		t.Fatal("expected database found")
	}
	if got.ID != database.ID {
		t.Fatalf("expected id %d, got %d", database.ID, got.ID)
	}
	if got.Name != "getdb" {
		t.Fatalf("expected name getdb, got %s", got.Name)
	}
	if got.Type != "mysql" {
		t.Fatalf("expected type mysql, got %s", got.Type)
	}
}

func TestDatabaseGetByID_NotFound(t *testing.T) {
	db := testDB(t)
	s := NewDatabaseStore(db)

	got, err := s.GetByID(ctx, 99999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent database")
	}
}

func TestDatabaseGetByName_Found(t *testing.T) {
	db := testDB(t)
	s := NewDatabaseStore(db)

	siteID := createTestSite(t, db, "getname.example.com")

	database := &Database{SiteID: siteID, Name: "nameddb", Type: "mysql"}
	if err := s.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}

	got, err := s.GetByName(ctx, "nameddb")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got == nil {
		t.Fatal("expected database found")
	}
	if got.ID != database.ID {
		t.Fatalf("expected id %d, got %d", database.ID, got.ID)
	}
}

func TestDatabaseGetByName_NotFound(t *testing.T) {
	db := testDB(t)
	s := NewDatabaseStore(db)

	got, err := s.GetByName(ctx, "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent name")
	}
}

func TestDatabaseListBySite(t *testing.T) {
	db := testDB(t)
	s := NewDatabaseStore(db)

	site1 := createTestSite(t, db, "list-a.example.com")
	site2 := createTestSite(t, db, "list-b.example.com")

	if err := s.Create(ctx, &Database{SiteID: site1, Name: "alpha", Type: "mysql"}); err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if err := s.Create(ctx, &Database{SiteID: site1, Name: "beta", Type: "mysql"}); err != nil {
		t.Fatalf("create beta: %v", err)
	}
	if err := s.Create(ctx, &Database{SiteID: site2, Name: "gamma", Type: "mysql"}); err != nil {
		t.Fatalf("create gamma: %v", err)
	}

	list, err := s.ListBySite(ctx, site1)
	if err != nil {
		t.Fatalf("list by site: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 databases for site %d, got %d", site1, len(list))
	}

	for _, d := range list {
		if d.SiteID != site1 {
			t.Fatalf("expected site_id %d, got %d", site1, d.SiteID)
		}
	}

	// Order check: ASC by id
	if list[0].Name != "alpha" || list[1].Name != "beta" {
		t.Fatalf("expected ordering alpha,beta; got %s,%s", list[0].Name, list[1].Name)
	}

	list2, err := s.ListBySite(ctx, site2)
	if err != nil {
		t.Fatalf("list by site 2: %v", err)
	}
	if len(list2) != 1 {
		t.Fatalf("expected 1 database for site %d, got %d", site2, len(list2))
	}
}

func TestDatabaseDelete(t *testing.T) {
	db := testDB(t)
	dbs := NewDatabaseStore(db)
	users := NewDBUserStore(db)

	siteID := createTestSite(t, db, "delete.example.com")

	database := &Database{SiteID: siteID, Name: "deletedb", Type: "mysql"}
	if err := dbs.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}

	// Attach a db user to verify CASCADE
	user := &DBUser{
		DatabaseID:   database.ID,
		Username:     "child",
		PasswordHash: "hash",
		HostPattern:  "localhost",
		Privileges:   "ALL",
	}
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create db user: %v", err)
	}

	if err := dbs.Delete(ctx, database.ID); err != nil {
		t.Fatalf("delete database: %v", err)
	}

	got, err := dbs.GetByID(ctx, database.ID)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if got != nil {
		t.Fatal("expected database to be deleted")
	}

	// CASCADE: db_users referencing this database should also be gone.
	gotUser, err := users.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user after cascade: %v", err)
	}
	if gotUser != nil {
		t.Fatalf("expected db_user to be cascaded-deleted, still found: %+v", gotUser)
	}
}

func TestDBUserCreate_Basic(t *testing.T) {
	db := testDB(t)
	dbs := NewDatabaseStore(db)
	users := NewDBUserStore(db)

	siteID := createTestSite(t, db, "userdb.example.com")

	database := &Database{SiteID: siteID, Name: "userdb", Type: "mysql"}
	if err := dbs.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}

	user := &DBUser{
		DatabaseID:   database.ID,
		Username:     "appuser",
		PasswordHash: "argon2id$hash",
		HostPattern:  "%",
		Privileges:   "SELECT,INSERT",
	}
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create db user: %v", err)
	}
	if user.ID == 0 {
		t.Fatal("expected db user ID to be set")
	}

	got, err := users.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get db user by id: %v", err)
	}
	if got == nil {
		t.Fatal("expected db user found")
	}
	if got.Username != "appuser" {
		t.Fatalf("expected username appuser, got %s", got.Username)
	}
	if got.HostPattern != "%" {
		t.Fatalf("expected host_pattern %%, got %s", got.HostPattern)
	}
	if got.Privileges != "SELECT,INSERT" {
		t.Fatalf("expected privileges SELECT,INSERT, got %s", got.Privileges)
	}
}

func TestDBUserListByDatabase(t *testing.T) {
	db := testDB(t)
	dbs := NewDatabaseStore(db)
	users := NewDBUserStore(db)

	siteID := createTestSite(t, db, "multiuserdb.example.com")

	database := &Database{SiteID: siteID, Name: "multiuserdb", Type: "mysql"}
	if err := dbs.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}

	other := &Database{SiteID: siteID, Name: "otheruserdb", Type: "mysql"}
	if err := dbs.Create(ctx, other); err != nil {
		t.Fatalf("create other database: %v", err)
	}

	if err := users.Create(ctx, &DBUser{DatabaseID: database.ID, Username: "u1", PasswordHash: "h1"}); err != nil {
		t.Fatalf("create u1: %v", err)
	}
	if err := users.Create(ctx, &DBUser{DatabaseID: database.ID, Username: "u2", PasswordHash: "h2"}); err != nil {
		t.Fatalf("create u2: %v", err)
	}
	if err := users.Create(ctx, &DBUser{DatabaseID: other.ID, Username: "u3", PasswordHash: "h3"}); err != nil {
		t.Fatalf("create u3: %v", err)
	}

	list, err := users.ListByDatabase(ctx, database.ID)
	if err != nil {
		t.Fatalf("list db users: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 db users, got %d", len(list))
	}
	for _, u := range list {
		if u.DatabaseID != database.ID {
			t.Fatalf("expected database_id %d, got %d", database.ID, u.DatabaseID)
		}
	}

	// Order: ASC by id (u1 then u2)
	if list[0].Username != "u1" || list[1].Username != "u2" {
		t.Fatalf("expected ordering u1,u2; got %s,%s", list[0].Username, list[1].Username)
	}
}

func TestDBUserDelete(t *testing.T) {
	db := testDB(t)
	dbs := NewDatabaseStore(db)
	users := NewDBUserStore(db)

	siteID := createTestSite(t, db, "deluserdb.example.com")

	database := &Database{SiteID: siteID, Name: "deluserdb", Type: "mysql"}
	if err := dbs.Create(ctx, database); err != nil {
		t.Fatalf("create database: %v", err)
	}

	user := &DBUser{
		DatabaseID:   database.ID,
		Username:     "toremove",
		PasswordHash: "h",
	}
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create db user: %v", err)
	}

	if err := users.Delete(ctx, user.ID); err != nil {
		t.Fatalf("delete db user: %v", err)
	}

	got, err := users.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if got != nil {
		t.Fatal("expected db user to be deleted")
	}

	// Database itself must remain intact.
	gotDB, err := dbs.GetByID(ctx, database.ID)
	if err != nil {
		t.Fatalf("get database after user delete: %v", err)
	}
	if gotDB == nil {
		t.Fatal("expected database to remain after db_user delete")
	}
}
