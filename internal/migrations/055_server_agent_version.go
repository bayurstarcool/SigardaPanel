package migrations

// ServerAgentVersionSchema stores the version each agent reports on /health so
// the panel can show agents that run a different release than the panel.
var ServerAgentVersionSchema = Migration{
	Version: 55,
	SQL:     `ALTER TABLE servers ADD COLUMN agent_version TEXT;`,
}
