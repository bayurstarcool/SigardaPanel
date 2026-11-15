package jobs

// Let's Encrypt checks a new site's domain right after the site is set up,
// often seconds after its DNS record was created (or before the user added
// one). Asking then fails with NXDOMAIN, and Let's Encrypt's resolvers may
// cache that answer. So the worker first checks public DNS: while the name
// does not resolve the job waits (without using an attempt), and failures
// that are DNS still propagating are retried for a while instead of being
// final. Error messages are cut down from certbot's output to what matters.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"sigardapanel/internal/jobmodel"
)

// sslDNSWindow is how long a new certificate waits for DNS before failing.
const sslDNSWindow = 45 * time.Minute

// dnsLookup reports whether name has an A or AAAA record in public DNS.
// notFound is a definite "no such name / no address" answer; err means
// DNS could not be asked (then the certificate is requested anyway).
// Replaced in tests.
var dnsLookup = func(ctx context.Context, name string) (found bool, notFound bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// DNS over HTTPS works where outbound port 53 is filtered.
	if found, nf, err := dohLookup(ctx, name); err == nil {
		return found, nf, nil
	}
	for _, server := range []string{"1.1.1.1:53", "8.8.8.8:53"} {
		r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 4 * time.Second}
			return d.DialContext(ctx, "udp", server)
		}}
		addrs, err := r.LookupIPAddr(ctx, name)
		if err == nil && len(addrs) > 0 {
			return true, false, nil
		}
		var de *net.DNSError
		if asDNSError(err, &de) && de.IsNotFound {
			return false, true, nil
		}
	}
	return false, false, fmt.Errorf("public DNS could not be reached")
}

func asDNSError(err error, target **net.DNSError) bool {
	de, ok := err.(*net.DNSError)
	if ok {
		*target = de
	}
	return ok
}

func dohLookup(ctx context.Context, name string) (bool, bool, error) {
	for _, typ := range []string{"A", "AAAA"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			"https://cloudflare-dns.com/dns-query?name="+url.QueryEscape(name)+"&type="+typ, nil)
		if err != nil {
			return false, false, err
		}
		req.Header.Set("Accept", "application/dns-json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false, false, err
		}
		var out struct {
			Status int `json:"Status"`
			Answer []struct {
				Type int    `json:"type"`
				Data string `json:"data"`
			} `json:"Answer"`
		}
		err = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			return false, false, fmt.Errorf("doh answer %d", resp.StatusCode)
		}
		for _, a := range out.Answer {
			if a.Type == 1 || a.Type == 28 { // A, AAAA (after any CNAME chain)
				return true, false, nil
			}
		}
		if out.Status != 0 && out.Status != 3 { // 3 = NXDOMAIN, 0 with no answer = no address yet
			return false, false, fmt.Errorf("doh status %d", out.Status)
		}
	}
	return false, true, nil
}

// sslDomain reads the domain from an ssl.issue payload.
func sslDomain(payload string) string {
	var p struct {
		Domain string `json:"domain"`
	}
	_ = json.Unmarshal([]byte(payload), &p)
	return strings.TrimSpace(p.Domain)
}

// sslDNSGate holds back an ssl.issue job while its domain does not resolve.
// It returns true when the job was put back in the queue.
func (w *Worker) sslDNSGate(ctx context.Context, job *jobmodel.Job) bool {
	if job.Type != jobmodel.TypeSSLIssue {
		return false
	}
	domain := sslDomain(job.Payload)
	if domain == "" || time.Since(job.CreatedAt) > sslDNSWindow {
		return false
	}
	found, notFound, err := dnsLookup(ctx, domain)
	if err != nil || found || !notFound {
		return false // resolves, or DNS could not be asked: let the CA decide
	}
	delay := dnsWaitDelay(time.Since(job.CreatedAt))
	msg := fmt.Sprintf("Waiting for DNS: %s does not resolve in public DNS yet; checking again in %s. Point its A record to this server (or let the panel create it in Cloudflare).",
		domain, delay.Round(time.Second))
	_ = w.store.Jobs.Requeue(ctx, job.ID, time.Now().Add(delay), msg)
	return true
}

func dnsWaitDelay(elapsed time.Duration) time.Duration {
	switch {
	case elapsed < 2*time.Minute:
		return 20 * time.Second
	case elapsed < 10*time.Minute:
		return time.Minute
	default:
		return 3 * time.Minute
	}
}

var dnsPendingMarkers = []string{"nxdomain", "dns problem", "no valid ip addresses found", "servfail", "query timed out looking up"}

// isDNSPending: the CA could not see the domain's DNS yet.
func isDNSPending(text string) bool {
	t := strings.ToLower(text)
	for _, m := range dnsPendingMarkers {
		if strings.Contains(t, m) {
			return true
		}
	}
	return false
}

var certbotDetail = regexp.MustCompile(`(?is)Detail:\s*(.+?)(?:\s+Hint:|\s+Domain:|\s+Type:|$)`)
var certbotRate = regexp.MustCompile(`(?i)(too many (?:certificates|failed authorizations|new orders)[^.]*\.)`)

// summarizeSSLError turns certbot's output into one or two sentences.
func summarizeSSLError(raw, domain string) string {
	t := strings.Join(strings.Fields(raw), " ")
	lower := strings.ToLower(t)
	switch {
	case strings.Contains(lower, "certbot not installed") || strings.Contains(lower, "certbot: not found"):
		return "Certbot is not installed on the server (Stack page → Certbot)."
	case strings.Contains(lower, "nxdomain"):
		return fmt.Sprintf("Let's Encrypt cannot find a DNS record for %s (NXDOMAIN). Point its A record to this server; DNS changes can take a few minutes to be visible.", domain)
	case strings.Contains(lower, "no valid ip addresses found"):
		return fmt.Sprintf("%s has no A/AAAA record Let's Encrypt can see yet.", domain)
	case certbotRate.MatchString(t):
		return "Let's Encrypt rate limit: " + certbotRate.FindStringSubmatch(t)[1] + " Try again later."
	case strings.Contains(lower, "timeout during connect") || strings.Contains(lower, "connection refused"):
		return fmt.Sprintf("Let's Encrypt could not reach %s on port 80: check that DNS points to this server and port 80 is open (firewall, cloud security group).", domain)
	case strings.Contains(lower, "unauthorized") && strings.Contains(lower, "invalid response"):
		return fmt.Sprintf("Let's Encrypt reached %s but not this server's challenge file: DNS may point elsewhere, or a proxy/CDN rule (e.g. forced redirects) blocks /.well-known/acme-challenge/.", domain)
	}
	if m := certbotDetail.FindStringSubmatch(t); m != nil {
		d := strings.TrimSpace(m[1])
		if len(d) > 300 {
			d = d[:300] + "…"
		}
		return "Let's Encrypt: " + d
	}
	if len(t) > 400 {
		return t[:400] + "…"
	}
	return t
}
