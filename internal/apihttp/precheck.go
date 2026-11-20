package apihttp

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"sigardapanel/internal/store"
)

// agentPrecheck asks the server's agent whether what a new site or database
// needs (kind: php, mysql, postgres) is installed, and answers 409 with the
// agent's explanation when it is not. An agent that cannot be asked (too old,
// unreachable) does not block: the job reports the problem then.
func agentPrecheck(ctx context.Context, s *store.Store, serverID int64, kind, version string) error {
	sh := &siteHandler{store: s}
	code, out, err := sh.siteAgentCall(ctx, &store.Site{ServerID: serverID}, http.MethodPost, "/tasks/precheck",
		map[string]any{"kind": kind, "version": version}, 20*time.Second)
	if err != nil || code != http.StatusOK || out == nil {
		return nil
	}
	if ok, _ := out["ok"].(bool); !ok {
		return echo.NewHTTPError(http.StatusConflict, agentMessage(out, "this server is missing software the request needs"))
	}
	return nil
}
