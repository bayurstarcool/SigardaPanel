package jobs

import (
	"testing"
	"time"

	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// Existing PHP sites are moved to their own pool once, only on servers whose
// agent knows pools, and not while a regeneration is already queued.
func TestMigratePHPPools(t *testing.T) {
	s, ctx := fullWorkerDB(t)
	current := &store.Server{Name: "new", Hostname: "a", Status: "active", AgentToken: "t"}
	old := &store.Server{Name: "old", Hostname: "b", Status: "active", AgentToken: "t"}
	s.Servers.Create(ctx, current)
	s.Servers.Create(ctx, old)
	s.Servers.UpdateAgentVersion(ctx, current.ID, buildinfo.Version)
	s.Servers.UpdateAgentVersion(ctx, old.ID, "0.0.1")
	mk := func(srv int64, name, runtime, status string) *store.Site {
		st := &store.Site{ServerID: srv, Name: name, Domain: name + ".test", Runtime: runtime, RootPath: "/home/u/htdocs/" + name + ".test", Status: status}
		if err := s.Sites.Create(ctx, st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	php := mk(current.ID, "php", "php", "active")
	done := mk(current.ID, "done", "php", "active")
	static := mk(current.ID, "static", "static", "active")
	oldAgent := mk(old.ID, "oldphp", "php", "active")
	suspended := mk(current.ID, "susp", "php", "suspended")
	store.SetSetting(ctx, s.DB(), phpPoolSettingKey(done.ID), "1")

	now := time.Now()
	migratePHPPools(ctx, s, now)
	queued := func(id int64) bool {
		ok, _ := s.Jobs.HasActiveJob(ctx, jobmodel.TypeSiteCreate, id)
		return ok
	}
	if !queued(php.ID) {
		t.Fatal("PHP site on a current agent not queued")
	}
	for _, st := range []*store.Site{done, static, oldAgent, suspended} {
		if queued(st.ID) {
			t.Fatalf("site %s must not be queued", st.Name)
		}
	}
	// Not queued twice while the first is pending, nor within 6 hours.
	migratePHPPools(ctx, s, now.Add(time.Minute))
	jobs, _ := s.Jobs.List(ctx, store.JobListFilter{SiteID: php.ID})
	if len(jobs) != 1 {
		t.Fatalf("jobs for php site = %d", len(jobs))
	}
}
