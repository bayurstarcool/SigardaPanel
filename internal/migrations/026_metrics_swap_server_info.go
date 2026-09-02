package migrations

var MetricsSwapServerInfoSchema = Migration{
	Version: 26,
	SQL: `
-- Add swap columns to server_metrics
ALTER TABLE server_metrics ADD COLUMN swap_used INTEGER;
ALTER TABLE server_metrics ADD COLUMN swap_total INTEGER;

-- Add IPv6 and provider columns to servers
ALTER TABLE servers ADD COLUMN ipv6 TEXT;
ALTER TABLE servers ADD COLUMN provider TEXT;
`,
}
