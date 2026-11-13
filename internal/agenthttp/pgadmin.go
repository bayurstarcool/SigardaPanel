package agenthttp

import (
	"context"
	"fmt"
	"os/user"
	"strconv"
	"strings"
)

// postgresAdmin runs SQL through psql as the "postgres" OS account, which
// PostgreSQL's default peer authentication maps to the superuser role. The
// agent runs as root, and "psql" as root connects as a role named "root" that
// does not exist, so every PostgreSQL task used to fail.
func (r *Router) postgresAdmin(ctx context.Context, db, sql string) (string, error) {
	u, err := user.Lookup("postgres")
	if err != nil {
		return "", fmt.Errorf("PostgreSQL is not installed on this server (no postgres account)")
	}
	uid, err1 := strconv.ParseUint(u.Uid, 10, 32)
	gid, err2 := strconv.ParseUint(u.Gid, 10, 32)
	if err1 != nil || err2 != nil || uid == 0 {
		return "", fmt.Errorf("unexpected postgres account")
	}
	if db == "" {
		db = "postgres"
	}
	return r.executor.RunAsUserInput(ctx, uint32(uid), uint32(gid), nil, strings.NewReader(sql),
		"psql", "-X", "-q", "-v", "ON_ERROR_STOP=1", "-d", db)
}
