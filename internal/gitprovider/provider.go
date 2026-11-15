// Package gitprovider talks to GitHub, GitLab and Gitea (including
// self-hosted GitLab/Gitea and GitHub Enterprise) with a user's access token:
// repositories and branches for the site's Git tab, a read-only deploy key
// per site, the push webhook, and the deployment status shown on commits.
package gitprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Kind is a provider type.
type Kind string

const (
	GitHub Kind = "github"
	GitLab Kind = "gitlab"
	Gitea  Kind = "gitea"
)

// DefaultBaseURL is the web address of the hosted service.
func DefaultBaseURL(k Kind) string {
	switch k {
	case GitHub:
		return "https://github.com"
	case GitLab:
		return "https://gitlab.com"
	}
	return "" // Gitea is always self-hosted (or codeberg.org, given by the user)
}

// Config describes one connection.
type Config struct {
	Kind    Kind
	BaseURL string // web address, e.g. https://github.com, https://git.example.com
	Token   string
	// AllowPrivate lets the client reach private / loopback addresses
	// (a self-hosted server on the LAN); only super admins may set it.
	AllowPrivate bool
	// HTTPClient replaces the guarded client (tests).
	HTTPClient *http.Client
}

// User is the account a token belongs to.
type User struct {
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// Repo is a repository the token can access.
type Repo struct {
	ID            string `json:"id"`
	FullName      string `json:"full_name"`
	Description   string `json:"description,omitempty"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	HTMLURL       string `json:"html_url"`
	SSHURL        string `json:"ssh_url"`
	CloneURL      string `json:"clone_url"`
	CanAdmin      bool   `json:"can_admin"` // may add deploy keys and webhooks
	UpdatedAt     string `json:"updated_at,omitempty"`
}

// Branch is a branch head.
type Branch struct {
	Name string `json:"name"`
	SHA  string `json:"sha"`
}

// Commit is the part of a commit the deploy history shows.
type Commit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Author  string `json:"author"`
	URL     string `json:"url"`
}

// State is a deployment state.
type State string

const (
	StatePending    State = "pending"
	StateInProgress State = "in_progress"
	StateSuccess    State = "success"
	StateFailure    State = "failure"
	StateError      State = "error"
)

// Provider is one connection to a git host.
type Provider interface {
	Kind() Kind
	BaseURL() string
	CurrentUser(ctx context.Context) (*User, error)
	// ListRepos returns the repositories the token can access, most
	// recently updated first, filtered by query (name contains).
	ListRepos(ctx context.Context, query string) ([]Repo, error)
	GetRepo(ctx context.Context, fullName string) (*Repo, error)
	ListBranches(ctx context.Context, fullName string) ([]Branch, error)
	GetCommit(ctx context.Context, fullName, ref string) (*Commit, error)
	// AddDeployKey installs a read-only SSH key; returns its id.
	AddDeployKey(ctx context.Context, fullName, title, publicKey string) (string, error)
	DeleteDeployKey(ctx context.Context, fullName, id string) error
	// CreateWebhook adds a push webhook signed with secret; returns its id.
	CreateWebhook(ctx context.Context, fullName, hookURL, secret string, verifySSL bool) (string, error)
	DeleteWebhook(ctx context.Context, fullName, id string) error
	WebhookExists(ctx context.Context, fullName, id string) (bool, error)
	// StartDeployment records a deployment of sha; returns a handle for
	// UpdateDeployment (GitHub: deployment id; others: the sha).
	StartDeployment(ctx context.Context, fullName, sha, ref, description string) (string, error)
	UpdateDeployment(ctx context.Context, fullName, handle, sha string, state State, targetURL, description string) error
	// KnownHosts returns SSH host key lines for the git host when the
	// provider publishes them (GitHub's meta API); nil otherwise.
	KnownHosts(ctx context.Context) []string
}

// ErrNotFound is returned for a missing repository, key or hook.
var ErrNotFound = errors.New("not found")

// Open is how the panel creates providers; tests replace it with a fake.
var Open = New

// New returns the provider for cfg.
func New(cfg Config) (Provider, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL(cfg.Kind)
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && cfg.AllowPrivate)) || u.User != nil {
		return nil, fmt.Errorf("invalid server URL %q (https://host)", cfg.BaseURL)
	}
	base = u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/")
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("an access token is required")
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = guardedClient(cfg.AllowPrivate)
	}
	c := &client{kind: cfg.Kind, base: base, token: strings.TrimSpace(cfg.Token), http: hc, host: u.Host}
	switch cfg.Kind {
	case GitHub:
		c.api = "https://api.github.com"
		if u.Host != "github.com" {
			c.api = base + "/api/v3" // GitHub Enterprise Server
		}
		return &github{c}, nil
	case GitLab:
		c.api = base + "/api/v4"
		return &gitlab{c}, nil
	case Gitea:
		c.api = base + "/api/v1"
		return &gitea{c}, nil
	}
	return nil, fmt.Errorf("unknown provider %q", cfg.Kind)
}

// client is the shared HTTP part.
type client struct {
	kind  Kind
	base  string // web base
	api   string
	host  string
	token string
	http  *http.Client
}

func (c *client) Kind() Kind      { return c.kind }
func (c *client) BaseURL() string { return c.base }

// do sends a request and decodes a JSON answer into out (when not nil).
func (c *client) do(ctx context.Context, method, path string, body, out any) (http.Header, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.api+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SigardaPanel")
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	switch c.kind {
	case GitHub:
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	case GitLab:
		req.Header.Set("PRIVATE-TOKEN", c.token)
	case Gitea:
		req.Header.Set("Authorization", "token "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", c.host, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return resp.Header, c.apiError(resp.StatusCode, raw)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.Header, fmt.Errorf("%s: unexpected answer (HTTP %d)", c.host, resp.StatusCode)
		}
	}
	return resp.Header, nil
}

func (c *client) apiError(code int, raw []byte) error {
	var e struct {
		Message any `json:"message"`
		Error   any `json:"error"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(raw, &e)
	msg := ""
	for _, v := range []any{e.Message, e.Error} {
		switch t := v.(type) {
		case string:
			if msg == "" {
				msg = t
			}
		case map[string]any, []any:
			if msg == "" {
				b, _ := json.Marshal(t)
				msg = string(b)
			}
		}
	}
	for _, x := range e.Errors {
		if x.Message != "" {
			msg += ": " + x.Message
		}
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	switch code {
	case http.StatusUnauthorized:
		return fmt.Errorf("%s rejected the token (expired or revoked)", c.host)
	case http.StatusForbidden:
		return fmt.Errorf("%s: the token lacks a permission for this (%s)", c.host, strings.TrimSpace(msg))
	case http.StatusNotFound:
		return fmt.Errorf("%w on %s (or the token has no access to it)", ErrNotFound, c.host)
	}
	if msg == "" {
		msg = http.StatusText(code)
	}
	return fmt.Errorf("%s: %s (HTTP %d)", c.host, msg, code)
}

// guardedClient refuses private, loopback, link-local and multicast
// addresses (unless allowed) at dial time, so a user-supplied server URL
// cannot reach the panel host's internal services; redirects are not
// followed (they could carry the token to another host).
func guardedClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("unresolved address %s", host)
			}
			if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				return fmt.Errorf("address %s is not allowed", ip)
			}
			if !allowPrivate && (ip.IsLoopback() || ip.IsPrivate() || isSharedOrReserved(ip)) {
				return fmt.Errorf("address %s is private; only a super admin can connect a server on a private network", ip)
			}
			return nil
		},
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = dialer.DialContext
	return &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return fmt.Errorf("redirect to %s refused", req.URL.Host)
		},
	}
}

var cgnat = func() *net.IPNet { _, n, _ := net.ParseCIDR("100.64.0.0/10"); return n }()

func isSharedOrReserved(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		return cgnat.Contains(v4) || v4[0] == 0
	}
	return false
}

// SSHHost is the host part of an SSH clone URL (git@host:o/r.git or
// ssh://git@host:port/o/r.git), for known_hosts.
func SSHHost(sshURL string) string {
	s := sshURL
	if rest, ok := strings.CutPrefix(s, "ssh://"); ok {
		if at := strings.IndexByte(rest, '@'); at >= 0 {
			rest = rest[at+1:]
		}
		if sl := strings.IndexByte(rest, '/'); sl >= 0 {
			rest = rest[:sl]
		}
		if h, p, err := net.SplitHostPort(rest); err == nil {
			return "[" + h + "]:" + p
		}
		return rest
	}
	if at := strings.IndexByte(s, '@'); at >= 0 {
		s = s[at+1:]
	}
	if c := strings.IndexByte(s, ':'); c >= 0 {
		s = s[:c]
	}
	return s
}

// splitFull splits owner/repo.
func splitFull(full string) (string, string, error) {
	o, r, ok := strings.Cut(full, "/")
	if !ok || o == "" || r == "" || strings.ContainsAny(full, "?#\\ ") || strings.Contains(full, "..") {
		return "", "", fmt.Errorf("invalid repository name %q", full)
	}
	return o, r, nil
}

func repoPath(full string) (string, error) {
	o, r, err := splitFull(full)
	if err != nil {
		return "", err
	}
	if strings.Contains(r, "/") { // GitHub/Gitea: exactly owner/name
		return "", fmt.Errorf("invalid repository name %q", full)
	}
	return url.PathEscape(o) + "/" + url.PathEscape(r), nil
}

func matches(full, query string) bool {
	return query == "" || strings.Contains(strings.ToLower(full), strings.ToLower(strings.TrimSpace(query)))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
