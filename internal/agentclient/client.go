package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"sigardapanel/internal/jobmodel"
)

// Client is an HTTP client for the SigardaPanel agent.
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
}

// TaskResult is the response from an agent task execution.
type TaskResult struct {
	OK            bool   `json:"ok"`
	Output        string `json:"output,omitempty"`
	Error         string `json:"error,omitempty"`
	AppStatus     string `json:"app_status,omitempty"`
	Path          string `json:"path,omitempty"`
	S3Path        string `json:"s3_path,omitempty"`
	S3UploadError string `json:"s3_upload_error,omitempty"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
	NginxConfig   string `json:"nginx_config,omitempty"`
	ProxyPassURL  string `json:"proxy_pass_url,omitempty"`
	// Expires is the certificate end date reported by ssl.issue/ssl.renew,
	// in openssl's "-enddate" format (e.g. "Jan  2 15:04:05 2027 GMT").
	Expires string `json:"expires,omitempty"`
}

var agentErrorSecretPattern = regexp.MustCompile(`(?i)\b(password|secret|token|access[_-]?key|authorization)\s*([:=])\s*([^\s,;]+)`)

// New creates a new agent client.
func New(baseURL, token string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Minute},
		token:      token,
		baseURL:    baseURL,
	}
}

// Execute dispatches a job to the agent. Returns parsed result or an error.
func (c *Client) Execute(ctx context.Context, jobType, payload string) (*TaskResult, error) {
	endpoint := endpointForType(jobType)
	if endpoint == "" {
		return nil, fmt.Errorf("unknown job type %q", jobType)
	}

	var body io.Reader
	if payload != "" {
		body = bytes.NewReader([]byte(payload))
	} else {
		body = bytes.NewReader([]byte("{}"))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	result := &TaskResult{}
	if err := json.Unmarshal(responseBody, result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode == http.StatusNotImplemented {
		result.OK = false
		result.Error = fmt.Sprintf("agent task not implemented: %s", jobType)
		return result, nil
	}
	if resp.StatusCode >= 500 {
		return result, agentResponseError(resp.StatusCode, result, responseBody)
	}
	if resp.StatusCode >= 400 {
		return result, agentResponseError(resp.StatusCode, result, responseBody)
	}

	return result, nil
}

func agentResponseError(statusCode int, result *TaskResult, responseBody []byte) error {
	message := strings.TrimSpace(result.Error)
	if message == "" {
		var errorResponse struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if err := json.Unmarshal(responseBody, &errorResponse); err == nil {
			message = strings.TrimSpace(errorResponse.Message)
			if message == "" {
				message = strings.TrimSpace(errorResponse.Error)
			}
		}
	}

	message = redactAgentError(message)
	if message == "" {
		if statusCode >= 500 {
			return fmt.Errorf("agent returned %d", statusCode)
		}
		return fmt.Errorf("agent error %d", statusCode)
	}
	if statusCode >= 500 {
		return fmt.Errorf("agent returned %d: %s", statusCode, message)
	}
	return fmt.Errorf("agent error %d: %s", statusCode, message)
}

func redactAgentError(message string) string {
	return agentErrorSecretPattern.ReplaceAllString(message, "$1$2[REDACTED]")
}

func endpointForType(t string) string {
	switch t {
	case "site.create":
		return "/tasks/site/create"
	case "site.delete":
		return "/tasks/site/delete"
	case "site.deploy":
		return "/tasks/site/deploy"
	case "ssl.issue":
		return "/tasks/ssl/issue"
	case "ssl.renew":
		return "/tasks/ssl/renew"
	case jobmodel.TypeBackupCreate:
		return "/tasks/backup/create"
	case jobmodel.TypeBackupRestore:
		return "/tasks/backup/restore"
	case jobmodel.TypeBackupDelete:
		return "/tasks/backup/delete"
	case jobmodel.TypeDBBackupCreate:
		return "/tasks/backup/database"
	case jobmodel.TypeNginxReload:
		return "/tasks/nginx/reload"
	case jobmodel.TypeDatabaseCreate:
		return "/tasks/database/create"
	case jobmodel.TypeDatabaseDelete:
		return "/tasks/database/delete"
	case jobmodel.TypeDBUserCreate:
		return "/tasks/dbuser/create"
	case jobmodel.TypeDBUserDelete:
		return "/tasks/dbuser/delete"
	case jobmodel.TypeAppStart:
		return "/tasks/app/start"
	case jobmodel.TypeAppStop:
		return "/tasks/app/stop"
	case jobmodel.TypeAppRestart:
		return "/tasks/app/restart"
	default:
		return ""
	}
}
