package agenthttp

import (
	"strings"
	"testing"
)

func TestSQLReturnsRows(t *testing.T) {
	for q, want := range map[string]bool{
		"SELECT 1": true, "  select * from t": true, "-- c\nSELECT 1": true, "/* x */ show tables": true,
		"WITH a AS (SELECT 1) SELECT * FROM a": true, "(SELECT 1)": true, "EXPLAIN SELECT 1": true,
		"INSERT INTO t VALUES (1)": false, "update t set a=1": false, "CREATE TABLE x (a int)": false,
		"DELETE FROM t WHERE id = 1 RETURNING *": true, "": false, "-- only a comment": false, "# c\nDROP TABLE t": false,
	} {
		if got := sqlReturnsRows(q); got != want {
			t.Errorf("%q: %v", q, got)
		}
	}
}

func s(v string) *string { return &v }

func TestKeyWhere(t *testing.T) {
	p := &placeholders{pg: true}
	got, err := keyWhere("postgres", []string{"a", "b"}, map[string]*string{"a": s("1"), "b": s("x")}, p)
	if err != nil || got != `"a" = $1 AND "b" = $2` || len(p.args) != 2 {
		t.Fatalf("%q %v %v", got, err, p.args)
	}
	for _, key := range []map[string]*string{{"a": s("1")}, {"a": s("1"), "c": s("2")}, {"a": nil, "b": s("x")}} {
		if _, err := keyWhere("postgres", []string{"a", "b"}, key, &placeholders{pg: true}); err == nil {
			t.Errorf("partial or null key accepted: %v", key)
		}
	}
	mp := &placeholders{}
	if got, _ := keyWhere("mysql", []string{"id"}, map[string]*string{"id": s("7")}, mp); got != "`id` = ?" {
		t.Fatalf("mysql: %q", got)
	}
}

func TestSortedColumnsUsesCatalog(t *testing.T) {
	cols := []string{"id", "name", "Email"}
	got, err := sortedColumns(cols, map[string]*string{"name": s("x"), "Email": nil})
	if err != nil || strings.Join(got, ",") != "Email,name" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := sortedColumns(cols, map[string]*string{"name; DROP TABLE x": s("1")}); err == nil {
		t.Fatal("unknown column accepted")
	}
}

func TestDumpCommandKeepsPasswordOutOfArgs(t *testing.T) {
	for _, typ := range []string{"mysql", "postgres"} {
		tool, args, env, err := dumpCommand(viewerRequest{Type: typ, Database: "shop", User: "u1", Password: "s3cr3t"})
		if err != nil || tool == "" {
			t.Fatalf("%s: %v", typ, err)
		}
		if strings.Contains(strings.Join(args, " "), "s3cr3t") {
			t.Fatalf("%s: password on the command line", typ)
		}
		if len(env) != 1 || !strings.HasSuffix(env[0], "=s3cr3t") {
			t.Fatalf("%s: env %v", typ, env)
		}
	}
}

func TestReadOnlySQLGuard(t *testing.T) {
	allowed := []struct{ db, q string }{
		{"mysql", "SELECT 1"},
		{"mysql", "  -- note\nselect * from t where a = ';' ;"},
		{"mysql", "SHOW TABLES"},
		{"mysql", "select 'it''s; fine', `odd;name` from t # trailing ; comment"},
		{"mysql", "SELECT 'a\\'; b' FROM t"},
		{"postgres", "WITH x AS (SELECT 1) SELECT * FROM x;"},
		{"postgres", "SELECT $$a;b$$, $tag$c;d$tag$"},
		{"postgres", "(SELECT 1) /* ; */"},
		{"postgres", "EXPLAIN SELECT * FROM t WHERE id = $1"},
	}
	for _, c := range allowed {
		if err := checkReadOnlySQL(c.db, c.q); err != nil {
			t.Errorf("%s %q refused: %v", c.db, c.q, err)
		}
	}
	refused := []struct{ db, q string }{
		{"mysql", "CREATE TABLE x (id int)"},
		{"mysql", "TRUNCATE t"},
		{"mysql", "DROP TABLE t"},
		{"mysql", "SELECT 1; DROP TABLE t"},
		{"mysql", "/* hi */ ALTER TABLE t ADD c int"},
		{"mysql", "SET autocommit=1"},
		{"mysql", "SELECT 'unterminated"},
		{"postgres", "COMMIT; CREATE TABLE x (id int)"},
		{"postgres", "SELECT 1; SELECT 2"},
		{"postgres", "SELECT 1;\n-- c\nDROP TABLE t"},
		{"postgres", "SELECT $$x$$; DELETE FROM t"},
		{"postgres", "SET TRANSACTION READ WRITE"},
		{"postgres", "DO $$ BEGIN END $$"},
		{"postgres", "SELECT 1 /* unterminated"},
		{"postgres", ""},
	}
	for _, c := range refused {
		if err := checkReadOnlySQL(c.db, c.q); err == nil {
			t.Errorf("%s %q allowed", c.db, c.q)
		}
	}
}
