package apihttp

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAddToZipSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "dir"), 0o755)
	os.WriteFile(filepath.Join(root, "dir", "ok.txt"), []byte("ok"), 0o644)
	secret := filepath.Join(t.TempDir(), "shadow")
	os.WriteFile(secret, []byte("SECRET"), 0o600)
	os.Symlink(secret, filepath.Join(root, "dir", "leak"))
	os.Symlink(secret, filepath.Join(root, "top-leak"))

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := addToZip(zw, filepath.Join(root, "dir"), "dir"); err != nil {
		t.Fatal(err)
	}
	if err := addToZip(zw, filepath.Join(root, "top-leak"), "top-leak"); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	zr, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		if bytes.Contains(data, []byte("SECRET")) {
			t.Fatalf("zip entry %s contains a file from outside the site", f.Name)
		}
	}
	if len(zr.File) == 0 {
		t.Fatal("regular files must still be zipped")
	}
}
