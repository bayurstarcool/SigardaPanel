package cloudflare

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://api.cloudflare.com/client/v4"

// Client handles communication with the Cloudflare API v4.
type Client struct {
	apiToken string
	email    string
	http     *http.Client
}

// NewClient creates a new Cloudflare API client.
func NewClient(cfg Config) *Client {
	return &Client{
		apiToken: cfg.APIToken,
		email:    cfg.Email,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Verify checks that the configured API token is valid and returns user info.
func (c *Client) Verify() (*VerifyResponse, error) {
	resp, err := c.request("GET", "/user/token/verify", nil)
	if err != nil {
		return nil, fmt.Errorf("cloudflare verify: %w", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("cloudflare verify decode: %w", err)
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("cloudflare verify failed: %s", formatErrors(apiResp.Errors))
	}

	result := apiResp.Result.(map[string]any)
	status, _ := result["status"].(string)
	user, _ := result["id"].(string)

	return &VerifyResponse{
		Status:  status,
		Message: "API token is valid",
		User:    user,
	}, nil
}

// ListZones returns all zones for the account.
func (c *Client) ListZones() ([]Zone, error) {
	var allZones []Zone
	page := 1

	for {
		resp, err := c.request("GET", fmt.Sprintf("/zones?per_page=50&page=%d", page), nil)
		if err != nil {
			return nil, fmt.Errorf("cloudflare list zones: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("cloudflare list zones read: %w", err)
		}

		var wrapper struct {
			Success    bool       `json:"success"`
			Errors     []APIError `json:"errors"`
			Result     []Zone     `json:"result"`
			ResultInfo ResultInfo `json:"result_info"`
		}
		if err := json.Unmarshal(body, &wrapper); err != nil {
			return nil, fmt.Errorf("cloudflare list zones decode: %w", err)
		}

		if !wrapper.Success {
			return nil, fmt.Errorf("cloudflare list zones failed: %s", formatErrors(wrapper.Errors))
		}

		allZones = append(allZones, wrapper.Result...)
		if page >= wrapper.ResultInfo.TotalPages {
			break
		}
		page++
	}

	return allZones, nil
}

// GetZone returns a specific zone by ID.
func (c *Client) GetZone(zoneID string) (*Zone, error) {
	resp, err := c.request("GET", fmt.Sprintf("/zones/%s", zoneID), nil)
	if err != nil {
		return nil, fmt.Errorf("cloudflare get zone: %w", err)
	}
	defer resp.Body.Close()

	var wrapper struct {
		Success bool   `json:"success"`
		Errors  []APIError `json:"errors"`
		Result  Zone   `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("cloudflare get zone decode: %w", err)
	}
	if !wrapper.Success {
		return nil, fmt.Errorf("cloudflare get zone failed: %s", formatErrors(wrapper.Errors))
	}

	// Populate plan info
	if wrapper.Result.Type == "" {
		wrapper.Result.Type = "full"
	}

	return &wrapper.Result, nil
}

// ListDNSRecords returns all DNS records for a zone.
func (c *Client) ListDNSRecords(zoneID string, recordType string) ([]DNSRecord, error) {
	path := fmt.Sprintf("/zones/%s/dns_records?per_page=100", zoneID)
	if recordType != "" {
		path += "&type=" + url.QueryEscape(recordType)
	}

	resp, err := c.request("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("cloudflare list dns records: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cloudflare list dns records read: %w", err)
	}

	var wrapper struct {
		Success    bool        `json:"success"`
		Errors     []APIError  `json:"errors"`
		Result     []DNSRecord `json:"result"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("cloudflare list dns records decode: %w", err)
	}
	if !wrapper.Success {
		return nil, fmt.Errorf("cloudflare list dns records failed: %s", formatErrors(wrapper.Errors))
	}

	return wrapper.Result, nil
}

// CreateDNSRecord creates a new DNS record.
func (c *Client) CreateDNSRecord(zoneID string, req CreateDNSRecordRequest) (*DNSRecord, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare create dns marshal: %w", err)
	}

	resp, err := c.request("POST", fmt.Sprintf("/zones/%s/dns_records", zoneID), body)
	if err != nil {
		return nil, fmt.Errorf("cloudflare create dns record: %w", err)
	}
	defer resp.Body.Close()

	var wrapper struct {
		Success bool      `json:"success"`
		Errors  []APIError `json:"errors"`
		Result  DNSRecord `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("cloudflare create dns decode: %w", err)
	}
	if !wrapper.Success {
		return nil, fmt.Errorf("cloudflare create dns failed: %s", formatErrors(wrapper.Errors))
	}

	return &wrapper.Result, nil
}

// DeleteDNSRecord deletes a DNS record.
func (c *Client) DeleteDNSRecord(zoneID, recordID string) error {
	resp, err := c.request("DELETE", fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, recordID), nil)
	if err != nil {
		return fmt.Errorf("cloudflare delete dns: %w", err)
	}
	defer resp.Body.Close()

	var wrapper struct {
		Success bool        `json:"success"`
		Errors  []APIError  `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return fmt.Errorf("cloudflare delete dns decode: %w", err)
	}
	if !wrapper.Success {
		return fmt.Errorf("cloudflare delete dns failed: %s", formatErrors(wrapper.Errors))
	}

	return nil
}

// UpdateDNSRecord updates an existing DNS record.
func (c *Client) UpdateDNSRecord(zoneID, recordID string, req CreateDNSRecordRequest) (*DNSRecord, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare update dns marshal: %w", err)
	}

	resp, err := c.request("PUT", fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, recordID), body)
	if err != nil {
		return nil, fmt.Errorf("cloudflare update dns record: %w", err)
	}
	defer resp.Body.Close()

	var wrapper struct {
		Success bool      `json:"success"`
		Errors  []APIError `json:"errors"`
		Result  DNSRecord `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("cloudflare update dns decode: %w", err)
	}
	if !wrapper.Success {
		return nil, fmt.Errorf("cloudflare update dns failed: %s", formatErrors(wrapper.Errors))
	}

	return &wrapper.Result, nil
}

// UpdateDNSRecordPartial updates specific fields of an existing DNS record using PATCH.
func (c *Client) UpdateDNSRecordPartial(zoneID, recordID string, req map[string]any) (*DNSRecord, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare patch dns marshal: %w", err)
	}

	resp, err := c.request("PATCH", fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, recordID), body)
	if err != nil {
		return nil, fmt.Errorf("cloudflare patch dns record: %w", err)
	}
	defer resp.Body.Close()

	var wrapper struct {
		Success bool      `json:"success"`
		Errors  []APIError `json:"errors"`
		Result  DNSRecord `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("cloudflare patch dns decode: %w", err)
	}
	if !wrapper.Success {
		return nil, fmt.Errorf("cloudflare patch dns failed: %s", formatErrors(wrapper.Errors))
	}

	return &wrapper.Result, nil
}

// PurgeCache purges the cache for a zone.
func (c *Client) PurgeCache(zoneID string, req PurgeCacheRequest) (*PurgeCacheResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare purge cache marshal: %w", err)
	}

	resp, err := c.request("POST", fmt.Sprintf("/zones/%s/purge_cache", zoneID), body)
	if err != nil {
		return nil, fmt.Errorf("cloudflare purge cache: %w", err)
	}
	defer resp.Body.Close()

	var wrapper struct {
		Success bool             `json:"success"`
		Errors  []APIError       `json:"errors"`
		Result  PurgeCacheResponse `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("cloudflare purge cache decode: %w", err)
	}
	if !wrapper.Success {
		return nil, fmt.Errorf("cloudflare purge cache failed: %s", formatErrors(wrapper.Errors))
	}

	return &wrapper.Result, nil
}

// GetAnalytics returns zone analytics for a given time range.
func (c *Client) GetAnalytics(zoneID string, since, until time.Time) (*Analytics, error) {
	s := since.UTC().Format(time.RFC3339)
	u := until.UTC().Format(time.RFC3339)
	path := fmt.Sprintf("/zones/%s/analytics/dashboard?since=%s&until=%s&continuous=true", zoneID, url.QueryEscape(s), url.QueryEscape(u))

	resp, err := c.request("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("cloudflare analytics: %w", err)
	}
	defer resp.Body.Close()

	var wrapper struct {
		Success bool        `json:"success"`
		Errors  []APIError  `json:"errors"`
		Result  struct {
			Totals struct {
				Since     string         `json:"since"`
				Until     string         `json:"until"`
				Requests  json.Number    `json:"requests"`
				Bandwidth json.Number    `json:"bandwidth"`
				Threats   json.Number    `json:"threats"`
				Uniques   json.Number    `json:"uniques"`
			} `json:"totals"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("cloudflare analytics decode: %w", err)
	}
	if !wrapper.Success {
		return nil, fmt.Errorf("cloudflare analytics failed: %s", formatErrors(wrapper.Errors))
	}

	toInt := func(n json.Number) int64 {
		v, _ := n.Int64()
		return v
	}

	return &Analytics{
		Since:     wrapper.Result.Totals.Since,
		Until:     wrapper.Result.Totals.Until,
		Requests:  toInt(wrapper.Result.Totals.Requests),
		Bandwidth: toInt(wrapper.Result.Totals.Bandwidth),
		Threats:   toInt(wrapper.Result.Totals.Threats),
		Uniques:   toInt(wrapper.Result.Totals.Uniques),
	}, nil
}

// --- HTTP helpers ---

func (c *Client) request(method, path string, body []byte) (*http.Response, error) {
	url := baseURL + path

	var reqBody io.Reader
	if body != nil {
		reqBody = strings.NewReader(string(body))
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("cloudflare request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	if c.email != "" {
		req.Header.Set("X-Auth-Email", c.email)
	}
	req.Header.Set("User-Agent", "sigardapanel/0.1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare http: %w", err)
	}

	return resp, nil
}

func formatErrors(errors []APIError) string {
	var msgs []string
	for _, e := range errors {
		msgs = append(msgs, fmt.Sprintf("[%d] %s", e.Code, e.Message))
	}
	return strings.Join(msgs, "; ")
}
