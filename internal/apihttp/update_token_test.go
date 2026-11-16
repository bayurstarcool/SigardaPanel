package apihttp

import (
	"testing"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/selfupdate"
	"sigardapanel/internal/store"
)

// The panel update reads a private release repository with a super admin's
// GitHub (github.com) connection when SIGARDAPANEL_UPDATE_TOKEN is not set;
// connections of other users or other hosts are never used, and the env
// token wins.
func TestUpdateTokenFromSuperAdminGitHubConnection(t *testing.T) {
	f := newAuthzFixture(t)
	t.Setenv("SIGARDAPANEL_UPDATE_TOKEN", "")
	src := githubConnectionToken(f.s)
	if got := src(); got != "" {
		t.Fatalf("no connection: %q", got)
	}

	user, _ := f.user("member", auth.RoleUser)
	if err := f.s.Git.CreateConnection(f.ctx, &store.GitConnection{UserID: user.ID, Provider: "github",
		BaseURL: "https://github.com", Username: "u", Token: "user-token"}); err != nil {
		t.Fatal(err)
	}
	super, _ := f.user("boss", auth.RoleSuperAdmin)
	f.s.Git.CreateConnection(f.ctx, &store.GitConnection{UserID: super.ID, Provider: "gitea",
		BaseURL: "https://git.example.com", Username: "g", Token: "gitea-token"})
	if got := src(); got != "" {
		t.Fatalf("used a non-super-admin or non-GitHub connection: %q", got)
	}

	f.s.Git.CreateConnection(f.ctx, &store.GitConnection{UserID: super.ID, Provider: "github",
		BaseURL: "https://github.com", Username: "boss", Token: "super-token"})
	if got := src(); got != "super-token" {
		t.Fatalf("token = %q, want the super admin's GitHub token", got)
	}

	selfupdate.TokenSource = src
	t.Cleanup(func() { selfupdate.TokenSource = nil })
	if got := selfupdate.ConnectedToken(); got != "super-token" {
		t.Fatalf("ConnectedToken = %q", got)
	}
	t.Setenv("SIGARDAPANEL_UPDATE_TOKEN", "env-token")
	if got := selfupdate.ConnectedToken(); got != "" {
		t.Fatalf("ConnectedToken with an env token = %q (the installer reads panel.env itself)", got)
	}
}
