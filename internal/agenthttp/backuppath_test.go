package agenthttp

import "testing"

func TestIsAllowedBackupPath(t *testing.T) {
	r := &Router{backupDir: "/var/backups/sigardapanel", siteRootDir: "/var/www"}
	cases := []struct {
		path string
		want bool
	}{
		{"/var/www/example.com/backups/files-1.tar.gz", true},       // files backup (plural)
		{"/var/www/example.com/backup/db/database-1.sql", true},     // db backup (singular)
		{"/var/backups/sigardapanel/sites/x.tar.gz", true},          // centralized dir
		{"/var/www/example.com/public/index.php", false},            // not a backup path
		{"/etc/passwd", false},                                      // system file
		{"/var/www/example.com/backups/../../../etc/shadow", false}, // traversal
	}
	for _, c := range cases {
		if got := r.isAllowedBackupPath(c.path); got != c.want {
			t.Errorf("isAllowedBackupPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
