package store

import (
	"context"
	"fmt"
	"strings"

	"sigardapanel/internal/dbserver"
)

// BackupDatabaseExists verifies MariaDB schema existence with the dedicated
// backup credential when one is configured, so stale panel DB records do not
// create failing jobs. Without it (nothing provisions one by default) the
// database is assumed to exist and the dump job reports a missing one.
func (s *Store) BackupDatabaseExists(ctx context.Context, database Database) (bool, error) {
	if !strings.EqualFold(database.Type, "mysql") && !strings.EqualFold(database.Type, "mariadb") {
		return true, nil
	}
	username, encrypted, err := s.DBStore.GetBackupCredentials(ctx)
	if err != nil {
		return true, nil
	}
	server, err := s.DBStore.GetDefault(ctx)
	if err != nil {
		return false, fmt.Errorf("get database server: %w", err)
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

// DatabaseBackupCredentials resolves the credential used by backup jobs: one
// of the database's own panel-managed accounts (it can read only that
// database, and it exists on whichever server hosts the site). MariaDB/MySQL
// databases without a usable account fall back to the dedicated backup
// account of the panel's default database server, when configured.
func (s *Store) DatabaseBackupCredentials(ctx context.Context, database Database) (string, string, error) {
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
	if strings.EqualFold(database.Type, "mysql") || strings.EqualFold(database.Type, "mariadb") {
		username, encrypted, err := s.DBStore.GetBackupCredentials(ctx)
		if err == nil {
			password, err := s.DBStore.DecryptPassword(encrypted)
			if err == nil && password != "" {
				return username, password, nil
			}
		}
	}
	return "", "", fmt.Errorf("database %s has no account the panel can use for backups; create a database user first", database.Name)
}
