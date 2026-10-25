package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"sigardapanel/internal/notify"
	"sigardapanel/internal/store"
)

// SecurityCollector periodically pulls compromise indicators from each
// server's agent and raises panel alerts when rules trip:
//
//   - SSH brute force: a burst of failed logins between two checks
//   - File integrity drift vs. the stored baseline (changed/missing/new)
//   - Suspicious outbound connections (miner/C2 ports)
//   - New root-equivalent (UID 0) accounts
//
// Alerts are deduplicated by fingerprint: the same condition only re-alerts
// after its fingerprint changes, so a steady state stays quiet.
type SecurityCollector struct {
	store  *store.Store
	client *http.Client
	stopCh chan struct{}

	// fingerprints maps rule-key -> last-alerted fingerprint
	fingerprints map[string]string
	// sshWatermark maps server id -> last seen failed-login total
	sshWatermark map[int64]int
}

func NewSecurityCollector(s *store.Store) *SecurityCollector {
	return &SecurityCollector{
		store:        s,
		client:       &http.Client{Timeout: 15 * time.Second},
		stopCh:       make(chan struct{}),
		fingerprints: map[string]string{},
		sshWatermark: map[int64]int{},
	}
}

func (sc *SecurityCollector) Start(ctx context.Context) {
	slog.Info("security collector started", "interval", "10m")
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		time.Sleep(30 * time.Second) // let services settle after boot
		sc.collectAll(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sc.stopCh:
				return
			case <-ticker.C:
				sc.collectAll(ctx)
			}
		}
	}()
}

func (sc *SecurityCollector) Stop() {
	close(sc.stopCh)
}

func (sc *SecurityCollector) collectAll(ctx context.Context) {
	servers, err := sc.store.Servers.List(ctx)
	if err != nil {
		slog.Error("security collector: list servers", "error", err)
		return
	}
	for _, server := range servers {
		if !server.AgentURL.Valid || server.AgentURL.String == "" || server.AgentToken == "" {
			continue
		}
		sc.checkServer(ctx, server.ID, server.Name, server.AgentURL.String, server.AgentToken)
	}
}

func (sc *SecurityCollector) fetch(ctx context.Context, agentURL, token, path string, out any) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, agentURL+path, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := sc.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Data) == 0 {
		return false
	}
	return json.Unmarshal(envelope.Data, out) == nil
}

// alertOnce sends an alert for key only when the fingerprint changed since
// the last alert for that key.
func (sc *SecurityCollector) alertOnce(key, fingerprint, severity, subject, message string) {
	if sc.fingerprints[key] == fingerprint {
		return
	}
	sc.fingerprints[key] = fingerprint
	sender := notify.NewSender(sc.store)
	sender.Send(context.Background(), severity, subject, message)
	slog.Warn("security collector: alert", "key", key, "subject", subject)
}

func (sc *SecurityCollector) checkServer(ctx context.Context, serverID int64, name, agentURL, token string) {
	serverLabel := fmt.Sprintf("server %d (%s)", serverID, name)

	// ── SSH brute force ──
	var ssh struct {
		FailedTotal  int `json:"failed_total"`
		TopAttackers []struct {
			IP    string `json:"ip"`
			Count int    `json:"count"`
		} `json:"top_attackers"`
	}
	if sc.fetch(ctx, agentURL, token, "/tasks/security/ssh-activity", &ssh) {
		last := sc.sshWatermark[serverID]
		delta := 0
		if ssh.FailedTotal >= last {
			delta = ssh.FailedTotal - last
		}
		// Log rotation resets the counter; treat a drop as a fresh baseline.
		sc.sshWatermark[serverID] = ssh.FailedTotal
		if last > 0 && delta >= 20 {
			top := ""
			if len(ssh.TopAttackers) > 0 {
				top = fmt.Sprintf(" Top attacker: %s (%d attempts).", ssh.TopAttackers[0].IP, ssh.TopAttackers[0].Count)
			}
			sc.alertOnce(
				fmt.Sprintf("ssh-bruteforce-%d", serverID),
				fmt.Sprintf("d%d", delta),
				"warning",
				"SSH brute force detected",
				fmt.Sprintf("%s: %d failed SSH logins in the last 10 minutes.%s", serverLabel, delta, top),
			)
		}
	}

	// ── File integrity vs baseline ──
	baseline, err := sc.store.SecurityBaselines.ListByServer(ctx, serverID)
	if err == nil && len(baseline) > 0 {
		var current []struct {
			Path   string `json:"path"`
			Exists bool   `json:"exists"`
			SHA256 string `json:"sha256"`
		}
		if sc.fetch(ctx, agentURL, token, "/tasks/security/integrity", &current) {
			baseByPath := map[string]store.SecurityBaseline{}
			for _, b := range baseline {
				baseByPath[b.Path] = b
			}
			curByPath := map[string]string{}
			for _, e := range current {
				if e.Exists {
					curByPath[e.Path] = e.SHA256
				}
			}
			var changed, missing, added []string
			for path, b := range baseByPath {
				hash, ok := curByPath[path]
				switch {
				case !ok:
					missing = append(missing, path)
				case hash != b.SHA256:
					changed = append(changed, path)
				}
			}
			for path := range curByPath {
				if _, ok := baseByPath[path]; !ok {
					added = append(added, path)
				}
			}
			if len(changed)+len(missing)+len(added) > 0 {
				sc.alertOnce(
					fmt.Sprintf("integrity-%d", serverID),
					fmt.Sprintf("c%d-m%d-a%d:%v%v%v", len(changed), len(missing), len(added), changed, missing, added),
					"error",
					"Critical file integrity drift",
					fmt.Sprintf("%s: security-critical files changed. changed=%v missing=%v new=%v — verify these changes were authorised, then re-save the baseline.",
						serverLabel, changed, missing, added),
				)
			}
		}
	}

	// ── Suspicious outbound connections ──
	var conns struct {
		Suspicious []struct {
			Remote string `json:"remote"`
			Reason string `json:"reason"`
		} `json:"suspicious_only"`
	}
	if sc.fetch(ctx, agentURL, token, "/tasks/security/connections", &conns) && len(conns.Suspicious) > 0 {
		fp := ""
		for _, s := range conns.Suspicious {
			fp += s.Remote + ","
		}
		detail := ""
		for i, s := range conns.Suspicious {
			if i >= 5 {
				detail += " …"
				break
			}
			if i > 0 {
				detail += ", "
			}
			detail += fmt.Sprintf("%s (%s)", s.Remote, s.Reason)
		}
		sc.alertOnce(
			fmt.Sprintf("connections-%d", serverID),
			fp,
			"error",
			"Suspicious outbound connections",
			fmt.Sprintf("%s: established connections to ports associated with miners/C2: %s. Identify the responsible process.", serverLabel, detail),
		)
	}

	// ── Root-equivalent accounts ──
	var persist struct {
		UIDZeroUsers []string `json:"uid_zero_users"`
	}
	if sc.fetch(ctx, agentURL, token, "/tasks/security/persistence", &persist) {
		fp := fmt.Sprintf("%d:%v", len(persist.UIDZeroUsers), persist.UIDZeroUsers)
		sc.alertOnce(
			fmt.Sprintf("uid0-%d", serverID),
			fp,
			"error",
			"Root-equivalent accounts changed",
			fmt.Sprintf("%s: accounts with UID 0 are now %v (only 'root' is expected).", serverLabel, persist.UIDZeroUsers),
		)
	}
}

