// Package rbac is the panel's permission registry. Every capability a route
// or handler gates on is a Permission here; roles are sets of permissions.
//
// Built-in roles (super_admin, admin, user) are derived from each
// permission's MinRole, so their behaviour is defined in code and upgrades
// can add permissions. Custom roles (stored in the database) hold an explicit
// set and always include the baseline every user has (their own and assigned
// sites, which ownership checks keep scoped).
//
// Adding a route that is not open to every signed-in user: add (or reuse) a
// Permission with the lowest built-in role that should have it, mark it
// Dangerous when it is root-equivalent or acts across tenants, and guard the
// route with RequirePermission (internal/apihttp).
package rbac

import "sort"

// Built-in role names (same strings as internal/auth).
const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleUser       = "user"
)

// Permission is one capability.
type Permission struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	// Dangerous permissions are root-equivalent on a server or act across
	// every tenant: only a super_admin may grant them.
	Dangerous bool `json:"dangerous"`
	// MinRole is the lowest built-in role that holds the permission.
	MinRole string `json:"min_role"`
}

// Permission names.
const (
	// Users
	UserManage         = "user.manage"     // users the caller created: list, create, edit, delete, assign sites/plans
	UserManageAll      = "user.manage_all" // every user, any role
	UserImpersonate    = "user.impersonate"
	UserImpersonateAny = "user.impersonate_any" // any user whose permissions are a subset of the caller's // sign in as a user the caller manages
	QuotaUnlimited     = "quota.unlimited"      // no default disk quota
	// Sites
	SiteViewAll   = "site.view_all"   // read any site (files, logs, settings), not only owned/assigned ones
	SiteManageAll = "site.manage_all" // every level on any site, including delete and user assignments
	SiteSuspend   = "site.suspend"    // suspend and resume sites the caller can manage
	SiteCron      = "site.cron"       // create and delete cron jobs on sites the caller can write
	SiteVhostRaw  = "site.vhost.raw"  // write raw nginx vhost text
	SSLRenewAll   = "ssl.renew_all"   // renew every certificate
	// Jobs and audit
	JobViewServer = "job.view_server" // server-level jobs (not tied to a site)
	JobViewAll    = "job.view_all"    // every job
	JobLogsWrite  = "job.logs.write"  // append job logs (agents, automation)
	AuditView     = "audit.view"      // own audit entries
	AuditViewAll  = "audit.view_all"  // every tenant's audit entries
	AuditPurge    = "audit.purge"     // delete old audit entries
	// Backups
	BackupBulk           = "backup.bulk"            // bulk backups of sites the caller can write
	BackupProviderView   = "backup.provider.view"   // list/test storage providers (no secrets)
	BackupProviderManage = "backup.provider.manage" // create/change/delete storage providers (global credentials)
	BackupConfigManage   = "backup.config.manage"   // global backup configs and their runs
	// Servers
	ServerManage      = "server.manage"       // add, change, delete servers, agent tokens and TLS, benchmark
	ServerCommand     = "server.command"      // run commands on a server
	ServerTerminal    = "server.terminal"     // root terminal on a server
	ServerStack       = "server.stack"        // install/remove packages and services (Stack)
	ServerFirewall    = "server.firewall"     // firewall, fail2ban, bot blocker, security monitor
	ServerServices    = "server.services"     // redis, pm2, mail
	ServerPanelHost   = "server.panel_host"   // root actions on the panel host itself
	MetricsIngest     = "metrics.ingest"      // push server metrics
	DockerManage      = "docker.manage"       // create/exec/compose containers, images, volumes, networks
	DockerControl     = "docker.control"      // start/stop/restart the caller's site containers
	DockerViewAll     = "docker.view_all"     // every container on a server
	SiteDocker        = "site.docker"         // create docker sites
	SiteAnyRoot       = "site.any_root"       // any root path and system user for a site
	SiteChooseOwner   = "site.choose_owner"   // own or created users' account and home for a site
	SiteProxyInternal = "site.proxy_internal" // proxy sites to private/internal addresses or host names
	// Databases
	DatabaseExecSQL = "database.exec_sql" // free SQL on the database server
	DatabaseAdmin   = "database.admin"    // database server settings, import, phpMyAdmin password reset
	// Panel
	PanelUpdate    = "panel.update"   // update/roll back the panel
	AgentUpdate    = "agent.update"   // update remote agents
	AgentRegister  = "agent.register" // create agent registration tokens
	ServiceTokens  = "service_token.manage"
	SettingsGlobal = "settings.global" // global settings (retention)
	SettingsView   = "settings.view"   // read global settings (retention)
	AlertManage    = "alert.manage"    // alert channels and alerts
	LicenseManage  = "license.manage"
	PlanManage     = "plan.manage"
	OrderManage    = "order.manage"
	RoleManage     = "role.manage" // create/change/delete custom roles
)

var registry = []Permission{
	{UserManage, "users", "Manage the users you created (list, create, edit, delete, assign sites and plans)", false, RoleAdmin},
	{UserManageAll, "users", "Manage every user, any role", true, RoleSuperAdmin},
	{UserImpersonate, "users", "Sign in as a user you manage", false, RoleAdmin},
	{UserImpersonateAny, "users", "Sign in as any user whose permissions you hold (support), not only users you created", true, RoleSuperAdmin},
	{QuotaUnlimited, "users", "No default disk quota", false, RoleAdmin},

	{SiteViewAll, "sites", "Read any site, not only owned or assigned ones", false, RoleSuperAdmin},
	{SiteManageAll, "sites", "Full access to any site (write, delete, user assignments)", true, RoleSuperAdmin},
	{SiteSuspend, "sites", "Suspend and resume sites", false, RoleAdmin},
	{SiteCron, "sites", "Create and delete cron jobs", false, RoleAdmin},
	{SiteVhostRaw, "sites", "Edit raw nginx vhost text (can read files and reach the agent)", true, RoleSuperAdmin},
	{SiteDocker, "sites", "Create docker sites (root containers)", true, RoleSuperAdmin},
	{SiteAnyRoot, "sites", "Any root path and system user for a site", true, RoleSuperAdmin},
	{SiteChooseOwner, "sites", "Create sites for your own or created users' accounts", false, RoleAdmin},
	{SiteProxyInternal, "sites", "Proxy sites to private addresses and host names", true, RoleSuperAdmin},
	{SSLRenewAll, "sites", "Renew every certificate", false, RoleSuperAdmin},

	{JobViewServer, "jobs", "See server-level jobs", false, RoleAdmin},
	{JobViewAll, "jobs", "See every job", false, RoleSuperAdmin},
	{JobLogsWrite, "jobs", "Append job logs", false, RoleAdmin},
	{AuditView, "audit", "See your own audit entries", false, RoleAdmin},
	{AuditViewAll, "audit", "See every tenant's audit entries", false, RoleSuperAdmin},
	{AuditPurge, "audit", "Delete old audit entries", true, RoleSuperAdmin},

	{BackupBulk, "backups", "Bulk backups", false, RoleAdmin},
	{BackupProviderView, "backups", "List and test backup storage providers", false, RoleAdmin},
	{BackupProviderManage, "backups", "Manage backup storage providers (global credentials)", true, RoleSuperAdmin},
	{BackupConfigManage, "backups", "Manage global backup configs", true, RoleSuperAdmin},

	{ServerManage, "servers", "Add, change and delete servers, agent tokens and TLS", true, RoleSuperAdmin},
	{ServerCommand, "servers", "Run commands on servers", true, RoleSuperAdmin},
	{ServerTerminal, "servers", "Root terminal on servers", true, RoleSuperAdmin},
	{ServerStack, "servers", "Install and remove packages and services", true, RoleSuperAdmin},
	{ServerFirewall, "servers", "Firewall, fail2ban, bot blocker, security monitor", true, RoleSuperAdmin},
	{ServerServices, "servers", "Redis, pm2 and mail", true, RoleSuperAdmin},
	{ServerPanelHost, "servers", "Root actions on the panel host", true, RoleSuperAdmin},
	{MetricsIngest, "servers", "Push server metrics", false, RoleAdmin},
	{DockerManage, "servers", "Create and exec containers, compose, images, volumes, networks", true, RoleSuperAdmin},
	{DockerControl, "servers", "Start, stop and restart your site containers", false, RoleAdmin},
	{DockerViewAll, "servers", "See every container on a server", false, RoleSuperAdmin},

	{DatabaseExecSQL, "databases", "Run free SQL on the database server", true, RoleSuperAdmin},
	{DatabaseAdmin, "databases", "Database server settings, import, phpMyAdmin password reset", true, RoleSuperAdmin},

	{PanelUpdate, "panel", "Update and roll back the panel", true, RoleSuperAdmin},
	{AgentUpdate, "panel", "Update remote agents", true, RoleSuperAdmin},
	{AgentRegister, "panel", "Create agent registration tokens", true, RoleSuperAdmin},
	{ServiceTokens, "panel", "Manage service tokens", true, RoleSuperAdmin},
	{SettingsView, "panel", "Read global settings", false, RoleAdmin},
	{SettingsGlobal, "panel", "Change global settings", true, RoleSuperAdmin},
	{AlertManage, "panel", "Alert channels and alerts", false, RoleSuperAdmin},
	{LicenseManage, "panel", "Activate and remove the license", true, RoleSuperAdmin},
	{PlanManage, "panel", "Manage plans", false, RoleSuperAdmin},
	{OrderManage, "panel", "Manage orders", false, RoleSuperAdmin},
	{RoleManage, "panel", "Create, change and delete custom roles", true, RoleSuperAdmin},
}

var byName = func() map[string]Permission {
	m := make(map[string]Permission, len(registry))
	for _, p := range registry {
		m[p.Name] = p
	}
	return m
}()

// All returns every permission, ordered by category and name.
func All() []Permission {
	out := append([]Permission(nil), registry...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Get returns the permission named name.
func Get(name string) (Permission, bool) {
	p, ok := byName[name]
	return p, ok
}

// IsBuiltinRole reports whether name is one of the three built-in roles.
func IsBuiltinRole(name string) bool {
	return name == RoleSuperAdmin || name == RoleAdmin || name == RoleUser
}

func rank(role string) int {
	switch role {
	case RoleSuperAdmin:
		return 3
	case RoleAdmin:
		return 2
	case RoleUser:
		return 1
	}
	return 0
}

// BuiltinPermissions returns the permission set of a built-in role (nil for
// any other name).
func BuiltinPermissions(role string) map[string]bool {
	r := rank(role)
	if r == 0 {
		return nil
	}
	out := map[string]bool{}
	for _, p := range registry {
		if rank(p.MinRole) <= r {
			out[p.Name] = true
		}
	}
	return out
}

// Sorted returns the names in set, sorted.
func Sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k, v := range set {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Subset reports whether every permission in a is in b.
func Subset(a, b map[string]bool) bool {
	for k, v := range a {
		if v && !b[k] {
			return false
		}
	}
	return true
}

// HasDangerous reports whether set holds a dangerous permission.
func HasDangerous(set map[string]bool) bool {
	for k, v := range set {
		if p, ok := byName[k]; v && ok && p.Dangerous {
			return true
		}
	}
	return false
}
