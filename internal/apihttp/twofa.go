package apihttp

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/pquerna/otp/totp"
	"sigardapanel/internal/store"
)

// TwoFactorHandler handles 2FA endpoints.
type TwoFactorHandler struct {
	store *store.Store
}

// newTwoFactorHandler creates a new TwoFactorHandler.
// twoFAFailures limits wrong authenticator codes per user on the 2FA
// management endpoints (a stolen session must not brute-force the 6 digits
// to disable 2FA or mint recovery codes); twoFAUsed rejects replays.
var (
	twoFAFailures = newFailureTracker(5, 15*time.Minute)
	twoFAUsed     usedTOTP
)

// checkTOTP validates code for userID with attempt limiting and replay
// protection. It returns a non-nil *echo.HTTPError to send (never the result
// of writing a response, which is nil and would let the caller continue),
// or nil when the code is good.
func checkTOTP(c *echo.Context, userID int64, code, secret string) error {
	key := "2fa:" + strconv.FormatInt(userID, 10)
	if twoFAFailures.blocked(key) {
		return echo.NewHTTPError(http.StatusTooManyRequests, "too many wrong codes; try again in 15 minutes")
	}
	if code == "" || secret == "" || !totp.Validate(code, secret) || !twoFAUsed.markUsed(userID, code) {
		twoFAFailures.fail(key)
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid TOTP code")
	}
	twoFAFailures.reset(key)
	return nil
}

// newRecoveryCodes generates and stores a fresh set of 10 recovery codes.
func (h *TwoFactorHandler) newRecoveryCodes(ctx context.Context, userID int64) ([]string, error) {
	codes := make([]string, 10)
	codeHashes := make([]string, 10)
	for i := 0; i < 10; i++ {
		raw := make([]byte, 10) // 80 bits of entropy per code
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw) // 16 chars
		codes[i] = enc[0:4] + "-" + enc[4:8] + "-" + enc[8:12] + "-" + enc[12:16]
		codeHashes[i] = hashRecoveryCode(codes[i])
	}
	if err := h.store.TwoFactor.StoreRecoveryCodes(ctx, userID, codeHashes); err != nil {
		return nil, err
	}
	return codes, nil
}

func newTwoFactorHandler(s *store.Store) *TwoFactorHandler {
	return &TwoFactorHandler{store: s}
}

// setup2FA generates a TOTP secret and returns the provisioning URI + QR code URL.
func (h *TwoFactorHandler) setup2FA(c *echo.Context) error {
	userID, ok := c.Get("user_id").(int64)
	if !ok || userID == 0 {
		// Service tokens have no user behind them; 2FA is a per-user setting.
		return echo.NewHTTPError(http.StatusForbidden, "2FA requires a user session")
	}

	// Generate a random secret
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to generate secret",
		})
	}
	secretBase32 := base32.StdEncoding.EncodeToString(secret)

	// Re-running setup while 2FA is on would overwrite the live secret without
	// proving possession of the current one (locking the user out, or letting a
	// hijacked session replace the second factor). Disable first.
	if enabled, err := h.store.TwoFactor.IsTOTPEnabled(c.Request().Context(), userID); err == nil && enabled {
		return c.JSON(http.StatusConflict, map[string]any{
			"error": "2FA is already enabled; disable it before setting it up again",
		})
	}

	// Get user for display name
	user, err := h.store.Users.GetByID(c.Request().Context(), userID)
	if err != nil || user == nil {
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
		"secret":    secretBase32,
		"qr_url":    key.URL(),
		"issuer":    "SigardaPanel",
		"account":   user.Email,
		"period":    30,
		"digits":    6,
		"algorithm": "SHA1",
	})
}

// enable2FA verifies a TOTP code and enables 2FA for the user.
func (h *TwoFactorHandler) enable2FA(c *echo.Context) error {
	userID, ok := c.Get("user_id").(int64)
	if !ok || userID == 0 {
		// Service tokens have no user behind them; 2FA is a per-user setting.
		return echo.NewHTTPError(http.StatusForbidden, "2FA requires a user session")
	}

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
	if err := checkTOTP(c, userID, body.Code, secret); err != nil {
		return err
	}

	// Enable 2FA
	if err := h.store.TwoFactor.EnableTOTP(c.Request().Context(), userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to enable 2FA",
		})
	}

	// The first recovery codes come with enabling; replacing them later
	// needs an authenticator code.
	resp := map[string]any{"message": "2FA enabled successfully", "enabled": true}
	if codes, err := h.newRecoveryCodes(c.Request().Context(), userID); err == nil {
		resp["recovery_codes"] = codes
	}
	return c.JSON(http.StatusOK, resp)
}

// disable2FA verifies a TOTP code and disables 2FA for the user.
func (h *TwoFactorHandler) disable2FA(c *echo.Context) error {
	userID, ok := c.Get("user_id").(int64)
	if !ok || userID == 0 {
		// Service tokens have no user behind them; 2FA is a per-user setting.
		return echo.NewHTTPError(http.StatusForbidden, "2FA requires a user session")
	}

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
	if err := checkTOTP(c, userID, body.Code, secret); err != nil {
		return err
	}

	// Disable 2FA
	if err := h.store.TwoFactor.DisableTOTP(c.Request().Context(), userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to disable 2FA",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "2FA disabled successfully",
		"enabled": false,
	})
}

// verify2FA verifies a TOTP code (used during login).
func (h *TwoFactorHandler) verify2FA(c *echo.Context) error {
	userID, ok := c.Get("user_id").(int64)
	if !ok || userID == 0 {
		// Service tokens have no user behind them; 2FA is a per-user setting.
		return echo.NewHTTPError(http.StatusForbidden, "2FA requires a user session")
	}

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

	if err := checkTOTP(c, userID, body.Code, secret); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"verified": true,
	})
}

// status2FA returns the 2FA status for the current user.
func (h *TwoFactorHandler) status2FA(c *echo.Context) error {
	userID, ok := c.Get("user_id").(int64)
	if !ok || userID == 0 {
		// Service tokens have no user behind them; 2FA is a per-user setting.
		return echo.NewHTTPError(http.StatusForbidden, "2FA requires a user session")
	}

	enabled, err := h.store.TwoFactor.IsTOTPEnabled(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to check 2FA status",
		})
	}

	secret, _ := h.store.TwoFactor.GetTOTPSecret(c.Request().Context(), userID)

	return c.JSON(http.StatusOK, map[string]any{
		"enabled": enabled,
		"setup":   secret != "",
	})
}

// generateRecoveryCodes generates backup recovery codes for 2FA.
func (h *TwoFactorHandler) generateRecoveryCodes(c *echo.Context) error {
	userID, ok := c.Get("user_id").(int64)
	if !ok || userID == 0 {
		// Service tokens have no user behind them; 2FA is a per-user setting.
		return echo.NewHTTPError(http.StatusForbidden, "2FA requires a user session")
	}

	// Check if 2FA is enabled
	enabled, err := h.store.TwoFactor.IsTOTPEnabled(c.Request().Context(), userID)
	if err != nil || !enabled {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "2FA is not enabled",
		})
	}

	// Replacing recovery codes always needs a current authenticator code
	// (also when all were used): a stolen session must not be able to mint
	// codes that log in without the second factor.
	var body struct {
		Code string `json:"code"`
	}
	_ = c.Bind(&body)
	secret, _ := h.store.TwoFactor.GetTOTPSecret(c.Request().Context(), userID)
	if err := checkTOTP(c, userID, body.Code, secret); err != nil {
		return err
	}
	codes, err := h.newRecoveryCodes(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "failed to generate recovery codes",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"recovery_codes": codes,
		"message":        "Save these codes securely. Each code can only be used once.",
	})
}
