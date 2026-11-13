package apihttp

import (
	"os"
	"path/filepath"
	"testing"

	"sigardapanel/internal/store"
)

func TestOpenBackupFileRefusesSymlinksAndOutsidePaths(t *testing.T) {
	root := t.TempDir()
	site := &store.Site{RootPath: root}
	os.MkdirAll(filepath.Join(root, "backups"), 0o755)
	good := filepath.Join(root, "backups", "files_1.tar.gz")
	os.WriteFile(good, []byte("archive"), 0o644)
	secret := filepath.Join(t.TempDir(), "encryption.key")
	os.WriteFile(secret, []byte("KEY"), 0o600)
	link := filepath.Join(root, "backups", "files_2.tar.gz")
	os.Symlink(secret, link)
	// A symlinked directory on the way.
	os.Symlink(filepath.Dir(secret), filepath.Join(root, "backup"))

	if f, err := openBackupFile(good, site); err != nil {
		t.Fatalf("regular backup refused: %v", err)
	} else {
		f.Close()
	}
	for _, bad := range []string{link, filepath.Join(root, "backup", "encryption.key"), secret, filepath.Join(root, "backups", "..", "x.tar.gz")} {
		if f, err := openBackupFile(bad, site); err == nil {
			f.Close()
			t.Errorf("opened %s", bad)
		}
	}
}
