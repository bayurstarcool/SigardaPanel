package apihttp

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// Deleting an external site (managed outside the panel, e.g. one domain of a
// multi-site CMS) removes only the panel record: no agent job (files, vhost,
// certificate stay), its databases are not dropped, even without
// ?delete_files=false.
func TestDeleteExternalSiteRemovesOnlyTheRecord(t *testing.T) {
	f := newAuthzFixture(t)
	owner, tok := f.user("owner", auth.RoleUser)
	st := &store.Site{
		ServerID: f.serverID, Name: "cms", Domain: "cms.example.com", Runtime: "php",
		RootPath:  "/home/owner/htdocs/cms.example.com",
		OwnerID:   sql.NullInt64{Int64: owner.ID, Valid: true},
		ManagedBy: sql.NullString{String: "external", Valid: true},
	}
	if err := f.s.Sites.Create(f.ctx, st); err != nil {
		t.Fatal(err)
	}
	db := &store.Database{SiteID: st.ID, Name: "cmsdb", Type: "mysql"}
	if err := f.s.Databases.Create(f.ctx, db); err != nil {
		t.Fatal(err)
	}

	rec := f.do("DELETE", "/api/v1/sites/"+strconv.FormatInt(st.ID, 10), nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Data["external"] != true || out.Data["files_deleted"] != false || out.Data["job_id"] != nil {
		t.Fatalf("response = %v", out.Data)
	}
	if got, _ := f.s.Sites.GetByID(f.ctx, st.ID); got != nil {
		t.Fatal("site record still there")
	}
	jobs, _ := f.s.Jobs.List(f.ctx, store.JobListFilter{})
	for _, j := range jobs {
		if j.Type == jobmodel.TypeSiteDelete || j.Type == jobmodel.TypeDatabaseDelete {
			t.Fatalf("external site delete queued %s", j.Type)
		}
	}
}
