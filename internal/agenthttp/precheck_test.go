package agenthttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestPrecheckPHP(t *testing.T) {
	run := t.TempDir()
	r := &Router{phpRunDir: run, phpEtcDir: filepath.Join(t.TempDir(), "php")}
	ask := func(version string) map[string]any {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/tasks/precheck", strings.NewReader(`{"kind":"php","version":"`+version+`"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		if err := r.precheckTask(e.NewContext(req, rec)); err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	out := ask("")
	if out["ok"] != false || !strings.Contains(out["message"].(string), "Stack") || strings.Contains(out["message"].(string), "invalid-version") {
		t.Fatalf("no PHP: %v", out)
	}
	os.WriteFile(filepath.Join(run, "php8.3-fpm.sock"), nil, 0o600)
	if out := ask("8.3"); out["ok"] != true {
		t.Fatalf("PHP 8.3 installed: %v", out)
	}
	if out := ask("8.1"); out["ok"] != false {
		t.Fatalf("PHP 8.1 missing: %v", out)
	}
}
