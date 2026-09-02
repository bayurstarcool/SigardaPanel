package store

import (
	"database/sql"
)

// CloudflareConfig represents the stored Cloudflare API settings.
type CloudflareConfig struct {
	UserID  int64  `json:"user_id,omitempty"`
	APIToken string `json:"api_token"`
	Email    string `json:"email,omitempty"`
}

// CloudflareStore handles cloudflare config persistence.
type CloudflareStore struct {
	db *sql.DB
}

// NewCloudflareStore creates a new CloudflareStore.
func NewCloudflareStore(db *sql.DB) *CloudflareStore {
	return &CloudflareStore{db: db}
}

// GetConfig returns the stored Cloudflare configuration for a specific user.
func (s *CloudflareStore) GetConfig(userID int64) (*CloudflareConfig, error) {
	var apiToken, email sql.NullString
	err := s.db.QueryRow(
		"SELECT api_token, email FROM cloudflare_config WHERE user_id = ? LIMIT 1", userID,
	).Scan(&apiToken, &email)
	if err != nil {
		if err == sql.ErrNoRows {
			return &CloudflareConfig{}, nil
		}
		return nil, err
	}
	cfg := &CloudflareConfig{UserID: userID}
	if apiToken.Valid {
		cfg.APIToken = apiToken.String
	}
	if email.Valid {
		cfg.Email = email.String
	}
	return cfg, nil
}

// GetConfigByDomain returns the Cloudflare config for the user who owns the given domain.
func (s *CloudflareStore) GetConfigByDomain(domain string) (*CloudflareConfig, error) {
	var apiToken, email sql.NullString
	var userID int64
	err := s.db.QueryRow(`
		SELECT cf.api_token, cf.email, cf.user_id
		FROM cloudflare_config cf
		JOIN sites s ON s.owner = cf.user_id
		WHERE s.domain = ?
		LIMIT 1
	`, domain).Scan(&apiToken, &email, &userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return &CloudflareConfig{}, nil
		}
		return nil, err
	}
	cfg := &CloudflareConfig{UserID: userID}
	if apiToken.Valid {
		cfg.APIToken = apiToken.String
	}
	if email.Valid {
		cfg.Email = email.String
	}
	return cfg, nil
}

// SaveConfig upserts the Cloudflare configuration for a specific user.
func (s *CloudflareStore) SaveConfig(userID int64, cfg CloudflareConfig) error {
	// Delete existing config for this user first, then insert
	_, err := s.db.Exec("DELETE FROM cloudflare_config WHERE user_id = ?", userID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO cloudflare_config (user_id, api_token, email, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, userID, cfg.APIToken, nullableEmail(cfg.Email))
	return err
}

// HasConfig reports whether a specific user has an API token stored.
func (s *CloudflareStore) HasConfig(userID int64) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM cloudflare_config WHERE user_id = ? AND api_token IS NOT NULL AND api_token != ''",
		userID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// HasAnyConfig reports whether any user has a Cloudflare API token stored.
func (s *CloudflareStore) HasAnyConfig() (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM cloudflare_config WHERE api_token IS NOT NULL AND api_token != ''",
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteConfig removes the Cloudflare configuration for a specific user.
func (s *CloudflareStore) DeleteConfig(userID int64) (int64, error) {
	res, err := s.db.Exec("DELETE FROM cloudflare_config WHERE user_id = ?", userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func nullableEmail(email string) *string {
	if email == "" {
		return nil
	}
	return &email
}
