package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPanelUpdateStatusTable(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system/panel-update" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"current_version": "1.6.2", "update_available": true, "auto_update": true,
			"auto_update_window": map[string]any{"start": 2, "end": 5},
			"latest":             map[string]any{"tag": "v1.7.0"},
			"status":             map[string]any{"state": "idle", "supported": true},
		}})
	})
	var out, errb bytes.Buffer
	if code := New(&out, &errb, strings.NewReader("")).Run([]string{"panel-update"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	for _, want := range []string{"1.6.2", "v1.7.0", "update available", "between 02:00 and 05:00"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestPanelUpdateApplyFollowsUntilNewVersion(t *testing.T) {
	orig := panelUpdatePoll
	panelUpdatePoll = time.Millisecond
	t.Cleanup(func() { panelUpdatePoll = orig })

	var started, polls atomic.Int32
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			io.Copy(io.Discard, r.Body)
			started.Add(1)
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{"data":{"tag":"v1.7.0"}}`))
		case started.Load() == 0:
			w.Write([]byte(`{"data":{"current_version":"1.6.2","update_available":true,"latest":{"tag":"v1.7.0"}}}`))
		default:
			n := polls.Add(1)
			switch {
			case n == 1:
				w.WriteHeader(http.StatusBadGateway) // restarting
			case n < 4:
				w.Write([]byte(`{"data":{"current_version":"1.6.2","status":{"state":"running","log":"Downloading v1.7.0\n"}}}`))
			default:
				w.Write([]byte(`{"data":{"current_version":"1.7.0","status":{"state":"succeeded","log":"Downloading v1.7.0\nUpdated to 1.7.0\n"}}}`))
			}
		}
	})
	var out, errb bytes.Buffer
	code := New(&out, &errb, strings.NewReader("")).Run([]string{"panel-update", "apply", "--yes"})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if started.Load() != 1 || strings.Count(out.String(), "Downloading v1.7.0") != 1 || !strings.Contains(out.String(), "now 1.7.0") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestPanelUpdateApplyUpToDate(t *testing.T) {
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("must not start an update: %s", r.Method)
		}
		w.Write([]byte(`{"data":{"current_version":"1.7.0","update_available":false,"latest":{"tag":"v1.7.0"}}}`))
	})
	var out, errb bytes.Buffer
	if code := New(&out, &errb, strings.NewReader("")).Run([]string{"panel-update", "apply", "--yes"}); code != 0 || !strings.Contains(out.String(), "Already on the latest") {
		t.Fatalf("exit %d, out=%s err=%s", code, out.String(), errb.String())
	}
}

func TestPanelUpdateAutoUsage(t *testing.T) {
	var got map[string]any
	setupMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"data":{}}`))
	})
	var out, errb bytes.Buffer
	if code := New(&out, &errb, strings.NewReader("")).Run([]string{"panel-update", "auto", "maybe"}); code != 2 {
		t.Fatalf("bad argument exit = %d", code)
	}
	if code := New(&out, &errb, strings.NewReader("")).Run([]string{"panel-update", "auto", "--window", "2-5", "on"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if got["auto_update"] != true || got["auto_update_window"] == nil {
		t.Fatalf("sent %v", got)
	}
}
