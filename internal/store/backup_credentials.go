package store

import (
	"context"
	"fmt"
	"strings"

	"sigardapanel/internal/dbserver"
)

// BackupDatabaseExists verifies MariaDB schema existence with dedicated
// backup credentials. Stale panel DB records must not create failing jobs.
func (s *Store) BackupDatabaseExists(ctx context.Context, database Database) (bool, error) {
	if !strings.EqualFold(database.Type, "mysql") && !strings.EqualFold(database.Type, "mariadb") {
		return true, nil
	}
	server, err := s.DBStore.GetDefault(ctx)
	if err != nil {
		return false, fmt.Errorf("get database server: %w", err)
	}
	username, encrypted, err := s.DBStore.GetBackupCredentials(ctx)
	if err != nil {
		return false, err
	}
	password, err := s.DBStore.DecryptPassword(encrypted)
	if err != nil {
		return false, fmt.Errorf("decrypt dedicated backup credential: %w", err)
	}
	server.Username = username
	conn, err := dbserver.Connect(server, password)
	if err != nil {
		return false, fmt.Errorf("connect with dedicated backup credential: %w", err)
	}
	defer conn.Close()
	return conn.DatabaseAccessible(ctx, database.Name)
}

// DatabaseBackupCredentials resolves the credential used by backup jobs.
// MariaDB/MySQL always use the dedicated backup account; PostgreSQL keeps its
// database-specific account because it is a separate database engine.
func (s *Store) DatabaseBackupCredentials(ctx context.Context, database Database) (string, string, error) {
	if strings.EqualFold(database.Type, "mysql") || strings.EqualFold(database.Type, "mariadb") {
		username, encrypted, err := s.DBStore.GetBackupCredentials(ctx)
		if err != nil {
			return "", "", err
		}
		password, err := s.DBStore.DecryptPassword(encrypted)
		if err != nil || password == "" {
			if err == nil {
				err = fmt.Errorf("empty dedicated backup password")
			}
			return "", "", fmt.Errorf("decrypt dedicated backup credential: %w", err)
		}
		return username, password, nil
	}

	dbUsers, err := s.DBUsers.ListByDatabase(ctx, database.ID)
	if err != nil {
		return "", "", fmt.Errorf("list database users: %w", err)
	}
	for _, dbUser := range dbUsers {
		password, decErr := s.DBStore.DecryptPassword(dbUser.EncryptedPassword)
		if decErr == nil && password != "" {
			return dbUser.Username, password, nil
		}
	}
	return "", "", fmt.Errorf("no decryptable database user password")
}
