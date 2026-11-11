package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClientHasTimeout(t *testing.T) {
	if httpClient.Timeout <= 0 || httpClient.Timeout > time.Minute {
		t.Fatalf("notifier http client timeout = %v, want a bounded timeout", httpClient.Timeout)
	}
}

func TestWebhookNotifier_HungEndpointTimesOut(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer ts.Close()
	defer close(release)

	orig := httpClient
	httpClient = &http.Client{Timeout: 200 * time.Millisecond}
	defer func() { httpClient = orig }()

	n := &WebhookNotifier{}
	ch := &Channel{Type: "webhook", Config: json.RawMessage(`{"url":"` + ts.URL + `"}`)}
	start := time.Now()
	err := n.Send(context.Background(), ch, &Alert{Severity: "error", Subject: "s", Message: "m"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("send took %v; timeout not applied", time.Since(start))
	}
}
