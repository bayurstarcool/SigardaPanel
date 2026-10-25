package apihttp

import (
	"bytes"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"sigardapanel/internal/store"
)

// Regression: the multipart filename is client-controlled and must never
// escape the site root, whichever separator style it carries. The handler
// may sanitize-and-accept (basename only) or reject, but a file must never
// appear outside the root.
func TestFileManagerUploadBlocksTraversalFilenames(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newUserToken(t, s, ctx)
	regular, _ := s.Users.GetByUsername(ctx, "regular")

	srv := &store.Server{Name: "s", Hostname: "h", Status: "active", AgentToken: "tok"}
	if err := s.Servers.Create(ctx, srv); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	site := &store.Site{ServerID: srv.ID, Name: "t", Domain: "t.example.com", Runtime: "static", RootPath: root, OwnerID: sql.NullInt64{Int64: regular.ID, Valid: true}}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}

	upload := func(filename string) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		mw.WriteField("path", ".")
		fw, _ := mw.CreateFormFile("file", filename)
		fw.Write([]byte("pwn"))
		mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+itoa(site.ID)+"/files/upload", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	outside := filepath.Join(filepath.Dir(root), "pwn.txt")
	for _, name := range []string{"../../pwn.txt", "a/../../pwn.txt", `..\..\pwn.txt`, "sub/../../../pwn.txt"} {
		rec := upload(name)
		if rec.Code == http.StatusInternalServerError {
			t.Errorf("upload %q: unexpected server error: %s", name, rec.Body.String())
		}
		if _, err := os.Stat(outside); err == nil {
			t.Fatalf("upload %q: file escaped the site root", name)
		}
	}

	// A plain filename still works and lands inside the root.
	if rec := upload("hello.txt"); rec.Code != http.StatusOK {
		t.Fatalf("normal upload failed: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "hello.txt")); err != nil {
		t.Fatal("uploaded file missing inside site root")
	}
}
