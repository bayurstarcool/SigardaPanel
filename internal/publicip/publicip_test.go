package publicip

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func noInterfaces(t *testing.T, addrs map[string][]net.Addr) {
	prev := interfaceAddrs
	interfaceAddrs = func() (map[string][]net.Addr, error) { return addrs, nil }
	t.Cleanup(func() { interfaceAddrs = prev })
}

func lookups(t *testing.T, bodies ...string) {
	prev := Lookups
	Lookups = nil
	for _, b := range bodies {
		b := b
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if b == "500" {
				w.WriteHeader(500)
				return
			}
			fmt.Fprint(w, b)
		}))
		t.Cleanup(ts.Close)
		Lookups = append(Lookups, ts.URL)
	}
	t.Cleanup(func() { Lookups = prev })
}

func TestDetectPrefersPublicInterface(t *testing.T) {
	noInterfaces(t, map[string][]net.Addr{"eth0": {&net.IPNet{IP: net.ParseIP("10.0.0.4"), Mask: net.CIDRMask(8, 32)}, &net.IPNet{IP: net.ParseIP("43.173.51.154"), Mask: net.CIDRMask(24, 32)}}})
	lookups(t, "8.8.4.4")
	r, err := Detect(context.Background(), nil)
	if err != nil || r.IP != "43.173.51.154" || r.Source != "interface eth0" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestDetectBehindNATUsesMajority(t *testing.T) {
	noInterfaces(t, map[string][]net.Addr{"eth0": {&net.IPNet{IP: net.ParseIP("10.0.0.4"), Mask: net.CIDRMask(8, 32)}}})
	lookups(t, "fl=1\nip=43.173.51.154\nts=1", "500", "9.9.9.9\n", "43.173.51.154\n")
	r, err := Detect(context.Background(), nil)
	if err != nil || r.IP != "43.173.51.154" || r.Source != "lookup" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestDetectRejectsPrivateAnswers(t *testing.T) {
	noInterfaces(t, nil)
	lookups(t, "192.168.1.1", "<html>", "500")
	if r, err := Detect(context.Background(), nil); err == nil {
		t.Fatalf("got %+v", r)
	}
}
