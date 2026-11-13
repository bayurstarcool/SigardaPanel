package apihttp

import (
	"net/http"
	"strconv"
	"testing"

	"sigardapanel/internal/auth"
)

func TestSiteEndpointPermissions(t *testing.T) {
	f := newAuthzFixture(t)
	owner, ownerTok := f.user("owner", auth.RoleUser)
	reader, readerTok := f.user("reader", auth.RoleAdmin) // admin role, read-only assignment
	_, strangerTok := f.user("stranger", auth.RoleUser)
	st := f.site(owner, "perms", "node")
	f.assign(reader, st, "read")
	id := strconv.FormatInt(st.ID, 10)

	// git rollback: read assignees are refused, an empty body is a 400 (no panic).
	if rec := f.do("POST", "/api/v1/sites/"+id+"/git/rollback", map[string]any{"commit": "abc123"}, readerTok); rec.Code != http.StatusNotFound {
		t.Errorf("read assignee rollback = %d", rec.Code)
	}
	if rec := f.do("POST", "/api/v1/sites/"+id+"/git/rollback", nil, ownerTok); rec.Code != http.StatusBadRequest {
		t.Errorf("owner rollback without commit = %d %s", rec.Code, rec.Body.String())
	}
	// app resource usage / limits are scoped to the site.
	if rec := f.do("GET", "/api/v1/sites/"+id+"/app/resource-usage", nil, strangerTok); rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Errorf("stranger resource usage = %d", rec.Code)
	}
	if rec := f.do("PUT", "/api/v1/sites/"+id+"/app/resource-limits", map[string]any{"memory_max": 256}, readerTok); rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Errorf("read assignee resource limits = %d", rec.Code)
	}
	if rec := f.do("PUT", "/api/v1/sites/"+id+"/app/resource-limits", map[string]any{"memory_max": 256}, ownerTok); rec.Code != http.StatusOK {
		t.Errorf("owner resource limits = %d %s", rec.Code, rec.Body.String())
	}
	// vhost validate swaps the live vhost: read assignees are refused.
	if rec := f.do("POST", "/api/v1/sites/"+id+"/vhost/validate", map[string]any{"vhost_config": "server {}"}, readerTok); rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Errorf("read assignee vhost validate = %d", rec.Code)
	}
}
