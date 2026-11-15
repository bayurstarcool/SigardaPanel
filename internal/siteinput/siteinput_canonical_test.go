package siteinput

import "testing"

func TestCanonicalPHPVersion(t *testing.T) {
	for _, ok := range []string{"8.3", "7.4", "5.6", "8.10"} {
		if v, good := CanonicalPHPVersion(ok); !good || v != ok {
			t.Errorf("%q: %q %v", ok, v, good)
		}
	}
	for _, bad := range []string{"", "8", "8.3/../x", "../8.3", "8.3 ", "10.1"} {
		if _, good := CanonicalPHPVersion(bad); good {
			t.Errorf("%q accepted", bad)
		}
	}
}
