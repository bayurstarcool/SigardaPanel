package jobs

import "testing"

func TestShouldAlertOnlyWhenLevelRises(t *testing.T) {
	mc := &MetricsCollector{}
	steps := []struct {
		level string
		want  bool
	}{{"warning", true}, {"warning", false}, {"critical", true}, {"critical", false}, {"warning", false}, {"", false}, {"warning", true}}
	for i, s := range steps {
		if got := mc.shouldAlert("disk:1", s.level); got != s.want {
			t.Fatalf("step %d (%q) = %v, want %v", i, s.level, got, s.want)
		}
	}
	if !mc.shouldAlert("disk:2", "warning") {
		t.Fatal("servers are tracked separately")
	}
}
