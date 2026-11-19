package agenthttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// Database accounts are server-wide: a site's "new" user may carry the name
// of an account that already exists (the PostgreSQL superuser, another site's
// user, an application's own account). Creating it must never take that
// account over by resetting its password, and deleting a site's user must
// never drop an account with server-wide powers.

// dbAccountState describes an account on the database server.
type dbAccountState struct {
	Exists     bool `json:"exists"`
	Privileged bool `json:"privileged"`  // server-wide powers (superuser, global grants, ...)
	OnDatabase bool `json:"on_database"` // already granted on the database in question
}

// isReservedDBAccount: names of the servers' own accounts.
func isReservedDBAccount(dbType, name string) bool {
	n := strings.ToLower(name)
	switch dbType {
	case "postgres", "postgresql":
		return n == "postgres" || strings.HasPrefix(n, "pg_") || n == "public"
	default:
		switch n {
		case "root", "mysql", "mariadb.sys", "debian-sys-maint", "phpmyadmin", "pma":
			return true
		}
		return strings.HasPrefix(n, "mysql.")
	}
}

func (r *Router) dbAccountState(ctx context.Context, dbType, user, host, db string) (dbAccountState, error) {
	var st dbAccountState
	switch dbType {
	case "mysql":
		dbAlt := strings.ReplaceAll(db, "_", `\_`) // GRANT may store the name escaped
		q := fmt.Sprintf("SELECT COUNT(*), COALESCE(MAX(Super_priv='Y' OR Grant_priv='Y' OR Create_user_priv='Y' OR File_priv='Y' OR Process_priv='Y'"+
			" OR Shutdown_priv='Y' OR Reload_priv='Y' OR Select_priv='Y' OR Insert_priv='Y' OR Update_priv='Y' OR Delete_priv='Y' OR Create_priv='Y'"+
			" OR Drop_priv='Y' OR Alter_priv='Y'), 0), (SELECT COUNT(*) FROM mysql.db WHERE User=%[1]s AND Host=%[2]s AND Db IN (%[3]s, %[4]s))"+
			" FROM mysql.user WHERE User=%[1]s AND Host=%[2]s;\n",
			sqlQuote(user), sqlQuote(host), sqlQuote(db), sqlQuote(dbAlt))
		out, err := r.executor.RunWithInput(ctx, q, "mysql", "-N", "-B")
		if err != nil {
			return st, fmt.Errorf("cannot read MySQL accounts: %s", strings.TrimSpace(out))
		}
		f := strings.Fields(lastLine(out))
		if len(f) != 3 {
			return st, fmt.Errorf("unexpected answer reading MySQL accounts")
		}
		st.Exists, st.Privileged, st.OnDatabase = f[0] != "0", f[1] != "0", f[2] != "0"
	case "postgres":
		q := fmt.Sprintf("\\t on\n\\a\nSELECT (SELECT count(*) FROM pg_roles WHERE rolname = %[1]s),"+
			" (SELECT count(*) FROM pg_roles WHERE rolname = %[1]s AND (rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)),"+
			" CASE WHEN EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %[1]s) AND EXISTS (SELECT 1 FROM pg_database WHERE datname = %[2]s)"+
			" THEN has_database_privilege(%[1]s, %[2]s, 'CREATE') ELSE false END;\n", sqlQuote(user), sqlQuote(db))
		out, err := r.postgresAdmin(ctx, "postgres", q)
		if err != nil {
			return st, fmt.Errorf("cannot read PostgreSQL roles: %s", pgErr(out, err))
		}
		f := strings.Split(strings.TrimSpace(lastLine(out)), "|")
		if len(f) != 3 {
			return st, fmt.Errorf("unexpected answer reading PostgreSQL roles")
		}
		st.Exists, st.Privileged, st.OnDatabase = f[0] != "0", f[1] != "0", f[2] == "t"
	default:
		return st, fmt.Errorf("unsupported database type")
	}
	if isReservedDBAccount(dbType, user) {
		st.Privileged = true
	}
	return st, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// errForeignAccount: an existing account the site's user may not become.
func errForeignAccount(name string) error {
	return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf(
		"an account named %s already exists on the database server and is not a user of this database; choose another name", name))
}

// dbAccountTask answers POST /tasks/db/account {type, username, host_pattern,
// database}: what the server knows about that account (no secrets).
func (r *Router) dbAccountTask(c *echo.Context) error {
	var p struct {
		Type        string `json:"type"`
		Username    string `json:"username"`
		HostPattern string `json:"host_pattern"`
		Database    string `json:"database"`
	}
	if err := c.Bind(&p); err != nil || !isValidDBIdentifier(p.Username) || (p.Database != "" && !isValidDBIdentifier(p.Database)) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if p.HostPattern == "" {
		p.HostPattern = "localhost"
	}
	if !isValidDBHostPattern(p.HostPattern) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid host_pattern")
	}
	st, err := r.dbAccountState(c.Request().Context(), p.Type, p.Username, p.HostPattern, p.Database)
	if err != nil {
		return echo.NewHTTPError(http.StatusFailedDependency, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "account": st})
}
