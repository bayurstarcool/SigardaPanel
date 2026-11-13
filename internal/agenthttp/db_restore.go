package agenthttp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// restoreDatabase is one of the site's databases a restore may import into.
// MySQL/MariaDB imports run as root over the local socket (the dedicated
// backup account is read-only); PostgreSQL uses the database's own account.
type restoreDatabase struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	User     string `json:"user"`
	Password string `json:"password"`
}

func isSQLDump(path string) bool { return strings.HasSuffix(strings.ToLower(path), ".sql") }

// importDatabaseDump loads a plain SQL dump into an existing database. Both
// clients stop at the first error; psql applies the dump in one transaction.
func (r *Router) importDatabaseDump(ctx context.Context, db restoreDatabase, file string) (string, error) {
	if !isValidDBIdentifier(db.Name) {
		return "", fmt.Errorf("invalid database name %q", db.Name)
	}
	if strings.ContainsAny(file, " \t\r\n;'\"\\") {
		return "", fmt.Errorf("unsafe dump path")
	}
	switch strings.ToLower(db.Type) {
	case "mysql", "mariadb":
		return r.executor.RunShell(ctx, "mysql", db.Name, "-e", "source "+file)
	case "postgres", "postgresql":
		if !isValidDBIdentifier(db.User) {
			return "", fmt.Errorf("invalid database user")
		}
		return r.executor.RunShellEnv(ctx, []string{"PGPASSWORD=" + db.Password}, "psql",
			"-U"+db.User, "-h", "localhost", "-d", db.Name, "-v", "ON_ERROR_STOP=1", "--single-transaction", "-q", "-f", file)
	default:
		return "", fmt.Errorf("unsupported database type %q", db.Type)
	}
}

// pickDumpDatabase chooses the database a single-database dump belongs to:
// its file name starts with database_<name>_ (see databaseBackupTask); a
// site with one database needs no match.
func pickDumpDatabase(dbs []restoreDatabase, fileName string) (restoreDatabase, bool) {
	var best restoreDatabase
	bestLen := 0
	for _, db := range dbs {
		prefix := "database_" + sanitizeBackupName(db.Name) + "_"
		if strings.HasPrefix(strings.ToLower(fileName), prefix) && len(prefix) > bestLen {
			best, bestLen = db, len(prefix)
		}
	}
	if bestLen > 0 {
		return best, true
	}
	if len(dbs) == 1 {
		return dbs[0], true
	}
	return restoreDatabase{}, false
}

// importBundleDumps imports the database_dumps/<name>.sql files of a full
// backup into the matching databases. Dumps without a matching database
// (e.g. one deleted since) are skipped and reported.
func (r *Router) importBundleDumps(ctx context.Context, dumpsDir string, dbs []restoreDatabase) (restored, skipped []string, err error) {
	entries, rerr := os.ReadDir(dumpsDir)
	if rerr != nil {
		return nil, nil, nil // a files-only archive
	}
	byFile := map[string]restoreDatabase{}
	for _, db := range dbs {
		byFile[sanitizeBackupName(db.Name)+".sql"] = db
	}
	names := []string{}
	for _, e := range entries {
		if e.Type().IsRegular() && isSQLDump(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		db, ok := byFile[n]
		if !ok {
			skipped = append(skipped, strings.TrimSuffix(n, ".sql"))
			continue
		}
		if out, err := r.importDatabaseDump(ctx, db, filepath.Join(dumpsDir, n)); err != nil {
			return restored, skipped, fmt.Errorf("import into %s failed: %s", db.Name, strings.TrimSpace(out+" "+err.Error()))
		}
		restored = append(restored, db.Name)
	}
	return restored, skipped, nil
}
