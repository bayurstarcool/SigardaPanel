package notify

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"
)

// httpTimeout bounds every outbound notifier request (connect, TLS, headers
// and body). A hung webhook endpoint must never stall alert delivery.
const httpTimeout = 15 * time.Second

// httpClient is shared by the HTTP-based notifiers. It is a variable so tests
// can swap it.
var httpClient = guardedNotifyClient()

// guardedNotifyClient: webhook URLs come from users with the alert
// permission, and the panel host's own services (the agent, the API, cloud
// metadata at 169.254.169.254) must not be reachable through them. Loopback,
// link-local, unspecified and multicast addresses are refused at dial time
// (after DNS, so a name that resolves there is refused too, also after a
// redirect). Private LAN addresses stay allowed for self-hosted receivers.
func guardedNotifyClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("unresolved address %s", host)
			}
			if blockedNotifyIP(ip) {
				return fmt.Errorf("notification target %s is not allowed (loopback, link-local or metadata address)", ip)
			}
			return nil
		},
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = dialer.DialContext
	tr.Proxy = nil
	return &http.Client{Timeout: httpTimeout, Transport: tr}
}

func blockedNotifyIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 0 {
		return true
	}
	return false
}

// drainAndClose reads (a bounded amount of) the remaining body and closes it so
// the underlying connection can be reused.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}
