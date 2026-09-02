package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestPrompt(t *testing.T) {
	stdin := strings.NewReader("hello world\n")
	var stdout bytes.Buffer

	result, err := Prompt("enter value", stdin, &stdout)
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}
	if result != "hello world" {
		t.Fatalf("expected %q, got %q", "hello world", result)
	}
	if !strings.Contains(stdout.String(), "enter value:") {
		t.Fatalf("expected prompt label, got %q", stdout.String())
	}
}

func TestPromptTrimSpace(t *testing.T) {
	stdin := strings.NewReader("  trimmed  \n")
	var stdout bytes.Buffer

	result, err := Prompt("value", stdin, &stdout)
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}
	if result != "trimmed" {
		t.Fatalf("expected %q, got %q", "trimmed", result)
	}
}

func TestPromptEOF(t *testing.T) {
	stdin := strings.NewReader("")
	var stdout bytes.Buffer

	_, err := Prompt("value", stdin, &stdout)
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestPromptPasswordPipe(t *testing.T) {
	stdin := strings.NewReader("secret123\n")
	var stderr bytes.Buffer

	result, err := PromptPassword("password", stdin, &stderr)
	if err != nil {
		t.Fatalf("PromptPassword error: %v", err)
	}
	if result != "secret123" {
		t.Fatalf("expected %q, got %q", "secret123", result)
	}
	if !strings.Contains(stderr.String(), "password:") {
		t.Fatalf("expected prompt label, got %q", stderr.String())
	}
	// Should print newline after input.
	if !strings.Contains(stderr.String(), "\n") {
		t.Fatalf("expected newline in stderr, got %q", stderr.String())
	}
}

func TestPromptPasswordPipeTrim(t *testing.T) {
	stdin := strings.NewReader("  secret  \n")
	var stderr bytes.Buffer

	result, err := PromptPassword("password", stdin, &stderr)
	if err != nil {
		t.Fatalf("PromptPassword error: %v", err)
	}
	if result != "secret" {
		t.Fatalf("expected %q, got %q", "secret", result)
	}
}

func TestPromptPasswordPipeEOF(t *testing.T) {
	stdin := strings.NewReader("")
	var stderr bytes.Buffer

	_, err := PromptPassword("password", stdin, &stderr)
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}
