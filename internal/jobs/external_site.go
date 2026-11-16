package jobs

import (
	"context"

	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// External sites (managed_by "external") are served by something the panel
// does not own: a vhost written by hand, or one application shared by several
// domains (a multi-site CMS). The panel lists them, shows their status and
// backs them up, but never changes them on the server: it does not rewrite
// their vhost, move them to a PHP-FPM pool, renew or issue their certificate,
// suspend, resume, deploy, transfer or delete them. Removing one from the
// panel removes only the panel record (see the site delete handler).

// externalSiteBlocked lists the jobs that would change an external site on the
// server.
var externalSiteBlocked = map[string]bool{
	jobmodel.TypeSiteCreate:   true, // also every vhost regeneration
	jobmodel.TypeSiteDelete:   true,
	jobmodel.TypeSiteDeploy:   true,
	jobmodel.TypeSiteSuspend:  true,
	jobmodel.TypeSiteResume:   true,
	jobmodel.TypeSSLIssue:     true,
	jobmodel.TypeSSLRenew:     true,
	jobmodel.TypeSSLAutoRenew: true,
	jobmodel.TypeSiteTransfer: true,
}

// ExternalSiteMessage explains a refused job.
const ExternalSiteMessage = "external site: the panel does not change it on the server (vhost, files and certificate are managed outside the panel)"

// IsExternalSite reports whether the site is managed outside the panel.
func IsExternalSite(site *store.Site) bool {
	return site != nil && site.ManagedBy.Valid && site.ManagedBy.String == "external"
}

// externalSiteRefuses reports whether the job must not reach the agent
// because it would change an external site.
func externalSiteRefuses(ctx context.Context, s *store.Store, job *jobmodel.Job) bool {
	if job == nil || !job.SiteID.Valid || !externalSiteBlocked[job.Type] {
		return false
	}
	site, err := s.Sites.GetByID(ctx, job.SiteID.Int64)
	if err != nil {
		return false
	}
	return IsExternalSite(site)
}
