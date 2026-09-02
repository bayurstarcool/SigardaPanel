// Package features defines feature tiers and access control for SigardaPanel licensing.
package features

// Tier constants
const (
	TierFree       = "free"
	TierBasic      = "basic"
	TierPro        = "pro"
	TierEnterprise = "enterprise"
	TierMax        = "max" // highest tier, all features (alias)
)

// Feature constants
const (
	// Free features
	FeatSingleServer     = "single_server"
	FeatStaticSites      = "static_sites"
	FeatPHP              = "php"
	FeatSSLCertificates  = "ssl_certificates"
	FeatBasicMonitoring  = "basic_monitoring"
	FeatFileManager      = "file_manager"
	FeatTerminal         = "terminal"
	FeatJobHistory       = "job_history"
	FeatSSL              = "ssl"

	// Basic features
	FeatNodeJS        = "nodejs"
	FeatPython        = "python"
	FeatBackups       = "backups"
	FeatBackupRestore = "backup_restore"
	FeatDatabase      = "database"
	FeatCronJobs      = "cron_jobs"

	// Pro features
	FeatMultiServer        = "multi_server"
	FeatAdvancedRuntimes   = "advanced_runtimes"   // Node.js, Python, Go
	FeatDatabaseManagement = "database_management" // MySQL, PostgreSQL
	FeatCloudBackups       = "cloud_backups"       // S3, GCS, etc
	FeatUserManagement     = "user_management"     // RBAC
	FeatAlerting           = "alerting"            // Telegram, Discord, Email
	FeatScheduledBackups   = "scheduled_backups"
	FeatAgentManagement    = "agent_management"
	FeatCustomDomains      = "custom_domains"
	FeatDocker             = "docker"
	FeatGitDeploy          = "git_deploy"
	FeatAdvancedVhost      = "advanced_vhost"
	FeatSSLManager         = "ssl_manager"
	FeatMonitoring         = "monitoring"
	FeatServiceTokens      = "service_tokens"

	// Enterprise features
	FeatWhiteLabel      = "white_label"
	FeatAPIAccess       = "api_access"
	FeatAuditLog        = "audit_log"
	FeatPrioritySupport = "priority_support"
	FeatCloudflare      = "cloudflare"
)

// TierLevel maps tier name to numeric level for comparison.
// Higher number = higher tier.
var TierLevel = map[string]int{
	TierFree:       0,
	TierBasic:      1,
	TierPro:        2,
	TierEnterprise: 2, // alias: same level as pro for now
	TierMax:        3,
}

// TierDescriptions provides human-readable names.
var TierDescriptions = map[string]string{
	TierFree:       "Free",
	TierBasic:      "Basic",
	TierPro:        "Pro",
	TierEnterprise: "Enterprise",
	TierMax:        "Max",
}

// FeatureTierMap maps each feature to its minimum required tier.
var FeatureTierMap = map[string]string{
	// Free
	FeatSingleServer:      TierFree,
	FeatStaticSites:       TierFree,
	FeatPHP:               TierFree,
	FeatSSLCertificates:   TierFree,
	FeatBasicMonitoring:   TierFree,
	FeatFileManager:       TierFree,
	FeatTerminal:          TierFree,
	FeatJobHistory:        TierFree,
	FeatSSL:               TierFree,
	FeatAPIAccess:         TierFree,
	FeatBackups:           TierFree,
	FeatBackupRestore:     TierFree,
	FeatCloudBackups:      TierFree,

	// Basic
	FeatNodeJS:   TierBasic,
	FeatPython:   TierBasic,
	FeatDatabase: TierBasic,
	FeatCronJobs: TierBasic,

	// Pro
	FeatMultiServer:        TierPro,
	FeatAdvancedRuntimes:   TierPro,
	FeatDatabaseManagement: TierPro,
	FeatUserManagement:     TierPro,
	FeatAlerting:           TierPro,
	FeatScheduledBackups:   TierPro,
	FeatAgentManagement:    TierPro,
	FeatCustomDomains:      TierPro,
	FeatDocker:             TierPro,
	FeatGitDeploy:          TierPro,
	FeatAdvancedVhost:      TierPro,
	FeatSSLManager:         TierPro,
	FeatMonitoring:         TierPro,
	FeatServiceTokens:      TierPro,

	// Enterprise
	FeatWhiteLabel:      TierEnterprise,
	FeatAuditLog:        TierEnterprise,
	FeatPrioritySupport: TierEnterprise,
	FeatCloudflare:      TierMax,
}

// FeatureLimits defines resource limits per tier.
type FeatureLimits struct {
	MaxServers int `json:"max_servers"`
	MaxSites   int `json:"max_sites"` // -1 = unlimited
}

// TierLimits maps tier to resource limits.
var TierLimits = map[string]FeatureLimits{
	TierFree:       {MaxServers: 1, MaxSites: 5},
	TierBasic:      {MaxServers: 3, MaxSites: 20},
	TierPro:        {MaxServers: 10, MaxSites: -1},
	TierEnterprise: {MaxServers: 50, MaxSites: -1},
	TierMax:        {MaxServers: -1, MaxSites: -1}, // -1 = unlimited
}

// HasFeature returns true if the given tier grants access to the feature.
func HasFeature(tier, feature string) bool {
	requiredTier, ok := FeatureTierMap[feature]
	if !ok {
		// Unknown feature: deny by default
		return false
	}
	tierLvl, ok := TierLevel[tier]
	if !ok {
		// Unknown tier: treat as free
		tierLvl = 0
	}
	reqLvl, ok := TierLevel[requiredTier]
	if !ok {
		reqLvl = 0
	}
	return tierLvl >= reqLvl
}

// IsPro checks if tier is pro or higher.
func IsPro(tier string) bool {
	lvl, ok := TierLevel[tier]
	if !ok {
		return false
	}
	return lvl >= TierLevel[TierPro]
}

// IsEnterprise checks if tier is enterprise or higher.
func IsEnterprise(tier string) bool {
	lvl, ok := TierLevel[tier]
	if !ok {
		return false
	}
	return lvl >= TierLevel[TierEnterprise]
}

// TierFeatures returns all features available for the given tier.
func TierFeatures(tier string) map[string]bool {
	result := make(map[string]bool, len(FeatureTierMap))
	for feature := range FeatureTierMap {
		result[feature] = HasFeature(tier, feature)
	}
	return result
}

// MinTier returns the minimum tier that grants access to a feature.
func MinTier(feature string) string {
	t, ok := FeatureTierMap[feature]
	if !ok {
		return TierFree
	}
	return t
}
