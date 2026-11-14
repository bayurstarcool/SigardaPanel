// Package publicip finds the public IPv4 address of the machine it runs on,
// the address a DNS A record for a site on that machine should point at.
//
// Cloud VMs usually only see a private address on their interfaces (the
// provider maps the public one with 1:1 NAT), so when no interface holds a
// public address, a few well-known "what is my IP" services are asked and
// the most common answer wins.
package publicip

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Result is a detected address and where it came from ("interface eth0",
// "lookup").
type Result struct {
	IP     string `json:"ip"`
	Source string `json:"source"`
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

// IsPublicIPv4 reports whether ip is an IPv4 address reachable from the
// internet (not private, shared, loopback, link-local, documentation,
// benchmark, multicast or reserved).
func IsPublicIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	for _, n := range reservedV4 {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// Lookups are the services asked when no interface has a public address.
// Each answers with the caller's address (Cloudflare's trace as "ip=...").
var Lookups = []string{
	"https://1.1.1.1/cdn-cgi/trace",
	"https://checkip.amazonaws.com",
	"https://api.ipify.org",
	"https://ipv4.icanhazip.com",
}

// interfaceAddrs is replaced in tests.
var interfaceAddrs = func() (map[string][]net.Addr, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := map[string][]net.Addr{}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err == nil {
			out[ifc.Name] = addrs
		}
	}
	return out, nil
}

// Detect returns the machine's public IPv4: a public address on one of its
// interfaces, otherwise the address the lookup services see (the most common
// answer; ties go to the earlier service).
func Detect(ctx context.Context, client *http.Client) (Result, error) {
	if m, err := interfaceAddrs(); err == nil {
		for name, addrs := range m {
			for _, a := range addrs {
				var ip net.IP
				switch v := a.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if IsPublicIPv4(ip) {
					return Result{IP: ip.To4().String(), Source: "interface " + name}, nil
				}
			}
		}
	}
	if client == nil {
		client = &http.Client{Timeout: 6 * time.Second}
	}
	answers := make([]string, len(Lookups))
	var wg sync.WaitGroup
	for i, u := range Lookups {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			answers[i] = ask(ctx, client, u)
		}(i, u)
	}
	wg.Wait()
	counts := map[string]int{}
	best := ""
	for _, a := range answers {
		if a == "" {
			continue
		}
		counts[a]++
		if best == "" || counts[a] > counts[best] {
			best = a
		}
	}
	if best == "" {
		return Result{}, fmt.Errorf("no public IPv4 on the interfaces and the lookup services did not answer")
	}
	return Result{IP: best, Source: "lookup"}, nil
}

// ask returns the public IPv4 one service reports, or "".
func ask(ctx context.Context, client *http.Client, url string) string {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 4096))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		line = strings.TrimPrefix(line, "ip=")
		if ip := net.ParseIP(line); ip != nil && IsPublicIPv4(ip) {
			return ip.To4().String()
		}
	}
	return ""
}
