package apihttp

import "sigardapanel/internal/config"

// publicConfigData is the body of GET /api/v1/public/config (alias
// GET /api/v1/auth/signup): deployment switches the web reads before sign-in.
//
//   - member_signup: POST /api/v1/member/register accepts accounts (member
//     database configured AND SIGARDAPANEL_ALLOW_MEMBER_SIGNUP=true).
//   - panel_signup:  POST /api/v1/auth/register is open
//     (SIGARDAPANEL_ALLOW_REGISTRATION=true).
//   - landing_page:  / and /pricing show the SaaS landing
//     (SIGARDAPANEL_LANDING_PAGE=true); otherwise they redirect to /login.
func publicConfigData(memberDBConfigured bool) map[string]any {
	return map[string]any{
		"member_signup": memberDBConfigured && config.AllowMemberSignup(),
		"panel_signup":  config.AllowRegistration(),
		"landing_page":  config.LandingPage(),
	}
}
