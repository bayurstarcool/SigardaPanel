package notify

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestWebhookRefusesPanelHostAddresses(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:7710/tasks/command/exec", "http://169.254.169.254/latest/meta-data", "http://[::1]:7700/", "http://0.0.0.0:80/", "http://localhost:7720/"} {
		ch := &Channel{Config: []byte(`{"url":"` + u + `"}`)}
		err := (&WebhookNotifier{}).Send(context.Background(), ch, &Alert{Subject: "s", Message: "m"})
		if err == nil || (!strings.Contains(err.Error(), "not allowed") && !strings.Contains(err.Error(), "address family")) {
			t.Errorf("%s: got %v, want a refusal", u, err)
		}
	}
	for _, ip := range []string{"10.0.0.5", "192.168.1.10", "8.8.8.8"} {
		if blockedNotifyIP(net.ParseIP(ip)) {
			t.Errorf("%s refused, want allowed", ip)
		}
	}
}
