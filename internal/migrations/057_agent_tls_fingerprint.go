package migrations

// AgentTLSFingerprintSchema stores the pinned TLS certificate fingerprint
// (SHA-256 of the DER certificate, hex) of each remote agent.
var AgentTLSFingerprintSchema = Migration{
	Version: 57,
	SQL:     `ALTER TABLE servers ADD COLUMN agent_tls_fingerprint TEXT NOT NULL DEFAULT '';`,
}
