package apihttp

// App ports: node, python, go and docker sites each get a port in
// 30000-40000 that is unique in the panel's database and free on the
// site's own server (the agent reports what listens there; the panel
// host's ports say nothing about a remote server). Allocation is serialized
// so two sites created at the same time never get the same port.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"sigardapanel/internal/store"
)

const appPortMin, appPortMax = 30000, 40000

// appPortMu serializes "pick a port" + "store the site" (one API process).
var appPortMu sync.Mutex

// serverListeningPorts asks the server's agent which TCP ports are in use.
// ok is false when the agent cannot tell (older agent, unreachable).
var serverListeningPorts = func(ctx context.Context, srv *store.Server) (map[int64]bool, bool) {
	if srv == nil || !srv.AgentURL.Valid || srv.AgentURL.String == "" {
		return nil, false
	}
	code, out, err := stackAgentRequest(ctx, http.MethodGet, agentBase(srv)+"/tasks/system/ports", srv.AgentToken, nil, 10*time.Second)
	if err != nil || code != http.StatusOK {
		return nil, false
	}
	data, _ := out["data"].(map[string]any)
	list, _ := data["ports"].([]any)
	ports := map[int64]bool{}
	for _, p := range list {
		if f, ok := p.(float64); ok {
			ports[int64(f)] = true
		}
	}
	return ports, true
}

// portBusyHere reports whether port is taken on the panel host itself (used
// for the local server when its agent cannot report).
func portBusyHere(port int64) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return true
	}
	ln.Close()
	return false
}

// usedAppPorts returns the ports assigned to sites in the database.
func usedAppPorts(ctx context.Context, s *store.Store) (map[int64]bool, error) {
	rows, err := s.DB().QueryContext(ctx, `SELECT app_port FROM sites WHERE app_port IS NOT NULL AND app_port > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	used := map[int64]bool{}
	for rows.Next() {
		var p int64
		if rows.Scan(&p) == nil {
			used[p] = true
		}
	}
	return used, rows.Err()
}

// allocateAppPort picks a free port for a new app on serverID. The caller
// holds appPortMu until the site row (with the port) is stored.
func (h *siteHandler) allocateAppPort(ctx context.Context, serverID int64) (int64, error) {
	used, err := usedAppPorts(ctx, h.store)
	if err != nil {
		return 0, err
	}
	srv, _ := h.store.Servers.GetByID(ctx, serverID)
	busy, known := serverListeningPorts(ctx, srv)
	local := srv != nil && isLocalServer(*srv)
	for p := int64(appPortMin); p <= appPortMax; p++ {
		if used[p] || busy[p] {
			continue
		}
		if !known && local && portBusyHere(p) {
			continue
		}
		return p, nil
	}
	return 0, fmt.Errorf("no free port in %d-%d on this server", appPortMin, appPortMax)
}

// appPortConflict explains why port cannot be used by site (empty when it
// can): another site has it, or (checkServer) something listens on it on
// the site's server.
func (h *siteHandler) appPortConflict(ctx context.Context, site *store.Site, port int64, checkServer bool) string {
	if port < 1024 || port > 65535 {
		return fmt.Sprintf("port %d is outside 1024-65535", port)
	}
	var otherDomain string
	err := h.store.DB().QueryRowContext(ctx, `SELECT domain FROM sites WHERE app_port = ? AND id != ? LIMIT 1`, port, site.ID).Scan(&otherDomain)
	if err == nil {
		return fmt.Sprintf("port %d is already used by site %s", port, otherDomain)
	}
	if !checkServer {
		return ""
	}
	srv, _ := h.store.Servers.GetByID(ctx, site.ServerID)
	if busy, ok := serverListeningPorts(ctx, srv); ok {
		if busy[port] {
			return fmt.Sprintf("port %d is in use on the server by another service", port)
		}
	} else if srv != nil && isLocalServer(*srv) && portBusyHere(port) {
		return fmt.Sprintf("port %d is in use on the server by another service", port)
	}
	return ""
}

// suggestAppPort finds a free port for site, near start.
func (h *siteHandler) suggestAppPort(ctx context.Context, site *store.Site) int64 {
	appPortMu.Lock()
	defer appPortMu.Unlock()
	p, err := h.allocateAppPort(ctx, site.ServerID)
	if err != nil {
		return 0
	}
	return p
}
