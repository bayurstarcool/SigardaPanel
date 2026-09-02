package agenthttp

import "testing"

func TestIsValidGitRef(t *testing.T) {
	cases := []struct {
		ref  string
		want bool
	}{
		{"main", true},
		{"feature/new-thing", true},
		{"release-1.2.3", true},
		{"a1b2c3d4e5f6", true},
		{"", false},      // empty
		{"-flag", false}, // leading dash (git flag injection)
		{"--upload-pack=touch /tmp/x", false},
		{"branch;rm -rf /", false}, // shell metachar
		{"a..b", false},            // double-dot ref
		{"branch name", false},     // space
		{"$(whoami)", false},       // command substitution
	}
	for _, c := range cases {
		if got := isValidGitRef(c.ref); got != c.want {
			t.Errorf("isValidGitRef(%q) = %v, want %v", c.ref, got, c.want)
		}
	}
}
