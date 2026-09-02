package migrations

var MetricsSchema = Migration{
	Version: 8,
	SQL: `
CREATE TABLE IF NOT EXISTS server_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    collected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    cpu_percent REAL,
    mem_used INTEGER,
    mem_total INTEGER,
    disk_used INTEGER,
    disk_total INTEGER,
    net_in_bytes INTEGER,
    net_out_bytes INTEGER
);
CREATE INDEX IF NOT EXISTS idx_server_metrics_server_id ON server_metrics(server_id);
CREATE INDEX IF NOT EXISTS idx_server_metrics_collected_at ON server_metrics(collected_at);
CREATE INDEX IF NOT EXISTS idx_server_metrics_server_time ON server_metrics(server_id, collected_at);
`,
}
