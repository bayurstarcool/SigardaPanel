package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, []string{"ID", "NAME"}, [][]string{
		{"1", "alice"},
		{"2", "bob"},
	})
	output := buf.String()
	if !strings.Contains(output, "ID") || !strings.Contains(output, "NAME") {
		t.Errorf("missing headers: %s", output)
	}
	if !strings.Contains(output, "alice") || !strings.Contains(output, "bob") {
		t.Errorf("missing rows: %s", output)
	}
}

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}
	if err := PrintJSON(&buf, data); err != nil {
		t.Fatalf("print json: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if got["key"] != "value" {
		t.Errorf("expected key=value, got %v", got)
	}
}

func TestGetOutputMode_Default(t *testing.T) {
	os.Unsetenv("SIGARDAPANEL_OUTPUT")
	if GetOutputMode() != OutputTable {
		t.Errorf("expected default table, got %s", GetOutputMode())
	}
}

func TestGetOutputMode_FromEnv(t *testing.T) {
	os.Setenv("SIGARDAPANEL_OUTPUT", "json")
	defer os.Unsetenv("SIGARDAPANEL_OUTPUT")
	if GetOutputMode() != OutputJSON {
		t.Errorf("expected json, got %s", GetOutputMode())
	}
}