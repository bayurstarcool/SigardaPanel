package agenthttp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
)

// Database viewer: list tables, show columns and browse rows (with filters,
// sorting and pages) of one database, with that database's own account
// (never root), on the server that hosts it.
//
// No SQL text from the request ever reaches the database: table and column
// names used in a query are the ones read back from the database's catalog
// (after looking up the requested name as a bound parameter), and filter
// values, limit and offset are bound parameters. Every request also runs in
// a READ ONLY transaction that is rolled back.

const (
	viewerMaxRows     = 500
	viewerMaxCell     = 10 << 10 // bytes shown per value
	viewerMaxResponse = 4 << 20  // bytes of values per response
	viewerTimeout     = 20 * time.Second
	viewerStmtTimeout = 15 * time.Second
)

type viewerRequest struct {
	Type     string         `json:"type"` // mysql | postgres
	Database string         `json:"database"`
	User     string         `json:"user"`
	Password string         `json:"password"`
	Action   string         `json:"action"` // tables | columns | rows
	Schema   string         `json:"schema"`
	Table    string         `json:"table"`
	Limit    int            `json:"limit"`
	Offset   int            `json:"offset"`
	OrderBy  string         `json:"order_by"`
	Desc     bool           `json:"desc"`
	Filters  []viewerFilter `json:"filters"`
}

// viewerFilter is one condition of a rows request: column OP value.
type viewerFilter struct {
	Column string `json:"column"`
	Op     string `json:"op"`
	Value  string `json:"value"`
}

// viewerOps maps the accepted operators to SQL (the value is always bound).
var viewerOps = map[string]string{
	"eq": "= %s", "ne": "<> %s", "lt": "< %s", "le": "<= %s", "gt": "> %s", "ge": ">= %s",
	"contains": "LIKE %s", "starts": "LIKE %s", "null": "IS NULL", "notnull": "IS NOT NULL",
}

const viewerMaxFilters = 10

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
	return viewerQuery(ctx, tx, q, nil, 100000)
}

// catalogTable looks the requested table up in the database's catalog (the
// name is a bound parameter) and returns the quoted identifier built from
// the name the catalog returns, with the table's column names (also from the
// catalog). Nothing from the request is put into SQL text.
func catalogTable(ctx context.Context, tx *sql.Tx, req viewerRequest) (string, []string, error) {
	if req.Table == "" {
		return "", nil, &viewerError{http.StatusBadRequest, "table required"}
	}
	schema := req.Schema
	if req.Type == "postgres" && schema == "" {
		schema = "public"
	}
	var catSchema, catTable string
	var err error
	if req.Type == "mysql" {
		err = tx.QueryRowContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?`, req.Table).Scan(&catTable)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT table_schema, table_name FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2`, schema, req.Table).Scan(&catSchema, &catTable)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, &viewerError{http.StatusNotFound, "table not found"}
	}
	if err != nil {
		return "", nil, err
	}
	var rows *sql.Rows
	if req.Type == "mysql" {
		rows, err = tx.QueryContext(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? ORDER BY ordinal_position`, catTable)
	} else {
		rows, err = tx.QueryContext(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position`, catSchema, catTable)
	}
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return "", nil, err
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	if req.Type == "postgres" {
		return quoteIdent("postgres", catSchema) + "." + quoteIdent("postgres", catTable), cols, nil
	}
	return quoteIdent("mysql", catTable), cols, nil
}

// catalogColumn returns the catalog's spelling of a requested column name.
func catalogColumn(cols []string, name string) (string, bool) {
	for _, c := range cols {
		if c == name {
			return c, true
		}
	}
	return "", false
}

func viewerColumns(ctx context.Context, tx *sql.Tx, req viewerRequest) (*viewerResult, error) {
	if _, _, err := catalogTable(ctx, tx, req); err != nil {
		return nil, err
	}
	if req.Type == "mysql" {
		return viewerQuery(ctx, tx, `SELECT column_name AS name, column_type AS type, is_nullable AS nullable, column_key AS `+"`key`"+`, column_default AS `+"`default`"+`, extra
			FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? ORDER BY ordinal_position`,
			[]any{req.Table}, 10000)
	}
	schema := req.Schema
	if schema == "" {
		schema = "public"
	}
	return viewerQuery(ctx, tx, `SELECT c.column_name AS name,
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
	table, cols, err := catalogTable(ctx, tx, req)
	if err != nil {
		return nil, err
	}
	limit := clampLimit(req.Limit)
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	var args []any
	ph := func(v any) string {
		args = append(args, v)
		if req.Type == "postgres" {
			return fmt.Sprintf("$%d", len(args))
		}
		return "?"
	}
	if len(req.Filters) > viewerMaxFilters {
		return nil, &viewerError{http.StatusBadRequest, fmt.Sprintf("at most %d filters", viewerMaxFilters)}
	}
	var where []string
	for _, f := range req.Filters {
		col, ok := catalogColumn(cols, f.Column)
		if !ok {
			return nil, &viewerError{http.StatusBadRequest, "unknown column to filter on"}
		}
		tmpl, ok := viewerOps[f.Op]
		if !ok {
			return nil, &viewerError{http.StatusBadRequest, "unknown filter operator"}
		}
		lhs := quoteIdent(req.Type, col)
		if f.Op == "contains" || f.Op == "starts" {
			// Compare as text so any column type can be searched.
			if req.Type == "postgres" {
				lhs += "::text"
			} else {
				lhs = "CAST(" + lhs + " AS CHAR)"
			}
		}
		switch f.Op {
		case "null", "notnull":
			where = append(where, lhs+" "+tmpl)
		case "contains":
			where = append(where, lhs+" "+fmt.Sprintf(tmpl, ph("%"+escapeLike(f.Value)+"%")))
		case "starts":
			where = append(where, lhs+" "+fmt.Sprintf(tmpl, ph(escapeLike(f.Value)+"%")))
		default:
			where = append(where, lhs+" "+fmt.Sprintf(tmpl, ph(f.Value)))
		}
	}
	q := "SELECT * FROM " + table
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	if req.OrderBy != "" {
		col, ok := catalogColumn(cols, req.OrderBy)
		if !ok {
			return nil, &viewerError{http.StatusBadRequest, "unknown column to sort on"}
		}
		q += " ORDER BY " + quoteIdent(req.Type, col)
		if req.Desc {
			q += " DESC"
		}
	}
	q += " LIMIT " + ph(limit+1) + " OFFSET " + ph(offset)
	return viewerQuery(ctx, tx, q, args, limit)
}

// escapeLike makes a filter value match literally inside LIKE (both engines
// use backslash as the default escape character).
func escapeLike(v string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(v)
}

// viewerQuery runs one of the viewer's own statements and returns at most limit rows as JSON-safe
// values (text; NULL as nil; binary as a placeholder; long values cut).
func viewerQuery(ctx context.Context, tx *sql.Tx, q string, args []any, limit int) (*viewerResult, error) {
	start := time.Now()
	rows, err := tx.QueryContext(ctx, q, args...)
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
