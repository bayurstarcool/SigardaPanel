package agenthttp

import "testing"

func TestQuoteIdent(t *testing.T) {
	if got := quoteIdent("mysql", "a`b"); got != "`a``b`" {
		t.Fatalf("mysql = %s", got)
	}
	if got := quoteIdent("postgres", `a"b`); got != `"a""b"` {
		t.Fatalf("postgres = %s", got)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`50%_off\x`); got != `50\%\_off\\x` {
		t.Fatalf("escapeLike = %s", got)
	}
}

func TestCatalogColumn(t *testing.T) {
	cols := []string{"id", "name"}
	if c, ok := catalogColumn(cols, "name"); !ok || c != "name" {
		t.Fatal("known column not found")
	}
	if _, ok := catalogColumn(cols, "name; DROP TABLE x"); ok {
		t.Fatal("unknown column accepted")
	}
}

func TestViewerOperatorsAreFixed(t *testing.T) {
	for _, op := range []string{"eq", "ne", "lt", "le", "gt", "ge", "contains", "starts", "null", "notnull"} {
		if _, ok := viewerOps[op]; !ok {
			t.Errorf("operator %s missing", op)
		}
	}
	if _, ok := viewerOps["; DROP"]; ok {
		t.Fatal("arbitrary operator accepted")
	}
}
