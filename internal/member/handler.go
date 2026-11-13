package member

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/config"
	"sigardapanel/internal/httpjson"
)

// Handler holds the member API handlers.
type Handler struct {
	store *Store
}

// NewHandler creates a new member handler.
func NewHandler(db *sql.DB) *Handler {
	s := NewStore(db)
	if err := s.Migrate(); err != nil {
		slog.Error("member: migration failed", "error", err)
	}
	return &Handler{store: s}
}

// Register creates a new member account.
func (h *Handler) Register(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	// Member (SaaS customer) sign-up is closed unless the operator opens it.
	if !config.AllowMemberSignup() {
		return c.JSON(http.StatusForbidden, httpjson.ErrorBody{
			Error: httpjson.APIError{
				Code:    "member_signup_disabled",
				Message: "member sign-up is disabled on this panel (SIGARDAPANEL_ALLOW_MEMBER_SIGNUP is not true); ask the panel administrator for an account",
			},
			RequestID: rid,
		})
	}

	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Company  string `json:"company"`
		Plan     string `json:"plan"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	body.Name = strings.TrimSpace(body.Name)
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	body.Company = strings.TrimSpace(body.Company)

	if body.Name == "" || body.Email == "" || body.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name, email, and password are required")
	}
	if len(body.Password) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 8 characters")
	}
	if !strings.Contains(body.Email, "@") {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid email")
	}

	// Check duplicate
	existing, _ := h.store.GetByEmail(body.Email)
	if existing != nil {
		return echo.NewHTTPError(http.StatusConflict, "email already registered")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to hash password")
	}

	// Self sign-up starts on the free plan; paid plans are assigned elsewhere.
	plan := "community"

	m := &Member{
		Name:         body.Name,
		Email:        body.Email,
		PasswordHash: string(hash),
		Company:      body.Company,
		Plan:         plan,
	}
	if err := h.store.Create(m); err != nil {
		slog.Error("member: create", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create account")
	}

	slog.Info("member registered", "member_id", m.ID, "email", m.Email, "plan", m.Plan)

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data: map[string]any{
			"id":      m.ID,
			"name":    m.Name,
			"email":   m.Email,
			"plan":    m.Plan,
			"message": "account created successfully",
		},
		RequestID: rid,
	})
}

// Login authenticates a member and returns a session token.
func (h *Handler) Login(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	body.Email = strings.TrimSpace(strings.ToLower(body.Email))

	// Brute-force limits: per client address and per account.
	ipKey, acctKey := "ip:"+c.RealIP(), "acct:"+body.Email
	if loginFailures.blocked(ipKey, 20) || loginFailures.blocked(acctKey, 10) {
		return echo.NewHTTPError(http.StatusTooManyRequests, "too many failed logins; try again later")
	}

	m, err := h.store.GetByEmail(body.Email)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(body.Password)) // same timing
		loginFailures.fail(ipKey, acctKey)
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(m.PasswordHash), []byte(body.Password)); err != nil {
		loginFailures.fail(ipKey, acctKey)
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}
	if m.Status != "" && m.Status != "active" {
		return echo.NewHTTPError(http.StatusForbidden, "this account is "+m.Status)
	}
	loginFailures.reset(acctKey)

	token, err := auth.NewToken()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create session")
	}

	sess := &Session{
		MemberID:  m.ID,
		TokenHash: auth.HashToken(token),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := h.store.CreateSession(sess); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create session")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"token": token,
			"member": map[string]any{
				"id":      m.ID,
				"name":    m.Name,
				"email":   m.Email,
				"plan":    m.Plan,
				"company": m.Company,
				"status":  m.Status,
			},
		},
		RequestID: rid,
	})
}

// Me returns the current member info.
func (h *Handler) Me(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	memberID := c.Get("member_id").(int64)

	m, err := h.store.GetByID(memberID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "member not found")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"id":      m.ID,
			"name":    m.Name,
			"email":   m.Email,
			"plan":    m.Plan,
			"company": m.Company,
			"status":  m.Status,
		},
		RequestID: rid,
	})
}

// UpdatePlan updates the member's plan.
func (h *Handler) UpdatePlan(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	memberID := c.Get("member_id").(int64)

	var body struct {
		Plan string `json:"plan"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if !validPlans[body.Plan] {
		return echo.NewHTTPError(http.StatusBadRequest, "unknown plan")
	}

	if err := h.store.UpdatePlan(memberID, body.Plan); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update plan")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"plan":    body.Plan,
			"message": "plan updated",
		},
		RequestID: rid,
	})
}
