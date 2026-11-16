package agenthttp

import (
	"context"
	"net/http"
	"os/exec"
	"os/user"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
)

// Database discovery: list the databases that exist on this server, created
// by the panel or not, so a super admin can import the ones made outside the
// panel as "external" databases (the panel shows and uses them but never
// drops, alters or re-grants them).
//
// Read only. Both queries are fixed strings sent on stdin (no name from the
// request reaches a command line or SQL text); the engines run as their admin
// accounts, like the other database tasks: MySQL/MariaDB as root through the
// local socket, PostgreSQL as the "postgres" OS account (peer authentication).
// An engine that is not installed is skipped silently.

// discoveredDatabase is one database found on the server.
type discoveredDatabase struct {
	Engine    string `json:"engine"` // "mysql" or "postgres"
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	SizeBytes int64  `json:"size_bytes"`
}

// mysqlDiscoverSQL lists every schema but the server's own, with the accounts
// that hold database-level grants on it (mysql.db stores "_" escaped as "\_")
// and its data + index size.
const mysqlDiscoverSQL = `SELECT s.SCHEMA_NAME,
  COALESCE((SELECT GROUP_CONCAT(DISTINCT d.User ORDER BY d.User SEPARATOR ',') FROM mysql.db d
            WHERE d.Db = s.SCHEMA_NAME OR d.Db = REPLACE(s.SCHEMA_NAME, '_', '\\_')), ''),
  COALESCE((SELECT SUM(t.DATA_LENGTH + t.INDEX_LENGTH) FROM information_schema.TABLES t
            WHERE t.TABLE_SCHEMA = s.SCHEMA_NAME), 0)
FROM information_schema.SCHEMATA s
WHERE s.SCHEMA_NAME NOT IN ('information_schema', 'performance_schema', 'mysql', 'sys', 'phpmyadmin')
ORDER BY s.SCHEMA_NAME;
`

// postgresDiscoverSQL lists every database but the templates and the
// maintenance database "postgres", with its owner and size.
const postgresDiscoverSQL = `SELECT datname, pg_get_userbyid(datdba), pg_database_size(datname)
FROM pg_database
WHERE NOT datistemplate AND datname <> 'postgres'
ORDER BY datname;
`

// databaseDiscoverTask answers POST /tasks/databases/discover.
func (r *Router) databaseDiscoverTask(c *echo.Context) error {
	ctx := c.Request().Context()
	out := []discoveredDatabase{}
	errs := map[string]string{}

	if mysqlInstalled() {
		dbs, err := r.discoverMySQL(ctx)
		if err != nil {
			errs["mysql"] = err.Error()
		}
		out = append(out, dbs...)
	}
	if postgresInstalled() {
		dbs, err := r.discoverPostgres(ctx)
		if err != nil {
			errs["postgres"] = err.Error()
		}
		out = append(out, dbs...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Engine != out[j].Engine {
			return out[i].Engine < out[j].Engine
		}
		return out[i].Name < out[j].Name
	})
	resp := map[string]any{"ok": true, "databases": out}
	if len(errs) > 0 {
		resp["errors"] = errs
	}
	return c.JSON(http.StatusOK, resp)
}

// mysqlInstalled reports whether a MySQL/MariaDB client is on this server.
func mysqlInstalled() bool {
	_, err := exec.LookPath("mysql")
	return err == nil
}

// postgresInstalled reports whether PostgreSQL (its OS account and psql) is
// on this server.
func postgresInstalled() bool {
	if _, err := user.Lookup("postgres"); err != nil {
		return false
	}
	_, err := exec.LookPath("psql")
	return err == nil
}

func (r *Router) discoverMySQL(ctx context.Context) ([]discoveredDatabase, error) {
	// -N -B: no column names, tab-separated rows.
	output, err := r.executor.RunWithInput(ctx, mysqlDiscoverSQL, "mysql", "-N", "-B")
	if err != nil {
		return nil, &discoverError{"mysql: " + pgErr(output, err)}
	}
	return parseDiscoveredRows("mysql", output), nil
}

func (r *Router) discoverPostgres(ctx context.Context) ([]discoveredDatabase, error) {
	u, err := user.Lookup("postgres")
	if err != nil {
		return nil, nil
	}
	uid, err1 := strconv.ParseUint(u.Uid, 10, 31)
	gid, err2 := strconv.ParseUint(u.Gid, 10, 31)
	if err1 != nil || err2 != nil || uid == 0 {
		return nil, &discoverError{"postgres: unexpected postgres account"}
	}
	// -A -t -F <tab>: unaligned, tuples only, tab-separated rows.
	output, err := r.executor.RunAsUserInput(ctx, uint32(uid), uint32(gid), nil, strings.NewReader(postgresDiscoverSQL),
		"psql", "-X", "-q", "-A", "-t", "-F", "\t", "-v", "ON_ERROR_STOP=1", "-d", "postgres")
	if err != nil {
		return nil, &discoverError{"postgres: " + pgErr(output, err)}
	}
	return parseDiscoveredRows("postgres", output), nil
}

type discoverError struct{ msg string }

func (e *discoverError) Error() string { return e.msg }

// parseDiscoveredRows reads "name<TAB>owner<TAB>size" lines. Lines without
// three fields (warnings, blank lines) are ignored.
func parseDiscoveredRows(engine, output string) []discoveredDatabase {
	var out []discoveredDatabase
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || fields[0] == "" {
			continue
		}
		size, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		if err != nil {
			continue
		}
		owner := fields[1]
		if owner == "NULL" {
			owner = ""
		}
		out = append(out, discoveredDatabase{Engine: engine, Name: fields[0], Owner: owner, SizeBytes: int64(size)})
	}
	return out
}
