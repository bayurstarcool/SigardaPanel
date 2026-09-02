package apihttp

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/httpjson"
)

type selfUpdateHandler struct{}

func newSelfUpdateHandler() *selfUpdateHandler {
	return &selfUpdateHandler{}
}

var (
	latestVersionCache string
	cacheExpiry        time.Time
	cacheMu            sync.Mutex
)

func getLatestVersion() string {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	if latestVersionCache != "" && time.Now().Before(cacheExpiry) {
		return latestVersionCache
	}

	if data, err := os.ReadFile("/tmp/sigardapanel_latest_version"); err == nil {
		content := strings.TrimSpace(string(data))
		parts := strings.SplitN(content, "|", 2)
		if len(parts) == 2 {
			if t, err := time.Parse(time.RFC3339, parts[1]); err == nil && time.Now().Before(t.Add(1*time.Hour)) {
				latestVersionCache = parts[0]
				cacheExpiry = t.Add(1 * time.Hour)
				return latestVersionCache
			}
		}
	}

	tagsURL := "https://api.github.com/repos/bayurstarcool/SigardaPanel/tags"
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(tagsURL)
	if err == nil {
		defer resp.Body.Close()
		var tags []struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(resp.Body).Decode(&tags) == nil && len(tags) > 0 {
			latestVersionCache = tags[0].Name
			cacheExpiry = time.Now().Add(10 * time.Minute)
			os.WriteFile("/tmp/sigardapanel_latest_version", []byte(latestVersionCache+"|"+time.Now().Format(time.RFC3339)), 0644)
			return latestVersionCache
		}
	}
	return latestVersionCache
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimSuffix(v, "-dev")
	parts := strings.Split(v, ".")
	var result [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		result[i], _ = strconv.Atoi(parts[i])
	}
	return result
}

func versionGreater(a, b string) bool {
	av := parseVersion(a)
	bv := parseVersion(b)
	for i := 0; i < 3; i++ {
		if av[i] > bv[i] {
			return true
		}
		if av[i] < bv[i] {
			return false
		}
	}
	return false
}

func (h *selfUpdateHandler) checkUpdate(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	currentVersion := "0.0.0"
	if out, err := exec.Command("/root/SigardaPanel/sigardapanel", "version").Output(); err == nil {
		currentVersion = strings.TrimSpace(string(out))
	}

	latestVersion := getLatestVersion()

	updateAvailable := false
	if latestVersion != "" {
		latestClean := strings.TrimPrefix(latestVersion, "v")
		currentClean := strings.TrimPrefix(currentVersion, "v")
		if latestClean != currentClean {
			updateAvailable = versionGreater(latestClean, currentClean)
		}
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"current_version":  currentVersion,
			"latest_version":   latestVersion,
			"update_available": updateAvailable,
			"can_update":       true,
		},
		RequestID: rid,
	})
}

func (h *selfUpdateHandler) triggerUpdate(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	updateScript := "/root/SigardaPanel/update.sh"
	if _, err := os.Stat(updateScript); os.IsNotExist(err) {
		return echo.NewHTTPError(http.StatusNotFound, "update script not found")
	}

	// Create a standalone wrapper script that will survive service restart
	wrapper := "#!/bin/bash\nexport HOME=/root\nsleep 2\nbash /root/SigardaPanel/update.sh > /tmp/sigardapanel_update.log 2>&1\n"
	os.WriteFile("/tmp/sigardapanel_updater.sh", []byte(wrapper), 0755)

	// Launch fully detached process via nohup to survive service restart
	cmd := exec.Command("nohup", "bash", "/tmp/sigardapanel_updater.sh")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to start update: "+err.Error())
	}

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"message":  "Update triggered. Rebuild takes ~90s. Page will auto-reload.",
			"restart":  true,
			"cooldown": 120,
		},
		RequestID: rid,
	})
}

func (h *selfUpdateHandler) restartService(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	go func() {
		time.Sleep(500 * time.Millisecond)
		_ = exec.Command("systemctl", "restart", "sigardapanel-api").Run()
		time.Sleep(1 * time.Second)
		_ = exec.Command("systemctl", "restart", "sigardapanel-agent").Run()
	}()

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"message":  "Service restarting...",
			"cooldown": 5,
		},
		RequestID: rid,
	})
}

func (h *selfUpdateHandler) getVersion(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	version := "0.0.0"
	if out, err := exec.Command("/root/SigardaPanel/sigardapanel", "version").Output(); err == nil {
		version = strings.TrimSpace(string(out))
	}

	buildTime := ""
	info, _ := os.Stat("/root/SigardaPanel/sigardapanel")
	if info != nil {
		buildTime = info.ModTime().Format(time.RFC3339)
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"version":    version,
			"build_time": buildTime,
			"binary":     "/root/SigardaPanel/sigardapanel",
		},
		RequestID: rid,
	})
}
