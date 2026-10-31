package apihttp

import (
	"encoding/json"

	"sigardapanel/internal/store"
)

// deployPayload builds the site.deploy job payload the agent expects. Manual
// deploys and git webhooks must send the same shape: plain values (not
// sql.Null* structs, which marshal as {"String":…,"Valid":…} and made the
// agent reject the payload) and the root_path the agent validates.
func deployPayload(site *store.Site, branch string) string {
	b, _ := json.Marshal(map[string]any{
		"site_id":        site.ID,
		"domain":         site.Domain,
		"root_path":      site.RootPath,
		"repo":           site.GitRepo.String,
		"branch":         branch,
		"runtime":        site.Runtime,
		"app_status":     site.AppStatus.String,
		"app_port":       site.AppPort.Int64,
		"app_command":    site.AppCommand.String,
		"system_user":    site.SystemUser.String,
		"app_memory_max": site.AppMemoryMax.Int64,
	})
	return string(b)
}
