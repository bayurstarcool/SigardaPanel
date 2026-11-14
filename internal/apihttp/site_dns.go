package apihttp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/cloudflare"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

// dnsAPI is the part of the Cloudflare client site DNS uses (tests fake it).
type dnsAPI interface {
	ListZones() ([]cloudflare.Zone, error)
	ListDNSRecordsByName(zoneID, name string) ([]cloudflare.DNSRecord, error)
	GetDNSRecord(zoneID, recordID string) (*cloudflare.DNSRecord, error)
	CreateDNSRecord(zoneID string, req cloudflare.CreateDNSRecordRequest) (*cloudflare.DNSRecord, error)
	UpdateDNSRecordPartial(zoneID, recordID string, req map[string]any) (*cloudflare.DNSRecord, error)
	DeleteDNSRecord(zoneID, recordID string) error
}

var newDNSClient = func(token, email string) dnsAPI {
	return cloudflare.NewClient(cloudflare.Config{APIToken: token, Email: email})
}

// dnsMarker is the comment the panel puts on the records it creates; a record
// is only changed or removed by the panel while it still carries it.
func dnsMarker(siteID int64) string { return "SigardaPanel site:" + strconv.FormatInt(siteID, 10) }

type dnsRecordView struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// siteDNSResult reports what happened (or, for a check, what is there).
// Status: created, updated, managed (ours, correct), exists (correct record
// that the panel did not create), replaced, conflict (other records, left
// alone), missing (check only: nothing there yet), skipped, error.
type siteDNSResult struct {
	Status   string          `json:"status"`
	Zone     string          `json:"zone,omitempty"`
	Name     string          `json:"name,omitempty"`
	Content  string          `json:"content,omitempty"`
	Message  string          `json:"message"`
	Existing []dnsRecordView `json:"existing,omitempty"`
	// Provisioned keeps the old response field for clients that read it.
	Provisioned bool   `json:"provisioned"`
	Error       string `json:"error,omitempty"`
}

func dnsFail(msg string) siteDNSResult {
	return siteDNSResult{Status: "error", Message: msg, Error: msg}
}

// publicServerIP is the IPv4 address DNS should point at: the server's
// address when it is public. Cloud VMs often report a private one (10.x,
// 172.16/12, 192.168.x, 100.64/10); pointing a domain there breaks it.
func publicServerIP(server *store.Server) (string, error) {
	if server == nil || !server.IPAddress.Valid || strings.TrimSpace(server.IPAddress.String) == "" {
		return "", fmt.Errorf("the server has no IP address; set its public IP on the Servers page")
	}
	raw := strings.TrimSpace(server.IPAddress.String)
	ip := net.ParseIP(raw).To4()
	if ip == nil {
		return "", fmt.Errorf("the server's address %s is not an IPv4 address; set its public IPv4 on the Servers page", raw)
	}
	if !isPublicIPv4(ip) {
		return "", fmt.Errorf("the server's address %s is private or reserved; set its public IP on the Servers page so DNS can point to it", raw)
	}
	return ip.String(), nil
}

var reservedV4 = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/3"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

func isPublicIPv4(ip net.IP) bool {
	for _, n := range reservedV4 {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// siteDNS runs the DNS logic for a site with userID's Cloudflare account.
// dryRun only reports. overwrite (explicit consent) replaces A/AAAA/CNAME
// records at the site's name that point elsewhere; without it they are left
// alone and reported as a conflict.
func (h *siteHandler) siteDNS(ctx context.Context, site *store.Site, userID int64, dryRun, overwrite bool) siteDNSResult {
	cfg, err := h.store.Cloudflare.GetConfig(userID)
	if err != nil || cfg == nil || cfg.APIToken == "" {
		return siteDNSResult{Status: "skipped", Message: "No Cloudflare account is connected (Cloudflare page)."}
	}
	server, _ := h.store.Servers.GetByID(ctx, site.ServerID)
	ip, err := publicServerIP(server)
	if err != nil {
		return dnsFail(err.Error())
	}
	cf := newDNSClient(cfg.APIToken, cfg.Email)
	zones, err := cf.ListZones()
	if err != nil {
		return dnsFail("Cloudflare: " + err.Error())
	}
	zone := cloudflare.FindMatchingZone(zones, site.Domain)
	if zone == nil {
		return dnsFail("No Cloudflare zone in your account contains " + site.Domain + ".")
	}
	res := siteDNSResult{Zone: zone.Name, Name: site.Domain, Content: ip}
	marker := dnsMarker(site.ID)

	// A record this site already owns: keep it pointing at the server.
	stored, _ := h.store.SiteDNS.ListBySite(ctx, site.ID)
	for _, rec := range stored {
		if rec.ZoneID != zone.ID || rec.Name != site.Domain {
			continue
		}
		live, err := cf.GetDNSRecord(rec.ZoneID, rec.RecordID)
		if err != nil {
			return dnsFail("Cloudflare: " + err.Error())
		}
		if live == nil {
			if !dryRun {
				_ = h.store.SiteDNS.Delete(ctx, rec.ID) // gone: re-evaluate below
			}
			continue
		}
		if live.Content == ip {
			res.Status, res.Provisioned = "managed", rec.Managed
			if !rec.Managed {
				res.Status = "exists"
			}
			res.Message = fmt.Sprintf("%s points to this server (%s).", site.Domain, ip)
			return res
		}
		if rec.Managed && strings.Contains(live.Comment, marker) && live.Type == "A" {
			if dryRun {
				res.Status, res.Message = "conflict", fmt.Sprintf("The panel's record points to %s, not this server (%s); update it.", live.Content, ip)
				res.Existing = []dnsRecordView{{Type: live.Type, Content: live.Content, Proxied: live.Proxied}}
				return res
			}
			if _, err := cf.UpdateDNSRecordPartial(rec.ZoneID, rec.RecordID, map[string]any{"content": ip}); err != nil {
				return dnsFail("Cloudflare: " + err.Error())
			}
			h.updateStoredContent(ctx, rec, ip)
			res.Status, res.Provisioned = "updated", true
			res.Message = fmt.Sprintf("%s updated from %s to this server (%s).", site.Domain, live.Content, ip)
			return res
		}
		// Changed outside the panel (or not ours): treat like any other record.
	}

	records, err := cf.ListDNSRecordsByName(zone.ID, site.Domain)
	if err != nil {
		return dnsFail("Cloudflare: " + err.Error())
	}
	var conflicts []cloudflare.DNSRecord
	correct := 0
	for _, r := range records {
		switch r.Type {
		case "A":
			if r.Content == ip {
				correct++
			} else {
				conflicts = append(conflicts, r)
			}
		case "AAAA", "CNAME":
			conflicts = append(conflicts, r)
		}
	}
	if len(conflicts) == 0 && correct > 0 {
		res.Status = "exists"
		res.Message = fmt.Sprintf("%s already points to this server (%s); left as it is.", site.Domain, ip)
		if !dryRun {
			for _, r := range records {
				if r.Type == "A" && r.Content == ip {
					managed := strings.Contains(r.Comment, marker)
					h.rememberDNS(ctx, site.ID, userID, zone, r, managed)
					break
				}
			}
		}
		return res
	}
	if len(conflicts) > 0 && !overwrite {
		res.Status = "conflict"
		for _, r := range conflicts {
			res.Existing = append(res.Existing, dnsRecordView{Type: r.Type, Content: r.Content, Proxied: r.Proxied})
		}
		res.Message = fmt.Sprintf("%s already has DNS records pointing elsewhere; nothing was changed.", site.Domain)
		return res
	}
	if dryRun {
		res.Status, res.Message = "missing", fmt.Sprintf("No record for %s yet.", site.Domain)
		return res
	}
	for _, r := range conflicts {
		if err := cf.DeleteDNSRecord(zone.ID, r.ID); err != nil {
			return dnsFail("Cloudflare: could not replace the existing " + r.Type + " record: " + err.Error())
		}
		res.Existing = append(res.Existing, dnsRecordView{Type: r.Type, Content: r.Content, Proxied: r.Proxied})
	}
	proxied := true
	created, err := cf.CreateDNSRecord(zone.ID, cloudflare.CreateDNSRecordRequest{
		Name: site.Domain, Type: "A", Content: ip, TTL: 1, Proxied: &proxied, Comment: marker,
	})
	if err != nil {
		return dnsFail("Cloudflare: " + err.Error())
	}
	h.rememberDNS(ctx, site.ID, userID, zone, *created, true)
	res.Provisioned = true
	if len(conflicts) > 0 {
		res.Status = "replaced"
		res.Message = fmt.Sprintf("Replaced %d record(s) for %s with an A record to this server (%s).", len(conflicts), site.Domain, ip)
		h.audit.RecordAction(ctx, userID, "dns_replace", audit.ResourceSite, site.ID, map[string]any{
			"name": site.Domain, "zone": zone.Name, "replaced": res.Existing, "content": ip,
		})
	} else {
		res.Status = "created"
		res.Message = fmt.Sprintf("Created an A record for %s pointing to this server (%s), proxied by Cloudflare.", site.Domain, ip)
		h.audit.RecordAction(ctx, userID, "dns_create", audit.ResourceSite, site.ID, map[string]any{"name": site.Domain, "zone": zone.Name, "content": ip})
	}
	return res
}

func (h *siteHandler) rememberDNS(ctx context.Context, siteID, userID int64, zone *cloudflare.Zone, r cloudflare.DNSRecord, managed bool) {
	if err := h.store.SiteDNS.Add(ctx, &store.SiteDNSRecord{
		SiteID: siteID, UserID: userID, ZoneID: zone.ID, ZoneName: zone.Name, RecordID: r.ID,
		Name: r.Name, Type: r.Type, Content: r.Content, Managed: managed,
	}); err != nil {
		slog.Warn("site dns: remember record", "site_id", siteID, "error", err)
	}
}

func (h *siteHandler) updateStoredContent(ctx context.Context, rec store.SiteDNSRecord, content string) {
	_ = h.store.SiteDNS.Delete(ctx, rec.ID)
	rec.Content = content
	_ = h.store.SiteDNS.Add(ctx, &rec)
}

// removeSiteDNS deletes the records the panel created for a site, with the
// account that created them, and only while each still carries the panel's
// marker and the content the panel set. Records that already existed, or
// were changed outside the panel since, stay. Returns what it did.
func (h *siteHandler) removeSiteDNS(ctx context.Context, site *store.Site) []string {
	stored, err := h.store.SiteDNS.ListBySite(ctx, site.ID)
	if err != nil || len(stored) == 0 {
		return nil
	}
	var notes []string
	marker := dnsMarker(site.ID)
	for _, rec := range stored {
		if !rec.Managed {
			notes = append(notes, rec.Name+": existing record left in place")
			continue
		}
		cfg, err := h.store.Cloudflare.GetConfig(rec.UserID)
		if err != nil || cfg == nil || cfg.APIToken == "" {
			notes = append(notes, rec.Name+": Cloudflare account no longer connected; record left in place")
			continue
		}
		cf := newDNSClient(cfg.APIToken, cfg.Email)
		live, err := cf.GetDNSRecord(rec.ZoneID, rec.RecordID)
		switch {
		case err != nil:
			notes = append(notes, rec.Name+": "+err.Error())
		case live == nil:
			notes = append(notes, rec.Name+": already removed")
		case live.Content != rec.Content || !strings.Contains(live.Comment, marker):
			notes = append(notes, rec.Name+": changed outside the panel; left in place")
		default:
			if err := cf.DeleteDNSRecord(rec.ZoneID, rec.RecordID); err != nil {
				notes = append(notes, rec.Name+": "+err.Error())
			} else {
				notes = append(notes, rec.Name+": DNS record removed")
			}
		}
	}
	_ = h.store.SiteDNS.DeleteBySite(ctx, site.ID)
	return notes
}

// GET /api/v1/sites/:id/dns: the site's DNS state with the caller's
// Cloudflare account (nothing is changed).
func (h *siteHandler) getSiteDNS(c *echo.Context) error {
	site, err := h.getSiteForUser(c)
	if err != nil {
		return err
	}
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	res := h.siteDNS(c.Request().Context(), site, sess.UserID, true, false)
	return c.JSON(http.StatusOK, httpjson.Success{Data: res, RequestID: c.Response().Header().Get(echo.HeaderXRequestID)})
}

// POST /api/v1/sites/:id/dns {overwrite}: create or repair the site's record.
func (h *siteHandler) syncSiteDNS(c *echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermWrite)
	if err != nil {
		return err
	}
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	var body struct {
		Overwrite bool `json:"overwrite"`
	}
	_ = c.Bind(&body)
	res := h.siteDNS(c.Request().Context(), site, sess.UserID, false, body.Overwrite)
	code := http.StatusOK
	if res.Status == "error" {
		code = http.StatusUnprocessableEntity
	}
	return c.JSON(code, httpjson.Success{Data: res, RequestID: c.Response().Header().Get(echo.HeaderXRequestID)})
}
