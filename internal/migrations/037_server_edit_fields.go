package migrations

var ServerEditFieldsSchema = Migration{
	Version: 37,
	SQL: `
ALTER TABLE servers ADD COLUMN ssh_port INTEGER DEFAULT 22;
ALTER TABLE servers ADD COLUMN description TEXT DEFAULT '';
`,
}
