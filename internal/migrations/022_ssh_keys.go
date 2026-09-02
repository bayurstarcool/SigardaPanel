package migrations

// SSHKeysSchema adds SSH private key support to site_ssh_ftp.
var SSHKeysSchema = Migration{
	Version: 22,
	SQL: `ALTER TABLE site_ssh_ftp ADD COLUMN ssh_private_key TEXT NOT NULL DEFAULT '';
ALTER TABLE site_ssh_ftp ADD COLUMN ssh_public_key TEXT NOT NULL DEFAULT '';`,
}
