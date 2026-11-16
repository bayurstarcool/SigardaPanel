package agenthttp

// Database manager: everything beyond browsing (dbviewer.go) for one
// database, always with that database's own account (never root):
//
//   - structure: indexes and foreign keys of a table;
//   - row edits: insert, update and delete rows, a row being identified by
//     its primary key (tables without one can be changed with SQL);
//   - SQL console: run statements, read-only (rolled back) or for real;
//   - export: a plain SQL dump (mysqldump / pg_dump), streamed;
//   - import: a plain SQL file, run by the database client.
//
// Identifiers in the statements the manager builds come from the catalog
// (looked up with bound parameters, as in the viewer); values are bound
// parameters. Console SQL is the user's own SQL, run with the account's own
// privileges; the MySQL driver refuses LOAD DATA LOCAL (no file of this
// server can be read) and runs one statement per request. Dump tools and
// the import client run as the site's Linux user (nobody without one).

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
)

const (
	managerTimeout     = 60 * time.Second
	managerStmtTimeout = 30 * time.Second
	maxConsoleSQL      = 1 << 20 // bytes of SQL per console run
)

type managerRequest struct {
	viewerRequest
	SystemUser string `json:"system_user"`
	// rows
	Op     string               `json:"op"` // insert | update | delete
	Keys   []map[string]*string `json:"keys"`
	Values map[string]*string   `json:"values"`
	// sql
	SQL      string `json:"sql"`
	ReadOnly bool   `json:"read_only"`
}

type managerResult struct {
	*viewerResult
	Kind         string `json:"kind,omitempty"` // rows | exec | structure
	RowsAffected int64  `json:"rows_affected"`
	Message      string `json:"message,omitempty"`
	// structure
	Indexes     *viewerResult `json:"indexes,omitempty"`
	ForeignKeys *viewerResult `json:"foreign_keys,omitempty"`
}

func managerBadRequest(c *echo.Context, msg string) error {
	return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": msg})
}

func validManagerCreds(req viewerRequest) bool {
	return (req.Type == "mysql" || req.Type == "postgres") && isValidDBIdentifier(req.Database) &&
		isValidDBIdentifier(req.User) && req.Password != ""
}

// managerConn opens one connection with statement timeouts set.
func managerConn(ctx context.Context, req viewerRequest) (*sql.DB, *sql.Conn, error) {
	db, err := openViewerDB(req)
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("cannot connect to the database: %s", dbErr(err))
	}
	if req.Type == "mysql" {
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("SET SESSION max_execution_time = %d", managerStmtTimeout.Milliseconds()))
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("SET SESSION max_statement_time = %d", int(managerStmtTimeout.Seconds())))
	} else {
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("SET statement_timeout = %d", managerStmtTimeout.Milliseconds()))
	}
	return db, conn, nil
}

// managerTask answers POST /tasks/db/manage.
func (r *Router) managerTask(c *echo.Context) error {
	var req managerRequest
	if err := c.Bind(&req); err != nil {
		return managerBadRequest(c, "invalid payload")
	}
	if !validManagerCreds(req.viewerRequest) {
		return managerBadRequest(c, "invalid database or credentials")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), managerTimeout)
	defer cancel()
	db, conn, err := managerConn(ctx, req.viewerRequest)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{"ok": false, "message": err.Error()})
	}
	defer db.Close()
	defer conn.Close()

	var res *managerResult
	switch req.Action {
	case "structure":
		res, err = managerStructure(ctx, conn, req)
	case "rows":
		res, err = managerRows(ctx, conn, req)
	case "sql":
		res, err = managerSQL(ctx, conn, req)
	default:
		return managerBadRequest(c, "unknown action")
	}
	if err != nil {
		var ve *viewerError
		if errors.As(err, &ve) {
			return c.JSON(ve.status, map[string]any{"ok": false, "message": ve.msg})
		}
		return managerBadRequest(c, dbErr(err))
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "result": res})
}

// ─── Structure ──────────────────────────────────────────────

func managerStructure(ctx context.Context, conn *sql.Conn, req managerRequest) (*managerResult, error) {
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, _, err := catalogTable(ctx, tx, req.viewerRequest); err != nil {
		return nil, err
	}
	schema := req.Schema
	if req.Type == "postgres" && schema == "" {
		schema = "public"
	}
	var idxQ, fkQ string
	var args []any
	if req.Type == "mysql" {
		idxQ = `SELECT index_name AS name, GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ', ') AS columns,
		          CASE WHEN non_unique = 0 THEN 'YES' ELSE 'NO' END AS ` + "`unique`" + `, index_type AS type
		        FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ?
		        GROUP BY index_name, non_unique, index_type ORDER BY index_name = 'PRIMARY' DESC, index_name`
		fkQ = `SELECT k.constraint_name AS name, GROUP_CONCAT(k.column_name ORDER BY k.ordinal_position SEPARATOR ', ') AS columns,
		         CONCAT(k.referenced_table_name, '(', GROUP_CONCAT(k.referenced_column_name ORDER BY k.ordinal_position SEPARATOR ', '), ')') AS ` + "`references`" + `,
		         r.update_rule AS on_update, r.delete_rule AS on_delete
		       FROM information_schema.key_column_usage k
		       JOIN information_schema.referential_constraints r ON r.constraint_schema = k.constraint_schema AND r.constraint_name = k.constraint_name
		       WHERE k.table_schema = DATABASE() AND k.table_name = ? AND k.referenced_table_name IS NOT NULL
		       GROUP BY k.constraint_name, k.referenced_table_name, r.update_rule, r.delete_rule ORDER BY k.constraint_name`
		args = []any{req.Table}
	} else {
		idxQ = `SELECT i.relname AS name,
		          (SELECT string_agg(a.attname, ', ' ORDER BY x.ord) FROM unnest(ix.indkey) WITH ORDINALITY x(attnum, ord)
		             JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = x.attnum) AS columns,
		          CASE WHEN ix.indisunique THEN 'YES' ELSE 'NO' END AS "unique", am.amname AS type
		        FROM pg_index ix JOIN pg_class t ON t.oid = ix.indrelid JOIN pg_class i ON i.oid = ix.indexrelid
		        JOIN pg_namespace n ON n.oid = t.relnamespace JOIN pg_am am ON am.oid = i.relam
		        WHERE n.nspname = $1 AND t.relname = $2 ORDER BY ix.indisprimary DESC, i.relname`
		fkQ = `SELECT c.conname AS name,
		         (SELECT string_agg(a.attname, ', ' ORDER BY x.ord) FROM unnest(c.conkey) WITH ORDINALITY x(attnum, ord)
		            JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = x.attnum) AS columns,
		         c.confrelid::regclass::text || '(' || (SELECT string_agg(a.attname, ', ' ORDER BY x.ord) FROM unnest(c.confkey) WITH ORDINALITY x(attnum, ord)
		            JOIN pg_attribute a ON a.attrelid = c.confrelid AND a.attnum = x.attnum) || ')' AS "references",
		         CASE c.confupdtype WHEN 'c' THEN 'CASCADE' WHEN 'n' THEN 'SET NULL' WHEN 'd' THEN 'SET DEFAULT' WHEN 'r' THEN 'RESTRICT' ELSE 'NO ACTION' END AS on_update,
		         CASE c.confdeltype WHEN 'c' THEN 'CASCADE' WHEN 'n' THEN 'SET NULL' WHEN 'd' THEN 'SET DEFAULT' WHEN 'r' THEN 'RESTRICT' ELSE 'NO ACTION' END AS on_delete
		       FROM pg_constraint c JOIN pg_class t ON t.oid = c.conrelid JOIN pg_namespace n ON n.oid = t.relnamespace
		       WHERE c.contype = 'f' AND n.nspname = $1 AND t.relname = $2 ORDER BY c.conname`
		args = []any{schema, req.Table}
	}
	idx, err := viewerQuery(ctx, tx, idxQ, args, 1000)
	if err != nil {
		return nil, err
	}
	fk, err := viewerQuery(ctx, tx, fkQ, args, 1000)
	if err != nil {
		return nil, err
	}
	return &managerResult{Kind: "structure", Indexes: idx, ForeignKeys: fk}, nil
}

// ─── Row edits ──────────────────────────────────────────────

// primaryKey returns the table's primary key columns (catalog spelling).
func primaryKey(ctx context.Context, tx *sql.Tx, req viewerRequest) ([]string, error) {
	var rows *sql.Rows
	var err error
	if req.Type == "mysql" {
		rows, err = tx.QueryContext(ctx, `SELECT column_name FROM information_schema.key_column_usage
			WHERE table_schema = DATABASE() AND table_name = ? AND constraint_name = 'PRIMARY' ORDER BY ordinal_position`, req.Table)
	} else {
		schema := req.Schema
		if schema == "" {
			schema = "public"
		}
		rows, err = tx.QueryContext(ctx, `SELECT a.attname FROM pg_index ix
			JOIN pg_class t ON t.oid = ix.indrelid JOIN pg_namespace n ON n.oid = t.relnamespace
			JOIN unnest(ix.indkey) WITH ORDINALITY x(attnum, ord) ON true
			JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = x.attnum
			WHERE ix.indisprimary AND n.nspname = $1 AND t.relname = $2 ORDER BY x.ord`, schema, req.Table)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pk []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		pk = append(pk, c)
	}
	return pk, rows.Err()
}

type placeholders struct {
	pg   bool
	args []any
}

func (p *placeholders) add(v *string) string {
	if v == nil {
		p.args = append(p.args, nil)
	} else {
		p.args = append(p.args, *v)
	}
	if p.pg {
		return fmt.Sprintf("$%d", len(p.args))
	}
	return "?"
}

// sortedColumns returns the catalog spelling of the requested columns, in a
// stable order; any unknown column is an error.
func sortedColumns(cols []string, m map[string]*string) ([]string, error) {
	out := make([]string, 0, len(m))
	for name := range m {
		c, ok := catalogColumn(cols, name)
		if !ok {
			return nil, &viewerError{http.StatusBadRequest, fmt.Sprintf("unknown column %q", name)}
		}
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}

// keyWhere builds "pk1 = ? AND pk2 = ?" from a key holding every primary key
// column (a NULL key value matches nothing: primary keys are NOT NULL).
func keyWhere(dbType string, pk []string, key map[string]*string, p *placeholders) (string, error) {
	if len(key) != len(pk) {
		return "", &viewerError{http.StatusBadRequest, "the row must be identified by its full primary key"}
	}
	parts := make([]string, 0, len(pk))
	for _, col := range pk {
		v, ok := key[col]
		if !ok || v == nil {
			return "", &viewerError{http.StatusBadRequest, "the row must be identified by its full primary key"}
		}
		parts = append(parts, quoteIdent(dbType, col)+" = "+p.add(v))
	}
	return strings.Join(parts, " AND "), nil
}

const maxRowEdits = 500

func managerRows(ctx context.Context, conn *sql.Conn, req managerRequest) (*managerResult, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	table, cols, err := catalogTable(ctx, tx, req.viewerRequest)
	if err != nil {
		return nil, err
	}
	p := &placeholders{pg: req.Type == "postgres"}
	var affected int64
	switch req.Op {
	case "insert":
		names, err := sortedColumns(cols, req.Values)
		if err != nil {
			return nil, err
		}
		var q string
		switch {
		case len(names) == 0 && req.Type == "postgres":
			q = "INSERT INTO " + table + " DEFAULT VALUES"
		case len(names) == 0:
			q = "INSERT INTO " + table + " () VALUES ()"
		default:
			qc, qv := make([]string, len(names)), make([]string, len(names))
			for i, n := range names {
				qc[i], qv[i] = quoteIdent(req.Type, n), p.add(req.Values[n])
			}
			q = "INSERT INTO " + table + " (" + strings.Join(qc, ", ") + ") VALUES (" + strings.Join(qv, ", ") + ")"
		}
		res, err := tx.ExecContext(ctx, q, p.args...)
		if err != nil {
			return nil, err
		}
		affected, _ = res.RowsAffected()
	case "update", "delete":
		pk, err := primaryKey(ctx, tx, req.viewerRequest)
		if err != nil {
			return nil, err
		}
		if len(pk) == 0 {
			return nil, &viewerError{http.StatusConflict, "this table has no primary key, so a row cannot be identified: change it with the SQL console"}
		}
		if len(req.Keys) == 0 || len(req.Keys) > maxRowEdits || (req.Op == "update" && len(req.Keys) != 1) {
			return nil, &viewerError{http.StatusBadRequest, "pick the row(s) to change"}
		}
		var set []string
		if req.Op == "update" {
			names, err := sortedColumns(cols, req.Values)
			if err != nil {
				return nil, err
			}
			if len(names) == 0 {
				return nil, &viewerError{http.StatusBadRequest, "nothing to change"}
			}
			for _, n := range names {
				set = append(set, n)
			}
		}
		for _, key := range req.Keys {
			kp := &placeholders{pg: p.pg}
			var q string
			if req.Op == "update" {
				parts := make([]string, len(set))
				for i, n := range set {
					parts[i] = quoteIdent(req.Type, n) + " = " + kp.add(req.Values[n])
				}
				q = "UPDATE " + table + " SET " + strings.Join(parts, ", ")
			} else {
				q = "DELETE FROM " + table
			}
			where, err := keyWhere(req.Type, pk, key, kp)
			if err != nil {
				return nil, err
			}
			res, err := tx.ExecContext(ctx, q+" WHERE "+where, kp.args...)
			if err != nil {
				return nil, err
			}
			n, _ := res.RowsAffected()
			if n == 0 && req.Op == "delete" {
				return nil, &viewerError{http.StatusConflict, "a row was not found (changed or deleted meanwhile?); nothing was deleted"}
			}
			if n > 1 {
				return nil, &viewerError{http.StatusConflict, "the key matched more than one row; nothing was changed"}
			}
			affected += n
		}
	default:
		return nil, &viewerError{http.StatusBadRequest, "op must be insert, update or delete"}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &managerResult{Kind: "exec", RowsAffected: affected}, nil
}

// ─── SQL console ────────────────────────────────────────────

// skipSQLComments drops leading whitespace and comments (--, # and /* */).
func skipSQLComments(s string) string {
	for {
		s = strings.TrimSpace(s)
		switch {
		case strings.HasPrefix(s, "--") || strings.HasPrefix(s, "#"):
			i := strings.IndexByte(s, '\n')
			if i < 0 {
				return ""
			}
			s = s[i+1:]
		case strings.HasPrefix(s, "/*"):
			i := strings.Index(s, "*/")
			if i < 0 {
				return ""
			}
			s = s[i+2:]
		default:
			return s
		}
	}
}

// firstKeyword is a statement's leading keyword, upper-cased (comments and
// opening parentheses skipped).
func firstKeyword(q string) string {
	s := strings.TrimLeft(skipSQLComments(q), "( \t\r\n")
	end := strings.IndexFunc(s, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_') })
	if end < 0 {
		end = len(s)
	}
	return strings.ToUpper(s[:end])
}

// sqlReturnsRows guesses from the first keyword whether a statement returns
// rows (run with Query) or a count (Exec).
func sqlReturnsRows(q string) bool {
	switch firstKeyword(q) {
	case "SELECT", "SHOW", "WITH", "EXPLAIN", "DESCRIBE", "DESC", "VALUES", "TABLE", "CHECK", "ANALYZE":
		return true
	}
	// "INSERT/UPDATE/DELETE ... RETURNING" returns rows too (PostgreSQL).
	return strings.Contains(strings.ToUpper(q), " RETURNING ")
}

// singleStatement reports whether q holds one statement: no ";" outside
// quotes, identifiers, dollar quotes and comments other than a trailing one.
// ok is false when q cannot be scanned (an unterminated quote or comment).
func singleStatement(dbType, q string) (single, ok bool) {
	mysql := dbType == "mysql"
	seenEnd := false // a ";" was seen; anything but comments after it is a second statement
	for i := 0; i < len(q); {
		c := q[i]
		switch {
		case c == '\'' || c == '"' || (mysql && c == '`'):
			j := i + 1
			for ; j < len(q); j++ {
				if mysql && q[j] == '\\' && c != '`' {
					j++
					continue
				}
				if q[j] == c {
					if j+1 < len(q) && q[j+1] == c { // doubled quote
						j++
						continue
					}
					break
				}
			}
			if j >= len(q) {
				return false, false
			}
			if seenEnd {
				return false, true
			}
			i = j + 1
		case c == '-' && i+1 < len(q) && q[i+1] == '-', mysql && c == '#':
			j := strings.IndexByte(q[i:], '\n')
			if j < 0 {
				return true, true
			}
			i += j + 1
		case c == '/' && i+1 < len(q) && q[i+1] == '*':
			j := strings.Index(q[i+2:], "*/")
			if j < 0 {
				return false, false
			}
			i += j + 4
		case !mysql && c == '$':
			// $tag$ ... $tag$ (a plain $1 parameter has no closing $).
			k := i + 1
			for k < len(q) && (q[k] == '_' || q[k] >= 'a' && q[k] <= 'z' || q[k] >= 'A' && q[k] <= 'Z' || q[k] >= '0' && q[k] <= '9' && k > i+1) {
				k++
			}
			if k < len(q) && q[k] == '$' {
				tag := q[i : k+1]
				j := strings.Index(q[k+1:], tag)
				if j < 0 {
					return false, false
				}
				if seenEnd {
					return false, true
				}
				i = k + 1 + j + len(tag)
				continue
			}
			if seenEnd {
				return false, true
			}
			i++
		case c == ';':
			seenEnd = true
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		default:
			if seenEnd {
				return false, true
			}
			i++
		}
	}
	return true, true
}

// readOnlyKeywords may run in the console's read-only mode. The mode is a
// guard against accidents (the same account can switch it off), but it must
// hold: MySQL commits implicitly before DDL (CREATE, DROP, TRUNCATE, ...),
// which would escape the READ ONLY transaction, and a second statement after
// a COMMIT would run outside it on PostgreSQL. So read-only runs take one
// statement whose kind only reads; the transaction refuses writes in it.
var readOnlyKeywords = map[string]bool{
	"SELECT": true, "SHOW": true, "WITH": true, "EXPLAIN": true, "DESCRIBE": true, "DESC": true, "VALUES": true, "TABLE": true,
}

func checkReadOnlySQL(dbType, q string) error {
	single, ok := singleStatement(dbType, q)
	if !ok {
		return &viewerError{http.StatusBadRequest, "the statement has an unterminated quote or comment"}
	}
	if !single {
		return &viewerError{http.StatusBadRequest, "read-only mode runs one statement at a time"}
	}
	if kw := firstKeyword(q); !readOnlyKeywords[kw] {
		if kw == "" {
			kw = "this statement"
		}
		return &viewerError{http.StatusBadRequest, kw + " can change the database: switch off read-only mode to run it"}
	}
	return nil
}

func managerSQL(ctx context.Context, conn *sql.Conn, req managerRequest) (*managerResult, error) {
	q := strings.TrimSpace(req.SQL)
	if q == "" {
		return nil, &viewerError{http.StatusBadRequest, "enter a statement"}
	}
	if len(q) > maxConsoleSQL {
		return nil, &viewerError{http.StatusBadRequest, "the statement is too long"}
	}
	start := time.Now()
	type execer interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	}
	var target execer = conn
	var tx *sql.Tx
	if req.ReadOnly {
		if err := checkReadOnlySQL(req.Type, q); err != nil {
			return nil, err
		}
		var err error
		tx, err = conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		target = tx
	}
	if sqlReturnsRows(q) {
		rows, err := target.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		res, err := collectRows(rows, viewerMaxRows)
		if err != nil {
			return nil, err
		}
		res.ElapsedMS = time.Since(start).Milliseconds()
		return &managerResult{viewerResult: res, Kind: "rows"}, nil
	}
	r, err := target.ExecContext(ctx, q)
	if err != nil {
		return nil, err
	}
	n, _ := r.RowsAffected()
	return &managerResult{viewerResult: &viewerResult{Columns: []string{}, Rows: [][]any{}, ElapsedMS: time.Since(start).Milliseconds()},
		Kind: "exec", RowsAffected: n}, nil
}

// collectRows turns a result into viewer values (same limits as the viewer).
func collectRows(rows *sql.Rows, limit int) (*viewerResult, error) {
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
		res.Rows = append(res.Rows, viewerValues(raw, res))
		for _, b := range raw {
			size += len(b)
		}
		if size > viewerMaxResponse {
			res.HasMore, res.Truncated = true, true
			break
		}
	}
	return res, rows.Err()
}

// ─── Export / import ────────────────────────────────────────

// dumpCommand returns the dump tool for the database and the environment
// that carries the password (never on the command line).
func dumpCommand(req viewerRequest) (string, []string, []string, error) {
	switch req.Type {
	case "mysql":
		tool := "mysqldump"
		if _, err := exec.LookPath(tool); err != nil {
			tool = "mariadb-dump"
		}
		return tool, []string{"--no-defaults", "--single-transaction", "--no-tablespaces", "--skip-lock-tables",
			"--default-character-set=utf8mb4", "-u" + req.User, "--", req.Database}, []string{"MYSQL_PWD=" + req.Password}, nil
	case "postgres":
		return "pg_dump", []string{"--no-owner", "--no-privileges", "--no-password", "-h", "localhost",
			"-U", req.User, "-d", req.Database}, []string{"PGPASSWORD=" + req.Password}, nil
	}
	return "", nil, nil, fmt.Errorf("unsupported database type")
}

// unprivileged returns the credentials export/import tools run with.
func unprivileged(systemUser string) *syscall.Credential {
	uid, gid := restoreRunAs(systemUser)
	return &syscall.Credential{Uid: uid, Gid: gid}
}

// exportTask answers POST /tasks/db/export with the dump as the body.
func (r *Router) exportTask(c *echo.Context) error {
	var req managerRequest
	if err := c.Bind(&req); err != nil {
		return managerBadRequest(c, "invalid payload")
	}
	if !validManagerCreds(req.viewerRequest) {
		return managerBadRequest(c, "invalid database or credentials")
	}
	tool, args, env, err := dumpCommand(req.viewerRequest)
	if err != nil {
		return managerBadRequest(c, err.Error())
	}
	if _, err := exec.LookPath(tool); err != nil {
		return c.JSON(http.StatusConflict, map[string]any{"ok": false, "message": tool + " is not installed on this server"})
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Hour)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Env = append([]string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/tmp", "LC_ALL=C.UTF-8"}, env...)
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: unprivileged(req.SystemUser)}
	var stderr strings.Builder
	cmd.Stderr = &limitedWriter{w: &stderr, n: 8 << 10}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "cannot start the dump"})
	}
	if err := cmd.Start(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "cannot start " + tool})
	}
	br := bufio.NewReaderSize(out, 64<<10)
	// Nothing written yet: a failure before the first byte is still an error answer.
	if _, perr := br.Peek(1); perr != nil {
		werr := cmd.Wait()
		msg := strings.TrimSpace(stderr.String())
		if msg == "" && werr != nil {
			msg = werr.Error()
		}
		if msg == "" {
			msg = "the dump is empty"
		}
		return c.JSON(http.StatusBadGateway, map[string]any{"ok": false, "message": tool + " failed: " + msg})
	}
	c.Response().Header().Set("Content-Type", "application/sql; charset=utf-8")
	c.Response().WriteHeader(http.StatusOK)
	_, copyErr := io.Copy(c.Response(), br)
	werr := cmd.Wait()
	if copyErr == nil && werr != nil {
		// Mid-stream failure: mark the file so it is never mistaken for complete.
		fmt.Fprintf(c.Response(), "\n-- SIGARDAPANEL EXPORT FAILED: %s\n", strings.ReplaceAll(strings.TrimSpace(stderr.String()), "\n", " "))
	}
	return nil
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	q := p
	if len(q) > l.n {
		q = q[:l.n]
	}
	l.n -= len(q)
	_, _ = l.w.Write(q)
	return len(p), nil
}

// importTask answers POST /tasks/db/import. The SQL is the body (an upload
// of at most maxUploadBytes), or a file in the site's folder named by the
// X-Source-Path header (any size; opened through the site's file system,
// so it cannot point outside the folder). The database and account come in
// headers, never in the URL.
func (r *Router) importTask(c *echo.Context) error {
	h := c.Request().Header
	db := restoreDatabase{Name: h.Get("X-Db-Name"), Type: h.Get("X-Db-Type"), User: h.Get("X-Db-User"), Password: h.Get("X-Db-Password")}
	if !validManagerCreds(viewerRequest{Type: db.Type, Database: db.Name, User: db.User, Password: db.Password}) {
		return managerBadRequest(c, "invalid database or credentials")
	}
	systemUser := h.Get("X-System-User")
	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Hour)
	defer cancel()

	var src io.Reader
	var size int64
	if rel := strings.TrimSpace(h.Get("X-Source-Path")); rel != "" {
		if !strings.HasSuffix(strings.ToLower(rel), ".sql") {
			return managerBadRequest(c, "pick a .sql file")
		}
		fs, err := r.siteFS(h.Get("X-Root-Path"), systemUser)
		if err != nil {
			return fsError(c, err)
		}
		f, info, err := fs.OpenDownload(rel)
		if err != nil {
			return fsError(c, err)
		}
		defer f.Close()
		src, size = f, info.Size()
	} else {
		tmp, err := os.CreateTemp("", "sigarda-import-*.sql")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "cannot store the upload"})
		}
		defer os.Remove(tmp.Name())
		defer tmp.Close()
		n, err := io.Copy(tmp, io.LimitReader(c.Request().Body, maxUploadBytes+1))
		if err != nil {
			return managerBadRequest(c, "the upload was interrupted")
		}
		if n > maxUploadBytes {
			return c.JSON(http.StatusRequestEntityTooLarge, map[string]any{"ok": false, "message": fmt.Sprintf("the file is larger than %d MB: put it in the site's folder and import it from there", maxUploadBytes>>20)})
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "cannot read the upload"})
		}
		src, size = tmp, n
	}
	if size == 0 {
		return managerBadRequest(c, "the file is empty")
	}
	// The agent reads the file; the client (site user) only gets it on stdin.
	uid, gid := restoreRunAs(systemUser)
	out, err := r.importDatabaseReader(ctx, db, src, uid, gid)
	if err != nil {
		msg := strings.TrimSpace(out)
		if len(msg) > 4000 {
			msg = msg[len(msg)-4000:]
		}
		if msg == "" {
			msg = err.Error()
		}
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{"ok": false, "message": "import failed: " + msg})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "bytes": size})
}
