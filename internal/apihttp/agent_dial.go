package apihttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"sigardapanel/internal/config"
)

// agentTransport carries every panel→agent request. It re-checks the
// address at connection time: a hostname validated when the server was
// saved may resolve to the panel host or a cloud metadata service later,
// and the panel sends the server's token there and returns the answers.
// Literal loopback (the panel's own agent) stays allowed.
var agentTransport = &http.Transport{
	Proxy:                 nil,
	DialContext:           dialAgent,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          50,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: time.Second,
}

// agentClient returns an HTTP client for agent requests.
func agentClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: agentTransport}
}

var agentDialer = &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}

func dialAgent(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := agentIPAllowed(ip, true); err != nil {
			return nil, err
		}
		return agentDialer.DialContext(ctx, network, addr)
	}
	if strings.EqualFold(host, "localhost") {
		return agentDialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error = fmt.Errorf("agent host %s has no usable address", host)
	for _, a := range ips {
		if err := agentIPAllowed(a.IP, false); err != nil {
			lastErr = err
			continue
		}
		conn, err := agentDialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// agentIPAllowed: never link-local (metadata), multicast or unspecified;
// loopback only when written literally; private only when allowed.
func agentIPAllowed(ip net.IP, literal bool) error {
	switch {
	case ip.IsLoopback():
		if !literal {
			return fmt.Errorf("agent host resolves to this panel host (%s)", ip)
		}
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified():
		return fmt.Errorf("agent address %s is not allowed", ip)
	case ip.IsPrivate() && !literal && !config.AllowPrivateAgents():
		return fmt.Errorf("agent host resolves to a private address (%s); set SIGARDAPANEL_ALLOW_PRIVATE_AGENTS=true to allow it", ip)
	}
	return nil
}
