package jobs

import (
	"strings"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/jobmodel"
)

// permanentSSLErrorMarkers are certbot / ACME failures that will fail the same
// way on an immediate retry: the DNS zone is not in the configured provider
// account, the certificate lineage does not exist, the name does not resolve,
// or Let's Encrypt rate limits (which last hours to a week). Retrying them
// three times only spams the CA and the alert channels.
var permanentSSLErrorMarkers = []string{
	"unable to determine zone_id",
	"unable to determine zone identifier",
	"no such certificate",
	"no certificate found",
	"certificate not found",
	"lineage not found",
	"not found in renewal",
	"nxdomain",
	"non-existent domain",
	"no valid ip addresses found",
	"too many certificates",
	"too many failed authorizations",
	"too many new orders",
	"rejectedidentifier",
	"policy forbids issuing",
}

// permanentSiteCreateErrorMarkers are site.create preconditions the agent
// checks before touching anything (e.g. the requested PHP-FPM is missing on
// the server); a retry fails the same way until the operator acts.
var permanentSiteCreateErrorMarkers = []string{
	"is not installed on this server",
}

// isPermanentJobError reports whether a failed job should be marked failed
// immediately instead of being retried. SSL jobs and site.create
// preconditions are classified.
func isPermanentJobError(jobType string, execErr error, result *agentclient.TaskResult) bool {
	markers := permanentSSLErrorMarkers
	switch jobType {
	case jobmodel.TypeSSLRenew, jobmodel.TypeSSLIssue:
	case jobmodel.TypeSiteCreate:
		markers = permanentSiteCreateErrorMarkers
	case jobmodel.TypeDatabaseCreate, jobmodel.TypeDBUserCreate:
		markers = []string{"already exists on the server", "reserved database name", "database record no longer exists"}
	default:
		return false
	}
	var b strings.Builder
	if execErr != nil {
		b.WriteString(execErr.Error())
	}
	if result != nil {
		b.WriteString("\n")
		b.WriteString(result.Error)
		b.WriteString("\n")
		b.WriteString(result.Output)
	}
	msg := strings.ToLower(b.String())
	for _, m := range markers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}
