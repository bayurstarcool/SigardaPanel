package jobs

import (
	"context"
	"encoding/json"

	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// External databases (managed_by "external") were created outside the panel
// and imported by a super admin. The panel lists them and uses them with the
// credentials it was given (viewer, manager, backups, phpMyAdmin), but never
// changes them on the server: it does not create, drop, alter or re-grant
// them, and never creates or drops their accounts. Removing one from the
// panel removes only the panel records (see the database delete handler).

// externalDatabaseBlocked lists the jobs that would change an external
// database (or one of its accounts) on the server.
var externalDatabaseBlocked = map[string]bool{
	jobmodel.TypeDatabaseCreate: true,
	jobmodel.TypeDatabaseDelete: true,
	jobmodel.TypeDBUserCreate:   true, // also every password rotation
	jobmodel.TypeDBUserDelete:   true,
}

// ExternalDatabaseMessage explains a refused job.
const ExternalDatabaseMessage = "external database: the panel does not change it on the server (it was imported; manage it where it was created)"

// externalDatabaseRefuses reports whether the job must not reach the agent
// because it would change an external database. The record is looked up by
// id, and (the record may already be gone when the job runs) by name and
// engine among the external databases of the job's server.
func externalDatabaseRefuses(ctx context.Context, s *store.Store, job *jobmodel.Job) bool {
	if job == nil || !externalDatabaseBlocked[job.Type] {
		return false
	}
	var p struct {
		DatabaseID   int64  `json:"database_id"`
		Name         string `json:"name"`
		DatabaseName string `json:"database_name"`
		Type         string `json:"type"`
	}
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return false
	}
	if p.DatabaseID > 0 {
		if d, err := s.Databases.GetByID(ctx, p.DatabaseID); err == nil && d.IsExternal() {
			return true
		}
	}
	name := p.DatabaseName
	if name == "" {
		name = p.Name
	}
	if name == "" {
		return false
	}
	all, err := s.Databases.ListAll(ctx)
	if err != nil {
		return false
	}
	for i := range all {
		d := &all[i]
		if !d.IsExternal() || d.Name != name || (p.Type != "" && d.Type != p.Type) {
			continue
		}
		if !job.ServerID.Valid {
			return true
		}
		if site, err := s.Sites.GetByID(ctx, d.SiteID); err == nil && site != nil && site.ServerID == job.ServerID.Int64 {
			return true
		}
	}
	return false
}
