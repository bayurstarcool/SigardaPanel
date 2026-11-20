package apihttp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"

	"sigardapanel/internal/features"
	"sigardapanel/internal/store"
)

// siteCount is the number of sites on the panel (for the license card).
func siteCount(ctx context.Context, s *store.Store) int {
	n, err := s.Sites.Count(ctx, store.SiteListFilter{})
	if err != nil {
		return 0
	}
	return n
}

// serverLimitError refuses one more server when the license's server count
// is used up. Free covers one server (the panel host itself); every feature
// works there. Existing servers are never touched by the limit.
func serverLimitError(ctx context.Context, s *store.Store) error {
	eff := currentLicense(ctx, s)
	max := eff.MaxServers
	if max < 0 {
		return nil // unlimited
	}
	servers, err := s.Servers.List(ctx)
	if err != nil || len(servers) < max {
		return nil
	}
	name := features.TierDescriptions[eff.Tier]
	if name == "" {
		name = eff.Tier
	}
	plural := "servers"
	if max == 1 {
		plural = "server"
	}
	return echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf(
		"your %s license covers %d %s and %d are in use: upgrade the plan under Settings -> License to add more servers", name, max, plural, len(servers)))
}
