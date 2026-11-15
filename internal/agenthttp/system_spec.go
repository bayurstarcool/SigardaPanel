package agenthttp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/httpjson"
)

// SystemSpec holds detailed server specification information.
type SystemSpec struct {
	OS            OSInfo        `json:"os"`
	CPU           CPUInfo       `json:"cpu"`
	GPU           GPUInfo       `json:"gpu"`
	Memory        MemoryInfo    `json:"memory"`
	Disk          []DiskInfo    `json:"disk"`
	Network       []NetworkInfo `json:"network"`
	Cloud         *CloudInfo    `json:"cloud,omitempty"`
	Uptime        string        `json:"uptime"`
	Hostname      string        `json:"hostname"`
	KernelVersion string        `json:"kernel_version"`
	// Virtualization: none (bare metal), kvm, qemu, xen, vmware, microsoft,
	// lxc, openvz, docker, … (systemd-detect-virt naming); "" when unknown.
	Virtualization string     `json:"virtualization,omitempty"`
	PublicIP       string     `json:"public_ip,omitempty"`
	System         SystemInfo `json:"system"`
}

type CloudInfo struct {
	Provider     string `json:"provider"`
	InstanceID   string `json:"instance_id,omitempty"`
	Region       string `json:"region,omitempty"`
	Zone         string `json:"zone,omitempty"`
	InstanceType string `json:"instance_type,omitempty"`
	PublicIP     string `json:"public_ip,omitempty"`
}

type OSInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	PrettyName string `json:"pretty_name"`
	Arch       string `json:"arch"`
}

type CPUInfo struct {
	ModelName    string `json:"model_name"`
	VendorID     string `json:"vendor_id"`
	Cores        int    `json:"cores"`   // physical cores (all sockets)
	Threads      int    `json:"threads"` // logical CPUs online
	Sockets      int    `json:"sockets"`
	Architecture string `json:"architecture"`
	MaxMHz       string `json:"max_mhz,omitempty"` // the best clock known: max, else nominal, else current
	CacheSize    string `json:"cache_size,omitempty"`
	Hypervisor   string `json:"hypervisor,omitempty"`

	CoresPerSocket int         `json:"cores_per_socket,omitempty"`
	ThreadsPerCore int         `json:"threads_per_core,omitempty"`
	OfflineCPUs    int         `json:"offline_cpus,omitempty"`
	NUMANodes      int         `json:"numa_nodes,omitempty"`
	Family         string      `json:"family,omitempty"`
	Model          string      `json:"model,omitempty"`
	Stepping       string      `json:"stepping,omitempty"`
	Microcode      string      `json:"microcode,omitempty"`
	BogoMIPS       string      `json:"bogomips,omitempty"`
	BaseMHz        string      `json:"base_mhz,omitempty"`      // nominal / base clock
	MaxTurboMHz    string      `json:"max_turbo_mhz,omitempty"` // cpufreq maximum (bare metal)
	CurrentMHz     string      `json:"current_mhz,omitempty"`
	MinMHz         string      `json:"min_mhz,omitempty"`
	Governor       string      `json:"governor,omitempty"`
	Caches         []CacheInfo `json:"caches,omitempty"`
	Features       []string    `json:"features,omitempty"`     // notable instruction set extensions
	VirtSupport    string      `json:"virt_support,omitempty"` // VT-x / AMD-V exposed to this system
	CPUQuota       string      `json:"cpu_quota,omitempty"`    // cgroup CPU limit (containers)
	Source         string      `json:"source,omitempty"`       // where topology came from
}

// CacheInfo is one cache level (sum over all instances).
type CacheInfo struct {
	Level     string `json:"level"` // L1d, L1i, L2, L3
	Size      string `json:"size"`  // per instance, e.g. "48 KiB"
	Instances int    `json:"instances"`
	Total     string `json:"total"` // e.g. "192 KiB"
}

// GPUInfo describes detected graphics/compute devices. Usage is populated when
// nvidia-smi is available; display-only or virtual GPUs may expose no usage API.
type GPUInfo struct {
	Detected       bool    `json:"detected"`
	Count          int     `json:"count"`
	Name           string  `json:"name,omitempty"`
	UsagePercent   float64 `json:"usage_percent,omitempty"`
	UsageAvailable bool    `json:"usage_available"`
	// Display is a virtual or BMC display adapter (QXL, Cirrus, ASPEED, …):
	// listed for completeness, never counted as a GPU.
	Display string `json:"display,omitempty"`
}

type MemoryInfo struct {
	// TotalBytes is what the kernel can use (MemTotal); InstalledBytes the
	// RAM the machine has (the kernel and firmware reserve the difference).
	InstalledBytes int64 `json:"installed_bytes,omitempty"`
	TotalBytes     int64 `json:"total_bytes"`
	FreeBytes      int64 `json:"free_bytes"`
	UsedBytes      int64 `json:"used_bytes"`
	SwapTotal      int64 `json:"swap_total"`
	SwapUsed       int64 `json:"swap_used"`
	// Modules: populated slots (dmidecode); empty when not available.
	Modules []MemoryModule `json:"modules,omitempty"`
}

type DiskInfo struct {
	Name               string `json:"name"`
	SizeBytes          int64  `json:"size_bytes"`
	Type               string `json:"type"`
	DiskType           string `json:"disk_type"`
	DiskTypeConfidence string `json:"disk_type_confidence,omitempty"`
	DetectionReason    string `json:"detection_reason,omitempty"`
	Transport          string `json:"transport,omitempty"`
	Vendor             string `json:"vendor,omitempty"`
	Serial             string `json:"serial,omitempty"`
	Virtual            bool   `json:"virtual"`
	Rotational         *bool  `json:"rotational,omitempty"`
	Model              string `json:"model,omitempty"`
	Mounted            bool   `json:"mounted"`
	MountPoint         string `json:"mount_point,omitempty"`
	// Filesystem usage, summed over the filesystems on this disk (its
	// partitions, LVM volumes, …); SizeBytes stays the disk's size.
	FsSizeBytes int64       `json:"fs_size_bytes,omitempty"`
	UsedBytes   int64       `json:"used_bytes,omitempty"`
	FreeBytes   int64       `json:"free_bytes,omitempty"`
	Mounts      []DiskMount `json:"mounts,omitempty"`
}

// DiskMount is one mounted filesystem on a disk.
type DiskMount struct {
	Device     string `json:"device"`
	MountPoint string `json:"mount_point"`
	FSType     string `json:"fs_type,omitempty"`
	SizeBytes  int64  `json:"size_bytes"`
	UsedBytes  int64  `json:"used_bytes"`
	FreeBytes  int64  `json:"free_bytes"`
}

type NetworkInfo struct {
	Interface string   `json:"interface"`
	IPAddress string   `json:"ip_address"` // first IPv4, without prefix
	IPv4Addrs []string `json:"ipv4_addresses,omitempty"`
	IPv6Addrs []string `json:"ipv6_addresses,omitempty"` // global addresses only
	MAC       string   `json:"mac,omitempty"`
	Status    string   `json:"status"`
	SpeedMbps int      `json:"speed_mbps,omitempty"`
	Duplex    string   `json:"duplex,omitempty"`
	MTU       int      `json:"mtu,omitempty"`
	Driver    string   `json:"driver,omitempty"`
	Virtual   bool     `json:"virtual,omitempty"`
}

// BenchmarkResult holds benchmark results.
type BenchmarkResult struct {
	CPU     CPUBenchmark     `json:"cpu"`
	Disk    DiskBenchmark    `json:"disk"`
	Network NetworkBenchmark `json:"network"`
	RunAt   string           `json:"run_at"`
}

type CPUBenchmark struct {
	SingleThread CPUBenchScore `json:"single_thread"`
	MultiThread  CPUBenchScore `json:"multi_thread"`
}

type CPUBenchScore struct {
	EventsPerSec float64 `json:"events_per_sec"`
	TotalEvents  int     `json:"total_events"`
	LatencyAvgMs float64 `json:"latency_avg_ms"`
	ElapsedSec   float64 `json:"elapsed_sec"`
}

type DiskBenchmark struct {
	SeqWriteMBs    float64 `json:"seq_write_mbs"`
	SeqReadMBs     float64 `json:"seq_read_mbs"`
	RandomReadIOPS float64 `json:"random_read_iops"`
	RandomReadMBs  float64 `json:"random_read_mbs"`
}

type NetworkBenchmark struct {
	DownloadMBs float64 `json:"download_mbs"`
	PingMsAvg   float64 `json:"ping_ms_avg"`
	PingMsMin   float64 `json:"ping_ms_min"`
	PingMsMax   float64 `json:"ping_ms_max"`
	PacketLoss  string  `json:"packet_loss"`
}

// systemSpecTask returns detailed server specifications.
func (r *Router) systemSpecTask(c *echo.Context) error {
	spec := SystemSpec{
		Uptime:        getUptime(),
		Hostname:      getHostname(),
		KernelVersion: getKernelVersion(),
	}

	spec.Virtualization = detectVirtualization()
	// The slow parts (cloud-init, the public IP lookup) run alongside the rest.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); spec.Cloud = detectCloudProvider(spec.Virtualization) }()
	go func() { defer wg.Done(); spec.PublicIP = specPublicIP(c.Request().Context()) }()
	spec.OS = getOSInfo()
	spec.CPU = getCPUInfo()
	spec.GPU = getGPUInfo()
	spec.Memory = getMemoryInfo(spec.Virtualization)
	if !isContainerVirt(spec.Virtualization) {
		spec.Memory.Modules = memoryModules()
	}
	spec.Disk = getDiskInfo(spec.Virtualization)
	spec.Network = getNetworkInfo()
	linkDetails(spec.Network)
	spec.System = getSystemInfo()
	wg.Wait()
	if spec.Cloud != nil && spec.Cloud.PublicIP == "" {
		spec.Cloud.PublicIP = spec.PublicIP
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      spec,
		RequestID: "",
	})
}

// systemBenchmarkTask runs CPU, disk, and network benchmarks.
func (r *Router) systemBenchmarkTask(c *echo.Context) error {
	ctx := c.Request().Context()

	result := BenchmarkResult{
		RunAt: time.Now().UTC().Format(time.RFC3339),
	}

	// CPU benchmark (sysbench)
	result.CPU = runCPUBenchmark(ctx)

	// Disk benchmark (dd)
	result.Disk = runDiskBenchmark(ctx)

	// Network benchmark (ping + curl)
	result.Network = runNetworkBenchmark(ctx)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      result,
		RequestID: "",
	})
}

// --- Benchmark helpers ---

func runCPUBenchmark(ctx context.Context) CPUBenchmark {
	bench := CPUBenchmark{}

	// Check if sysbench is available
	if _, err := exec.LookPath("sysbench"); err != nil {
		return bench
	}

	// Single thread
	singleOut, err := exec.CommandContext(ctx, "sysbench", "cpu", "--threads=1", "--time=5", "run").Output()
	if err == nil {
		bench.SingleThread = parseSysbenchOutput(string(singleOut))
	}

	// Multi thread (use all available CPUs)
	threads := runtime.NumCPU()
	multiOut, err := exec.CommandContext(ctx, "sysbench", "cpu", fmt.Sprintf("--threads=%d", threads), "--time=5", "run").Output()
	if err == nil {
		bench.MultiThread = parseSysbenchOutput(string(multiOut))
	}

	return bench
}

func parseSysbenchOutput(output string) CPUBenchScore {
	score := CPUBenchScore{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "events per second:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "events per second:"))
			score.EventsPerSec, _ = strconv.ParseFloat(val, 64)
		} else if strings.HasPrefix(line, "total time:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "total time:"))
			val = strings.TrimSuffix(val, "s")
			score.ElapsedSec, _ = strconv.ParseFloat(val, 64)
		} else if strings.HasPrefix(line, "total number of events:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "total number of events:"))
			score.TotalEvents, _ = strconv.Atoi(val)
		} else if strings.HasPrefix(line, "avg:") && score.LatencyAvgMs == 0 {
			val := strings.TrimSpace(strings.TrimPrefix(line, "avg:"))
			score.LatencyAvgMs, _ = strconv.ParseFloat(val, 64)
		}
	}
	return score
}

func runDiskBenchmark(ctx context.Context) DiskBenchmark {
	bench := DiskBenchmark{}

	// Fixed names in /tmp are predictable: a local user could pre-plant a
	// symlink and make this root process overwrite any file. Use a private dir.
	dir, err := os.MkdirTemp("", "sigarda-bench-")
	if err != nil {
		return bench
	}
	defer os.RemoveAll(dir)
	diskTest := filepath.Join(dir, "disktest")
	fioTest := filepath.Join(dir, "fiotest")

	// Sequential write test (512MB)
	start := time.Now()
	out, err := exec.CommandContext(ctx, "dd", "if=/dev/zero", "of="+diskTest, "bs=1M", "count=512", "oflag=direct").CombinedOutput()
	elapsed := time.Since(start).Seconds()
	if err == nil {
		bench.SeqWriteMBs = 512.0 / elapsed
	} else {
		// Fallback without oflag=direct
		start2 := time.Now()
		out, err = exec.CommandContext(ctx, "dd", "if=/dev/zero", "of="+diskTest, "bs=1M", "count=512").CombinedOutput()
		elapsed = time.Since(start2).Seconds()
		if err == nil {
			_ = out
			bench.SeqWriteMBs = 512.0 / elapsed
		}
	}

	// Sequential read test
	start = time.Now()
	_, err = exec.CommandContext(ctx, "dd", "if="+diskTest, "of=/dev/null", "bs=1M").CombinedOutput()
	elapsed = time.Since(start).Seconds()
	if err == nil {
		bench.SeqReadMBs = 512.0 / elapsed
	}

	// Clean up
	os.Remove(diskTest)

	// Random read IOPS (fio)
	if _, fioErr := exec.LookPath("fio"); fioErr == nil {
		fioOut, err := exec.CommandContext(ctx, "fio", "--name=_sigarda_fio", "--ioengine=libaio", "--direct=1",
			"--bs=4k", "--iodepth=64", "--size=64M", "--rw=randread", "--runtime=5", "--time_based",
			"--filename="+fioTest, "--output-format=terse").Output()
		if err == nil {
			fields := strings.Split(strings.TrimSpace(string(fioOut)), ";")
			if len(fields) > 48 {
				bench.RandomReadIOPS, _ = strconv.ParseFloat(fields[48], 64)
				bwKB, _ := strconv.ParseFloat(fields[47], 64)
				bench.RandomReadMBs = bwKB / 1024.0
			}
		}
		os.Remove(fioTest)
	}

	return bench
}

func runNetworkBenchmark(ctx context.Context) NetworkBenchmark {
	bench := NetworkBenchmark{}

	// Download speed test (100MB from Google)
	out, err := exec.CommandContext(ctx, "curl", "-o", "/dev/null", "-s",
		"-w", "%{speed_download}",
		"https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb").Output()

	if err == nil {
		speedBps, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		bench.DownloadMBs = (speedBps / 1024.0) / 1024.0 // bytes/sec -> MB/s
	}

	// Ping test
	pingOut, err := exec.CommandContext(ctx, "ping", "-c", "10", "-q", "8.8.8.8").Output()
	if err == nil {
		pingStr := string(pingOut)
		for _, line := range strings.Split(pingStr, "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "avg") && strings.Contains(line, "/") {
				// rtt min/avg/max/mdev = 0.123/0.456/0.789/0.012 ms
				parts := strings.Split(line, "=")
				if len(parts) == 2 {
					vals := strings.Split(strings.TrimSpace(parts[1]), "/")
					if len(vals) >= 3 {
						bench.PingMsMin, _ = strconv.ParseFloat(vals[0], 64)
						bench.PingMsAvg, _ = strconv.ParseFloat(vals[1], 64)
						bench.PingMsMax, _ = strconv.ParseFloat(vals[2], 64)
					}
				}
			} else if strings.Contains(line, "packet loss") {
				parts := strings.Split(line, ",")
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if strings.HasSuffix(p, "packet loss") {
						bench.PacketLoss = strings.TrimSpace(strings.TrimSuffix(p, "packet loss"))
					}
				}
			}
		}
	}

	return bench
}
