package apihttp

import (
	"context"
	"fmt"
	"log/slog"
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

// hasDNSMarker is an exact match: "SigardaPanel site:1" must not match the
// record of site 12, nor a comment someone wrote around the marker.
func hasDNSMarker(comment, marker string) bool { return strings.TrimSpace(comment) == marker }

type dnsRecordView struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// siteDNSResult reports what happened (or, for a check, what is there).
// Status: created, updated, managed (ours, correct), exists (correct record
// that the panel did not create), replaced, conflict (other records, left
// alone), missing (check only: nothing there yet), skipped, error.
type siteDNSResult struct {
	Status  string `json:"status"`
	Zone    string `json:"zone,omitempty"`
	Name    string `json:"name,omitempty"`
	Content string `json:"content,omitempty"`
	// ContentSource says where the server IP came from (server address, detected).
	ContentSource string          `json:"content_source,omitempty"`
	Message       string          `json:"message"`
	Existing      []dnsRecordView `json:"existing,omitempty"`
	// Provisioned keeps the old response field for clients that read it.
	Provisioned bool   `json:"provisioned"`
	Error       string `json:"error,omitempty"`
}

func dnsFail(msg string) siteDNSResult {
	return siteDNSResult{Status: "error", Message: msg, Error: msg}
}

// siteDNS runs the DNS logic for a site with its owner's Cloudflare account
// (actorID is who asked, for the audit log). dryRun only reports. overwrite
// (explicit consent) replaces A/AAAA/CNAME records at the site's name that
// point elsewhere; without it they are left alone and reported as a conflict.
// replaceIDs, when not nil, limits that consent to the records the user saw.
func (h *siteHandler) siteDNS(ctx context.Context, site *store.Site, actorID int64, dryRun, overwrite bool, replaceIDs map[string]bool) siteDNSResult {
	if !site.OwnerID.Valid {
		return siteDNSResult{Status: "skipped", Message: "The site has no owner, so no Cloudflare account to use."}
	}
	ownerID := site.OwnerID.Int64
	cfg, err := h.store.Cloudflare.GetConfig(ownerID)
	if err != nil || cfg == nil || cfg.APIToken == "" {
		msg := "No Cloudflare account is connected (Cloudflare page)."
		if actorID != ownerID {
			msg = "The site owner has not connected a Cloudflare account."
		}
		return siteDNSResult{Status: "skipped", Message: msg}
	}
	server, _ := h.store.Servers.GetByID(ctx, site.ServerID)
	ip, ipSource, err := dnsTargetIP(ctx, h.store, server)
	if err != nil {
		return dnsFail(err.Error())
	}
	cf := newDNSClient(cfg.APIToken, cfg.Email)
	zones, err := cf.ListZones()
	if err != nil {
		return dnsFail("Cloudflare: " + err.Error())
	}
	// Only active zones: a pending zone's records are not served yet, and a
	// moved or deactivated one belongs to nameservers elsewhere.
	var active []cloudflare.Zone
	for _, z := range zones {
		if z.Status == "" || z.Status == "active" {
			active = append(active, z)
		}
	}
	zone := cloudflare.FindMatchingZone(active, site.Domain)
	if zone == nil {
		if z := cloudflare.FindMatchingZone(zones, site.Domain); z != nil {
			return dnsFail(fmt.Sprintf("The Cloudflare zone %s is %s, not active; finish its setup in Cloudflare first.", z.Name, z.Status))
		}
		return dnsFail("No Cloudflare zone in the site owner's account contains " + site.Domain + ".")
	}
	res := siteDNSResult{Zone: zone.Name, Name: site.Domain, Content: ip, ContentSource: ipSource}
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
		if rec.Managed && hasDNSMarker(live.Comment, marker) && live.Type == "A" {
			if dryRun {
				res.Status, res.Message = "conflict", fmt.Sprintf("The panel's record points to %s, not this server (%s); update it.", live.Content, ip)
				res.Existing = []dnsRecordView{{ID: live.ID, Type: live.Type, Content: live.Content, Proxied: live.Proxied}}
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
					managed := hasDNSMarker(r.Comment, marker)
					h.rememberDNS(ctx, site.ID, ownerID, zone, r, managed)
					break
				}
			}
		}
		return res
	}
	view := func(r cloudflare.DNSRecord) dnsRecordView {
		return dnsRecordView{ID: r.ID, Type: r.Type, Content: r.Content, Proxied: r.Proxied}
	}
	// replaceIDs (from the replace dialog) lists the records the user saw and
	// confirmed; any other conflicting record means the name changed since.
	unconfirmed := false
	if replaceIDs != nil {
		for _, r := range conflicts {
			if !replaceIDs[r.ID] {
				unconfirmed = true
			}
		}
	}
	if len(conflicts) > 0 && (!overwrite || unconfirmed) {
		res.Status = "conflict"
		for _, r := range conflicts {
			res.Existing = append(res.Existing, view(r))
		}
		res.Message = fmt.Sprintf("%s already has DNS records pointing elsewhere; nothing was changed.", site.Domain)
		if overwrite {
			res.Message = fmt.Sprintf("The DNS records for %s changed since you confirmed; nothing was changed. Review them and confirm again.", site.Domain)
		}
		return res
	}
	if dryRun {
		res.Status, res.Message = "missing", fmt.Sprintf("No record for %s yet.", site.Domain)
		return res
	}

	deleteConflict := func(r cloudflare.DNSRecord) error {
		if err := cf.DeleteDNSRecord(zone.ID, r.ID); err != nil {
			return fmt.Errorf("Cloudflare: could not remove the existing %s record (%s): %w", r.Type, r.Content, err)
		}
		res.Existing = append(res.Existing, view(r))
		h.audit.RecordAction(ctx, actorID, "dns_replace", audit.ResourceSite, site.ID, map[string]any{
			"name": site.Domain, "zone": zone.Name, "deleted": view(r), "content": ip,
		})
		return nil
	}
	partial := func(err error) siteDNSResult {
		out := dnsFail(err.Error())
		out.Zone, out.Name, out.Content, out.Existing = res.Zone, res.Name, res.Content, res.Existing
		if len(res.Existing) > 0 {
			out.Message += fmt.Sprintf(" (%d record(s) were already removed; see the audit log)", len(res.Existing))
			out.Error = out.Message
		}
		return out
	}

	// A correct A record already there: only the conflicting ones go.
	if correct > 0 {
		for _, r := range conflicts {
			if err := deleteConflict(r); err != nil {
				return partial(err)
			}
		}
		for _, r := range records {
			if r.Type == "A" && r.Content == ip {
				h.rememberDNS(ctx, site.ID, ownerID, zone, r, hasDNSMarker(r.Comment, marker))
				break
			}
		}
		res.Status = "replaced"
		res.Message = fmt.Sprintf("Removed %d record(s) for %s; the existing A record to this server (%s) was kept.", len(conflicts), site.Domain, ip)
		return res
	}

	// A CNAME cannot coexist with an A record, so it goes first; A/AAAA
	// records go only once the new record exists, so a failed create leaves
	// the name resolving as before.
	var later []cloudflare.DNSRecord
	for _, r := range conflicts {
		if r.Type == "CNAME" {
			if err := deleteConflict(r); err != nil {
				return partial(err)
			}
		} else {
			later = append(later, r)
		}
	}
	proxied := true
	created, err := cf.CreateDNSRecord(zone.ID, cloudflare.CreateDNSRecordRequest{
		Name: site.Domain, Type: "A", Content: ip, TTL: 1, Proxied: &proxied, Comment: marker,
	})
	if err != nil {
		return partial(fmt.Errorf("Cloudflare: %w", err))
	}
	h.rememberDNS(ctx, site.ID, ownerID, zone, *created, true)
	res.Provisioned = true
	for _, r := range later {
		if err := deleteConflict(r); err != nil {
			return partial(err)
		}
	}
	if len(conflicts) > 0 {
		res.Status = "replaced"
		res.Message = fmt.Sprintf("Replaced %d record(s) for %s with an A record to this server (%s).", len(conflicts), site.Domain, ip)
	} else {
		res.Status = "created"
		res.Message = fmt.Sprintf("Created an A record for %s pointing to this server (%s), proxied by Cloudflare.", site.Domain, ip)
		h.audit.RecordAction(ctx, actorID, "dns_create", audit.ResourceSite, site.ID, map[string]any{"name": site.Domain, "zone": zone.Name, "content": ip})
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
			_ = h.store.SiteDNS.Delete(ctx, rec.ID)
			continue
		}
		cfg, err := h.store.Cloudflare.GetConfig(rec.UserID)
		if err != nil || cfg == nil || cfg.APIToken == "" {
			notes = append(notes, rec.Name+": Cloudflare account no longer connected; record left in place, remove it in Cloudflare")
			_ = h.store.SiteDNS.Delete(ctx, rec.ID)
			continue
		}
		cf := newDNSClient(cfg.APIToken, cfg.Email)
		live, err := cf.GetDNSRecord(rec.ZoneID, rec.RecordID)
		handled := true
		switch {
		case err != nil:
			notes = append(notes, rec.Name+": could not check the record ("+err.Error()+"); remove it in Cloudflare if it is no longer needed")
			handled = false
		case live == nil:
			notes = append(notes, rec.Name+": already removed")
		case live.Content != rec.Content || !hasDNSMarker(live.Comment, marker):
			notes = append(notes, rec.Name+": changed outside the panel; left in place")
		default:
			if err := cf.DeleteDNSRecord(rec.ZoneID, rec.RecordID); err != nil {
				notes = append(notes, rec.Name+": could not remove the record ("+err.Error()+"); remove it in Cloudflare")
				handled = false
			} else {
				notes = append(notes, rec.Name+": DNS record removed")
			}
		}
		// A row whose removal failed stays, as the record of what is left
		// in Cloudflare (site ids are never reused).
		if handled {
			_ = h.store.SiteDNS.Delete(ctx, rec.ID)
		}
	}
	return notes
}

// GET /api/v1/sites/:id/dns: the site's DNS state with the site owner's
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
	res := h.siteDNS(c.Request().Context(), site, sess.UserID, true, false, nil)
	return c.JSON(http.StatusOK, httpjson.Success{Data: res, RequestID: c.Response().Header().Get(echo.HeaderXRequestID)})
}

// POST /api/v1/sites/:id/dns {overwrite, replace_ids}: create or repair the
// site's record with the owner's Cloudflare account. Only the owner (or a
// super_admin) may use that account; replace_ids are the conflicting record
// ids the user confirmed, and nothing else is deleted.
func (h *siteHandler) syncSiteDNS(c *echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermWrite)
	if err != nil {
		return err
	}
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	if !h.isSiteOwnerOrSuperAdmin(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusForbidden, "only the site owner can change its DNS records")
	}
	var body struct {
		Overwrite  bool     `json:"overwrite"`
		ReplaceIDs []string `json:"replace_ids"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	var replace map[string]bool
	if body.Overwrite {
		replace = map[string]bool{}
		for _, id := range body.ReplaceIDs {
			replace[id] = true
		}
	}
	res := h.siteDNS(c.Request().Context(), site, sess.UserID, false, body.Overwrite, replace)
	code := http.StatusOK
	if res.Status == "error" {
		code = http.StatusUnprocessableEntity
	}
	return c.JSON(code, httpjson.Success{Data: res, RequestID: c.Response().Header().Get(echo.HeaderXRequestID)})
}
