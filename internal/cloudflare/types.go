package cloudflare

import "time"

// --- Request / Response types for Cloudflare API v4 ---

type Config struct {
	APIToken string `json:"api_token"`
	Email    string `json:"email,omitempty"`
}

type VerifyResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	User    string `json:"user,omitempty"`
}

type Zone struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Paused        bool     `json:"paused"`
	Type          string   `json:"type"`
	NameServers   []string `json:"name_servers,omitempty"`
	OriginalNS    []string `json:"original_name_servers,omitempty"`
	Plan          PlanInfo `json:"plan"`
	CreatedOn     string   `json:"created_on,omitempty"`
	ModifiedOn    string   `json:"modified_on,omitempty"`
	ActivatedOn   string   `json:"activated_on,omitempty"`
}

type PlanInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Price   int    `json:"price"`
	LegacyID string `json:"legacy_id,omitempty"`
}

type DNSRecord struct {
	ID        string `json:"id"`
	ZoneID    string `json:"zone_id"`
	ZoneName  string `json:"zone_name,omitempty"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	TTL       int    `json:"ttl"`
	Priority  int    `json:"priority,omitempty"`
	Proxied   bool   `json:"proxied"`
	Proxiable bool   `json:"proxiable,omitempty"`
	CreatedOn string `json:"created_on,omitempty"`
	ModifiedOn string `json:"modified_on,omitempty"`
	Comment   string `json:"comment,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

type CreateDNSRecordRequest struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Content  string   `json:"content"`
	TTL      int      `json:"ttl,omitempty"`
	Priority int      `json:"priority,omitempty"`
	Proxied  *bool    `json:"proxied,omitempty"`
	Comment  string   `json:"comment,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type PurgeCacheRequest struct {
	PurgeEverything bool     `json:"purge_everything"`
	Files           []string `json:"files,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Hosts           []string `json:"hosts,omitempty"`
}

type PurgeCacheResponse struct {
	Success bool `json:"success"`
}

type Analytics struct {
	Since    string              `json:"since"`
	Until    string              `json:"until"`
	Requests int64               `json:"requests"`
	Bandwidth int64              `json:"bandwidth"`
	Threats  int64               `json:"threats"`
	Uniques  int64               `json:"uniques"`
	Statuses map[string]int64    `json:"statuses,omitempty"`
	TopCountries []CountryStat   `json:"top_countries,omitempty"`
}

type CountryStat struct {
	Country string `json:"country"`
	Requests int64 `json:"requests"`
}

// --- CF API v4 response wrapper ---

type APIResponse struct {
	Success  bool            `json:"success"`
	Errors   []APIError      `json:"errors"`
	Messages []string        `json:"messages"`
	Result   any             `json:"result"`
	ResultInfo *ResultInfo   `json:"result_info,omitempty"`
}

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ResultInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
	Count      int `json:"count"`
	Total      int `json:"total_count"`
}

// --- Time helpers ---

func (z *Zone) CreatedAt() time.Time {
	t, _ := time.Parse(time.RFC3339, z.CreatedOn)
	return t
}

func (z *Zone) ModifiedAt() time.Time {
	t, _ := time.Parse(time.RFC3339, z.ModifiedOn)
	return t
}
