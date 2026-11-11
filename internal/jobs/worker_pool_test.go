package jobs

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/jobmodel"
)

// A slow job on one site must not block other sites, and two jobs of the same
// site must never run at the same time.
func TestWorkerPool_ParallelAcrossSitesSerialPerSite(t *testing.T) {
	s, ctx := testWorkerDB(t)

	res, err := s.DB().ExecContext(ctx, `INSERT INTO servers (name, hostname, agent_token) VALUES ('s','h','t')`)
	if err != nil {
		t.Fatal(err)
	}
	srvID, _ := res.LastInsertId()
	var sites []int64
	for _, d := range []string{"a.test", "b.test"} {
		r, err := s.DB().ExecContext(ctx, `INSERT INTO sites (server_id, name, domain, root_path) VALUES (?, ?, ?, '/x')`, srvID, d, d)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := r.LastInsertId()
		sites = append(sites, id)
	}

	releaseSlow := make(chan struct{})
	fastDone := make(chan struct{})
	var mu sync.Mutex
	running := map[string]int{}
	overlap := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p struct {
			Site string `json:"site"`
			Slow bool   `json:"slow"`
		}
		_ = json.Unmarshal(body, &p)
		mu.Lock()
		running[p.Site]++
		if running[p.Site] > 1 {
			overlap = true
		}
		mu.Unlock()
		if p.Slow {
			<-releaseSlow
		} else if p.Site == "b" {
			select {
			case fastDone <- struct{}{}:
			default:
			}
		}
		mu.Lock()
		running[p.Site]--
		mu.Unlock()
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: true})
	}))
	defer ts.Close()

	mk := func(siteID int64, payload string) *jobmodel.Job {
		j := &jobmodel.Job{SiteID: sql.NullInt64{Int64: siteID, Valid: true}, Type: jobmodel.TypeSiteDeploy, Payload: payload}
		if err := s.Jobs.Create(ctx, j); err != nil {
			t.Fatal(err)
		}
		return j
	}
	slow := mk(sites[0], `{"site":"a","slow":true}`)
	sameSite := mk(sites[0], `{"site":"a"}`)
	other := mk(sites[1], `{"site":"b"}`)

	w := NewWorker(s, agentclient.New(ts.URL, "token"))
	w.pollInterval = 20 * time.Millisecond
	w.concurrency = 3
	w.Start(ctx)

	select {
	case <-fastDone:
	case <-time.After(5 * time.Second):
		close(releaseSlow)
		w.Stop()
		t.Fatal("job for site B never ran while site A's slow job was running")
	}

	// While the slow job holds site A, its second job must stay queued.
	time.Sleep(100 * time.Millisecond)
	if j, _ := s.Jobs.GetByID(ctx, sameSite.ID); j.Status != jobmodel.StatusQueued {
		t.Fatalf("same-site job status = %s while slow job running, want queued", j.Status)
	}

	close(releaseSlow)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		done := true
		for _, id := range []int64{slow.ID, sameSite.ID, other.ID} {
			j, _ := s.Jobs.GetByID(ctx, id)
			if j.Status != jobmodel.StatusSucceeded {
				done = false
			}
		}
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	w.Stop()

	for _, id := range []int64{slow.ID, sameSite.ID, other.ID} {
		if j, _ := s.Jobs.GetByID(ctx, id); j.Status != jobmodel.StatusSucceeded {
			t.Fatalf("job %d status = %s, want succeeded", id, j.Status)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if overlap {
		t.Fatal("two jobs of the same site ran concurrently")
	}
}

func TestWorkerConcurrencyFromEnv(t *testing.T) {
	t.Setenv("SIGARDAPANEL_WORKER_CONCURRENCY", "")
	if got := workerConcurrencyFromEnv(); got != defaultWorkerConcurrency {
		t.Fatalf("default = %d", got)
	}
	t.Setenv("SIGARDAPANEL_WORKER_CONCURRENCY", "5")
	if got := workerConcurrencyFromEnv(); got != 5 {
		t.Fatalf("got %d, want 5", got)
	}
	t.Setenv("SIGARDAPANEL_WORKER_CONCURRENCY", "999")
	if got := workerConcurrencyFromEnv(); got != defaultWorkerConcurrency {
		t.Fatalf("out of range accepted: %d", got)
	}
}
