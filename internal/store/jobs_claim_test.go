package store

import (
	"database/sql"
	"sync"
	"testing"

	"sigardapanel/internal/jobmodel"
)

func seedServerAndSites(t *testing.T, d *sql.DB, nSites int) (serverID int64, siteIDs []int64) {
	t.Helper()
	res, err := d.Exec(`INSERT INTO servers (name, hostname, agent_token) VALUES ('s', 'h', 't')`)
	if err != nil {
		t.Fatalf("insert server: %v", err)
	}
	serverID, _ = res.LastInsertId()
	for i := 0; i < nSites; i++ {
		res, err := d.Exec(`INSERT INTO sites (server_id, name, domain, root_path) VALUES (?, 'n', ?, '/x')`,
			serverID, "d"+string(rune('a'+i))+".test")
		if err != nil {
			t.Fatalf("insert site: %v", err)
		}
		id, _ := res.LastInsertId()
		siteIDs = append(siteIDs, id)
	}
	return serverID, siteIDs
}

func nullID(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

func TestClaimNext_SerializesPerSite(t *testing.T) {
	d := testDB(t)
	s := NewJobStore(d)
	srv, sites := seedServerAndSites(t, d, 2)

	a1 := &jobmodel.Job{ServerID: nullID(srv), SiteID: nullID(sites[0]), Type: jobmodel.TypeSSLRenew}
	a2 := &jobmodel.Job{ServerID: nullID(srv), SiteID: nullID(sites[0]), Type: jobmodel.TypeSiteDeploy}
	b1 := &jobmodel.Job{ServerID: nullID(srv), SiteID: nullID(sites[1]), Type: jobmodel.TypeSiteDeploy}
	for _, j := range []*jobmodel.Job{a1, a2, b1} {
		if err := s.Create(ctx, j); err != nil {
			t.Fatal(err)
		}
	}

	first, err := s.ClaimNext(ctx, "")
	if err != nil || first == nil || first.ID != a1.ID {
		t.Fatalf("first claim = %+v, %v; want job %d", first, err, a1.ID)
	}
	// a2 shares site A with the running a1, so b1 must be picked instead.
	second, err := s.ClaimNext(ctx, "")
	if err != nil || second == nil || second.ID != b1.ID {
		t.Fatalf("second claim = %+v, %v; want job %d", second, err, b1.ID)
	}
	if third, err := s.ClaimNext(ctx, ""); err != nil || third != nil {
		t.Fatalf("third claim = %+v, %v; want nothing (site A busy)", third, err)
	}

	if err := s.MarkCompleted(ctx, a1.ID, true, "", ""); err != nil {
		t.Fatal(err)
	}
	next, err := s.ClaimNext(ctx, "")
	if err != nil || next == nil || next.ID != a2.ID {
		t.Fatalf("after a1 finished got %+v, %v; want job %d", next, err, a2.ID)
	}
}

func TestClaimNext_OneBackupPerServer(t *testing.T) {
	d := testDB(t)
	s := NewJobStore(d)
	srv, sites := seedServerAndSites(t, d, 2)

	b1 := &jobmodel.Job{ServerID: nullID(srv), SiteID: nullID(sites[0]), Type: jobmodel.TypeBackupCreate}
	b2 := &jobmodel.Job{ServerID: nullID(srv), SiteID: nullID(sites[1]), Type: jobmodel.TypeDBBackupCreate}
	other := &jobmodel.Job{ServerID: nullID(srv), SiteID: nullID(sites[1]), Type: jobmodel.TypeSiteDeploy}
	for _, j := range []*jobmodel.Job{b1, b2, other} {
		if err := s.Create(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	got1, _ := s.ClaimNext(ctx, "")
	got2, _ := s.ClaimNext(ctx, "")
	if got1 == nil || got1.ID != b1.ID || got2 == nil || got2.ID != other.ID {
		t.Fatalf("claims = %+v, %+v; want backup %d then non-backup %d", got1, got2, b1.ID, other.ID)
	}
	if got3, _ := s.ClaimNext(ctx, ""); got3 != nil {
		t.Fatalf("second backup on same server claimed while one is running: %+v", got3)
	}
}

func TestClaimNext_ConcurrentNoDoubleClaim(t *testing.T) {
	d := testDB(t)
	s := NewJobStore(d)
	const n = 40
	for i := 0; i < n; i++ {
		if err := s.Create(ctx, &jobmodel.Job{Type: jobmodel.TypeNginxReload}); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	seen := map[int64]int{}
	var wg sync.WaitGroup
	for w := 0; w < 6; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				j, err := s.ClaimNext(ctx, "")
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				if j == nil {
					return
				}
				mu.Lock()
				seen[j.ID]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != n {
		t.Fatalf("claimed %d distinct jobs, want %d", len(seen), n)
	}
	for id, c := range seen {
		if c != 1 {
			t.Fatalf("job %d claimed %d times", id, c)
		}
	}
}

func TestFailOrphanedRunning(t *testing.T) {
	d := testDB(t)
	s := NewJobStore(d)
	j := &jobmodel.Job{Type: jobmodel.TypeNginxReload}
	if err := s.Create(ctx, j); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimNext(ctx, ""); err != nil {
		t.Fatal(err)
	}
	n, err := s.FailOrphanedRunning(ctx, "restart")
	if err != nil || n != 1 {
		t.Fatalf("FailOrphanedRunning = %d, %v", n, err)
	}
	got, _ := s.GetByID(ctx, j.ID)
	if got.Status != jobmodel.StatusFailed || got.Error.String != "restart" {
		t.Fatalf("job = %s/%q", got.Status, got.Error.String)
	}
}
