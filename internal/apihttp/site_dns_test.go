package apihttp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/cloudflare"
	"sigardapanel/internal/store"
)

// fakeCF is an in-memory Cloudflare account.
type fakeCF struct {
	zones   []cloudflare.Zone
	records map[string]*cloudflare.DNSRecord
	next    int
	deleted []string
}

func newFakeCF(zones ...string) *fakeCF {
	f := &fakeCF{records: map[string]*cloudflare.DNSRecord{}}
	for i, z := range zones {
		f.zones = append(f.zones, cloudflare.Zone{ID: fmt.Sprintf("z%d", i+1), Name: z})
	}
	return f
}

func (f *fakeCF) add(zoneID, name, typ, content, comment string) string {
	f.next++
	id := fmt.Sprintf("r%d", f.next)
	f.records[id] = &cloudflare.DNSRecord{ID: id, ZoneID: zoneID, Name: name, Type: typ, Content: content, Comment: comment, Proxied: true}
	return id
}
func (f *fakeCF) ListZones() ([]cloudflare.Zone, error) { return f.zones, nil }
func (f *fakeCF) ListDNSRecordsByName(zoneID, name string) ([]cloudflare.DNSRecord, error) {
	var out []cloudflare.DNSRecord
	for _, r := range f.records {
		if r.ZoneID == zoneID && r.Name == name {
			out = append(out, *r)
		}
	}
	return out, nil
}
func (f *fakeCF) GetDNSRecord(zoneID, id string) (*cloudflare.DNSRecord, error) {
	if r, ok := f.records[id]; ok && r.ZoneID == zoneID {
		c := *r
		return &c, nil
	}
	return nil, nil
}
func (f *fakeCF) CreateDNSRecord(zoneID string, req cloudflare.CreateDNSRecordRequest) (*cloudflare.DNSRecord, error) {
	id := f.add(zoneID, req.Name, req.Type, req.Content, req.Comment)
	return f.records[id], nil
}
func (f *fakeCF) UpdateDNSRecordPartial(zoneID, id string, req map[string]any) (*cloudflare.DNSRecord, error) {
	r := f.records[id]
	r.Content = req["content"].(string)
	return r, nil
}
func (f *fakeCF) DeleteDNSRecord(zoneID, id string) error {
	delete(f.records, id)
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeCF) byName(name string) []cloudflare.DNSRecord {
	var out []cloudflare.DNSRecord
	for _, r := range f.records {
		if r.Name == name {
			out = append(out, *r)
		}
	}
	return out
}

func dnsFixture(t *testing.T, serverIP string, cf *fakeCF) (*authzFixture, *store.User, string) {
	t.Helper()
	f := newAuthzFixture(t)
	owner, tok := f.user("owner", auth.RoleUser)
	srv, _ := f.s.Servers.GetByID(f.ctx, f.serverID)
	srv.IPAddress = sql.NullString{String: serverIP, Valid: serverIP != ""}
	if _, err := f.s.DB().ExecContext(f.ctx, `UPDATE servers SET ip_address = ? WHERE id = ?`, serverIP, f.serverID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Cloudflare.SaveConfig(owner.ID, store.CloudflareConfig{APIToken: "tok-" + strconv.FormatInt(owner.ID, 10)}); err != nil {
		t.Fatal(err)
	}
	prev := newDNSClient
	newDNSClient = func(token, email string) dnsAPI { return cf }
	t.Cleanup(func() { newDNSClient = prev })
	return f, owner, tok
}

func createSiteDNS(t *testing.T, f *authzFixture, tok, domain string, overwrite bool) (int64, map[string]any) {
	t.Helper()
	body := map[string]any{"server_id": f.serverID, "name": "s", "domain": domain, "runtime": "static", "cloudflare_overwrite": overwrite}
	rec := f.do("POST", "/api/v1/sites", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s = %d %s", domain, rec.Code, rec.Body.String())
	}
	var out struct {
		Data struct {
			Site struct {
				ID int64 `json:"id"`
			} `json:"site"`
			DNS map[string]any `json:"cloudflare_dns"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Data.Site.ID, out.Data.DNS
}

func TestSiteDNSCreateAndSafeDelete(t *testing.T) {
	cf := newFakeCF("example.com")
	f, _, tok := dnsFixture(t, "203.0.114.10", cf)
	id, dns := createSiteDNS(t, f, tok, "shop.example.com", false)
	if dns["status"] != "created" {
		t.Fatalf("create dns = %v", dns)
	}
	recs := cf.byName("shop.example.com")
	if len(recs) != 1 || recs[0].Type != "A" || recs[0].Content != "203.0.114.10" || !strings.Contains(recs[0].Comment, "SigardaPanel site:") {
		t.Fatalf("records = %+v", recs)
	}
	// An unrelated record with the same name in another type is never touched.
	cf.add("z1", "shop.example.com", "TXT", "verify=1", "")
	if rec := f.do("DELETE", "/api/v1/sites/"+strconv.FormatInt(id, 10), nil, tok); rec.Code != http.StatusOK {
		t.Fatalf("delete = %d", rec.Code)
	}
	left := cf.byName("shop.example.com")
	if len(left) != 1 || left[0].Type != "TXT" {
		t.Fatalf("after delete = %+v", left)
	}
}

func TestSiteDNSNeverReplacesWithoutConsent(t *testing.T) {
	cf := newFakeCF("example.com")
	cf.add("z1", "blog.example.com", "A", "198.51.99.7", "")
	f, _, tok := dnsFixture(t, "203.0.114.10", cf)
	_, dns := createSiteDNS(t, f, tok, "blog.example.com", false)
	if dns["status"] != "conflict" {
		t.Fatalf("dns = %v", dns)
	}
	if recs := cf.byName("blog.example.com"); len(recs) != 1 || recs[0].Content != "198.51.99.7" {
		t.Fatalf("existing record changed: %+v", recs)
	}

	cf.add("z1", "api.example.com", "CNAME", "elsewhere.example.net", "")
	_, dns = createSiteDNS(t, f, tok, "api.example.com", false)
	if dns["status"] != "conflict" {
		t.Fatalf("cname dns = %v", dns)
	}

	// With consent: replaced, and the panel's record is the only A/CNAME.
	_, dns = createSiteDNS(t, f, tok, "www.example.com", false)
	cf.add("z1", "app.example.com", "A", "198.51.99.8", "")
	_, dns = createSiteDNS(t, f, tok, "app.example.com", true)
	if dns["status"] != "replaced" {
		t.Fatalf("overwrite dns = %v", dns)
	}
	if recs := cf.byName("app.example.com"); len(recs) != 1 || recs[0].Content != "203.0.114.10" {
		t.Fatalf("after overwrite = %+v", recs)
	}
}

func TestSiteDNSExistingCorrectRecordIsKeptOnDelete(t *testing.T) {
	cf := newFakeCF("example.com")
	existing := cf.add("z1", "keep.example.com", "A", "203.0.114.10", "")
	f, _, tok := dnsFixture(t, "203.0.114.10", cf)
	id, dns := createSiteDNS(t, f, tok, "keep.example.com", false)
	if dns["status"] != "exists" {
		t.Fatalf("dns = %v", dns)
	}
	f.do("DELETE", "/api/v1/sites/"+strconv.FormatInt(id, 10), nil, tok)
	if _, ok := cf.records[existing]; !ok {
		t.Fatal("a record the panel did not create was deleted")
	}
}

func TestSiteDNSRecordChangedOutsideIsKeptOnDelete(t *testing.T) {
	cf := newFakeCF("example.com")
	f, _, tok := dnsFixture(t, "203.0.114.10", cf)
	id, _ := createSiteDNS(t, f, tok, "moved.example.com", false)
	for _, r := range cf.records {
		if r.Name == "moved.example.com" {
			r.Content = "198.51.99.1" // the user re-pointed it in Cloudflare
		}
	}
	f.do("DELETE", "/api/v1/sites/"+strconv.FormatInt(id, 10), nil, tok)
	if recs := cf.byName("moved.example.com"); len(recs) != 1 {
		t.Fatalf("re-pointed record deleted: %+v", recs)
	}
}

func TestSiteDNSZoneBoundaryAndPrivateIP(t *testing.T) {
	cf := newFakeCF("example.com")
	f, _, tok := dnsFixture(t, "203.0.114.10", cf)
	_, dns := createSiteDNS(t, f, tok, "myexample.com", false)
	if dns["status"] != "error" || len(cf.records) != 0 {
		t.Fatalf("myexample.com used zone example.com: %v %+v", dns, cf.records)
	}

	cf2 := newFakeCF("example.com")
	f2, _, tok2 := dnsFixture(t, "10.0.0.5", cf2)
	_, dns = createSiteDNS(t, f2, tok2, "shop.example.com", false)
	if dns["status"] != "error" || !strings.Contains(fmt.Sprint(dns["message"]), "private") || len(cf2.records) != 0 {
		t.Fatalf("private IP: %v", dns)
	}
}

func TestSiteDNSSyncUpdatesAfterIPChange(t *testing.T) {
	cf := newFakeCF("example.com", "dev.example.com")
	f, _, tok := dnsFixture(t, "203.0.114.10", cf)
	id, dns := createSiteDNS(t, f, tok, "a.dev.example.com", false)
	if dns["status"] != "created" || dns["zone"] != "dev.example.com" {
		t.Fatalf("sub-zone: %v", dns)
	}
	f.s.DB().ExecContext(f.ctx, `UPDATE servers SET ip_address = '203.0.114.20' WHERE id = ?`, f.serverID)
	sid := strconv.FormatInt(id, 10)
	rec := f.do("GET", "/api/v1/sites/"+sid+"/dns", nil, tok)
	if !strings.Contains(rec.Body.String(), `"status":"conflict"`) {
		t.Fatalf("check after IP change = %s", rec.Body.String())
	}
	rec = f.do("POST", "/api/v1/sites/"+sid+"/dns", map[string]any{}, tok)
	if !strings.Contains(rec.Body.String(), `"status":"updated"`) {
		t.Fatalf("sync = %d %s", rec.Code, rec.Body.String())
	}
	if recs := cf.byName("a.dev.example.com"); len(recs) != 1 || recs[0].Content != "203.0.114.20" {
		t.Fatalf("after sync = %+v", recs)
	}
}

func TestFindMatchingZone(t *testing.T) {
	zones := []cloudflare.Zone{{ID: "1", Name: "example.com"}, {ID: "2", Name: "dev.example.com"}, {ID: "3", Name: "sigarda.co.id"}}
	cases := map[string]string{
		"example.com": "1", "www.example.com": "1", "a.dev.example.com": "2", "dev.example.com": "2",
		"myexample.com": "", "example.com.evil.net": "", "albi.sigarda.co.id": "3",
	}
	for domain, want := range cases {
		got := ""
		if z := cloudflare.FindMatchingZone(zones, domain); z != nil {
			got = z.ID
		}
		if got != want {
			t.Errorf("%s: zone %q, want %q", domain, got, want)
		}
	}
}

func dnsSiteID(t *testing.T, f *authzFixture, tok, domain string) string {
	t.Helper()
	id, _ := createSiteDNS(t, f, tok, domain, false)
	return strconv.FormatInt(id, 10)
}

func TestSiteDNSOverwriteKeepsCorrectRecordAndOrder(t *testing.T) {
	cf := newFakeCF("example.com")
	keep := cf.add("z1", "both.example.com", "A", "203.0.114.10", "")
	stale := cf.add("z1", "both.example.com", "A", "198.51.99.9", "")
	f, _, tok := dnsFixture(t, "203.0.114.10", cf)
	_, dns := createSiteDNS(t, f, tok, "both.example.com", true)
	if dns["status"] != "replaced" {
		t.Fatalf("dns = %v", dns)
	}
	if _, ok := cf.records[keep]; !ok {
		t.Fatal("the correct existing A record was removed")
	}
	if _, ok := cf.records[stale]; ok {
		t.Fatal("the conflicting A record was kept")
	}
	if recs := cf.byName("both.example.com"); len(recs) != 1 {
		t.Fatalf("a duplicate record was created: %+v", recs)
	}
}

func TestSiteDNSReplaceOnlyConfirmedRecords(t *testing.T) {
	cf := newFakeCF("example.com")
	seen := cf.add("z1", "c.example.com", "A", "198.51.99.1", "")
	f, _, tok := dnsFixture(t, "203.0.114.10", cf)
	sid := dnsSiteID(t, f, tok, "c.example.com")
	// A record appears after the user opened the dialog.
	late := cf.add("z1", "c.example.com", "AAAA", "2001:db8::1", "")
	rec := f.do("POST", "/api/v1/sites/"+sid+"/dns", map[string]any{"overwrite": true, "replace_ids": []string{seen}}, tok)
	if !strings.Contains(rec.Body.String(), `"status":"conflict"`) {
		t.Fatalf("unconfirmed record replaced: %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := cf.records[seen]; !ok {
		t.Fatal("deleted although another record was unconfirmed")
	}
	rec = f.do("POST", "/api/v1/sites/"+sid+"/dns", map[string]any{"overwrite": true, "replace_ids": []string{seen, late}}, tok)
	if !strings.Contains(rec.Body.String(), `"status":"replaced"`) {
		t.Fatalf("confirmed replace = %d %s", rec.Code, rec.Body.String())
	}
	if recs := cf.byName("c.example.com"); len(recs) != 1 || recs[0].Content != "203.0.114.10" {
		t.Fatalf("after replace = %+v", recs)
	}
}

func TestSiteDNSUsesOwnerAccountAndOwnerOnlySync(t *testing.T) {
	cf := newFakeCF("example.com")
	f, owner, tok := dnsFixture(t, "203.0.114.10", cf)
	sid := dnsSiteID(t, f, tok, "own.example.com")
	id, _ := strconv.ParseInt(sid, 10, 64)
	writer, wtok := f.user("writer", auth.RoleUser)
	if err := f.s.UserSites.Assign(f.ctx, writer.ID, id, "write"); err != nil {
		t.Fatal(err)
	}
	// The writer has no Cloudflare account; the check still uses the owner's.
	rec := f.do("GET", "/api/v1/sites/"+sid+"/dns", nil, wtok)
	if !strings.Contains(rec.Body.String(), `"status":"managed"`) {
		t.Fatalf("writer check = %s", rec.Body.String())
	}
	if rec := f.do("POST", "/api/v1/sites/"+sid+"/dns", map[string]any{}, wtok); rec.Code != http.StatusForbidden {
		t.Fatalf("writer sync = %d", rec.Code)
	}
	_ = owner
}

func TestSiteDNSExactMarkerAndInactiveZone(t *testing.T) {
	if hasDNSMarker("SigardaPanel site:12", dnsMarker(1)) || hasDNSMarker("x SigardaPanel site:1", dnsMarker(1)) || !hasDNSMarker(" SigardaPanel site:1 ", dnsMarker(1)) {
		t.Fatal("marker match is not exact")
	}
	cf := newFakeCF("example.com")
	cf.zones[0].Status = "pending"
	f, _, tok := dnsFixture(t, "203.0.114.10", cf)
	_, dns := createSiteDNS(t, f, tok, "p.example.com", false)
	if dns["status"] != "error" || !strings.Contains(fmt.Sprint(dns["message"]), "pending") || len(cf.records) != 0 {
		t.Fatalf("pending zone: %v", dns)
	}
}

type failingDeleteCF struct{ *fakeCF }

func (f failingDeleteCF) DeleteDNSRecord(zoneID, id string) error { return fmt.Errorf("rate limited") }

func TestSiteDNSFailedRemovalIsKept(t *testing.T) {
	cf := newFakeCF("example.com")
	f, _, tok := dnsFixture(t, "203.0.114.10", cf)
	sid := dnsSiteID(t, f, tok, "gone.example.com")
	id, _ := strconv.ParseInt(sid, 10, 64)
	newDNSClient = func(token, email string) dnsAPI { return failingDeleteCF{cf} }
	rec := f.do("DELETE", "/api/v1/sites/"+sid, nil, tok)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "remove it in Cloudflare") {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	if recs, _ := f.s.SiteDNS.ListBySite(f.ctx, id); len(recs) != 1 {
		t.Fatalf("tracking row dropped although the record is still there: %+v", recs)
	}
}
