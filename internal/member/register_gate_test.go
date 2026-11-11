package member

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func registerRequest(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/member/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := h.Register(e.NewContext(req, rec)); err != nil {
		e.HTTPErrorHandler(err, e.NewContext(req, rec))
	}
	return rec
}

func TestRegister_ClosedByDefault(t *testing.T) {
	t.Setenv("SIGARDAPANEL_ALLOW_MEMBER_SIGNUP", "")
	// No store: a closed sign-up must answer before touching the database.
	rec := registerRequest(t, &Handler{}, `{"name":"a","email":"a@b.c","password":"longenough"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "member_signup_disabled") ||
		!strings.Contains(rec.Body.String(), "SIGARDAPANEL_ALLOW_MEMBER_SIGNUP") {
		t.Fatalf("expected a clear error, got %s", rec.Body.String())
	}
}

func TestRegister_OpenWhenAllowed(t *testing.T) {
	t.Setenv("SIGARDAPANEL_ALLOW_MEMBER_SIGNUP", "true")
	// Past the gate: request validation runs (and rejects the empty body).
	rec := registerRequest(t, &Handler{}, `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from validation, got %d: %s", rec.Code, rec.Body.String())
	}
}
