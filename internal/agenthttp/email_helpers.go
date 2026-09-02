package agenthttp

import "strings"

// sanitizeInput removes dangerous characters from input.
func sanitizeInput(input string) string {
	input = strings.ReplaceAll(input, "\x00", "")
	input = strings.ReplaceAll(input, "\n", "")
	input = strings.ReplaceAll(input, "\r", "")
	return strings.TrimSpace(input)
}

// isShellSafe checks if input is safe for shell commands.
func isShellSafe(input string) bool {
	if strings.Contains(input, "\x00") {
		return false
	}
	dangerous := []string{";", "&", "|", "`", "$", "(", ")", "{", "}", "[", "]", "!", "#", "~", "<", ">", "\"", "'", "\\", "\n", "\r"}
	for _, char := range dangerous {
		if strings.Contains(input, char) {
			return false
		}
	}
	return true
}
