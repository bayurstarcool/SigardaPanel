package agenthttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"

	"github.com/labstack/echo/v4"
)

// DockerContainer represents a Docker container.
type DockerContainer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	State   string `json:"state"`
	Ports   string `json:"ports"`
	Created string `json:"created"`
}

// dockerContainersTask lists all Docker containers.
func (r *Router) dockerContainersTask(c echo.Context) error {
	ctx := c.Request().Context()

	// Check if Docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":         true,
			"containers": []DockerContainer{},
			"message":    "Docker not installed",
		})
	}

	// List containers with custom format
	output, err := r.executor.RunShell(ctx, "docker", "ps", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.State}}\t{{.Ports}}\t{{.CreatedAt}}")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":         false,
			"containers": []DockerContainer{},
			"error":      err.Error(),
		})
	}

	var containers []DockerContainer
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 7)
		if len(parts) < 6 {
			continue
		}
		containers = append(containers, DockerContainer{
			ID:      parts[0],
			Name:    parts[1],
			Image:   parts[2],
			Status:  parts[3],
			State:   parts[4],
			Ports:   parts[5],
			Created: safeIndex(parts, 6),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":         true,
		"containers": containers,
		"total":      len(containers),
	})
}

// dockerContainerStartTask starts a Docker container.
func (r *Router) dockerContainerStartTask(c echo.Context) error {
	return r.dockerContainerAction(c, "start", "started")
}

// dockerContainerStopTask stops a Docker container.
func (r *Router) dockerContainerStopTask(c echo.Context) error {
	return r.dockerContainerAction(c, "stop", "stopped")
}

// dockerContainerRestartTask restarts a Docker container.
func (r *Router) dockerContainerRestartTask(c echo.Context) error {
	return r.dockerContainerAction(c, "restart", "restarted")
}

// dockerContainerRemoveTask removes a Docker container.
func (r *Router) dockerContainerRemoveTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		ContainerID string `json:"container_id"`
		Force       bool   `json:"force"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	args := []string{"rm"}
	if payload.Force {
		args = append(args, "-f")
	}
	args = append(args, payload.ContainerID)

	output, err := r.executor.RunShell(ctx, "docker", args...)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("remove failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: docker container removed", "container_id", payload.ContainerID)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("container %s removed", payload.ContainerID),
	})
}

// dockerContainerCreateTask creates and optionally starts a Docker container.
func (r *Router) dockerContainerCreateTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Image     string            `json:"image"`
		Name      string            `json:"name"`
		Command   string            `json:"command"`
		Ports     []string          `json:"ports"`      // e.g. ["8080:80", "443:443"]
		Volumes   []string          `json:"volumes"`    // e.g. ["/host/path:/container/path"]
		Env       map[string]string `json:"env"`        // e.g. {"KEY":"value"}
		Restart   string            `json:"restart"`    // no, always, unless-stopped
		Network   string            `json:"network"`    // bridge, host, none, or custom
		AutoStart bool              `json:"auto_start"` // start after create
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if payload.Image == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "image is required")
	}

	args := []string{"run", "-d"}
	if payload.Name != "" {
		args = append(args, "--name", payload.Name)
	}
	if payload.Restart != "" && payload.Restart != "no" {
		args = append(args, "--restart", payload.Restart)
	}
	if payload.Network != "" {
		args = append(args, "--network", payload.Network)
	}
	for _, p := range payload.Ports {
		args = append(args, "-p", p)
	}
	for _, v := range payload.Volumes {
		args = append(args, "-v", v)
	}
	for k, val := range payload.Env {
		args = append(args, "-e", k+"="+val)
	}
	if !payload.AutoStart {
		// Create without start: use --rm won't work, so create then stop
		// Actually docker run -d always starts. For create-only, use docker create
		args[0] = "create"
	}
	args = append(args, payload.Image)
	if payload.Command != "" {
		args = append(args, "sh", "-c", payload.Command)
	}

	output, err := r.executor.RunShell(ctx, "docker", args...)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("create failed: %s", strings.TrimSpace(output)),
		})
	}

	containerID := strings.TrimSpace(output)
	// docker run -d outputs pull logs to stderr + container ID to stdout.
	// CombinedOutput mixes them. The container ID is always the last line.
	if lines := strings.Split(strings.TrimSpace(output), "\n"); len(lines) > 0 {
		last := strings.TrimSpace(lines[len(lines)-1])
		// Container IDs are hex, 64 chars
		if len(last) == 64 {
			containerID = last
		}
	}

	slog.Info("agent: docker container created", "image", payload.Image, "name", payload.Name, "container_id", containerID)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":           true,
		"message":      fmt.Sprintf("container %s created", containerID[:12]),
		"container_id": containerID,
	})
}

// dockerContainerLogsTask gets logs from a Docker container.
func (r *Router) dockerContainerLogsTask(c echo.Context) error {
	ctx := c.Request().Context()
	containerID := c.QueryParam("container_id")
	lines := c.QueryParam("lines")
	if lines == "" {
		lines = "100"
	}

	if containerID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "container_id is required")
	}

	args := []string{"logs", "--tail", lines, containerID}
	output, err := r.executor.RunShell(ctx, "docker", args...)
	if err != nil {
		// docker logs may exit non-zero for stopped containers
		return c.JSON(http.StatusOK, map[string]any{
			"ok":    true,
			"logs":  strings.TrimSpace(output),
			"lines": lines,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":    true,
		"logs":  strings.TrimSpace(output),
		"lines": lines,
	})
}

// dockerContainerExecTask executes a command inside a Docker container.
func (r *Router) dockerContainerExecTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		ContainerID string `json:"container_id"`
		Command     string `json:"command"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if payload.ContainerID == "" || payload.Command == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "container_id and command are required")
	}

	args := []string{"exec", payload.ContainerID, "sh", "-c", payload.Command}
	output, err := r.executor.RunShell(ctx, "docker", args...)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"output":  strings.TrimSpace(output),
			"message": fmt.Sprintf("exec failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: docker exec", "container", payload.ContainerID, "command", payload.Command)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":     true,
		"output": strings.TrimSpace(output),
	})
}

// dockerImagesTask lists Docker images.
func (r *Router) dockerImagesTask(c echo.Context) error {
	ctx := c.Request().Context()

	if _, err := exec.LookPath("docker"); err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":     true,
			"images": []map[string]string{},
		})
	}

	output, err := r.executor.RunShell(ctx, "docker", "images", "--format", "{{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedAt}}")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":     false,
			"images": []map[string]string{},
			"error":  err.Error(),
		})
	}

	var images []map[string]string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 3 {
			continue
		}
		images = append(images, map[string]string{
			"repository": parts[0],
			"tag":        parts[1],
			"size":       parts[2],
			"created":    safeIndex(parts, 3),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":     true,
		"images": images,
		"total":  len(images),
	})
}

// dockerComposeUpTask runs docker-compose up.
func (r *Router) dockerComposeUpTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Path     string `json:"path"`
		Detached bool   `json:"detached"`
		Build    bool   `json:"build"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if payload.Path == "" {
		payload.Path = "."
	}

	args := []string{"compose", "-f", payload.Path, "up", "-d"}
	if payload.Build {
		args = append(args, "--build")
	}

	output, err := r.executor.RunShell(ctx, "docker", args...)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("compose up failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: docker compose up", "path", payload.Path)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "compose up completed",
		"output":  strings.TrimSpace(output),
	})
}

// dockerComposeDownTask runs docker-compose down.
func (r *Router) dockerComposeDownTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Path    string `json:"path"`
		Timeout int    `json:"timeout"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if payload.Path == "" {
		payload.Path = "."
	}

	args := []string{"compose", "-f", payload.Path, "down"}
	if payload.Timeout > 0 {
		args = append(args, "--timeout", fmt.Sprintf("%d", payload.Timeout))
	}

	output, err := r.executor.RunShell(ctx, "docker", args...)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("compose down failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: docker compose down", "path", payload.Path)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "compose down completed",
		"output":  strings.TrimSpace(output),
	})
}

// dockerContainerAction performs start/stop/restart on a container.
func (r *Router) dockerContainerAction(c echo.Context, action, pastTense string) error {
	ctx := c.Request().Context()
	var payload struct {
		ContainerID string `json:"container_id"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	output, err := r.executor.RunShell(ctx, "docker", action, payload.ContainerID)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("%s failed: %s", action, strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: docker container "+action, "container_id", payload.ContainerID)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("container %s %s", payload.ContainerID, pastTense),
	})
}

// ─── Image Management ──────────────────────────────────────────────

// dockerImagePullTask pulls a Docker image.
func (r *Router) dockerImagePullTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Image string `json:"image"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if payload.Image == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "image is required")
	}

	output, err := r.executor.RunShell(ctx, "docker", "pull", payload.Image)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("pull failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: docker image pull", "image", payload.Image)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("image %s pulled", payload.Image),
		"output":  strings.TrimSpace(output),
	})
}

// dockerImageRemoveTask removes a Docker image.
func (r *Router) dockerImageRemoveTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Image string `json:"image"`
		Force bool   `json:"force"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if payload.Image == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "image is required")
	}

	args := []string{"rmi"}
	if payload.Force {
		args = append(args, "-f")
	}
	args = append(args, payload.Image)

	output, err := r.executor.RunShell(ctx, "docker", args...)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("remove failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: docker image remove", "image", payload.Image)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("image %s removed", payload.Image),
	})
}

// ─── Volume Management ──────────────────────────────────────────────

// dockerVolumesTask lists Docker volumes.
func (r *Router) dockerVolumesTask(c echo.Context) error {
	ctx := c.Request().Context()

	output, err := r.executor.RunShell(ctx, "docker", "volume", "ls", "--format", "{{.Name}}	{{.Driver}}")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"volumes": []map[string]string{},
			"error":   err.Error(),
		})
	}

	var volumes []map[string]string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "	", 4)
		if len(parts) < 2 {
			continue
		}
		volumes = append(volumes, map[string]string{
			"name":       safeIndex(parts, 0),
			"driver":     safeIndex(parts, 1),
			"mountpoint": safeIndex(parts, 2),
			"created":    safeIndex(parts, 3),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"volumes": volumes,
		"total":   len(volumes),
	})
}

// dockerVolumeCreateTask creates a Docker volume.
func (r *Router) dockerVolumeCreateTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Name   string `json:"name"`
		Driver string `json:"driver"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if payload.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	args := []string{"volume", "create"}
	if payload.Driver != "" {
		args = append(args, "--driver", payload.Driver)
	}
	args = append(args, payload.Name)

	output, err := r.executor.RunShell(ctx, "docker", args...)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("create failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: docker volume create", "name", payload.Name)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("volume %s created", payload.Name),
	})
}

// dockerVolumeRemoveTask removes a Docker volume.
func (r *Router) dockerVolumeRemoveTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if payload.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	output, err := r.executor.RunShell(ctx, "docker", "volume", "rm", payload.Name)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("remove failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: docker volume remove", "name", payload.Name)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("volume %s removed", payload.Name),
	})
}

// ─── Network Management ──────────────────────────────────────────────

// dockerNetworksTask lists Docker networks.
func (r *Router) dockerNetworksTask(c echo.Context) error {
	ctx := c.Request().Context()

	output, err := r.executor.RunShell(ctx, "docker", "network", "ls", "--format", "{{.Name}}	{{.Driver}}	{{.Scope}}	{{.CreatedAt}}")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":       false,
			"networks": []map[string]string{},
			"error":    err.Error(),
		})
	}

	var networks []map[string]string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "	", 4)
		if len(parts) < 2 {
			continue
		}
		networks = append(networks, map[string]string{
			"name":    safeIndex(parts, 0),
			"driver":  safeIndex(parts, 1),
			"scope":   safeIndex(parts, 2),
			"created": safeIndex(parts, 3),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":       true,
		"networks": networks,
		"total":    len(networks),
	})
}

// dockerNetworkCreateTask creates a Docker network.
func (r *Router) dockerNetworkCreateTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Name   string `json:"name"`
		Driver string `json:"driver"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if payload.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	args := []string{"network", "create"}
	if payload.Driver != "" {
		args = append(args, "--driver", payload.Driver)
	}
	args = append(args, payload.Name)

	output, err := r.executor.RunShell(ctx, "docker", args...)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("create failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: docker network create", "name", payload.Name)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("network %s created", payload.Name),
	})
}

// dockerNetworkRemoveTask removes a Docker network.
func (r *Router) dockerNetworkRemoveTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if payload.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	output, err := r.executor.RunShell(ctx, "docker", "network", "rm", payload.Name)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("remove failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: docker network remove", "name", payload.Name)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("network %s removed", payload.Name),
	})
}

// ─── System Info ──────────────────────────────────────────────

// dockerInfoTask returns Docker system information.
func (r *Router) dockerInfoTask(c echo.Context) error {
	ctx := c.Request().Context()

	output, err := r.executor.RunShell(ctx, "docker", "info", "--format", "{{.ServerVersion}}	{{.Driver}}	{{.OperatingSystem}}	{{.NCPU}}	{{.MemTotal}}	{{.DockerRootDir}}	{{.ContainersRunning}}	{{.ContainersStopped}}	{{.Images}}	")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("docker info failed: %s", strings.TrimSpace(output)),
		})
	}

	parts := strings.SplitN(strings.TrimSpace(output), "	", 10)
	if len(parts) < 6 {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": "failed to parse docker info",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok": true,
		"info": map[string]string{
			"version":     safeIndex(parts, 0),
			"driver":      safeIndex(parts, 1),
			"os":          safeIndex(parts, 2),
			"cpus":        safeIndex(parts, 3),
			"memory":      safeIndex(parts, 4),
			"docker_root": safeIndex(parts, 5),
			"running":     safeIndex(parts, 6),
			"stopped":     safeIndex(parts, 7),
			"images":      safeIndex(parts, 8),
		},
	})
}

// dockerDiskUsageTask returns Docker disk usage.
func (r *Router) dockerDiskUsageTask(c echo.Context) error {
	ctx := c.Request().Context()

	output, err := r.executor.RunShell(ctx, "docker", "system", "df", "--format", "table {{.Type}}	{{.TotalCount}}	{{.Size}}	{{.Reclaimable}}")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("disk usage failed: %s", strings.TrimSpace(output)),
		})
	}

	var usage []map[string]string
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		usage = append(usage, map[string]string{
			"type":        parts[0],
			"count":       safeIndex(parts, 1),
			"size":        safeIndex(parts, 2),
			"reclaimable": safeIndex(parts, 3),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":    true,
		"usage": usage,
	})
}

// safeIndex returns the string at index i or empty string.
func safeIndex(parts []string, i int) string {
	if i < len(parts) {
		return parts[i]
	}
	return ""
}
