package apihttp

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestTrustedRealIPExtractor(t *testing.T) {
	ex := echo.ExtractIPFromRealIPHeader(
		echo.TrustLoopback(true),
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(false),
	)
	cases := []struct {
		name, remote, xff, xreal, want string
	}{
		{"nginx on loopback sets X-Real-IP", "127.0.0.1:5555", "6.6.6.6", "203.0.113.9", "203.0.113.9"},
		{"spoofed XFF ignored", "127.0.0.1:5555", "6.6.6.6", "", "127.0.0.1"},
		{"direct client cannot spoof X-Real-IP", "198.51.100.7:4444", "", "6.6.6.6", "198.51.100.7"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = tc.remote
		if tc.xff != "" {
			req.Header.Set("X-Forwarded-For", tc.xff)
		}
		if tc.xreal != "" {
			req.Header.Set("X-Real-IP", tc.xreal)
		}
		if got := ex(req); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}
