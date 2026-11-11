package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultAPIURL is used when neither SIGARDAPANEL_API_URL nor the config
// file names an API.
const DefaultAPIURL = "http://localhost:7700"

type Client struct {
	BaseURL string
	Token   string
	// TokenSource says where Token came from: "env" (SIGARDAPANEL_TOKEN),
	// "file" (the token file written by login) or "" (none).
	TokenSource string
	HTTP        *http.Client
}

type APIResponse struct {
	Data      json.RawMessage `json:"data"`
	Error     *APIError       `json:"error,omitempty"`
	RequestID string          `json:"request_id"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ResolveAPIURL returns the API base URL: SIGARDAPANEL_API_URL, then the
// api_url saved by login in config.json, then DefaultAPIURL. A trailing
// slash is removed.
func ResolveAPIURL() string {
	base := strings.TrimSpace(os.Getenv("SIGARDAPANEL_API_URL"))
	if base == "" {
		if cfg, err := LoadConfig(); err == nil && cfg != nil {
			base = strings.TrimSpace(cfg.APIURL)
		}
	}
	if base == "" {
		base = DefaultAPIURL
	}
	return strings.TrimRight(base, "/")
}

// ResolveToken returns the bearer token and its source. SIGARDAPANEL_TOKEN
// (for example a personal API key in CI) overrides the token file saved by
// `sigardapanel login`.
func ResolveToken() (token, source string) {
	if t := strings.TrimSpace(os.Getenv("SIGARDAPANEL_TOKEN")); t != "" {
		return t, "env"
	}
	if t, err := LoadToken(); err == nil {
		if t = strings.TrimSpace(t); t != "" {
			return t, "file"
		}
	}
	return "", ""
}

func NewClient() (*Client, error) {
	token, source := ResolveToken()
	return &Client{
		BaseURL:     ResolveAPIURL(),
		Token:       token,
		TokenSource: source,
		HTTP:        &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) Request(method, path string, body any) (*APIResponse, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, strings.TrimRight(c.BaseURL, "/")+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("cannot reach API at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	var apiResp APIResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		if resp.StatusCode < 400 {
			return nil, resp.StatusCode, fmt.Errorf("decode response from %s: %w", c.BaseURL, err)
		}
		// Non-JSON error page (e.g. a proxy's 502): keep a short excerpt.
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		apiResp.Error = &APIError{Code: fmt.Sprintf("http_%d", resp.StatusCode), Message: msg}
	}

	return &apiResp, resp.StatusCode, nil
}

func (c *Client) Get(path string) (*APIResponse, int, error) {
	return c.Request(http.MethodGet, path, nil)
}

func (c *Client) Post(path string, body any) (*APIResponse, int, error) {
	return c.Request(http.MethodPost, path, body)
}

func (c *Client) Patch(path string, body any) (*APIResponse, int, error) {
	return c.Request(http.MethodPatch, path, body)
}

func (c *Client) Put(path string, body any) (*APIResponse, int, error) {
	return c.Request(http.MethodPut, path, body)
}

func (c *Client) Delete(path string) (*APIResponse, int, error) {
	return c.Request(http.MethodDelete, path, nil)
}
