package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
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

func NewClient() (*Client, error) {
	cfg, _ := LoadConfig()
	token, _ := LoadToken()
	base := ""
	if cfg != nil {
		base = cfg.APIURL
	}
	if base == "" {
		base = os.Getenv("SIGARDAPANEL_API_URL")
	}
	if base == "" {
		base = "http://localhost:7700"
	}
	return &Client{
		BaseURL: base,
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
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

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil && resp.StatusCode < 400 {
		return nil, resp.StatusCode, fmt.Errorf("decode: %w", err)
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

func (c *Client) Delete(path string) (*APIResponse, int, error) {
	return c.Request(http.MethodDelete, path, nil)
}
