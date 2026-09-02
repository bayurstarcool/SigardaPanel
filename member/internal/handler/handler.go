package handler

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"member/internal/store"
)

type Handler struct {
	store *store.Store
}

func New(s *store.Store) *Handler {
	return &Handler{store: s}
}

func (h *Handler) Register(c echo.Context) error {
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Company  string `json:"company"`
		Plan     string `json:"plan"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(400, "invalid request")
	}

	body.Name = strings.TrimSpace(body.Name)
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))

	if body.Name == "" || body.Email == "" || body.Password == "" {
		return echo.NewHTTPError(400, "name, email, password required")
	}
	if len(body.Password) < 8 {
		return echo.NewHTTPError(400, "password min 8 characters")
	}

	existing, _ := h.store.GetByEmail(body.Email)
	if existing != nil {
		return echo.NewHTTPError(409, "email already registered")
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)

	plan := body.Plan
	if plan == "" {
		plan = "community"
	}

	m := &store.Member{
		Name:         body.Name,
		Email:        body.Email,
		PasswordHash: string(hash),
		Company:      body.Company,
		Plan:         plan,
	}
	if err := h.store.Create(m); err != nil {
		slog.Error("register", "error", err)
		return echo.NewHTTPError(500, "failed to create account")
	}

	// Generate license key for premium/enterprise
	if plan == "premium" || plan == "enterprise" {
		licenseKey := generateLicenseKey(m.ID)
		h.store.SetLicenseKey(m.ID, licenseKey)
		m.LicenseKey = licenseKey
	}

	slog.Info("member registered", "id", m.ID, "email", m.Email, "plan", m.Plan)

	return c.JSON(201, map[string]any{
		"data": map[string]any{
			"id":           m.ID,
			"name":         m.Name,
			"email":        m.Email,
			"plan":         m.Plan,
			"license_key":  m.LicenseKey,
			"message":      "account created",
		},
	})
}

func (h *Handler) Login(c echo.Context) error {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(400, "invalid request")
	}

	body.Email = strings.TrimSpace(strings.ToLower(body.Email))

	m, err := h.store.GetByEmail(body.Email)
	if err != nil {
		return echo.NewHTTPError(401, "invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(m.PasswordHash), []byte(body.Password)); err != nil {
		return echo.NewHTTPError(401, "invalid credentials")
	}

	token := generateToken()
	sess := &store.Session{
		MemberID:  m.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	h.store.CreateSession(sess)

	return c.JSON(200, map[string]any{
		"data": map[string]any{
			"token": token,
			"member": map[string]any{
				"id":           m.ID,
				"name":         m.Name,
				"email":        m.Email,
				"plan":         m.Plan,
				"company":      m.Company,
				"license_key":  m.LicenseKey,
				"status":       m.Status,
			},
		},
	})
}

func (h *Handler) Me(c echo.Context) error {
	memberID := c.Get("member_id").(int64)
	m, err := h.store.GetByID(memberID)
	if err != nil {
		return echo.NewHTTPError(404, "member not found")
	}

	return c.JSON(200, map[string]any{
		"data": map[string]any{
			"id":           m.ID,
			"name":         m.Name,
			"email":        m.Email,
			"plan":         m.Plan,
			"company":      m.Company,
			"license_key":  m.LicenseKey,
			"status":       m.Status,
		},
	})
}

func (h *Handler) UpdateProfile(c echo.Context) error {
	memberID := c.Get("member_id").(int64)
	var body struct {
		Name    string `json:"name"`
		Company string `json:"company"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(400, "invalid request")
	}

	if err := h.store.UpdateProfile(memberID, body.Name, body.Company); err != nil {
		return echo.NewHTTPError(500, "failed to update profile")
	}

	return c.JSON(200, map[string]any{"message": "profile updated"})
}

func (h *Handler) UpdatePlan(c echo.Context) error {
	memberID := c.Get("member_id").(int64)
	var body struct {
		Plan string `json:"plan"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(400, "invalid request")
	}

	if err := h.store.UpdatePlan(memberID, body.Plan); err != nil {
		return echo.NewHTTPError(500, "failed to update plan")
	}

	// Generate license key for premium/enterprise
	if body.Plan == "premium" || body.Plan == "enterprise" {
		licenseKey := generateLicenseKey(memberID)
		h.store.SetLicenseKey(memberID, licenseKey)
	}

	return c.JSON(200, map[string]any{"message": "plan updated", "plan": body.Plan})
}

// License API endpoints

func (h *Handler) ValidateLicense(c echo.Context) error {
	var body struct {
		LicenseKey string `json:"license_key"`
		Email      string `json:"email"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(400, "invalid request")
	}

	m, err := h.store.GetByLicenseKey(body.LicenseKey)
	if err != nil {
		return c.JSON(200, map[string]any{
			"valid":  false,
			"reason": "license not found",
		})
	}

	if m.Status != "active" {
		return c.JSON(200, map[string]any{
			"valid":  false,
			"reason": "license inactive",
		})
	}

	return c.JSON(200, map[string]any{
		"valid":   true,
		"plan":    m.Plan,
		"name":    m.Name,
		"email":   m.Email,
		"company": m.Company,
		"features": getPlanFeatures(m.Plan),
	})
}

func (h *Handler) ActivateLicense(c echo.Context) error {
	var body struct {
		LicenseKey string `json:"license_key"`
		Email      string `json:"email"`
		ServerIP   string `json:"server_ip"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(400, "invalid request")
	}

	m, err := h.store.GetByLicenseKey(body.LicenseKey)
	if err != nil {
		return echo.NewHTTPError(401, "invalid license")
	}

	slog.Info("license activated", "member_id", m.ID, "plan", m.Plan, "server_ip", body.ServerIP)

	return c.JSON(200, map[string]any{
		"valid":   true,
		"plan":    m.Plan,
		"message": "license activated",
	})
}

func (h *Handler) LicenseInfo(c echo.Context) error {
	key := c.QueryParam("key")
	if key == "" {
		return echo.NewHTTPError(400, "key required")
	}

	m, err := h.store.GetByLicenseKey(key)
	if err != nil {
		return echo.NewHTTPError(404, "license not found")
	}

	return c.JSON(200, map[string]any{
		"plan":    m.Plan,
		"name":    m.Name,
		"email":   m.Email,
		"company": m.Company,
		"status":  m.Status,
	})
}

// Helpers

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateLicenseKey(memberID int64) string {
	b := make([]byte, 16)
	rand.Read(b)
	return "SP-" + hex.EncodeToString(b)
}

func getPlanFeatures(plan string) []string {
	switch plan {
	case "community":
		return []string{"single_server", "unlimited_sites", "docker", "firewall", "redis", "ssl", "git_deploy", "backups", "cli", "community_support"}
	case "premium":
		return []string{"multi_server", "unlimited_sites", "docker", "firewall", "redis", "ssl", "git_deploy", "backups", "cli", "priority_support", "managed_updates", "white_label", "staging"}
	case "enterprise":
		return []string{"multi_server", "unlimited_sites", "docker", "firewall", "redis", "ssl", "git_deploy", "backups", "cli", "priority_support_4h", "sla_9999", "dedicated_manager", "custom_integrations", "on_premise", "security_audit", "phone_support", "source_code"}
	default:
		return []string{}
	}
}
