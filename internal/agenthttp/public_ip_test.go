package agenthttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sigardapanel/internal/publicip"
)

func TestPublicIPTaskCaches(t *testing.T) {
	calls := 0
	prev := detectPublicIP
	detectPublicIP = func(ctx context.Context, c *http.Client) (publicip.Result, error) {
		calls++
		return publicip.Result{IP: "43.173.51.154", Source: "lookup"}, nil
	}
	t.Cleanup(func() { detectPublicIP = prev; publicIPCache.res = publicip.Result{} })
	e := NewRouterWithOptions(RouterOptions{NginxConfDir: t.TempDir()})
	for _, q := range []string{"", "", "?refresh=1"} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/system/public-ip"+q, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "43.173.51.154") {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	}
	if calls != 2 {
		t.Fatalf("detections = %d, want 2 (cached once, refreshed once)", calls)
	}
}
