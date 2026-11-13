package apihttp

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigardapanel/internal/agenthttp"
	"sigardapanel/internal/store"
)

// fileManagerFixture: an API whose site lives on a real (test) agent, so the
// file manager is exercised end to end: API permission checks → agent →
// filesystem.
func fileManagerFixture(t *testing.T) (http.Handler, string, *store.Site, string) {
	t.Helper()
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newUserToken(t, s, ctx)
	regular, _ := s.Users.GetByUsername(ctx, "regular")
	sitesDir := t.TempDir()
	agent := httptest.NewServer(agenthttp.NewRouterWithOptions(agenthttp.RouterOptions{SiteRootDir: sitesDir}))
	t.Cleanup(agent.Close)
	srv := &store.Server{Name: "s", Hostname: "h", Status: "active", AgentToken: "tok", AgentURL: sql.NullString{String: agent.URL, Valid: true}}
	if err := s.Servers.Create(ctx, srv); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(sitesDir, "t.example.com")
	os.MkdirAll(root, 0o755)
	site := &store.Site{ServerID: srv.ID, Name: "t", Domain: "t.example.com", Runtime: "static", RootPath: root, OwnerID: sql.NullInt64{Int64: regular.ID, Valid: true}}
	if err := s.Sites.Create(ctx, site); err != nil {
		t.Fatal(err)
	}
	return e, token, site, root
}

func TestFileManagerUploadBlocksTraversalFilenames(t *testing.T) {
	e, token, site, root := fileManagerFixture(t)
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
	if rec := upload("hello.txt"); rec.Code != http.StatusOK {
		t.Fatalf("normal upload failed: %d %s", rec.Code, rec.Body.String())
	}
	if got, err := os.ReadFile(filepath.Join(root, "hello.txt")); err != nil || string(got) != "pwn" {
		t.Fatal("uploaded file missing inside site root")
	}
}

func TestFileManagerOperationsGoThroughTheAgent(t *testing.T) {
	e, token, site, root := fileManagerFixture(t)
	id := itoa(site.ID)
	call := func(method, path string, body any) (int, string) {
		var r *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		} else {
			r = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, r)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	steps := []struct {
		method, path string
		body         any
		want         int
	}{
		{"POST", "/api/v1/sites/" + id + "/files/create", map[string]any{"path": "app", "is_dir": true}, 201},
		{"POST", "/api/v1/sites/" + id + "/files/write", map[string]any{"path": "app/index.php", "content": "<?php echo 1;"}, 200},
		{"POST", "/api/v1/sites/" + id + "/files/create", map[string]any{"path": "app/index.php"}, 409},
		{"POST", "/api/v1/sites/" + id + "/files/rename", map[string]any{"old_path": "app/index.php", "new_name": "main.php"}, 200},
		{"POST", "/api/v1/sites/" + id + "/files/compress", map[string]any{"paths": []string{"app"}, "name": "app", "dest": "."}, 200},
		{"POST", "/api/v1/sites/" + id + "/files/create", map[string]any{"path": "dest", "is_dir": true}, 201},
		{"POST", "/api/v1/sites/" + id + "/files/move", map[string]any{"paths": []string{"app.zip"}, "dest": "dest"}, 200},
		{"POST", "/api/v1/sites/" + id + "/files/write", map[string]any{"path": "../escape.txt", "content": "x"}, 400},
		{"DELETE", "/api/v1/sites/" + id + "/files?path=.", nil, 400},
	}
	for _, st := range steps {
		if code, body := call(st.method, st.path, st.body); code != st.want {
			t.Fatalf("%s %s = %d, want %d: %s", st.method, st.path, code, st.want, body)
		}
	}
	code, body := call("GET", "/api/v1/sites/"+id+"/files/read?path=app/main.php", nil)
	if code != 200 || !strings.Contains(body, "echo 1") {
		t.Fatalf("read = %d %s", code, body)
	}
	code, body = call("GET", "/api/v1/sites/"+id+"/files?path=dest", nil)
	if code != 200 || !strings.Contains(body, "app.zip") {
		t.Fatalf("list = %d %s", code, body)
	}
	code, body = call("GET", "/api/v1/sites/"+id+"/files/download?path=app/main.php", nil)
	if code != 200 || body != "<?php echo 1;" {
		t.Fatalf("download = %d %q", code, body)
	}
	if code, _ := call("DELETE", "/api/v1/sites/"+id+"/files?path=dest", nil); code != 200 {
		t.Fatalf("delete = %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, "dest")); err == nil {
		t.Fatal("deleted folder still exists")
	}
}
