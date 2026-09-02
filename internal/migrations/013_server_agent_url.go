package migrations

var ServerAgentURLSchema = Migration{
	Version: 13,
	SQL: `
ALTER TABLE servers ADD COLUMN agent_url TEXT;
`,
}
