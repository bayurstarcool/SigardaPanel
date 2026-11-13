package agenthttp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPickDumpDatabase(t *testing.T) {
	dbs := []restoreDatabase{{Name: "shop", Type: "mysql"}, {Name: "shop_logs", Type: "mysql"}}
	if db, ok := pickDumpDatabase(dbs, "database_shop_logs_20261101-030000_9.sql"); !ok || db.Name != "shop_logs" {
		t.Fatalf("picked %v %v", db, ok)
	}
	if db, ok := pickDumpDatabase(dbs, "database_shop_20261101-030000_9.sql"); !ok || db.Name != "shop" {
		t.Fatalf("picked %v %v", db, ok)
	}
	if _, ok := pickDumpDatabase(dbs, "database_other_1.sql"); ok {
		t.Fatal("matched a dump of an unknown database")
	}
	if db, ok := pickDumpDatabase(dbs[:1], "restore_9.sql"); !ok || db.Name != "shop" {
		t.Fatal("single database not used")
	}
}

func TestImportBundleDumps(t *testing.T) {
	bin := setupTestBinaries(t)
	log := filepath.Join(t.TempDir(), "calls")
	fakeCommand(t, bin, "mysql", log, 0)
	fakeCommand(t, bin, "psql", log, 0)
	r := &Router{executor: newTestExecutor()}
	dir := t.TempDir()
	for _, n := range []string{"shop.sql", "pg.sql", "gone.sql"} {
		os.WriteFile(filepath.Join(dir, n), []byte("-- dump"), 0o600)
	}
	restored, skipped, err := r.importBundleDumps(t.Context(), dir, []restoreDatabase{
		{Name: "shop", Type: "mysql"}, {Name: "pg", Type: "postgres", User: "pguser", Password: "pw"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(restored, ",") != "pg,shop" || strings.Join(skipped, ",") != "gone" {
		t.Fatalf("restored=%v skipped=%v", restored, skipped)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "mysql shop -e source "+filepath.Join(dir, "shop.sql")) ||
		!strings.Contains(string(calls), "psql -Upguser -h localhost -d pg -v ON_ERROR_STOP=1 --single-transaction") {
		t.Fatalf("calls:\n%s", calls)
	}

	// A failing import is reported (the caller then leaves the files alone).
	fakeCommand(t, bin, "mysql", log, 1)
	if _, _, err := r.importBundleDumps(t.Context(), dir, []restoreDatabase{{Name: "shop", Type: "mysql"}}); err == nil {
		t.Fatal("failed import not reported")
	}
}
