package agenthttp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
)

// Database viewer: list tables, show columns, browse rows and run read-only
// queries for one database, with that database's own account (never root),
// on the server that hosts it.
//
// Read-only is enforced three ways: only SELECT-like statements are accepted
// (a DDL statement would implicitly commit a MySQL read-only transaction and
// run anyway), every request runs in a READ ONLY transaction that is rolled
// back, and a query is a single statement (MySQL without multiStatements,
// PostgreSQL through a prepared statement, which refuses several commands).

const (
	viewerMaxRows     = 500
	viewerMaxCell     = 10 << 10 // bytes shown per value
	viewerMaxResponse = 4 << 20  // bytes of values per response
	viewerTimeout     = 20 * time.Second
	viewerStmtTimeout = 15 * time.Second
)

type viewerRequest struct {
	Type     string `json:"type"` // mysql | postgres
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
	Action   string `json:"action"` // tables | columns | rows | query
	Schema   string `json:"schema"`
	Table    string `json:"table"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
	OrderBy  string `json:"order_by"`
	Desc     bool   `json:"desc"`
	SQL      string `json:"sql"`
}

type viewerResult struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	HasMore   bool     `json:"has_more"`
	Truncated bool     `json:"truncated,omitempty"`
	ElapsedMS int64    `json:"elapsed_ms"`
}

var mysqlSockets = []string{"/run/mysqld/mysqld.sock", "/var/run/mysqld/mysqld.sock", "/var/lib/mysql/mysql.sock", "/tmp/mysql.sock"}

func openViewerDB(req viewerRequest) (*sql.DB, error) {
	switch req.Type {
	case "mysql":
		cfg := mysql.NewConfig()
		cfg.User, cfg.Passwd, cfg.DBName = req.User, req.Password, req.Database
		cfg.Net, cfg.Addr = "tcp", "127.0.0.1:3306"
		for _, s := range mysqlSockets {
			if fi, err := os.Stat(s); err == nil && fi.Mode()&os.ModeSocket != 0 {
				cfg.Net, cfg.Addr = "unix", s
				break
			}
		}
		cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, viewerTimeout, viewerTimeout
		cfg.MultiStatements = false
		cfg.InterpolateParams = false
		return sql.Open("mysql", cfg.FormatDSN())
	case "postgres":
		// TCP to localhost: the local socket uses peer authentication, which
		// would compare the role with the agent's OS user (root).
		dsn := url.URL{
			Scheme: "postgres", User: url.UserPassword(req.User, req.Password), Host: "localhost:5432",
			Path: "/" + req.Database, RawQuery: "sslmode=disable&connect_timeout=5&application_name=sigardapanel-viewer",
		}
		return sql.Open("postgres", dsn.String())
	}
	return nil, fmt.Errorf("unsupported database type")
}

// readOnlyStatement accepts one statement that only reads: SELECT, WITH,
// SHOW, DESCRIBE/DESC, EXPLAIN (not EXPLAIN ANALYZE, which executes) and
// TABLE/VALUES.
var leadingComments = regexp.MustCompile(`^(\s|--[^\n]*\n|/\*.*?\*/)+`)

func readOnlyStatement(q string) (string, error) {
	q = strings.TrimSpace(q)
	q = strings.TrimSpace(strings.TrimSuffix(q, ";"))
	if q == "" {
		return "", errors.New("query is empty")
	}
	if len(q) > 64<<10 {
		return "", errors.New("query is too long")
	}
	body := strings.TrimLeft(leadingComments.ReplaceAllString(q, ""), "( \t\r\n")
	word := strings.ToUpper(strings.FieldsFunc(body+" ", func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
	})[0])
	switch word {
	case "SELECT", "WITH", "SHOW", "DESCRIBE", "DESC", "TABLE", "VALUES":
	case "EXPLAIN":
		if regexp.MustCompile(`(?i)^EXPLAIN\s*(\(|\s)[^;]*\bANALY[SZ]E\b`).MatchString(body) {
			return "", errors.New("EXPLAIN ANALYZE runs the statement: not allowed in the read-only viewer")
		}
	default:
		return "", fmt.Errorf("the viewer is read-only: %s statements are not allowed (use SELECT, SHOW, DESCRIBE or EXPLAIN)", word)
	}
	// MySQL: SELECT ... INTO OUTFILE/DUMPFILE writes files; FOR UPDATE takes locks.
	if regexp.MustCompile(`(?i)\bINTO\s+(OUTFILE|DUMPFILE)\b`).MatchString(q) {
		return "", errors.New("INTO OUTFILE/DUMPFILE is not allowed")
	}
	return q, nil
}

func quoteIdent(dbType, name string) string {
	if dbType == "mysql" {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// viewerTask answers POST /tasks/db/view.
func (r *Router) viewerTask(c echo.Context) error {
	var req viewerRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	if (req.Type != "mysql" && req.Type != "postgres") || !isValidDBIdentifier(req.Database) || req.User == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid database or credentials"})
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), viewerTimeout)
	defer cancel()
	db, err := openViewerDB(req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{"ok": false, "message": "cannot connect to the database: " + dbErr(err)})
	}
	defer conn.Close()

	// Statement timeouts (best effort: names differ between MySQL, MariaDB
	// and PostgreSQL), then a read-only transaction that is always rolled back.
	if req.Type == "mysql" {
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("SET SESSION max_execution_time = %d", viewerStmtTimeout.Milliseconds()))
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("SET SESSION max_statement_time = %d", int(viewerStmtTimeout.Seconds())))
	}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{"ok": false, "message": "cannot start a read-only transaction: " + dbErr(err)})
	}
	defer tx.Rollback()
	if req.Type == "postgres" {
		_, _ = tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", viewerStmtTimeout.Milliseconds()))
	}

	var res *viewerResult
	switch req.Action {
	case "tables":
		res, err = viewerTables(ctx, tx, req)
	case "columns":
		res, err = viewerColumns(ctx, tx, req)
	case "rows":
		res, err = viewerRows(ctx, tx, req)
	case "query":
		var q string
		if q, err = readOnlyStatement(req.SQL); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
		}
		res, err = viewerQuery(ctx, tx, req.Type, q, nil, clampLimit(req.Limit))
	default:
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "unknown action"})
	}
	if err != nil {
		var ve *viewerError
		if errors.As(err, &ve) {
			return c.JSON(ve.status, map[string]any{"ok": false, "message": ve.msg})
		}
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": dbErr(err)})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "result": res})
}

type viewerError struct {
	status int
	msg    string
}

func (e *viewerError) Error() string { return e.msg }

func clampLimit(n int) int {
	if n <= 0 || n > viewerMaxRows {
		return 100
	}
	return n
}

// dbErr trims driver errors to the server's message.
func dbErr(err error) string {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Message
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("the query took longer than %d seconds", int(viewerStmtTimeout.Seconds()))
	}
	msg := err.Error()
	msg = strings.TrimPrefix(msg, "pq: ")
	return msg
}

func viewerTables(ctx context.Context, tx *sql.Tx, req viewerRequest) (*viewerResult, error) {
	// Same column names for both engines: schema, name, type, rows (estimate), size.
	q := `SELECT '' AS ` + "`schema`" + `, table_name AS name, table_type AS type, COALESCE(table_rows, 0) AS ` + "`rows`" + `, COALESCE(data_length + index_length, 0) AS size
	      FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name`
	if req.Type == "postgres" {
		q = `SELECT CASE WHEN n.nspname = 'public' THEN '' ELSE n.nspname END AS schema, c.relname AS name,
		        CASE c.relkind WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'MATERIALIZED VIEW' ELSE 'BASE TABLE' END AS type,
		        GREATEST(c.reltuples, 0)::bigint AS rows, pg_total_relation_size(c.oid) AS size
		     FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		     WHERE c.relkind IN ('r','p','v','m') AND n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%'
		       AND has_schema_privilege(n.oid, 'USAGE')
		     ORDER BY n.nspname <> 'public', n.nspname, c.relname`
	}
	return viewerQuery(ctx, tx, req.Type, q, nil, 100000)
}

// tableExists makes sure schema/table name a table of this database before
// they are put (quoted) into SQL.
func tableExists(ctx context.Context, tx *sql.Tx, req viewerRequest) (string, error) {
	if req.Table == "" {
		return "", &viewerError{http.StatusBadRequest, "table required"}
	}
	var n int
	var err error
	if req.Type == "mysql" {
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?`, req.Table).Scan(&n)
	} else {
		schema := req.Schema
		if schema == "" {
			schema = "public"
		}
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2`, schema, req.Table).Scan(&n)
	}
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", &viewerError{http.StatusNotFound, "table not found"}
	}
	if req.Type == "postgres" {
		schema := req.Schema
		if schema == "" {
			schema = "public"
		}
		return quoteIdent("postgres", schema) + "." + quoteIdent("postgres", req.Table), nil
	}
	return quoteIdent("mysql", req.Table), nil
}

func viewerColumns(ctx context.Context, tx *sql.Tx, req viewerRequest) (*viewerResult, error) {
	if _, err := tableExists(ctx, tx, req); err != nil {
		return nil, err
	}
	if req.Type == "mysql" {
		return viewerQuery(ctx, tx, "mysql", `SELECT column_name AS name, column_type AS type, is_nullable AS nullable, column_key AS `+"`key`"+`, column_default AS `+"`default`"+`, extra
			FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? ORDER BY ordinal_position`,
			[]any{req.Table}, 10000)
	}
	schema := req.Schema
	if schema == "" {
		schema = "public"
	}
	return viewerQuery(ctx, tx, "postgres", `SELECT c.column_name AS name,
			CASE WHEN c.character_maximum_length IS NOT NULL THEN c.data_type || '(' || c.character_maximum_length || ')' ELSE c.data_type END AS type,
			c.is_nullable AS nullable,
			CASE WHEN EXISTS (SELECT 1 FROM information_schema.table_constraints tc JOIN information_schema.key_column_usage k
			  ON k.constraint_name = tc.constraint_name AND k.table_schema = tc.table_schema
			  WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = c.table_schema AND tc.table_name = c.table_name AND k.column_name = c.column_name)
			  THEN 'PRI' ELSE '' END AS key,
			c.column_default AS "default", '' AS extra
		FROM information_schema.columns c WHERE c.table_schema = $1 AND c.table_name = $2 ORDER BY c.ordinal_position`,
		[]any{schema, req.Table}, 10000)
}

func viewerRows(ctx context.Context, tx *sql.Tx, req viewerRequest) (*viewerResult, error) {
	table, err := tableExists(ctx, tx, req)
	if err != nil {
		return nil, err
	}
	limit := clampLimit(req.Limit)
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	q := "SELECT * FROM " + table
	if req.OrderBy != "" {
		// Only a real column of this table can be sorted on.
		cols, err := viewerColumns(ctx, tx, req)
		if err != nil {
			return nil, err
		}
		ok := false
		for _, row := range cols.Rows {
			if len(row) > 0 && row[0] == req.OrderBy {
				ok = true
				break
			}
		}
		if !ok {
			return nil, &viewerError{http.StatusBadRequest, "unknown column to sort on"}
		}
		q += " ORDER BY " + quoteIdent(req.Type, req.OrderBy)
		if req.Desc {
			q += " DESC"
		}
	}
	q += fmt.Sprintf(" LIMIT %d OFFSET %d", limit+1, offset)
	return viewerQuery(ctx, tx, req.Type, q, nil, limit)
}

// viewerQuery runs one statement and returns at most limit rows as JSON-safe
// values (text; NULL as nil; binary as a placeholder; long values cut).
func viewerQuery(ctx context.Context, tx *sql.Tx, dbType, q string, args []any, limit int) (*viewerResult, error) {
	start := time.Now()
	var rows *sql.Rows
	var err error
	if dbType == "postgres" && args == nil {
		// A prepared statement holds exactly one command: "SELECT 1; COMMIT;
		// DROP ..." cannot leave the read-only transaction.
		stmt, perr := tx.PrepareContext(ctx, q)
		if perr != nil {
			return nil, perr
		}
		defer stmt.Close()
		rows, err = stmt.QueryContext(ctx)
	} else {
		rows, err = tx.QueryContext(ctx, q, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	res := &viewerResult{Columns: cols, Rows: [][]any{}}
	size := 0
	for rows.Next() {
		if len(res.Rows) >= limit {
			res.HasMore = true
			break
		}
		raw := make([]sql.RawBytes, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		out := make([]any, len(cols))
		for i, b := range raw {
			switch {
			case b == nil:
				out[i] = nil
			case !utf8.Valid(b):
				out[i] = fmt.Sprintf("[binary, %d bytes]", len(b))
			case len(b) > viewerMaxCell:
				out[i] = string(b[:viewerMaxCell]) + "…"
				res.Truncated = true
			default:
				out[i] = string(b)
			}
			size += len(b)
		}
		res.Rows = append(res.Rows, out)
		if size > viewerMaxResponse {
			res.HasMore, res.Truncated = true, true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	res.ElapsedMS = time.Since(start).Milliseconds()
	return res, nil
}
