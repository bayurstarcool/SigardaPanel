package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSiteList_WithSites(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"sites": []map[string]any{
					{"id": 1, "name": "alpha", "domain": "a.test", "runtime": "static", "status": "active"},
					{"id": 2, "name": "beta", "domain": "b.test", "runtime": "php", "status": "pending"},
				},
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"site", "list"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "alpha") || !strings.Contains(output, "beta") {
		t.Errorf("missing sites: %s", output)
	}
}

func TestSiteList_FilterByServer(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query().Get("server_id")
		if q != "42" {
			t.Errorf("expected server_id=42, got %s", q)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"sites": []map[string]any{}},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"site", "list", "--server", "42"})
	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
}

func TestSiteCreate_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites" || r.Method != "POST" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "test" {
			t.Errorf("expected name=test, got %v", body["name"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"site": map[string]any{"id": 7, "domain": "test.example.com", "status": "pending"},
				"job":  map[string]any{"id": 99, "type": "site.create", "status": "queued"},
			},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"site", "create", "--server", "1", "--name", "test", "--domain", "test.example.com"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "job_id: 99") || !strings.Contains(stdout.String(), "id=7") {
		t.Errorf("expected job_id in output: %s", stdout.String())
	}
}

func TestSiteCreate_MissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"site", "create", "--server", "1", "--name", "test"})
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestSiteDelete_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/3" || r.Method != "DELETE" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"deleted": true}})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"site", "delete", "3"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "deleted") {
		t.Errorf("expected deleted text: %s", stdout.String())
	}
}

func TestSiteDeploy_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/5/deploy" || r.Method != "POST" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"job_id": 21}})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"site", "deploy", "5"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "job_id=21") {
		t.Errorf("expected job_id in output: %s", stdout.String())
	}
}

func TestSiteConfig_Success(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/7/config" || r.Method != "POST" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["git_repo"] != "https://github.com/user/repo.git" {
			t.Errorf("expected git_repo, got %v", body["git_repo"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"ok": true, "webhook_secret": "abcd1234"},
		})
	})

	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"site", "config", "--id", "7", "--git-repo", "https://github.com/user/repo.git"})

	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "site 7 updated") || !strings.Contains(stdout.String(), "abcd1234") {
		t.Errorf("expected update confirmation in output: %s", stdout.String())
	}
}

func TestSiteGet_TableAndYAML(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/4" || r.Method != "GET" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"id": 4, "domain": "four.test", "status": "suspended", "ssl_status": "active",
		}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"site", "show", "4"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status:") || !strings.Contains(stdout.String(), "suspended") {
		t.Errorf("table output: %s", stdout.String())
	}
	stdout.Reset()
	if code := app.Run([]string{"site", "get", "-o", "yaml", "4"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "domain: four.test\n") {
		t.Errorf("yaml output: %s", stdout.String())
	}
}

func TestSiteSuspendResume(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		switch r.URL.Path {
		case "/api/v1/sites/5/suspend":
			w.WriteHeader(202)
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"job_id": 31, "status": "queued"}})
		case "/api/v1/sites/5/resume":
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "conflict", "message": `site is not suspended (status is "active")`}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"site", "suspend", "5"}); code != 0 {
		t.Fatalf("suspend exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "job_id=31") {
		t.Errorf("suspend output: %s", stdout.String())
	}
	if code := app.Run([]string{"site", "resume", "--id", "5"}); code != 1 {
		t.Fatalf("resume conflict: expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "409") || !strings.Contains(stderr.String(), "site is not suspended") {
		t.Errorf("expected API message on stderr, got %s", stderr.String())
	}
}

func TestSiteList_FiltersAndJSON(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("status") != "suspended" || q.Get("search") != "shop" || q.Get("limit") != "5" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"sites": []map[string]any{{"id": 1, "domain": "shop.test", "status": "suspended"}}, "total_count": 1,
		}})
	})
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	code := app.Run([]string{"site", "list", "--status", "suspended", "--search", "shop", "--limit", "5", "--output", "json"})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || len(got) != 1 {
		t.Fatalf("expected JSON array, got %q (%v)", stdout.String(), err)
	}
}

func TestSiteList_InvalidOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"site", "list", "-o", "xml"}); code != 2 {
		t.Fatalf("expected 2, got %d", code)
	}
}

func TestSiteUpdate_NothingToUpdate(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"site", "update", "3"}); code != 2 {
		t.Fatalf("expected 2, got %d", code)
	}
	// Renaming is not supported by the API: the flag no longer exists.
	if code := app.Run([]string{"site", "update", "3", "--name", "x"}); code != 2 {
		t.Fatalf("expected 2 for --name, got %d", code)
	}
}

func TestSiteSubcommandHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := New(&stdout, &stderr, strings.NewReader(""))
	if code := app.Run([]string{"site", "create", "--help"}); code != 0 {
		t.Fatalf("expected 0 for --help, got %d", code)
	}
	if !strings.Contains(stdout.String(), "usage: sigardapanel site create") || !strings.Contains(stdout.String(), "-domain") {
		t.Errorf("help output: %s", stdout.String())
	}
}
