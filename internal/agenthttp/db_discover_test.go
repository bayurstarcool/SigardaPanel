package agenthttp

import "testing"

// The discovery output of mysql -N -B and psql -A -t -F<tab> is read line by
// line; warnings and blank lines are ignored, NULL owners become "".
func TestParseDiscoveredRows(t *testing.T) {
	out := "legacy_app\tappuser,other\t16384\nempty_db\tNULL\t0\n\nmysql: [Warning] something\nshop\t\t1.2e3\r\n"
	got := parseDiscoveredRows("mysql", out)
	if len(got) != 3 {
		t.Fatalf("rows = %+v", got)
	}
	if got[0] != (discoveredDatabase{Engine: "mysql", Name: "legacy_app", Owner: "appuser,other", SizeBytes: 16384}) {
		t.Fatalf("row 0 = %+v", got[0])
	}
	if got[1].Owner != "" || got[1].Name != "empty_db" {
		t.Fatalf("row 1 = %+v", got[1])
	}
	if got[2].SizeBytes != 1200 || got[2].Name != "shop" {
		t.Fatalf("row 2 = %+v", got[2])
	}
	if pg := parseDiscoveredRows("postgres", "app\tapp_owner\t8000000\n"); len(pg) != 1 || pg[0].Engine != "postgres" || pg[0].Owner != "app_owner" {
		t.Fatalf("postgres = %+v", pg)
	}
}
