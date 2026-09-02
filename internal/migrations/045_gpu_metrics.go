package migrations

var GPUMetricsSchema = Migration{
	Version: 45,
	SQL: `
-- Add GPU columns to server_metrics
ALTER TABLE server_metrics ADD COLUMN gpu_name TEXT;
ALTER TABLE server_metrics ADD COLUMN gpu_count INTEGER;
ALTER TABLE server_metrics ADD COLUMN gpu_utilization REAL;
ALTER TABLE server_metrics ADD COLUMN gpu_memory_used INTEGER;
ALTER TABLE server_metrics ADD COLUMN gpu_memory_total INTEGER;
ALTER TABLE server_metrics ADD COLUMN gpu_temperature REAL;
ALTER TABLE server_metrics ADD COLUMN gpu_power_draw REAL;
ALTER TABLE server_metrics ADD COLUMN gpu_fan_speed REAL;
`,
}
