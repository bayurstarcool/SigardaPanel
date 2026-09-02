package agenthttp

import (
	"regexp"
	"strings"
)

// SQL injection prevention utilities

// isValidDBName validates database names (alphanumeric, underscores, hyphens)
var validDBNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)

// isValidDBUsername validates database usernames
var validDBUsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)

// isValidDBHost validates database host patterns
var validDBHostRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.%*?]+$`)

// isValidDBPrivileges validates SQL privileges string
var validDBPrivilegesRegex = regexp.MustCompile(`^[A-Z,\s]+$`)

// ValidateDBName checks if a database name is safe
func ValidateDBName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	return validDBNameRegex.MatchString(name)
}

// ValidateDBUsername checks if a database username is safe
func ValidateDBUsername(username string) bool {
	if len(username) == 0 || len(username) > 32 {
		return false
	}
	return validDBUsernameRegex.MatchString(username)
}

// ValidateDBHost checks if a database host pattern is safe
func ValidateDBHost(host string) bool {
	if len(host) == 0 || len(host) > 255 {
		return false
	}
	return validDBHostRegex.MatchString(host)
}

// ValidateDBPrivileges checks if a privileges string is safe
func ValidateDBPrivileges(privileges string) bool {
	if len(privileges) == 0 {
		return false
	}
	// Allow common privilege keywords
	allowedPrivileges := map[string]bool{
		"ALL": true, "SELECT": true, "INSERT": true, "UPDATE": true,
		"DELETE": true, "CREATE": true, "DROP": true, "ALTER": true,
		"INDEX": true, "EXECUTE": true, "PROCESS": true, "RELOAD": true,
		"SHUTDOWN": true, "SUPER": true, "FILE": true, "LOCK TABLES": true,
		"REFERENCES": true, "REPLICATION CLIENT": true, "REPLICATION SLAVE": true,
		"CREATE VIEW": true, "SHOW VIEW": true, "CREATE ROUTINE": true,
		"ALTER ROUTINE": true, "CREATE TEMPORARY TABLES": true,
		"EVENT": true, "TRIGGER": true, "CREATE TABLESPACE": true,
		"CREATE USER": true,
	}

	parts := strings.Split(privileges, ",")
	for _, part := range parts {
		priv := strings.TrimSpace(strings.ToUpper(part))
		if !allowedPrivileges[priv] {
			return false
		}
	}
	return true
}

// SanitizeSQLString escapes a string for SQL (more comprehensive than sqlQuote)
func SanitizeSQLString(s string) string {
	// Replace backslashes first (order matters)
	s = strings.ReplaceAll(s, "\\", "\\\\")
	// Replace single quotes
	s = strings.ReplaceAll(s, "'", "''")
	// Replace null bytes
	s = strings.ReplaceAll(s, "\x00", "")
	// Replace newlines
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

// SanitizeSQLIdent escapes an identifier for SQL
func SanitizeSQLIdent(s string) string {
	// Remove dangerous characters
	s = strings.NewReplacer(
		"`", "",
		"\\", "",
		";", "",
		"\n", "",
		"\r", "",
		"\x00", "",
		"'", "",
		"\"", "",
	).Replace(s)
	return s
}
