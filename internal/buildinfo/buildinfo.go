// Package buildinfo holds the release version stamped into the binary. The CLI,
// the API and the agent all report this one value so they can never disagree.
package buildinfo

// Version is overridden at release time: -ldflags "-X sigardapanel/internal/buildinfo.Version=1.2.3"
var Version = "dev"
