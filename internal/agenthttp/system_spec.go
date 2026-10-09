package agenthttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
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
	Cores        int    `json:"cores"`
	Threads      int    `json:"threads"`
	Sockets      int    `json:"sockets"`
	Architecture string `json:"architecture"`
	MaxMHz       string `json:"max_mhz,omitempty"`
	CacheSize    string `json:"cache_size,omitempty"`
}

// GPUInfo describes detected graphics/compute devices. Usage is populated when
// nvidia-smi is available; display-only or virtual GPUs may expose no usage API.
type GPUInfo struct {
	Detected       bool    `json:"detected"`
	Count          int     `json:"count"`
	Name           string  `json:"name,omitempty"`
	UsagePercent   float64 `json:"usage_percent,omitempty"`
	UsageAvailable bool    `json:"usage_available"`
}

type MemoryInfo struct {
	TotalBytes int64 `json:"total_bytes"`
	FreeBytes  int64 `json:"free_bytes"`
	UsedBytes  int64 `json:"used_bytes"`
	SwapTotal  int64 `json:"swap_total"`
	SwapUsed   int64 `json:"swap_used"`
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
	UsedBytes          int64  `json:"used_bytes,omitempty"`
	FreeBytes          int64  `json:"free_bytes,omitempty"`
}

type NetworkInfo struct {
	Interface string   `json:"interface"`
	IPAddress string   `json:"ip_address"`
	IPv6Addrs []string `json:"ipv6_addresses,omitempty"`
	Status    string   `json:"status"`
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
func (r *Router) systemSpecTask(c echo.Context) error {
	spec := SystemSpec{
		Uptime:        getUptime(),
		Hostname:      getHostname(),
		KernelVersion: getKernelVersion(),
	}

	spec.OS = getOSInfo()
	spec.CPU = getCPUInfo()
	spec.GPU = getGPUInfo()
	spec.Memory = getMemoryInfo()
	spec.Disk = getDiskInfo()
	spec.Network = getNetworkInfo()
	spec.Cloud = detectCloudProvider()

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      spec,
		RequestID: "",
	})
}

// systemBenchmarkTask runs CPU, disk, and network benchmarks.
func (r *Router) systemBenchmarkTask(c echo.Context) error {
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

// --- Spec helpers ---

func getUptime() string {
	out, err := exec.Command("uptime", "-p").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	out, err = exec.Command("uptime").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return "unknown"
}

func getHostname() string {
	out, err := exec.Command("hostname").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return "unknown"
}

func getKernelVersion() string {
	out, err := exec.Command("uname", "-r").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return "unknown"
}

func getOSInfo() OSInfo {
	info := OSInfo{
		Arch: runtime.GOARCH,
	}
	// Parse /etc/os-release
	out, err := exec.Command("cat", "/etc/os-release").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				info.PrettyName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			} else if strings.HasPrefix(line, "NAME=") {
				info.Name = strings.Trim(strings.TrimPrefix(line, "NAME="), "\"")
			} else if strings.HasPrefix(line, "VERSION_ID=") {
				info.Version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
			}
		}
	}
	if info.PrettyName == "" {
		out2, err := exec.Command("uname", "-sr").Output()
		if err == nil {
			info.PrettyName = strings.TrimSpace(string(out2))
		}
	}
	return info
}

func getCPUInfo() CPUInfo {
	info := CPUInfo{
		Architecture: runtime.GOARCH,
	}
	coresPerSocket := 0
	out, err := exec.Command("lscpu").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Model name:") {
				info.ModelName = strings.TrimSpace(strings.TrimPrefix(line, "Model name:"))
			} else if strings.HasPrefix(line, "Vendor ID:") {
				info.VendorID = strings.TrimSpace(strings.TrimPrefix(line, "Vendor ID:"))
			} else if strings.HasPrefix(line, "CPU(s):") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "CPU(s):"))
				info.Threads, _ = strconv.Atoi(val)
			} else if strings.HasPrefix(line, "Core(s) per socket:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "Core(s) per socket:"))
				coresPerSocket, _ = strconv.Atoi(val)
				info.Cores = coresPerSocket
			} else if strings.HasPrefix(line, "Socket(s):") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "Socket(s):"))
				info.Sockets, _ = strconv.Atoi(val)
				if coresPerSocket > 0 {
					info.Cores = coresPerSocket * info.Sockets
				}
			} else if strings.HasPrefix(line, "Architecture:") {
				info.Architecture = strings.TrimSpace(strings.TrimPrefix(line, "Architecture:"))
			} else if strings.HasPrefix(line, "CPU max MHz:") {
				info.MaxMHz = strings.TrimSpace(strings.TrimPrefix(line, "CPU max MHz:"))
			} else if strings.HasPrefix(line, "L2 cache:") || strings.HasPrefix(line, "L3 cache:") {
				if info.CacheSize == "" {
					info.CacheSize = strings.TrimSpace(strings.TrimPrefix(line, "L2 cache:"))
					info.CacheSize = strings.TrimSpace(strings.TrimPrefix(line, "L3 cache:"))
				}
			}
		}
	}
	return info
}

func getGPUInfo() GPUInfo {
	info := GPUInfo{}
	// NVIDIA: one CSV row per GPU, with live utilization when driver exposes it.
	if out, err := exec.Command("nvidia-smi", "--query-gpu=name,utilization.gpu", "--format=csv,noheader,nounits").Output(); err == nil {
		for _, raw := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			parts := strings.SplitN(raw, ",", 2)
			if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
				continue
			}
			info.Count++
			info.Detected = true
			if info.Name == "" {
				info.Name = strings.TrimSpace(parts[0])
			}
			if len(parts) == 2 {
				if usage, parseErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); parseErr == nil {
					info.UsagePercent += usage
					info.UsageAvailable = true
				}
			}
		}
		if info.Count > 0 && info.UsageAvailable {
			info.UsagePercent /= float64(info.Count)
		}
		if info.Detected {
			return info
		}
	}

	// Fallback inventory for AMD/Intel/virtual display devices.
	if out, err := exec.Command("lspci").Output(); err == nil {
		for _, raw := range strings.Split(string(out), "\n") {
			line := strings.TrimSpace(raw)
			lower := strings.ToLower(line)
			if !strings.Contains(lower, "vga compatible controller") && !strings.Contains(lower, "3d controller") && !strings.Contains(lower, "display controller") {
				continue
			}
			info.Count++
			info.Detected = true
			if info.Name == "" {
				if idx := strings.Index(line, ": "); idx >= 0 {
					info.Name = strings.TrimSpace(line[idx+2:])
				} else {
					info.Name = line
				}
			}
		}
	}
	return info
}

func getMemoryInfo() MemoryInfo {
	info := MemoryInfo{}
	out, err := exec.Command("cat", "/proc/meminfo").Output()
	if err != nil {
		return info
	}
	var memTotal, memAvail, swapTotal, swapFree int64
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		val, _ := strconv.ParseInt(parts[1], 10, 64)
		val *= 1024 // convert from kB to bytes
		if strings.HasPrefix(line, "MemTotal:") {
			memTotal = val
		} else if strings.HasPrefix(line, "MemAvailable:") {
			memAvail = val
		} else if strings.HasPrefix(line, "SwapTotal:") {
			swapTotal = val
		} else if strings.HasPrefix(line, "SwapFree:") {
			swapFree = val
		}
	}
	info.TotalBytes = memTotal
	info.FreeBytes = memAvail
	info.UsedBytes = memTotal - memAvail
	info.SwapTotal = swapTotal
	info.SwapUsed = swapTotal - swapFree
	return info
}

func getDiskInfo() []DiskInfo {
	var disks []DiskInfo

	// Key/value output preserves MODEL/VENDOR fields containing spaces.
	out2, err2 := exec.Command("lsblk", "-P", "-b", "-d", "-o", "NAME,SIZE,TYPE,ROTA,TRAN,MODEL,VENDOR,SERIAL,WWN").Output()
	if err2 == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out2)), "\n") {
			fields := parseLsblkKV(line)
			if fields["NAME"] == "" || fields["TYPE"] == "" {
				continue
			}
			rotational, known := parseRotational(fields["ROTA"])
			classification := classifyDisk(fields["NAME"], fields["TRAN"], fields["MODEL"], fields["VENDOR"], rotational, known)
			disk := DiskInfo{
				Name:               "/dev/" + fields["NAME"],
				Type:               fields["TYPE"],
				DiskType:           classification.Type,
				DiskTypeConfidence: classification.Confidence,
				DetectionReason:    classification.Reason,
				Transport:          fields["TRAN"],
				Vendor:             fields["VENDOR"],
				Serial:             fields["SERIAL"],
				Virtual:            classification.Virtual,
				Model:              fields["MODEL"],
			}
			if known {
				disk.Rotational = &rotational
			}
			disk.SizeBytes, _ = strconv.ParseInt(fields["SIZE"], 10, 64)
			disks = append(disks, disk)
		}
	}

	// Add filesystem usage without changing device classification.
	out, err := exec.Command("df", "-B1", "-t", "ext4,xfs,btrfs,ntfs,vfat,fuseblk,overlay,zfs").Output()
	if err == nil {
		for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if i == 0 {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 6 {
				continue
			}
			used, _ := strconv.ParseInt(fields[2], 10, 64)
			free, _ := strconv.ParseInt(fields[3], 10, 64)
			size, _ := strconv.ParseInt(fields[1], 10, 64)
			found := false
			for j := range disks {
				if sameBlockDevice(disks[j].Name, fields[0]) {
					disks[j].Mounted, disks[j].MountPoint = true, fields[5]
					disks[j].UsedBytes, disks[j].FreeBytes, disks[j].SizeBytes = used, free, size
					found = true
					break
				}
			}
			if !found {
				disks = append(disks, DiskInfo{Name: fields[0], SizeBytes: size, Mounted: true, MountPoint: fields[5], UsedBytes: used, FreeBytes: free, DiskType: "Unknown", DiskTypeConfidence: "unknown", DetectionReason: "filesystem source not present in lsblk inventory"})
			}
		}
	}
	return disks
}

func parseLsblkKV(line string) map[string]string {
	result := map[string]string{}
	for len(strings.TrimSpace(line)) > 0 {
		line = strings.TrimSpace(line)
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			break
		}
		key := line[:eq]
		line = line[eq+1:]
		if strings.HasPrefix(line, `"`) {
			line = line[1:]
			end := strings.IndexByte(line, '"')
			if end < 0 {
				result[key] = line
				break
			}
			result[key] = line[:end]
			line = line[end+1:]
		} else {
			end := strings.IndexByte(line, ' ')
			if end < 0 {
				result[key] = line
				break
			}
			result[key] = line[:end]
			line = line[end:]
		}
	}
	return result
}

func parseRotational(value string) (bool, bool) {
	return value == "1", value == "0" || value == "1"
}

func sameBlockDevice(inventoryName, filesystemDevice string) bool {
	if inventoryName == filesystemDevice {
		return true
	}
	return strings.TrimPrefix(inventoryName, "/dev/") == strings.TrimPrefix(filesystemDevice, "/dev/")
}

func getNetworkInfo() []NetworkInfo {
	var nets []NetworkInfo
	out, err := exec.Command("ip", "-br", "addr").Output()
	if err != nil {
		return nets
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		iface := fields[0]
		if iface == "lo" {
			continue
		}
		status := "down"
		if len(fields) > 1 {
			status = fields[1]
		}
		ip := ""
		if len(fields) > 2 {
			ip = fields[2]
		}
		var ipv6s []string
		if len(fields) > 3 {
			for _, f := range fields[3:] {
				if strings.Contains(f, ":") && strings.Contains(f, "/") {
					ipv6s = append(ipv6s, f)
				}
			}
		}
		nets = append(nets, NetworkInfo{
			Interface: iface,
			IPAddress: ip,
			IPv6Addrs: ipv6s,
			Status:    status,
		})
	}
	return nets
}

func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	multiplier := int64(1)
	if strings.HasSuffix(s, "T") {
		multiplier = 1024 * 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "T")
	} else if strings.HasSuffix(s, "G") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "G")
	} else if strings.HasSuffix(s, "M") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "M")
	} else if strings.HasSuffix(s, "K") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "K")
	}
	val, _ := strconv.ParseFloat(s, 64)
	return int64(val * float64(multiplier))
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

// detectCloudProvider identifies the cloud provider and instance details.
func detectCloudProvider() *CloudInfo {
	cloud := &CloudInfo{}

	// 1. CheckDMI for cloud-specific SMBIOS/DMI data (works on most clouds)
	dmiVendor := execDMIBIOSVendor()
	dmiProductName := execDMIProductName()
	dmiSystemVendor := execDMISystemVendor()

	// 2. Check cloud-init (available on most clouds)
	cloudInitData := execCloudInitQuery()

	// 3. Check specific cloud metadata APIs (169.254.169.254)
	// Most cloud providers use this metadata endpoint

	// === AWS ===
	if dmiVendor == "Amazon EC2" || strings.Contains(dmiProductName, "amazon") || strings.Contains(strings.ToLower(dmiSystemVendor), "amazon") {
		cloud.Provider = "AWS"
		cloud.InstanceID = cloudInitData["instance_id"]
		cloud.Region = cloudInitData["availability_zone"]
		cloud.InstanceType = cloudInitData["instance_type"]
		cloud.Zone = cloudInitData["availability_zone"]
	} else if dmiVendor == "Google Cloud" || strings.Contains(dmiProductName, "google") || strings.Contains(strings.ToLower(dmiSystemVendor), "google") {
		cloud.Provider = "GCP"
		cloud.InstanceID = cloudInitData["instance_id"]
		cloud.Zone = cloudInitData["availability_zone"]
		cloud.InstanceType = cloudInitData["instance_type"]
		cloud.Region = cloudInitData["region"]
	} else if dmiVendor == "DigitalOcean" || strings.Contains(strings.ToLower(dmiProductName), "digitalocean") {
		cloud.Provider = "DigitalOcean"
		cloud.InstanceID = cloudInitData["instance_id"]
		cloud.Region = cloudInitData["region"]
		cloud.InstanceType = cloudInitData["instance_type"]
	} else if dmiVendor == "Linode" || strings.Contains(strings.ToLower(dmiSystemVendor), "linode") || strings.Contains(strings.ToLower(dmiProductName), "linode") {
		cloud.Provider = "Linode"
		cloud.InstanceID = cloudInitData["instance_id"]
		cloud.Region = cloudInitData["region"]
		cloud.InstanceType = cloudInitData["instance_type"]
	} else if dmiVendor == "Vultr" || strings.Contains(strings.ToLower(dmiSystemVendor), "vultr") {
		cloud.Provider = "Vultr"
		cloud.InstanceID = cloudInitData["instance_id"]
		cloud.Region = cloudInitData["region"]
		cloud.InstanceType = cloudInitData["instance_type"]
	} else if dmiVendor == "Hetzner" || strings.Contains(strings.ToLower(dmiSystemVendor), "hetzner") {
		cloud.Provider = "Hetzner"
		cloud.InstanceID = cloudInitData["instance_id"]
		cloud.InstanceType = cloudInitData["instance_type"]
	} else if strings.Contains(strings.ToLower(dmiSystemVendor), "oracle") || strings.Contains(strings.ToLower(dmiProductName), "oracle") {
		cloud.Provider = "Oracle Cloud"
		cloud.InstanceID = cloudInitData["instance_id"]
		cloud.Zone = cloudInitData["availability_zone"]
		cloud.Region = cloudInitData["region"]
		cloud.InstanceType = cloudInitData["instance_type"]
	} else if strings.Contains(strings.ToLower(dmiSystemVendor), "microsoft") || strings.Contains(strings.ToLower(dmiProductName), "virtual machine") || strings.Contains(strings.ToLower(dmiProductName), "hyper-v") {
		cloud.Provider = "Azure"
		cloud.InstanceID = cloudInitData["instance_id"]
		cloud.Region = cloudInitData["region"]
		cloud.InstanceType = cloudInitData["instance_type"]
	} else if dmiVendor == "Alibaba Cloud" || strings.Contains(strings.ToLower(dmiSystemVendor), "alibaba") {
		cloud.Provider = "Alibaba Cloud"
		cloud.InstanceID = cloudInitData["instance_id"]
		cloud.Region = cloudInitData["region"]
		cloud.InstanceType = cloudInitData["instance_type"]
	} else if dmiVendor == "Vultr" || strings.Contains(strings.ToLower(dmiSystemVendor), "vultr") {
		cloud.Provider = "Vultr"
		cloud.InstanceID = cloudInitData["instance_id"]
	} else {
		// Check if QEMU/KVM (might be a VPS on a smaller provider)
		if strings.Contains(strings.ToLower(dmiProductName), "qemu") || strings.Contains(strings.ToLower(dmiVendor), "qemu") {
			// Could be a VPS provider - check /etc/os-release or other hints
			cloud.Provider = "VPS (KVM/QEMU)"
		} else {
			return nil // Not a cloud/VM
		}
	}

	// Get public IP
	publicIP := getPublicIP()
	if publicIP != "" {
		cloud.PublicIP = publicIP
	}

	return cloud
}

// execDMIBIOSVendor reads the DMI BIOS vendor.
func execDMIBIOSVendor() string {
	out, err := exec.Command("cat", "/sys/class/dmi/id/bios_vendor").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// execDMIProductName reads the DMI product name.
func execDMIProductName() string {
	out, err := exec.Command("cat", "/sys/class/dmi/id/product_name").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// execDMISystemVendor reads the DMI system vendor.
func execDMISystemVendor() string {
	out, err := exec.Command("cat", "/sys/class/dmi/id/sys_vendor").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// execCloudInitQuery queries cloud-init for instance data.
func execCloudInitQuery() map[string]string {
	result := make(map[string]string)

	// cloud-init query
	out, err := exec.Command("cloud-init", "query", "--format", "json").Output()
	if err == nil {
		var data map[string]any
		if jsonErr := json.Unmarshal(out, &data); jsonErr == nil {
			if ds, ok := data["ds"].(map[string]any); ok {
				if meta, ok := ds["meta_data"].(map[string]any); ok {
					if v, ok := meta["instance-id"].(string); ok {
						result["instance_id"] = v
					}
					if v, ok := meta["instance-type"].(string); ok {
						result["instance_type"] = v
					}
					if v, ok := meta["availability-zone"].(string); ok {
						result["availability_zone"] = v
					}
					if v, ok := meta["region"].(string); ok {
						result["region"] = v
					}
				}
			}
		}
	}

	// Fallback: check /var/lib/cloud/data/instance
	if result["instance_id"] == "" {
		if out, err := exec.Command("cat", "/var/lib/cloud/data/instance").Output(); err == nil {
			result["instance_id"] = strings.TrimSpace(string(out))
		}
	}

	return result
}

// getPublicIP gets the public IP address.
func getPublicIP() string {
	// Try multiple sources
	sources := []string{
		"curl -s --connect-timeout 3 ifconfig.me",
		"curl -s --connect-timeout 3 icanhazip.com",
		"curl -s --connect-timeout 3 api.ipify.org",
	}
	for _, cmd := range sources {
		out, err := exec.Command("sh", "-c", cmd).Output()
		if err == nil {
			ip := strings.TrimSpace(string(out))
			if ip != "" && strings.Count(ip, ".") == 3 {
				return ip
			}
		}
	}
	return ""
}

type diskClassification struct {
	Type       string
	Confidence string
	Reason     string
	Virtual    bool
}

// classifyDisk refuses to infer physical media behind a virtual block device.
// Guest ROTA is a hint about the virtual device, not proof of host storage.
func classifyDisk(device, transport, model, vendor string, rotational, rotationalKnown bool) diskClassification {
	lower := strings.ToLower(strings.Join([]string{device, transport, model, vendor}, " "))
	if strings.HasPrefix(strings.ToLower(device), "nvme") || strings.Contains(lower, "nvme") {
		return diskClassification{"NVMe", "high", "NVMe device/model identified", false}
	}
	virtual := transport == "virtio" || strings.Contains(lower, "qemu") || strings.Contains(lower, "virtio") || strings.Contains(lower, "virtual") || strings.Contains(lower, "0x1af4")
	if virtual {
		reason := "virtual block device; physical backend type hidden"
		if rotationalKnown {
			reason += fmt.Sprintf("; guest rotational=%t", rotational)
		}
		return diskClassification{"Virtual", "unknown", reason, true}
	}
	if strings.Contains(lower, "solid state") || strings.Contains(lower, "ssd") {
		return diskClassification{"SSD", "medium", "model identifies solid-state media", false}
	}
	if strings.Contains(lower, "hard disk") || strings.Contains(lower, "hdd") {
		return diskClassification{"HDD", "medium", "model identifies hard-disk media", false}
	}
	if rotationalKnown && rotational {
		return diskClassification{"HDD", "low", "kernel rotational flag=1", false}
	}
	if rotationalKnown && !rotational {
		return diskClassification{"SSD", "low", "kernel non-rotational flag=0", false}
	}
	return diskClassification{"Unknown", "unknown", "insufficient device metadata", false}
}

// detectDiskType retained for compatibility with older tests/callers.
func detectDiskType(deviceName string, fields []string) string {
	model := ""
	if len(fields) > 4 {
		model = strings.Join(fields[4:], " ")
	}
	rotational, known := false, false
	if len(fields) > 3 {
		rotational, known = parseRotational(fields[3])
	}
	return classifyDisk(deviceName, "", model, "", rotational, known).Type
}
