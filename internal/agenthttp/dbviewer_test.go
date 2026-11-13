package agenthttp

import "testing"

func TestReadOnlyStatement(t *testing.T) {
	ok := []string{
		"SELECT * FROM t", "  select 1;", "(SELECT 1) UNION (SELECT 2)", "WITH x AS (SELECT 1) SELECT * FROM x",
		"SHOW TABLES", "DESCRIBE t", "desc t", "EXPLAIN SELECT * FROM t", "/* hi */ SELECT 1", "-- c\nSELECT 1",
		"TABLE t", "VALUES (1)",
	}
	for _, q := range ok {
		if _, err := readOnlyStatement(q); err != nil {
			t.Errorf("%q refused: %v", q, err)
		}
	}
	bad := []string{
		"", ";", "DROP TABLE t", "delete from t", "UPDATE t SET a=1", "INSERT INTO t VALUES (1)", "TRUNCATE t",
		"ALTER TABLE t ADD c int", "GRANT ALL ON *.* TO x", "SET GLOBAL x=1", "CALL p()", "LOCK TABLES t READ",
		"/* SELECT */ DROP TABLE t", "SELECT * FROM t INTO OUTFILE '/tmp/x'", "select 1 into dumpfile '/x'",
		"EXPLAIN ANALYZE DELETE FROM t", "EXPLAIN (ANALYZE, BUFFERS) DELETE FROM t", "COMMIT", "BEGIN",
	}
	for _, q := range bad {
		if _, err := readOnlyStatement(q); err == nil {
			t.Errorf("%q accepted", q)
		}
	}
}

func TestQuoteIdent(t *testing.T) {
	if got := quoteIdent("mysql", "a`b"); got != "`a``b`" {
		t.Fatalf("mysql = %s", got)
	}
	if got := quoteIdent("postgres", `a"b`); got != `"a""b"` {
		t.Fatalf("postgres = %s", got)
	}
}
