package jobs

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/jobmodel"
)

func sslJobFixture(t *testing.T, agentBody string, agentCode int) (*Worker, func() *jobmodel.Job, *int) {
	t.Helper()
	s, ctx := testWorkerDB(t)
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(agentCode)
		w.Write([]byte(agentBody))
	}))
	t.Cleanup(ts.Close)
	res, _ := s.DB().ExecContext(ctx, `INSERT INTO servers (name, hostname, agent_token) VALUES ('s','h','t')`)
	srv, _ := res.LastInsertId()
	res, _ = s.DB().ExecContext(ctx, `INSERT INTO sites (server_id, name, domain, root_path) VALUES (?, 'x', 'albi.example.com', '/x')`, srv)
	site, _ := res.LastInsertId()
	j := &jobmodel.Job{SiteID: sql.NullInt64{Int64: site, Valid: true}, Type: jobmodel.TypeSSLIssue, Payload: `{"domain":"albi.example.com"}`, MaxAttempts: 3}
	if err := s.Jobs.Create(ctx, j); err != nil {
		t.Fatal(err)
	}
	get := func() *jobmodel.Job { got, _ := s.Jobs.GetByID(ctx, j.ID); return got }
	return NewWorker(s, agentclient.New(ts.URL, "t")), get, &calls
}

func stubDNS(t *testing.T, found, notFound bool) {
	prev := dnsLookup
	dnsLookup = func(context.Context, string) (bool, bool, error) { return found, notFound, nil }
	t.Cleanup(func() { dnsLookup = prev })
}

func TestSSLWaitsForDNSWithoutUsingAttempts(t *testing.T) {
	stubDNS(t, false, true)
	w, get, calls := sslJobFixture(t, `{"ok":true}`, 200)
	w.processOne(context.Background())
	j := get()
	if *calls != 0 {
		t.Fatal("certbot ran although the domain does not resolve")
	}
	if j.Status != jobmodel.StatusQueued || j.Attempts != 0 || !strings.Contains(j.Error.String, "Waiting for DNS") || !j.NextRetryAt.Valid {
		t.Fatalf("job = %+v", j)
	}
}

func TestSSLNXDOMAINIsRetriedWithASummary(t *testing.T) {
	stubDNS(t, true, false)
	raw := `{"message":"certbot failed: Saving debug log to /var/log/letsencrypt/letsencrypt.log Requesting a certificate for albi.example.com Certbot failed to authenticate some domains (authenticator: webroot). The Certificate Authority reported these problems: Domain: albi.example.com Type: dns Detail: DNS problem: NXDOMAIN looking up A for albi.example.com - check that a DNS record exists for this domain; DNS problem: NXDOMAIN looking up AAAA for albi.example.com Hint: The Certificate Authority failed to download the temporary challenge files"}`
	w, get, calls := sslJobFixture(t, raw, 500)
	w.processOne(context.Background())
	j := get()
	if *calls != 1 || j.Status != jobmodel.StatusQueued || j.Attempts != 0 {
		t.Fatalf("job = %+v calls=%d", j, *calls)
	}
	if strings.Contains(j.Error.String, "letsencrypt.log") || !strings.Contains(j.Error.String, "cannot find a DNS record for albi.example.com") || !strings.Contains(j.Error.String, "Retrying automatically") {
		t.Fatalf("error = %q", j.Error.String)
	}
}

func TestSSLFailureAfterWindowIsSummarized(t *testing.T) {
	stubDNS(t, true, false)
	w, get, _ := sslJobFixture(t, `{"message":"certbot failed: ... Detail: DNS problem: NXDOMAIN looking up A for albi.example.com"}`, 500)
	j := get()
	w.store.DB().Exec(`UPDATE jobs SET created_at = datetime('now', '-2 hours') WHERE id = ?`, j.ID)
	w.processOne(context.Background())
	j = get()
	if j.Status != jobmodel.StatusFailed || !strings.HasPrefix(j.Error.String, "Let's Encrypt cannot find a DNS record") {
		t.Fatalf("job = %+v", j)
	}
}

func TestSummarizeSSLError(t *testing.T) {
	cases := map[string]string{
		"Detail: Timeout during connect (likely firewall problem)":                                         "port 80",
		"urn:ietf:params:acme:error:rateLimited :: too many certificates already issued for exact set.":    "rate limit",
		"agent error 424: certbot not installed":                                                           "Certbot is not installed",
		"Detail: 1.2.3.4: Invalid response from http://x/.well-known/acme-challenge/abc: 404 unauthorized": "challenge file",
	}
	for in, want := range cases {
		if got := summarizeSSLError(in, "x.example.com"); !strings.Contains(got, want) {
			t.Errorf("%q -> %q (want %q)", in, got, want)
		}
	}
}
