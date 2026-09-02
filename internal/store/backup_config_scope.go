package store

import "context"

// ResolveBackupConfigSiteIDs returns explicit site IDs when configured. For
// database/full configs, an empty site_ids means all active sites so newly
// created sites are covered automatically. File-only configs remain explicit.
func (s *Store) ResolveBackupConfigSiteIDs(ctx context.Context, cfg BackupConfig) ([]int64, error) {
	if len(cfg.SiteIDs) > 0 {
		return append([]int64(nil), cfg.SiteIDs...), nil
	}
	if cfg.ScopeType != "database" && cfg.ScopeType != "both" {
		return nil, nil
	}
	sites, err := s.Sites.List(ctx, SiteListFilter{Limit: 10000})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(sites))
	for _, site := range sites {
		if site.Status == "active" {
			ids = append(ids, site.ID)
		}
	}
	return ids, nil
}
