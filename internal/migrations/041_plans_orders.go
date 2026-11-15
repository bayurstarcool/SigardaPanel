package migrations

// PlansLicenseOrdersSchema once created the plans and license_orders tables
// and users.plan_id. Selling licenses (plans, orders) belongs to the master
// panel, not to the panel itself, so new installs skip it; databases that
// already ran it keep the (unused) tables.
var PlansLicenseOrdersSchema = Migration{
	Version: 41,
	SQL:     `SELECT 1;`,
}
