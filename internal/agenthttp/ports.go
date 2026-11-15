package agenthttp

import (
	"bufio"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/httpjson"
)

// procNetRoot is where /proc/net lives (tests point it elsewhere).
var procNetRoot = "/proc/net"

// listeningTCPPorts returns the TCP ports something listens on (any address,
// IPv4 and IPv6), read from /proc/net/tcp{,6} (no ss/netstat needed).
func listeningTCPPorts() []int {
	set := map[int]bool{}
	for _, f := range []string{"tcp", "tcp6"} {
		fh, err := os.Open(procNetRoot + "/" + f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		first := true
		for sc.Scan() {
			if first {
				first = false
				continue
			}
			fields := strings.Fields(sc.Text())
			if len(fields) < 4 || fields[3] != "0A" { // 0A = LISTEN
				continue
			}
			_, portHex, ok := strings.Cut(fields[1], ":")
			if !ok {
				continue
			}
			if p, err := strconv.ParseInt(portHex, 16, 32); err == nil && p > 0 {
				set[int(p)] = true
			}
		}
		fh.Close()
	}
	out := make([]int, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// GET /tasks/system/ports: the TCP ports in use on this server, so the panel
// never gives a site a port another service (or a site of another panel
// database) already listens on.
func (r *Router) listeningPortsTask(c *echo.Context) error {
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"ports": listeningTCPPorts()}})
}
