package agenthttp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPasswdAndLoginShell(t *testing.T) {
	dir := t.TempDir()
	oldP, oldS := passwdPath, shellsPath
	passwdPath, shellsPath = filepath.Join(dir, "passwd"), filepath.Join(dir, "shells")
	t.Cleanup(func() { passwdPath, shellsPath = oldP, oldS })
	os.WriteFile(passwdPath, []byte("root:x:0:0:root:/root:/bin/bash\nalice:x:1001:1001::/home/alice:/bin/bash\nsvc:x:1002:1002::/nonexistent:/usr/sbin/nologin\n"), 0o644)
	os.WriteFile(shellsPath, []byte("# shells\n/bin/sh\n/bin/bash\n"), 0o644)

	pw, err := passwdByUID(1001)
	if err != nil || pw.Name != "alice" || pw.Home != "/home/alice" || pw.Shell != "/bin/bash" {
		t.Fatalf("passwd: %+v %v", pw, err)
	}
	if _, err := passwdByUID(4242); err == nil {
		t.Fatal("unknown uid found")
	}
	for shell, want := range map[string]bool{"/bin/bash": true, "/bin/sh": true, "/usr/sbin/nologin": false, "/bin/false": false, "/bin/zsh": false, "bash": false, "": false} {
		if got := loginShellAllowed(shell); got != want {
			t.Errorf("%q: %v", shell, got)
		}
	}
}
