package apihttp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
	"sigardapanel/internal/store"
)

// TwoFactorHandler handles 2FA endpoints.
type TwoFactorHandler struct {
	store *store.Store
}

// newTwoFactorHandler creates a new TwoFactorHandler.
func newTwoFactorHandler(s *store.Store) *TwoFactorHandler {
	return &TwoFactorHandler{store: s}
}

// setup2FA generates a TOTP secret and returns the provisioning URI + QR code URL.
func (h *TwoFactorHandler) setup2FA(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	// Generate a random secret
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to generate secret",
		})
	}
	secretBase32 := base32.StdEncoding.EncodeToString(secret)

	// Get user for display name
	user, err := h.store.Users.GetByID(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{
			"error": "user not found",
		})
	}

	// Generate TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "SigardaPanel",
		AccountName: user.Email,
		Secret:      secret,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to generate TOTP key",
		})
	}

	// Store the secret temporarily (not enabled yet)
	if err := h.store.TwoFactor.SetTOTPSecret(c.Request().Context(), userID, secretBase32); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to store secret",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"secret":     secretBase32,
		"qr_url":     key.URL(),
		"issuer":     "SigardaPanel",
		"account":    user.Email,
		"period":     30,
		"digits":     6,
		"algorithm":  "SHA1",
	})
}

// enable2FA verifies a TOTP code and enables 2FA for the user.
func (h *TwoFactorHandler) enable2FA(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	var body struct {
		Code string `json:"code"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "invalid request body",
		})
	}

	if body.Code == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "code is required",
		})
	}

	// Get the stored secret
	secret, err := h.store.TwoFactor.GetTOTPSecret(c.Request().Context(), userID)
	if err != nil || secret == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "2FA not set up. Call /setup first.",
		})
	}

	// Validate the TOTP code
	valid := totp.Validate(body.Code, secret)
	if !valid {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "invalid TOTP code",
		})
	}

	// Enable 2FA
	if err := h.store.TwoFactor.EnableTOTP(c.Request().Context(), userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to enable 2FA",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message":  "2FA enabled successfully",
		"enabled":  true,
	})
}

// disable2FA verifies a TOTP code and disables 2FA for the user.
func (h *TwoFactorHandler) disable2FA(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	var body struct {
		Code string `json:"code"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "invalid request body",
		})
	}

	if body.Code == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "code is required",
		})
	}

	// Get the stored secret
	secret, err := h.store.TwoFactor.GetTOTPSecret(c.Request().Context(), userID)
	if err != nil || secret == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "2FA not set up",
		})
	}

	// Validate the TOTP code
	valid := totp.Validate(body.Code, secret)
	if !valid {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "invalid TOTP code",
		})
	}

	// Disable 2FA
	if err := h.store.TwoFactor.DisableTOTP(c.Request().Context(), userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to disable 2FA",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message":  "2FA disabled successfully",
		"enabled":  false,
	})
}

// verify2FA verifies a TOTP code (used during login).
func (h *TwoFactorHandler) verify2FA(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	var body struct {
		Code string `json:"code"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "invalid request body",
		})
	}

	if body.Code == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "code is required",
		})
	}

	// Get the stored secret
	secret, err := h.store.TwoFactor.GetTOTPSecret(c.Request().Context(), userID)
	if err != nil || secret == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "2FA not set up",
		})
	}

	// Validate the TOTP code with a time window
	valid := totp.Validate(body.Code, secret)
	if !valid {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "invalid TOTP code",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"verified": true,
	})
}

// status2FA returns the 2FA status for the current user.
func (h *TwoFactorHandler) status2FA(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	enabled, err := h.store.TwoFactor.IsTOTPEnabled(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to check 2FA status",
		})
	}

	secret, _ := h.store.TwoFactor.GetTOTPSecret(c.Request().Context(), userID)

	return c.JSON(http.StatusOK, map[string]any{
		"enabled":  enabled,
		"setup":    secret != "",
	})
}

// generateRecoveryCodes generates backup recovery codes for 2FA.
func (h *TwoFactorHandler) generateRecoveryCodes(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	// Check if 2FA is enabled
	enabled, err := h.store.TwoFactor.IsTOTPEnabled(c.Request().Context(), userID)
	if err != nil || !enabled {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "2FA is not enabled",
		})
	}

	// Generate 10 recovery codes
	codes := make([]string, 10)
	codeHashes := make([]string, 10)
	for i := 0; i < 10; i++ {
		code := make([]byte, 4)
		rand.Read(code)
		codes[i] = base32.StdEncoding.EncodeToString(code)[:8]
		h := sha256.Sum256([]byte(codes[i]))
		codeHashes[i] = hex.EncodeToString(h[:])
	}

	// Store hashed recovery codes in database
	if err := h.store.TwoFactor.StoreRecoveryCodes(c.Request().Context(), userID, codeHashes); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to store recovery codes",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"recovery_codes": codes,
		"message":        "Save these codes securely. Each code can only be used once.",
	})
}
