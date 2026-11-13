package jobs

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/db"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"
)

func regenWorkerDB(t *testing.T) *store.Store {
	t.Helper()
	f, _ := os.CreateTemp("", "worker_regen_*.db")
	f.Close()
	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close(); os.Remove(f.Name()) })
	if err := migrations.Up(d, migrations.AllMigrations); err != nil {
		t.Fatal(err)
	}
	return store.New(d)
}

func TestSiteCreateRegenKeepsStatusAndSettings(t *testing.T) {
	s := regenWorkerDB(t)
	ctx := t.Context()
	var mu sync.Mutex
	var got []map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: true})
	}))
	defer ts.Close()
	w := NewWorker(s, agentclient.New(ts.URL, "token"))

	srv := &store.Server{Name: "s", Hostname: "s.example.com", Status: "active", AgentToken: "t"}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "n", Domain: "app.example.com", Runtime: "node", RootPath: "/home/u/htdocs/app.example.com",
		Status: "active", AppPort: sql.NullInt64{Int64: 30001, Valid: true}}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	s.Sites.UpdateStatus(ctx, site.ID, "active")
	s.Security.Upsert(ctx, &store.SiteSecurity{SiteID: site.ID, BlockedIPs: []string{"1.2.3.4"}})

	job := &jobmodel.Job{Type: jobmodel.TypeSiteCreate, SiteID: sql.NullInt64{Int64: site.ID, Valid: true},
		Payload: `{"site_id":1,"domain":"app.example.com","runtime":"node","root_path":"/home/u/htdocs/app.example.com"}`}
	s.Jobs.Create(ctx, job)
	w.processOne(ctx)

	if len(got) != 1 {
		t.Fatalf("agent calls = %d", len(got))
	}
	p := got[0]
	sec, _ := p["security"].(map[string]any)
	if p["vhost_regen"] != true || p["port"] != float64(30001) || sec == nil || len(sec["blocked_ips"].([]any)) != 1 {
		t.Fatalf("payload not completed from the site: %v", p)
	}
	// A failing regeneration must not mark a live site failed either; first
	// check success kept "active" and queued no certificate.
	after, _ := s.Sites.GetByID(ctx, site.ID)
	if after.Status != "active" {
		t.Fatalf("status = %s", after.Status)
	}
	jobs, _ := s.Jobs.List(ctx, store.JobListFilter{})
	for _, j := range jobs {
		if j.Type == jobmodel.TypeSSLIssue {
			t.Fatal("a vhost regeneration queued ssl.issue")
		}
	}

	// Suspended: refused without calling the agent.
	s.Sites.UpdateStatus(ctx, site.ID, "suspended")
	job2 := &jobmodel.Job{Type: jobmodel.TypeSiteCreate, SiteID: sql.NullInt64{Int64: site.ID, Valid: true}, Payload: `{"site_id":1}`}
	s.Jobs.Create(ctx, job2)
	w.processOne(ctx)
	if len(got) != 1 {
		t.Fatal("the agent was called for a suspended site")
	}
	j2, _ := s.Jobs.GetByID(ctx, job2.ID)
	if j2.Status != jobmodel.StatusFailed {
		t.Fatalf("suspended site job status = %s", j2.Status)
	}
	if after, _ := s.Sites.GetByID(ctx, site.ID); after.Status != "suspended" {
		t.Fatalf("suspended site status changed to %s", after.Status)
	}
}

func TestSiteCreateInitialActivatesAndQueuesSSL(t *testing.T) {
	s := regenWorkerDB(t)
	ctx := t.Context()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(agentclient.TaskResult{OK: true})
	}))
	defer ts.Close()
	w := NewWorker(s, agentclient.New(ts.URL, "token"))
	srv := &store.Server{Name: "s", Hostname: "s.example.com", Status: "active", AgentToken: "t"}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "n", Domain: "new.example.com", Runtime: "static", RootPath: "/home/u/htdocs/new.example.com", Status: "pending"}
	s.Sites.Create(ctx, site)
	s.Jobs.Create(ctx, &jobmodel.Job{Type: jobmodel.TypeSiteCreate, SiteID: sql.NullInt64{Int64: site.ID, Valid: true}, Payload: `{}`})
	w.processOne(ctx)
	if after, _ := s.Sites.GetByID(ctx, site.ID); after.Status != "active" {
		t.Fatalf("status = %s", after.Status)
	}
	jobs, _ := s.Jobs.List(ctx, store.JobListFilter{})
	ssl := 0
	for _, j := range jobs {
		if j.Type == jobmodel.TypeSSLIssue {
			ssl++
		}
	}
	if ssl != 1 {
		t.Fatalf("ssl.issue jobs = %d", ssl)
	}
}

func TestSiteCreatePayloadIncludesIPRestrictions(t *testing.T) {
	s := regenWorkerDB(t)
	ctx := t.Context()
	w := NewWorker(s, agentclient.New("http://127.0.0.1:1", "token"))
	srv := &store.Server{Name: "s", Hostname: "s.example.com", Status: "active", AgentToken: "t"}
	s.Servers.Create(ctx, srv)
	site := &store.Site{ServerID: srv.ID, Name: "n", Domain: "ip.example.com", Runtime: "static", RootPath: "/home/u/htdocs/ip.example.com", Status: "active"}
	s.Sites.Create(ctx, site)
	s.Sites.UpdateStatus(ctx, site.ID, "active")
	site, _ = s.Sites.GetByID(ctx, site.ID)
	s.Security.Upsert(ctx, &store.SiteSecurity{SiteID: site.ID, BlockedIPs: []string{"1.2.3.4"}})
	s.IPRestrictions.Create(ctx, &store.IPRestriction{SiteID: site.ID, IP: "5.6.7.8", Action: "deny"})
	s.IPRestrictions.Create(ctx, &store.IPRestriction{SiteID: site.ID, IP: "10.0.0.0/8", Action: "allow"})
	s.IPRestrictions.Create(ctx, &store.IPRestriction{SiteID: site.ID, IP: "1.2.3.4", Action: "deny"})

	raw, err := w.siteCreatePayload(ctx, site, `{"site_id":1,"domain":"ip.example.com"}`)
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Security struct {
			Blocked []string `json:"blocked_ips"`
			Allowed []string `json:"allowed_ips"`
		} `json:"security"`
	}
	json.Unmarshal([]byte(raw), &p)
	if len(p.Security.Blocked) != 2 || len(p.Security.Allowed) != 1 || p.Security.Allowed[0] != "10.0.0.0/8" {
		t.Fatalf("security = %+v (%s)", p.Security, raw)
	}
}
