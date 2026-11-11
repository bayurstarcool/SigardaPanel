package jobs

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/jobmodel"
)

func TestIsPermanentJobError(t *testing.T) {
	cases := []struct {
		typ  string
		msg  string
		want bool
	}{
		{jobmodel.TypeSSLRenew, "agent returned 500: certbot renew failed: Unable to determine zone_id for example.com", true},
		{jobmodel.TypeSSLRenew, "certbot renew failed: No certificate found with name example.com", true},
		{jobmodel.TypeSSLIssue, "DNS problem: NXDOMAIN looking up A for example.com", true},
		{jobmodel.TypeSSLRenew, "Error creating new order :: too many certificates already issued", true},
		{jobmodel.TypeSSLRenew, "do request: connection refused", false},
		{jobmodel.TypeSSLRenew, "Timeout during connect (likely firewall problem)", false},
		{jobmodel.TypeSiteDeploy, "No such certificate", false},
		{jobmodel.TypeSiteCreate, "agent returned 422: PHP-FPM 8.3 is not installed on this server (no /run/php/php8.3-fpm.sock)", true},
		{jobmodel.TypeSiteCreate, "agent returned 500: nginx test failed", false},
		{jobmodel.TypeSiteCreate, "No such certificate", false},
	}
	for _, c := range cases {
		if got := isPermanentJobError(c.typ, errors.New(c.msg), nil); got != c.want {
			t.Errorf("isPermanentJobError(%s, %q) = %v, want %v", c.typ, c.msg, got, c.want)
		}
	}
	if !isPermanentJobError(jobmodel.TypeSSLRenew, errors.New("agent 500"), &agentclient.TaskResult{Error: "No such certificate"}) {
		t.Error("marker in result.Error must count")
	}
}

func TestProcessOne_SSLRenewDeterministicErrorNotRetried(t *testing.T) {
	s, ctx := testWorkerDB(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"certbot renew failed: Unable to determine zone_id for example.com"}`))
	}))
	defer ts.Close()

	res, _ := s.DB().ExecContext(ctx, `INSERT INTO servers (name, hostname, agent_token) VALUES ('s','h','t')`)
	srv, _ := res.LastInsertId()
	res, _ = s.DB().ExecContext(ctx, `INSERT INTO sites (server_id, name, domain, root_path) VALUES (?, 'x', 'example.com', '/x')`, srv)
	site, _ := res.LastInsertId()

	j := &jobmodel.Job{SiteID: sql.NullInt64{Int64: site, Valid: true}, Type: jobmodel.TypeSSLRenew, Payload: `{"domain":"example.com"}`, MaxAttempts: 3}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatal(err)
	}
	NewWorker(s, agentclient.New(ts.URL, "t")).processOne(ctx)

	got, _ := s.Jobs.GetByID(ctx, j.ID)
	if got.Status != jobmodel.StatusFailed {
		t.Fatalf("status = %s, want failed after one attempt", got.Status)
	}
	if got.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", got.Attempts)
	}
}

func TestSSLExpiryNotifier(t *testing.T) {
	n := &sslExpiryNotifier{last: map[int64]sslExpiryNotice{}}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !n.shouldNotify(1, 20, t0) {
		t.Fatal("first notice must be sent")
	}
	if n.shouldNotify(1, 20, t0.Add(6*time.Hour)) {
		t.Fatal("repeat within 24h must be suppressed")
	}
	if !n.shouldNotify(2, 20, t0.Add(6*time.Hour)) {
		t.Fatal("other site must be independent")
	}
	if !n.shouldNotify(1, 7, t0.Add(12*time.Hour)) {
		t.Fatal("crossing the 7-day threshold must notify")
	}
	if n.shouldNotify(1, 6, t0.Add(18*time.Hour)) {
		t.Fatal("same level within 24h must be suppressed")
	}
	if !n.shouldNotify(1, 6, t0.Add(37*time.Hour)) {
		t.Fatal("after 24h must notify again")
	}
}
