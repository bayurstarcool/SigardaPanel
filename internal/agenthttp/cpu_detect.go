package agenthttp

// CPU detection. The kernel's own view comes first (/sys topology, cache
// and cpufreq), because lscpu's text changes between versions (flat
// "L3 cache:" vs. the tree format's "L3:", "Core(s) per cluster" on ARM,
// "-" for unknown values) and may be missing on minimal images. lscpu
// (JSON when available) and /proc/cpuinfo fill in names and identifiers.

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// sysRoot prefixes /sys and /proc paths (tests point it at a fake tree).
var sysRoot = ""

func sysRead(p string) string { return readSpecFile(sysRoot + p) }

func sysGlob(pattern string) []string {
	m, _ := filepath.Glob(sysRoot + pattern)
	for i := range m {
		m[i] = strings.TrimPrefix(m[i], sysRoot)
	}
	sort.Slice(m, func(i, j int) bool { return cpuNum(m[i]) < cpuNum(m[j]) })
	return m
}

var cpuDirRe = regexp.MustCompile(`/cpu(\d+)(/|$)`)

func cpuNum(p string) int {
	if m := cpuDirRe.FindStringSubmatch(p); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

func getCPUInfo() CPUInfo {
	lscpuJSON, _ := runSpecCmd(10*time.Second, "lscpu", "-J")
	kv := parseLscpuJSON(lscpuJSON)
	if len(kv) == 0 {
		text, _ := runSpecCmd(10*time.Second, "lscpu")
		kv = parseLscpuText(text)
	}
	info := buildCPU(kv, sysRead("/proc/cpuinfo"), runtime.NumCPU())
	applySysfsCPU(&info)
	if info.ModelName == "" || info.ModelName == "-" {
		// ARM boards often only name the core in DMI.
		if out, err := runSpecCmd(5*time.Second, "dmidecode", "-s", "processor-version"); err == nil {
			if v := firstNonEmptyLine(out); v != "" && !strings.EqualFold(v, "Not Specified") {
				info.ModelName = v
			}
		}
	}
	info.CPUQuota = cgroupCPUQuota()
	finishCPU(&info)
	return info
}

// parseLscpu is kept for callers that have lscpu's text output.
func parseCPU(lscpu, cpuinfo string, numCPU int) CPUInfo {
	info := buildCPU(parseLscpuText(lscpu), cpuinfo, numCPU)
	finishCPU(&info)
	return info
}

// parseLscpuJSON flattens `lscpu -J` (util-linux ≥ 2.30; nested
// "children" since 2.37) into field → value.
func parseLscpuJSON(out string) map[string]string {
	var doc struct {
		Lscpu []lscpuEntry `json:"lscpu"`
	}
	kv := map[string]string{}
	if json.Unmarshal([]byte(out), &doc) != nil {
		return kv
	}
	var walk func([]lscpuEntry)
	walk = func(es []lscpuEntry) {
		for _, e := range es {
			k := strings.TrimSuffix(strings.TrimSpace(e.Field), ":")
			if _, seen := kv[k]; !seen && k != "" {
				kv[k] = strings.TrimSpace(fmt.Sprint(e.Data))
				if e.Data == nil {
					kv[k] = ""
				}
			}
			walk(e.Children)
		}
	}
	walk(doc.Lscpu)
	return normalizeLscpu(kv)
}

type lscpuEntry struct {
	Field    string       `json:"field"`
	Data     any          `json:"data"`
	Children []lscpuEntry `json:"children"`
}

// parseLscpuText reads both the flat and the tree (indented) text formats.
func parseLscpuText(out string) map[string]string {
	kv := map[string]string{}
	last := ""
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(k) == "" {
			// Wrapped continuation of a long value (Flags in the tree format).
			if last != "" && strings.HasPrefix(line, "   ") {
				kv[last] += strings.TrimSpace(line)
			}
			continue
		}
		k = strings.TrimSpace(k)
		if _, seen := kv[k]; !seen {
			kv[k] = strings.TrimSpace(v)
			last = k
		} else {
			last = ""
		}
	}
	return normalizeLscpu(kv)
}

// normalizeLscpu maps the tree format's short cache names to the flat ones
// and drops "-" (unknown).
func normalizeLscpu(kv map[string]string) map[string]string {
	for _, l := range []string{"L1d", "L1i", "L2", "L3", "L4"} {
		if v, ok := kv[l]; ok {
			if _, has := kv[l+" cache"]; !has {
				kv[l+" cache"] = v
			}
		}
	}
	for k, v := range kv {
		if v == "-" {
			delete(kv, k)
		}
	}
	return kv
}

var cpuGHz = regexp.MustCompile(`@\s*([0-9.]+)\s*GHz`)

func buildCPU(kv map[string]string, cpuinfo string, numCPU int) CPUInfo {
	num := func(k string) int { n, _ := strconv.Atoi(strings.TrimSpace(kv[k])); return n }
	info := CPUInfo{
		ModelName:    kv["Model name"],
		VendorID:     kv["Vendor ID"],
		Architecture: kv["Architecture"],
		Threads:      num("CPU(s)"),
		Sockets:      num("Socket(s)"),
		Hypervisor:   kv["Hypervisor vendor"],
		Family:       kv["CPU family"],
		Model:        kv["Model"],
		Stepping:     kv["Stepping"],
		BogoMIPS:     kv["BogoMIPS"],
		Source:       "lscpu",
	}
	info.ThreadsPerCore = num("Thread(s) per core")
	info.CoresPerSocket = num("Core(s) per socket")
	if info.CoresPerSocket == 0 {
		info.CoresPerSocket = num("Core(s) per cluster")
	}
	if info.Sockets == 0 {
		info.Sockets = num("Cluster(s)")
	}
	if info.CoresPerSocket > 0 {
		info.Cores = info.CoresPerSocket * max(info.Sockets, 1)
	}
	info.NUMANodes = num("NUMA node(s)")

	ci := parseProcCPUInfo(cpuinfo)
	if info.ModelName == "" {
		info.ModelName = ci.model
	}
	if info.VendorID == "" {
		info.VendorID = ci.vendor
	}
	if info.Family == "" {
		info.Family = ci.family
	}
	if info.Model == "" {
		info.Model = ci.modelID
	}
	if info.Stepping == "" {
		info.Stepping = ci.stepping
	}
	if info.BogoMIPS == "" {
		info.BogoMIPS = ci.bogomips
	}
	info.Microcode = ci.microcode
	if info.Threads == 0 {
		info.Threads = ci.processors
	}
	if info.Threads == 0 {
		info.Threads = numCPU
	}
	if info.Cores == 0 && ci.cores > 0 {
		info.Cores = ci.cores
		info.Source = "/proc/cpuinfo"
	}
	if info.Sockets == 0 {
		info.Sockets = ci.sockets
	}
	info.Features, info.VirtSupport = cpuFeatures(ci.flags)

	// Clocks from lscpu / cpuinfo (sysfs cpufreq overrides on bare metal).
	if v := parseFloat(kv["CPU max MHz"]); v > 0 {
		info.MaxTurboMHz = mhz(v)
	}
	if v := parseFloat(kv["CPU min MHz"]); v > 0 {
		info.MinMHz = mhz(v)
	}
	cur := parseFloat(kv["CPU MHz"])
	if cur == 0 {
		cur = ci.mhz
	}
	if cur > 0 {
		info.CurrentMHz = mhz(cur)
	}
	if m := cpuGHz.FindStringSubmatch(info.ModelName); m != nil {
		info.BaseMHz = mhz(parseFloat(m[1]) * 1000)
	}

	// Caches from lscpu (sysfs replaces them when it has them).
	for _, l := range []string{"L1d", "L1i", "L2", "L3", "L4"} {
		if v := kv[l+" cache"]; v != "" {
			info.Caches = append(info.Caches, cacheFromLscpu(l, v, info.Sockets))
		}
	}
	if len(info.Caches) == 0 && ci.cache != "" {
		info.Caches = append(info.Caches, CacheInfo{Level: "L2/L3", Size: ci.cache, Instances: 1, Total: ci.cache})
	}
	return info
}

var cacheInst = regexp.MustCompile(`^(.*?)\s*\((\d+)\s+instances?\)$`)

// cacheFromLscpu reads "8 MiB (4 instances)" (sum) or an older per-core
// value like "1024K".
func cacheFromLscpu(level, v string, sockets int) CacheInfo {
	c := CacheInfo{Level: level, Total: v, Instances: 1}
	if m := cacheInst.FindStringSubmatch(v); m != nil {
		c.Total = m[1]
		c.Instances, _ = strconv.Atoi(m[2])
		if b := parseSizeIEC(m[1]); b > 0 && c.Instances > 0 {
			c.Size = fmtIEC(b / int64(c.Instances))
		}
	} else if b := parseSizeIEC(v); b > 0 {
		// Old lscpu prints the size of one cache.
		c.Size, c.Total = fmtIEC(b), fmtIEC(b)
	}
	if c.Size == "" {
		c.Size = c.Total
	}
	return c
}

type procCPU struct {
	model, vendor, cache, family, modelID, stepping, microcode, bogomips string
	processors, cores, sockets                                           int
	mhz                                                                  float64
	flags                                                                []string
}

func parseProcCPUInfo(text string) procCPU {
	var p procCPU
	physical := map[string]bool{}
	coreIDs := map[string]bool{}
	cur := ""
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "processor":
			p.processors++
		case "model name", "Hardware", "cpu model", "uarch":
			if p.model == "" {
				p.model = v
			}
		case "Model": // Raspberry Pi and other boards
			if p.model == "" && !isNumber(v) {
				p.model = v
			}
		case "vendor_id", "CPU implementer":
			if p.vendor == "" {
				p.vendor = v
			}
		case "cpu family", "CPU architecture":
			if p.family == "" {
				p.family = v
			}
		case "model", "CPU part":
			if p.modelID == "" {
				p.modelID = v
			}
		case "stepping", "CPU revision":
			if p.stepping == "" {
				p.stepping = v
			}
		case "microcode":
			if p.microcode == "" {
				p.microcode = v
			}
		case "bogomips", "BogoMIPS":
			if p.bogomips == "" {
				p.bogomips = v
			}
		case "cpu MHz":
			if p.mhz == 0 {
				p.mhz = parseFloat(v)
			}
		case "cache size":
			if p.cache == "" {
				p.cache = v
			}
		case "flags", "Features":
			if p.flags == nil {
				p.flags = strings.Fields(v)
			}
		case "physical id":
			cur = v
			physical[v] = true
		case "core id":
			coreIDs[cur+"/"+v] = true
		}
	}
	p.sockets = len(physical)
	p.cores = len(coreIDs)
	if v, ok := armImplementers[strings.ToLower(p.vendor)]; ok {
		p.vendor = v
	}
	return p
}

var armImplementers = map[string]string{"0x41": "ARM", "0x42": "Broadcom", "0x43": "Cavium", "0x46": "Fujitsu", "0x48": "HiSilicon",
	"0x4e": "NVIDIA", "0x50": "APM", "0x51": "Qualcomm", "0x53": "Samsung", "0x61": "Apple", "0xc0": "Ampere"}

func isNumber(s string) bool { _, err := strconv.Atoi(strings.TrimSpace(s)); return err == nil }

// cpuFeatures picks the extensions people look for on a server.
func cpuFeatures(flags []string) ([]string, string) {
	set := map[string]bool{}
	for _, f := range flags {
		set[strings.ToLower(f)] = true
	}
	var out []string
	add := func(flag, label string) {
		if set[flag] {
			out = append(out, label)
		}
	}
	// x86
	add("sse4_2", "SSE4.2")
	add("avx", "AVX")
	add("avx2", "AVX2")
	add("avx512f", "AVX-512")
	add("amx_tile", "AMX")
	add("aes", "AES-NI")
	add("sha_ni", "SHA")
	add("rdrand", "RDRAND")
	// ARM
	add("asimd", "NEON")
	add("sve", "SVE")
	add("sve2", "SVE2")
	add("sha2", "SHA2")
	add("atomics", "LSE atomics")
	if set["pmull"] && set["aes"] && !set["sse4_2"] {
		// ARM "aes" was labelled AES-NI above; rename for ARM.
		for i, f := range out {
			if f == "AES-NI" {
				out[i] = "AES"
			}
		}
	}
	virt := ""
	switch {
	case set["vmx"]:
		virt = "VT-x"
	case set["svm"]:
		virt = "AMD-V"
	}
	return out, virt
}

// applySysfsCPU fills topology, caches and clocks from /sys.
func applySysfsCPU(info *CPUInfo) {
	cpus := sysGlob("/sys/devices/system/cpu/cpu[0-9]*")
	if len(cpus) == 0 {
		return
	}
	online, offline := 0, 0
	packages := map[string]bool{}
	cores := map[string]bool{}
	threadsPerCore := 0
	type cacheKey struct{ level, typ string }
	cacheSize := map[cacheKey]int64{}
	cacheShared := map[cacheKey]map[string]bool{}
	for _, d := range cpus {
		if v := sysRead(d + "/online"); v == "0" {
			offline++
			continue
		}
		online++
		pkg := sysRead(d + "/topology/physical_package_id")
		core := sysRead(d + "/topology/core_id")
		die := sysRead(d + "/topology/die_id")
		if pkg != "" && pkg != "-1" {
			packages[pkg] = true
		}
		if core != "" {
			cores[pkg+"/"+die+"/"+core] = true
		}
		if sib := sysRead(d + "/topology/thread_siblings_list"); sib != "" {
			if n := countCPUList(sib); n > threadsPerCore {
				threadsPerCore = n
			}
		} else if sib := sysRead(d + "/topology/core_cpus_list"); sib != "" {
			if n := countCPUList(sib); n > threadsPerCore {
				threadsPerCore = n
			}
		}
		for _, idx := range sysGlob(d + "/cache/index[0-9]*") {
			level := sysRead(idx + "/level")
			typ := sysRead(idx + "/type")
			size := parseSizeIEC(sysRead(idx + "/size"))
			if level == "" || size == 0 {
				continue
			}
			k := cacheKey{level, typ}
			cacheSize[k] = size
			if cacheShared[k] == nil {
				cacheShared[k] = map[string]bool{}
			}
			shared := sysRead(idx + "/shared_cpu_list")
			if shared == "" {
				shared = d
			}
			cacheShared[k][shared] = true
		}
	}
	if online > 0 {
		info.Threads = online
		info.OfflineCPUs = offline
		info.Source = "sysfs"
	}
	if len(packages) > 0 {
		info.Sockets = len(packages)
	}
	if len(cores) > 0 {
		info.Cores = len(cores)
	}
	if threadsPerCore > 0 {
		info.ThreadsPerCore = threadsPerCore
	}
	if info.Sockets > 0 && info.Cores > 0 {
		info.CoresPerSocket = info.Cores / info.Sockets
	}
	if n := len(sysGlob("/sys/devices/system/node/node[0-9]*")); n > 0 {
		info.NUMANodes = n
	}
	if len(cacheSize) > 0 {
		var caches []CacheInfo
		for k, size := range cacheSize {
			level := "L" + k.level
			switch k.typ {
			case "Data":
				level += "d"
			case "Instruction":
				level += "i"
			}
			n := len(cacheShared[k])
			caches = append(caches, CacheInfo{Level: level, Size: fmtIEC(size), Instances: n, Total: fmtIEC(size * int64(n))})
		}
		sort.Slice(caches, func(i, j int) bool { return caches[i].Level < caches[j].Level })
		info.Caches = caches
	}
	// cpufreq (bare metal and some clouds; most VMs have none).
	if v := parseFloat(sysRead("/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq")); v > 0 {
		info.MaxTurboMHz = mhz(v / 1000)
	}
	if v := parseFloat(sysRead("/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_min_freq")); v > 0 {
		info.MinMHz = mhz(v / 1000)
	}
	if v := parseFloat(sysRead("/sys/devices/system/cpu/cpu0/cpufreq/base_frequency")); v > 0 {
		info.BaseMHz = mhz(v / 1000)
	}
	if v := parseFloat(sysRead("/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq")); v > 0 {
		info.CurrentMHz = mhz(v / 1000)
	}
	info.Governor = sysRead("/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor")
}

// finishCPU derives the summary fields.
func finishCPU(info *CPUInfo) {
	if info.Architecture == "" {
		info.Architecture = runtime.GOARCH
	}
	if info.Cores == 0 {
		if info.ThreadsPerCore > 0 && info.Threads > 0 {
			info.Cores = info.Threads / info.ThreadsPerCore
		} else {
			info.Cores = info.Threads
		}
	}
	if info.Sockets == 0 {
		info.Sockets = 1
	}
	// Some hypervisors give every vCPU the same core id without sibling
	// information: 4 CPUs would read as "1 core × 1 thread". The CPUs are
	// what counts; a topology that cannot hold them is corrected.
	if info.ThreadsPerCore == 0 && info.Cores > 0 && info.Threads >= info.Cores {
		info.ThreadsPerCore = info.Threads / info.Cores
	}
	if info.Threads > 0 && info.Cores > 0 {
		tpc := max(info.ThreadsPerCore, 1)
		if info.Cores*tpc < info.Threads {
			info.Cores = (info.Threads + tpc - 1) / tpc
			info.CoresPerSocket = 0
		}
		if info.Cores > info.Threads {
			info.Cores = info.Threads
		}
	}
	if info.ThreadsPerCore == 0 && info.Cores > 0 && info.Threads >= info.Cores {
		info.ThreadsPerCore = info.Threads / info.Cores
	}
	if info.CoresPerSocket == 0 && info.Cores > 0 {
		info.CoresPerSocket = info.Cores / info.Sockets
	}
	for _, v := range []string{info.MaxTurboMHz, info.BaseMHz, info.CurrentMHz} {
		if v != "" {
			info.MaxMHz = v
			break
		}
	}
	// The cache summary: the largest level.
	info.CacheSize = ""
	for i := len(info.Caches) - 1; i >= 0; i-- {
		if c := info.Caches[i]; c.Level != "L1d" && c.Level != "L1i" {
			info.CacheSize = c.Total
			break
		}
	}
	info.ModelName = strings.Join(strings.Fields(info.ModelName), " ")
}

func mhz(v float64) string { return strconv.Itoa(int(math.Round(v))) }

// countCPUList counts the CPUs in a list like "0-3,8,10-11".
func countCPUList(s string) int {
	n := 0
	for _, part := range strings.Split(strings.TrimSpace(s), ",") {
		a, b, rng := strings.Cut(part, "-")
		x, err1 := strconv.Atoi(strings.TrimSpace(a))
		if err1 != nil {
			continue
		}
		if !rng {
			n++
			continue
		}
		y, err2 := strconv.Atoi(strings.TrimSpace(b))
		if err2 == nil && y >= x {
			n += y - x + 1
		}
	}
	return n
}

var sizeRe = regexp.MustCompile(`(?i)^\s*([0-9.]+)\s*([KMGT]?)(I?B)?\s*$`)

// parseSizeIEC reads "48K", "1024 KiB", "8 MiB", "35.8 MiB" as bytes.
func parseSizeIEC(s string) int64 {
	m := sizeRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	v := parseFloat(m[1])
	mult := map[string]float64{"": 1, "K": 1 << 10, "M": 1 << 20, "G": 1 << 30, "T": 1 << 40}[strings.ToUpper(m[2])]
	return int64(v * mult)
}

func fmtIEC(b int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	v := float64(b)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if v == math.Trunc(v) {
		return fmt.Sprintf("%d %s", int64(v), units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

// cgroupCPUQuota reports a CPU limit set on the agent's cgroup (containers,
// some VPS products), e.g. "2 CPUs".
func cgroupCPUQuota() string {
	if v := sysRead("/sys/fs/cgroup/cpu.max"); v != "" {
		f := strings.Fields(v)
		if len(f) == 2 && f[0] != "max" {
			q, p := parseFloat(f[0]), parseFloat(f[1])
			if q > 0 && p > 0 {
				return trimFloat(q/p) + " CPUs"
			}
		}
		return ""
	}
	q := parseFloat(sysRead("/sys/fs/cgroup/cpu/cpu.cfs_quota_us"))
	p := parseFloat(sysRead("/sys/fs/cgroup/cpu/cpu.cfs_period_us"))
	if q > 0 && p > 0 {
		return trimFloat(q/p) + " CPUs"
	}
	return ""
}

func trimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}

func firstNonEmptyLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}
