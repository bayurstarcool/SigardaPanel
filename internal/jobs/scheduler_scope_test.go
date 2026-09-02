package jobs

import (
	"testing"

	"sigardapanel/internal/store"
)

func TestEffectiveBackupSiteIDsAutoDiscoversActiveSites(t *testing.T) {
	cfg := store.BackupConfig{ScopeType: "database", SiteIDs: nil}
	sites := []store.Site{
		{ID: 10, Status: "active"},
		{ID: 11, Status: "pending"},
		{ID: 12, Status: "active"},
	}
	got := effectiveBackupSiteIDs(cfg, sites)
	want := []int64{10, 12}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestEffectiveBackupSiteIDsPreservesExplicitScope(t *testing.T) {
	cfg := store.BackupConfig{ScopeType: "database", SiteIDs: []int64{42}}
	sites := []store.Site{{ID: 10, Status: "active"}, {ID: 42, Status: "active"}}
	got := effectiveBackupSiteIDs(cfg, sites)
	if len(got) != 1 || got[0] != 42 {
		t.Fatalf("got %v, want [42]", got)
	}
}

func TestEffectiveBackupSiteIDsFilesEmptyDoesNotAutoDiscover(t *testing.T) {
	cfg := store.BackupConfig{ScopeType: "files", SiteIDs: nil}
	sites := []store.Site{{ID: 10, Status: "active"}}
	got := effectiveBackupSiteIDs(cfg, sites)
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestEffectiveBackupSiteIDsBothAutoDiscovers(t *testing.T) {
	cfg := store.BackupConfig{ScopeType: "both", SiteIDs: []int64{}}
	sites := []store.Site{{ID: 7, Status: "active"}}
	got := effectiveBackupSiteIDs(cfg, sites)
	if len(got) != 1 || got[0] != 7 {
		t.Fatalf("got %v, want [7]", got)
	}
}

func TestEffectiveBackupSiteIDsDoesNotMutateConfig(t *testing.T) {
	cfg := store.BackupConfig{ScopeType: "database", SiteIDs: nil}
	sites := []store.Site{{ID: 7, Status: "active"}}
	_ = effectiveBackupSiteIDs(cfg, sites)
	if len(cfg.SiteIDs) != 0 {
		t.Fatalf("config mutated: %v", cfg.SiteIDs)
	}
}
