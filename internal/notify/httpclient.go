package notify

import (
	"io"
	"net/http"
	"time"
)

// httpTimeout bounds every outbound notifier request (connect, TLS, headers
// and body). A hung webhook endpoint must never stall alert delivery.
const httpTimeout = 15 * time.Second

// httpClient is shared by the HTTP-based notifiers. It is a variable so tests
// can swap it.
var httpClient = &http.Client{Timeout: httpTimeout}

// drainAndClose reads (a bounded amount of) the remaining body and closes it so
// the underlying connection can be reused.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}
