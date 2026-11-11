package apihttp

import (
	"net/http"
	"testing"
)

func TestErrorCodeFromStatus(t *testing.T) {
	for status, want := range map[int]string{
		http.StatusConflict:            "conflict",
		http.StatusNotFound:            "not_found",
		http.StatusMethodNotAllowed:    "method_not_allowed",
		http.StatusBadGateway:          "bad_gateway",
		http.StatusInternalServerError: "internal_error",
	} {
		if got := errorCodeFromStatus(status); got != want {
			t.Errorf("errorCodeFromStatus(%d) = %q, want %q", status, got, want)
		}
	}
}
