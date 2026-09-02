package migrations

var AgentHealthSchema = Migration{
	Version: 38,
	SQL: `
ALTER TABLE servers ADD COLUMN last_health_check DATETIME;
ALTER TABLE servers ADD COLUMN health_status TEXT DEFAULT 'unknown';
ALTER TABLE servers ADD COLUMN health_latency_ms INTEGER DEFAULT 0;
`,
}
