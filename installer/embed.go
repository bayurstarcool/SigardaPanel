// Package installer embeds install.sh so the agent can run the exact,
// signature-verifying updater that shipped with this release (the panel's
// one-click update never downloads and executes an unsigned script).
package installer

import _ "embed"

// Script is installer/install.sh. It verifies every download against the
// pinned minisign key, backs up the binary, web bundle and database, and
// rolls back automatically when the updated panel is not healthy.
//
//go:embed install.sh
var Script []byte

// AgentScript is installer/agent-update.sh: the updater a remote agent runs
// (signature check, binary backup, automatic restore).
//
//go:embed agent-update.sh
var AgentScript []byte
