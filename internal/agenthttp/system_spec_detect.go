package agenthttp

// Server specification detection (GET /tasks/system/spec).
//
// Every command runs with LC_ALL=C (lscpu and friends are translated) and a
// timeout. Parsers are pure functions over the command output or file text,
// so they are tested with real outputs from bare metal, KVM, Xen, LXC, ARM.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// runSpecCmd runs a command in the C locale with a timeout (replaced in tests).
var runSpecCmd = func(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.Output()
	return string(out), err
}

// readSpecFile reads a small system file (replaced in tests).
var readSpecFile = func(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// ---------------------------------------------------------------- host basics

func getUptime() string {
	raw := readSpecFile("/proc/uptime")
	f := strings.Fields(raw)
	if len(f) == 0 {
		return "unknown"
	}
	secs, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return "unknown"
	}
	return formatUptime(time.Duration(secs) * time.Second)
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	var parts []string
	if days > 0 {
		parts = append(parts, plural(days, "day"))
	}
	if hours > 0 {
		parts = append(parts, plural(hours, "hour"))
	}
	if mins > 0 || len(parts) == 0 {
		parts = append(parts, plural(mins, "minute"))
	}
	return "up " + strings.Join(parts, ", ")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func getHostname() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}

func getKernelVersion() string {
	if v := readSpecFile("/proc/sys/kernel/osrelease"); v != "" {
		return v
	}
	if out, err := runSpecCmd(5*time.Second, "uname", "-r"); err == nil {
		return strings.TrimSpace(out)
	}
	return "unknown"
}

func getOSInfo() OSInfo {
	info := parseSpecOSRelease(readSpecFile("/etc/os-release"))
	info.Arch = runtime.GOARCH
	if m := readSpecFile("/proc/sys/kernel/arch"); m != "" {
		info.Arch = m // the machine's, not the agent binary's (x86_64, aarch64)
	} else if out, err := runSpecCmd(5*time.Second, "uname", "-m"); err == nil && strings.TrimSpace(out) != "" {
		info.Arch = strings.TrimSpace(out)
	}
	if info.PrettyName == "" {
		if out, err := runSpecCmd(5*time.Second, "uname", "-sr"); err == nil {
			info.PrettyName = strings.TrimSpace(out)
		}
	}
	return info
}

func parseSpecOSRelease(text string) OSInfo {
	var info OSInfo
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "PRETTY_NAME":
			info.PrettyName = v
		case "NAME":
			info.Name = v
		case "VERSION_ID":
			info.Version = v
		}
	}
	return info
}

// ----------------------------------------------------------- virtualization

// detectVirtualization names the hypervisor or container the system runs in
// ("none" on bare metal), like systemd-detect-virt.
func detectVirtualization() string {
	if out, err := runSpecCmd(5*time.Second, "systemd-detect-virt"); strings.TrimSpace(out) != "" {
		_ = err // exit status 1 with "none" on bare metal
		return strings.TrimSpace(out)
	}
	// Containers first: inside one, DMI shows the host.
	if _, err := os.Stat("/proc/vz/veinfo"); err == nil {
		if _, err := os.Stat("/proc/vz/version"); err != nil {
			return "openvz"
		}
	}
	if env := readSpecFile("/proc/1/environ"); strings.Contains(env, "container=lxc") {
		return "lxc"
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}
	return virtFromDMI(readSpecFile("/sys/class/dmi/id/sys_vendor"), readSpecFile("/sys/class/dmi/id/product_name"),
		readSpecFile("/sys/class/dmi/id/bios_vendor"), cpuHasHypervisorFlag())
}

func virtFromDMI(sysVendor, product, bios string, hypervisorFlag bool) string {
	all := strings.ToLower(sysVendor + " " + product + " " + bios)
	switch {
	case strings.Contains(all, "vmware"):
		return "vmware"
	case strings.Contains(all, "virtualbox") || strings.Contains(all, "innotek"):
		return "oracle"
	case strings.Contains(all, "microsoft") && strings.Contains(all, "virtual"):
		return "microsoft"
	case strings.Contains(all, "xen") || strings.Contains(all, "hvm domu"):
		return "xen"
	case strings.Contains(all, "qemu") || strings.Contains(all, "kvm") || strings.Contains(all, "bochs") ||
		strings.Contains(all, "openstack") || strings.Contains(all, "seabios"):
		return "kvm"
	case strings.Contains(all, "amazon ec2"):
		return "amazon"
	case strings.Contains(all, "google"):
		return "kvm"
	}
	if hypervisorFlag {
		return "vm-other"
	}
	if sysVendor == "" && product == "" {
		return ""
	}
	return "none"
}

func cpuHasHypervisorFlag() bool {
	for _, line := range strings.Split(readSpecFile("/proc/cpuinfo"), "\n") {
		if strings.HasPrefix(line, "flags") {
			return strings.Contains(" "+line+" ", " hypervisor ")
		}
	}
	return false
}

func isContainerVirt(v string) bool {
	switch v {
	case "lxc", "lxc-libvirt", "openvz", "docker", "podman", "systemd-nspawn", "rkt", "wsl", "proot", "pouch", "container-other":
		return true
	}
	return false
}

func isVMVirt(v string) bool { return v != "" && v != "none" && !isContainerVirt(v) }

// ---------------------------------------------------------------------- CPU

func getCPUInfo() CPUInfo {
	out, _ := runSpecCmd(10*time.Second, "lscpu")
	info := parseCPU(out, readSpecFile("/proc/cpuinfo"), runtime.NumCPU())
	if info.Architecture == "" {
		info.Architecture = runtime.GOARCH
	}
	return info
}

var cpuGHz = regexp.MustCompile(`@\s*([0-9.]+)\s*GHz`)

// parseCPU combines lscpu (C locale) with /proc/cpuinfo: VMs often lack
// "CPU max MHz", ARM lscpu reports clusters instead of sockets, and minimal
// images have no lscpu at all.
func parseCPU(lscpu, cpuinfo string, numCPU int) CPUInfo {
	kv := map[string]string{}
	for _, line := range strings.Split(lscpu, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if _, seen := kv[k]; !seen {
			kv[k] = strings.TrimSpace(v)
		}
	}
	num := func(k string) int { n, _ := strconv.Atoi(kv[k]); return n }

	info := CPUInfo{
		ModelName:    kv["Model name"],
		VendorID:     kv["Vendor ID"],
		Architecture: kv["Architecture"],
		Threads:      num("CPU(s)"),
		Sockets:      num("Socket(s)"),
		Hypervisor:   kv["Hypervisor vendor"],
	}
	perSocket := num("Core(s) per socket")
	if perSocket == 0 {
		perSocket = num("Core(s) per cluster")
	}
	if info.Sockets == 0 {
		info.Sockets = num("Cluster(s)")
	}
	if perSocket > 0 {
		info.Cores = perSocket * max(info.Sockets, 1)
	}
	if info.Cores == 0 {
		if tpc := num("Thread(s) per core"); tpc > 0 && info.Threads > 0 {
			info.Cores = info.Threads / tpc
		}
	}

	// /proc/cpuinfo fills what lscpu did not give.
	ci := parseProcCPUInfo(cpuinfo)
	if info.ModelName == "" {
		info.ModelName = ci.model
	}
	if info.VendorID == "" {
		info.VendorID = ci.vendor
	}
	if info.Threads == 0 {
		info.Threads = ci.processors
	}
	if info.Threads == 0 {
		info.Threads = numCPU
	}
	if info.Cores == 0 && ci.cores > 0 {
		info.Cores = ci.cores
	}
	if info.Cores == 0 {
		info.Cores = info.Threads
	}
	if info.Sockets == 0 {
		info.Sockets = max(ci.sockets, 1)
	}

	// Clock: the maximum when known, else the current or nominal speed.
	mhz := parseFloat(kv["CPU max MHz"])
	if mhz == 0 {
		mhz = parseFloat(kv["CPU MHz"])
	}
	if mhz == 0 {
		mhz = ci.mhz
	}
	if mhz == 0 {
		if m := cpuGHz.FindStringSubmatch(info.ModelName); m != nil {
			mhz = parseFloat(m[1]) * 1000
		}
	}
	if mhz > 0 {
		info.MaxMHz = strconv.Itoa(int(math.Round(mhz)))
	}

	// Largest cache level, value only ("32 MiB"), never the label.
	for _, k := range []string{"L3 cache", "L2 cache"} {
		if v := kv[k]; v != "" {
			if i := strings.Index(v, " ("); i > 0 {
				v = v[:i]
			}
			info.CacheSize = v
			break
		}
	}
	if info.CacheSize == "" && ci.cache != "" {
		info.CacheSize = ci.cache
	}
	return info
}

type procCPU struct {
	model, vendor, cache string
	processors, cores    int
	sockets              int
	mhz                  float64
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
		case "model name", "Model", "Hardware", "cpu model":
			if p.model == "" {
				p.model = v
			}
		case "vendor_id", "CPU implementer":
			if p.vendor == "" {
				p.vendor = v
			}
		case "cpu MHz":
			if p.mhz == 0 {
				p.mhz = parseFloat(v)
			}
		case "cache size":
			if p.cache == "" {
				p.cache = v
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
	if p.vendor == "0x41" {
		p.vendor = "ARM"
	}
	return p
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// ---------------------------------------------------------------------- GPU

// virtualDisplay matches emulated or management display adapters, which are
// not GPUs: QEMU/KVM (Cirrus, QXL, Bochs/stdvga, virtio), VMware, Hyper-V,
// VirtualBox, Xen, and server BMCs (ASPEED, Matrox G200).
var virtualDisplay = regexp.MustCompile(`(?i)cirrus|qxl|bochs|virtio|vmware|svga ii|hyper-v|virtualbox|vboxvga|innotek|xen|aspeed|matrox|g200|device 1234:1111|red hat, inc\. (?:qxl|virtio)`)

func getGPUInfo() GPUInfo {
	info := GPUInfo{}
	if out, err := runSpecCmd(10*time.Second, "nvidia-smi", "--query-gpu=name,utilization.gpu", "--format=csv,noheader,nounits"); err == nil {
		info = parseNvidiaSMI(out)
		if info.Detected {
			return info
		}
	}
	out, err := runSpecCmd(10*time.Second, "lspci")
	if err != nil {
		return info
	}
	return parseLspciGPU(out)
}

func parseNvidiaSMI(out string) GPUInfo {
	info := GPUInfo{}
	for _, raw := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(raw, ",", 2)
		if strings.TrimSpace(parts[0]) == "" {
			continue
		}
		info.Count++
		info.Detected = true
		if info.Name == "" {
			info.Name = strings.TrimSpace(parts[0])
		}
		if len(parts) == 2 {
			if usage, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
				info.UsagePercent += usage
				info.UsageAvailable = true
			}
		}
	}
	if info.Count > 0 && info.UsageAvailable {
		info.UsagePercent /= float64(info.Count)
	}
	return info
}

func parseLspciGPU(out string) GPUInfo {
	info := GPUInfo{}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "vga compatible controller") && !strings.Contains(lower, "3d controller") && !strings.Contains(lower, "display controller") {
			continue
		}
		name := line
		if i := strings.Index(line, ": "); i >= 0 {
			name = strings.TrimSpace(line[i+2:])
		}
		if virtualDisplay.MatchString(name) {
			if info.Display == "" {
				info.Display = name
			}
			continue
		}
		info.Count++
		info.Detected = true
		if info.Name == "" {
			info.Name = name
		}
	}
	return info
}

// ------------------------------------------------------------------- memory

func getMemoryInfo(virt string) MemoryInfo {
	info := parseMeminfo(readSpecFile("/proc/meminfo"))
	// Inside a container /sys shows the host's memory, not the container's.
	if !isContainerVirt(virt) {
		info.InstalledBytes = installedMemory(info.TotalBytes)
	}
	return info
}

func parseMeminfo(text string) MemoryInfo {
	vals := map[string]int64{}
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) == 0 {
			continue
		}
		n, _ := strconv.ParseInt(f[0], 10, 64)
		vals[strings.TrimSpace(k)] = n * 1024
	}
	avail, ok := vals["MemAvailable"]
	if !ok { // kernels before 3.14
		avail = vals["MemFree"] + vals["Buffers"] + vals["Cached"]
	}
	return MemoryInfo{
		TotalBytes: vals["MemTotal"],
		FreeBytes:  avail,
		UsedBytes:  max(vals["MemTotal"]-avail, 0),
		SwapTotal:  vals["SwapTotal"],
		SwapUsed:   max(vals["SwapTotal"]-vals["SwapFree"], 0),
	}
}

// installedMemory is the RAM the machine has: the online memory blocks in
// /sys (exact on VMs and most bare metal), else MemTotal rounded up to the
// usual sizes. 0 when it cannot be told.
func installedMemory(memTotal int64) int64 {
	blockHex := readSpecFile("/sys/devices/system/memory/block_size_bytes")
	if block, err := strconv.ParseInt(blockHex, 16, 64); err == nil && block > 0 {
		matches, _ := filepath.Glob("/sys/devices/system/memory/memory*/online")
		var online int64
		for _, m := range matches {
			if readSpecFile(m) == "1" {
				online++
			}
		}
		if total := online * block; total >= memTotal && total > 0 {
			return total
		}
	}
	return roundMemory(memTotal)
}

// roundMemory rounds MemTotal (which lacks the kernel's and firmware's
// reservations, a few %) up to a whole GiB, or 256 MiB below 2 GiB.
func roundMemory(memTotal int64) int64 {
	if memTotal <= 0 {
		return 0
	}
	step := int64(1 << 30)
	if memTotal < 2<<30 {
		step = 256 << 20
	}
	return (memTotal + step - 1) / step * step
}

// --------------------------------------------------------------------- disks

// lsblkRow is one line of `lsblk -P -b -o ...`.
type lsblkRow map[string]string

const lsblkColumns = "NAME,KNAME,PKNAME,TYPE,SIZE,ROTA,TRAN,MODEL,VENDOR,SERIAL,MOUNTPOINT,FSTYPE,DISC-GRAN"

// statfsFn is replaced in tests.
var statfsFn = func(path string) (size, used, free int64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, 0, false
	}
	bs := int64(st.Bsize)
	size = int64(st.Blocks) * bs
	free = int64(st.Bavail) * bs
	used = (int64(st.Blocks) - int64(st.Bfree)) * bs
	return size, used, free, true
}

func getDiskInfo(virt string) []DiskInfo {
	out, err := runSpecCmd(10*time.Second, "lsblk", "-P", "-b", "-o", lsblkColumns)
	if err != nil || strings.TrimSpace(out) == "" {
		// Older util-linux lacks a column: retry without the optional ones.
		out, _ = runSpecCmd(10*time.Second, "lsblk", "-P", "-b", "-o", "NAME,KNAME,PKNAME,TYPE,SIZE,ROTA,TRAN,MODEL,VENDOR,SERIAL,MOUNTPOINT,FSTYPE")
	}
	var rows []lsblkRow
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if r := parseLsblkKV(line); r["NAME"] != "" {
			rows = append(rows, r)
		}
	}
	return buildDisks(rows, readSpecFile("/proc/self/mounts"), virt)
}

// skipDevice drops devices that are not storage: loop devices (snaps),
// zram/ram disks, CD-ROMs, floppies.
func skipDevice(r lsblkRow) bool {
	switch r["TYPE"] {
	case "loop", "rom", "ram":
		return true
	}
	name := r["NAME"]
	for _, p := range []string{"loop", "zram", "ram", "sr", "fd"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// buildDisks groups lsblk's tree (disk → partitions → LVM/crypt) into one
// entry per physical/virtual disk with the filesystems mounted on it, and
// adds the root filesystem when it is not on a block device lsblk knows
// (ZFS, overlay in a container, /dev/root).
func buildDisks(rows []lsblkRow, mounts string, virt string) []DiskInfo {
	parent := map[string]string{}
	byKname := map[string]lsblkRow{}
	for _, r := range rows {
		k := r["KNAME"]
		if k == "" {
			k = r["NAME"]
		}
		byKname[k] = r
		if r["PKNAME"] != "" {
			parent[k] = r["PKNAME"]
		}
	}
	top := func(k string) string {
		for i := 0; i < 16; i++ {
			p, ok := parent[k]
			if !ok || p == "" {
				return k
			}
			k = p
		}
		return k
	}

	var disks []DiskInfo
	index := map[string]int{}
	for _, r := range rows {
		k := r["KNAME"]
		if k == "" {
			k = r["NAME"]
		}
		if r["TYPE"] != "disk" || parent[k] != "" || skipDevice(r) {
			continue
		}
		if _, dup := index[k]; dup {
			continue
		}
		size, _ := strconv.ParseInt(r["SIZE"], 10, 64)
		if size == 0 {
			continue
		}
		rotational, known := parseRotational(r["ROTA"])
		c := classifyDiskFull(r["NAME"], r["TRAN"], r["MODEL"], r["VENDOR"], rotational, known, virt, r["DISC-GRAN"])
		d := DiskInfo{
			Name: "/dev/" + r["NAME"], SizeBytes: size, Type: r["TYPE"],
			DiskType: c.Type, DiskTypeConfidence: c.Confidence, DetectionReason: c.Reason, Virtual: c.Virtual,
			Transport: r["TRAN"], Vendor: strings.TrimSpace(r["VENDOR"]), Serial: r["SERIAL"], Model: strings.TrimSpace(r["MODEL"]),
		}
		if known {
			d.Rotational = &rotational
		}
		index[k] = len(disks)
		disks = append(disks, d)
	}

	seenDev := map[string]bool{}
	rootCovered := false
	addMount := func(i int, dev, mp, fstype string) {
		size, used, free, ok := statfsFn(mp)
		if !ok {
			return
		}
		d := &disks[i]
		d.Mounts = append(d.Mounts, DiskMount{Device: dev, MountPoint: mp, FSType: fstype, SizeBytes: size, UsedBytes: used, FreeBytes: free})
		d.FsSizeBytes += size
		d.UsedBytes += used
		d.FreeBytes += free
		d.Mounted = true
		if d.MountPoint == "" || mp == "/" {
			d.MountPoint = mp
		}
		if mp == "/" {
			rootCovered = true
		}
	}
	for _, r := range rows {
		mp := r["MOUNTPOINT"]
		if mp == "" || strings.HasPrefix(mp, "[") { // [SWAP]
			continue
		}
		k := r["KNAME"]
		if k == "" {
			k = r["NAME"]
		}
		if seenDev[k] { // the same filesystem appears under several parents (RAID, LVM mirrors)
			continue
		}
		i, ok := index[top(k)]
		if !ok {
			continue
		}
		seenDev[k] = true
		addMount(i, "/dev/"+r["NAME"], mp, r["FSTYPE"])
	}

	// The root filesystem on something lsblk does not show.
	if !rootCovered {
		src, fstype := rootMountSource(mounts)
		if size, used, free, ok := statfsFn("/"); ok && size > 0 {
			reason := "root filesystem (" + fstype + ") not on a block device listed by lsblk"
			if isContainerVirt(virt) {
				reason = "container filesystem; the host's disks are not visible"
			}
			disks = append(disks, DiskInfo{
				Name: src, SizeBytes: size, FsSizeBytes: size, UsedBytes: used, FreeBytes: free, Type: "fs",
				Mounted: true, MountPoint: "/", DiskType: "Unknown", DiskTypeConfidence: "unknown", DetectionReason: reason,
				Virtual: isVMVirt(virt) || isContainerVirt(virt),
				Mounts:  []DiskMount{{Device: src, MountPoint: "/", FSType: fstype, SizeBytes: size, UsedBytes: used, FreeBytes: free}},
			})
		}
	}
	for i := range disks {
		sort.SliceStable(disks[i].Mounts, func(a, b int) bool { return disks[i].Mounts[a].MountPoint < disks[i].Mounts[b].MountPoint })
	}
	return disks
}

func rootMountSource(mounts string) (string, string) {
	src, fstype := "rootfs", ""
	sc := bufio.NewScanner(strings.NewReader(mounts))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 3 && f[1] == "/" {
			src, fstype = f[0], f[2] // the last entry for / wins (overmounts)
		}
	}
	return src, fstype
}

// parseLsblkKV parses one `lsblk -P` line (values may contain spaces and
// \xHH escapes).
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
				result[key] = unescapeLsblk(line)
				break
			}
			result[key] = unescapeLsblk(line[:end])
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

var lsblkEscape = regexp.MustCompile(`\\x([0-9a-fA-F]{2})`)

func unescapeLsblk(s string) string {
	return lsblkEscape.ReplaceAllStringFunc(s, func(m string) string {
		b, _ := strconv.ParseUint(m[2:], 16, 8)
		return string(rune(b))
	})
}

func parseRotational(value string) (bool, bool) {
	return value == "1", value == "0" || value == "1"
}

type diskClassification struct {
	Type       string
	Confidence string
	Reason     string
	Virtual    bool
}

// virtualDiskModel matches the models hypervisors and clouds give their
// virtual disks.
var virtualDiskModel = regexp.MustCompile(`(?i)qemu|virtio|vbox|virtual|vmware|persistentdisk|google|amazon elastic block store|ec2|xen|msft|hyper-v|block device|nutanix|0x1af4`)

// classifyDisk keeps the old signature for callers and tests.
func classifyDisk(device, transport, model, vendor string, rotational, rotationalKnown bool) diskClassification {
	return classifyDiskFull(device, transport, model, vendor, rotational, rotationalKnown, "", "")
}

// classifyDiskFull refuses to infer physical media behind a virtual disk:
// the guest's rotational flag describes the virtual device (virtio reports
// rotational=1 even on NVMe-backed hosts), not the storage behind it.
func classifyDiskFull(device, transport, model, vendor string, rotational, rotationalKnown bool, virt, discGran string) diskClassification {
	dev := strings.ToLower(device)
	ident := strings.Join([]string{model, vendor}, " ")
	isNVMe := strings.HasPrefix(dev, "nvme") || transport == "nvme" || strings.Contains(strings.ToLower(model), "nvme")
	virtual := transport == "virtio" || strings.HasPrefix(dev, "vd") || strings.HasPrefix(dev, "xvd") ||
		virtualDiskModel.MatchString(ident)

	if virtual {
		reason := "virtual disk; the storage type behind it (SSD/HDD/NVMe) is not visible from inside the VM"
		if g, _ := strconv.ParseInt(discGran, 10, 64); g > 0 {
			reason += "; supports discard/TRIM (common on SSD-backed storage)"
		}
		if isNVMe {
			reason = "virtual NVMe disk (cloud block storage); the media behind it is not visible from inside the VM"
		}
		return diskClassification{"Virtual", "unknown", reason, true}
	}
	if isNVMe {
		if isVMVirt(virt) {
			return diskClassification{"NVMe", "medium", "NVMe interface inside a VM (local NVMe or passthrough)", false}
		}
		return diskClassification{"NVMe", "high", "NVMe device", false}
	}
	lower := strings.ToLower(ident)
	if strings.Contains(lower, "solid state") || strings.Contains(lower, "ssd") {
		return diskClassification{"SSD", "high", "model identifies solid-state media", false}
	}
	if strings.Contains(lower, "hard disk") || strings.Contains(lower, "hdd") {
		return diskClassification{"HDD", "medium", "model identifies hard-disk media", false}
	}
	if isVMVirt(virt) {
		reason := "disk inside a VM; the storage type behind it is not visible"
		return diskClassification{"Virtual", "unknown", reason, true}
	}
	// The kernel's flag is reliable on bare metal; when we could not tell
	// whether this is a VM, it is only a hint.
	conf := "low"
	if virt == "none" {
		conf = "medium"
	}
	if rotationalKnown && rotational {
		return diskClassification{"HDD", conf, "kernel reports a rotational disk", false}
	}
	if rotationalKnown && !rotational {
		return diskClassification{"SSD", conf, "kernel reports a non-rotational disk", false}
	}
	return diskClassification{"Unknown", "unknown", "insufficient device metadata", false}
}

// detectDiskType retained for compatibility with older callers.
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

// ------------------------------------------------------------------ network

// skipInterface hides virtual plumbing: container veths and bridges,
// tunnels made by tools, traffic shaping devices.
var skipInterface = regexp.MustCompile(`^(lo|veth|docker|br-|virbr|vnet|cni|flannel|cali|vxlan|kube-|tunl|ifb|dummy|lxcbr|lxdbr|tap|sit|ip6tnl|gre|erspan|nlmon|wg-quick)`)

func getNetworkInfo() []NetworkInfo {
	if out, err := runSpecCmd(10*time.Second, "ip", "-j", "addr"); err == nil {
		if nets, ok := parseIPJSON(out); ok {
			return nets
		}
	}
	out, err := runSpecCmd(10*time.Second, "ip", "-br", "addr")
	if err != nil {
		return nil
	}
	return parseIPBrief(out)
}

func parseIPJSON(out string) ([]NetworkInfo, bool) {
	var links []struct {
		Ifname    string   `json:"ifname"`
		Flags     []string `json:"flags"`
		Operstate string   `json:"operstate"`
		Address   string   `json:"address"`
		LinkType  string   `json:"link_type"`
		AddrInfo  []struct {
			Family    string `json:"family"`
			Local     string `json:"local"`
			Scope     string `json:"scope"`
			Prefixlen int    `json:"prefixlen"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal([]byte(out), &links); err != nil {
		return nil, false
	}
	var nets []NetworkInfo
	for _, l := range links {
		if skipInterface.MatchString(l.Ifname) || l.LinkType == "loopback" {
			continue
		}
		n := NetworkInfo{Interface: l.Ifname, MAC: l.Address, Status: l.Operstate}
		for _, a := range l.AddrInfo {
			switch {
			case a.Family == "inet":
				n.IPv4Addrs = append(n.IPv4Addrs, fmt.Sprintf("%s/%d", a.Local, a.Prefixlen))
				if n.IPAddress == "" {
					n.IPAddress = a.Local
				}
			case a.Family == "inet6" && a.Scope == "global":
				n.IPv6Addrs = append(n.IPv6Addrs, fmt.Sprintf("%s/%d", a.Local, a.Prefixlen))
			}
		}
		n.Status = linkStatus(l.Operstate, hasFlag(l.Flags, "UP"), hasFlag(l.Flags, "LOWER_UP"))
		nets = append(nets, n)
	}
	sortInterfaces(nets)
	return nets, true
}

func parseIPBrief(out string) []NetworkInfo {
	var nets []NetworkInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		name := f[0]
		if i := strings.IndexByte(name, '@'); i > 0 {
			name = name[:i]
		}
		if skipInterface.MatchString(name) {
			continue
		}
		n := NetworkInfo{Interface: name, Status: linkStatus(f[1], f[1] != "DOWN", f[1] == "UP")}
		for _, a := range f[2:] {
			ip, _, _ := strings.Cut(a, "/")
			if strings.Contains(ip, ":") {
				if !strings.HasPrefix(strings.ToLower(ip), "fe80") {
					n.IPv6Addrs = append(n.IPv6Addrs, a)
				}
				continue
			}
			n.IPv4Addrs = append(n.IPv4Addrs, a)
			if n.IPAddress == "" {
				n.IPAddress = ip
			}
		}
		nets = append(nets, n)
	}
	sortInterfaces(nets)
	return nets
}

// linkStatus: "UNKNOWN" is what tun/venet/some virtual NICs report while
// working; an administratively up link with carrier is UP.
func linkStatus(oper string, up, lowerUp bool) string {
	switch {
	case oper == "UP":
		return "UP"
	case oper == "UNKNOWN" && up && lowerUp:
		return "UP"
	case oper == "UNKNOWN" && up:
		return "UNKNOWN"
	case oper == "":
		if up {
			return "UP"
		}
		return "DOWN"
	}
	return oper
}

// sortInterfaces lists interfaces with an address first, then by name.
func sortInterfaces(nets []NetworkInfo) {
	sort.SliceStable(nets, func(i, j int) bool {
		ai, aj := nets[i].IPAddress != "" || len(nets[i].IPv6Addrs) > 0, nets[j].IPAddress != "" || len(nets[j].IPv6Addrs) > 0
		if ai != aj {
			return ai
		}
		return nets[i].Interface < nets[j].Interface
	})
}

func hasFlag(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// specPublicIP is the server's public IPv4 (the agent's cached detection).
func specPublicIP(ctx context.Context) string {
	publicIPCache.Lock()
	defer publicIPCache.Unlock()
	if publicIPCache.res.IP != "" && time.Since(publicIPCache.at) < 30*time.Minute {
		return publicIPCache.res.IP
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	res, err := detectPublicIP(ctx, nil)
	if err != nil {
		return ""
	}
	publicIPCache.res, publicIPCache.at = res, time.Now()
	return res.IP
}

// -------------------------------------------------------------------- cloud

type dmiInfo struct{ sysVendor, product, bios, chassisTag, boardVendor string }

func readDMI() dmiInfo {
	return dmiInfo{
		sysVendor:   readSpecFile("/sys/class/dmi/id/sys_vendor"),
		product:     readSpecFile("/sys/class/dmi/id/product_name"),
		bios:        readSpecFile("/sys/class/dmi/id/bios_vendor"),
		chassisTag:  readSpecFile("/sys/class/dmi/id/chassis_asset_tag"),
		boardVendor: readSpecFile("/sys/class/dmi/id/board_vendor"),
	}
}

// detectCloudProvider names the cloud or hypervisor the server runs on.
// nil on bare metal.
func detectCloudProvider(virt string) *CloudInfo {
	ci := cloudInitData()
	provider := cloudProvider(readDMI(), ci["cloud_name"], virt)
	if provider == "" {
		return nil
	}
	cloud := &CloudInfo{
		Provider: provider, InstanceID: ci["instance_id"], InstanceType: ci["instance_type"],
		Region: ci["region"], Zone: ci["availability_zone"],
	}
	// AWS-style zones (ap-southeast-1a) carry the region.
	if cloud.Region == "" && cloud.Zone != "" && provider == "AWS" {
		cloud.Region = strings.TrimRight(cloud.Zone, "abcdefghijklmnopqrstuvwxyz")
	}
	return cloud
}

var cloudNames = map[string]string{
	"aws": "AWS", "ec2": "AWS", "gce": "Google Cloud", "azure": "Azure", "digitalocean": "DigitalOcean",
	"hetzner": "Hetzner", "linode": "Linode (Akamai)", "akamai": "Linode (Akamai)", "vultr": "Vultr", "oracle": "Oracle Cloud",
	"aliyun": "Alibaba Cloud", "alibaba": "Alibaba Cloud", "openstack": "OpenStack", "upcloud": "UpCloud",
	"scaleway": "Scaleway", "exoscale": "Exoscale", "ibmcloud": "IBM Cloud", "tencent": "Tencent Cloud",
	"huawei": "Huawei Cloud", "outscale": "Outscale", "vmware": "VMware", "lxd": "LXD",
}

// cloudProvider decides from DMI first (the hypervisor's own description),
// then cloud-init's cloud name, then the virtualization type.
func cloudProvider(d dmiInfo, cloudName, virt string) string {
	all := strings.ToLower(strings.Join([]string{d.sysVendor, d.product, d.bios, d.boardVendor}, " "))
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(all, s) {
				return true
			}
		}
		return false
	}
	if !isContainerVirt(virt) {
		switch {
		case has("amazon ec2", "amazon"):
			return "AWS"
		case has("google"):
			return "Google Cloud"
		case strings.Contains(strings.ToLower(d.sysVendor), "microsoft") && d.chassisTag == "7783-7084-3265-9085-8269-3286-77":
			return "Azure"
		case has("tencent"):
			return "Tencent Cloud"
		case has("alibaba", "aliyun"):
			return "Alibaba Cloud"
		case has("huawei"):
			return "Huawei Cloud"
		case has("digitalocean"):
			return "DigitalOcean"
		case has("hetzner"):
			return "Hetzner"
		case has("linode", "akamai"):
			return "Linode (Akamai)"
		case has("vultr"):
			return "Vultr"
		case has("upcloud"):
			return "UpCloud"
		case has("scaleway"):
			return "Scaleway"
		case has("exoscale"):
			return "Exoscale"
		case has("ovh"):
			return "OVHcloud"
		case has("contabo"):
			return "Contabo"
		case strings.EqualFold(d.chassisTag, "OracleCloud.com") || has("oraclecloud"):
			return "Oracle Cloud"
		}
	}
	if p, ok := cloudNames[strings.ToLower(cloudName)]; ok && !isContainerVirt(virt) {
		return p
	}
	switch {
	case isContainerVirt(virt):
		return "Container (" + virtLabel(virt) + ")"
	case has("openstack"):
		return "OpenStack"
	case has("vmware"):
		return "VMware"
	case has("proxmox"):
		return "Proxmox"
	case strings.Contains(strings.ToLower(d.sysVendor), "microsoft"):
		return "Hyper-V"
	case has("virtualbox", "innotek"):
		return "VirtualBox"
	case isVMVirt(virt):
		return "VPS (" + virtLabel(virt) + ")"
	}
	return ""
}

func virtLabel(v string) string {
	switch v {
	case "kvm", "qemu":
		return "KVM"
	case "xen":
		return "Xen"
	case "microsoft":
		return "Hyper-V"
	case "oracle":
		return "VirtualBox"
	case "vmware":
		return "VMware"
	case "lxc", "lxc-libvirt":
		return "LXC"
	case "openvz":
		return "OpenVZ"
	case "docker":
		return "Docker"
	case "amazon":
		return "Nitro"
	}
	return v
}

// cloudInitData reads instance details from cloud-init (standardized "v1"
// keys first, then the datasource's metadata).
func cloudInitData() map[string]string {
	result := map[string]string{}
	out, err := runSpecCmd(10*time.Second, "cloud-init", "query", "--all")
	if err != nil || strings.TrimSpace(out) == "" {
		out = readSpecFile("/run/cloud-init/instance-data.json")
	}
	var data map[string]any
	if json.Unmarshal([]byte(out), &data) == nil {
		if v1, ok := data["v1"].(map[string]any); ok {
			for src, dst := range map[string]string{"cloud_name": "cloud_name", "instance_id": "instance_id", "region": "region",
				"availability_zone": "availability_zone", "instance_type": "instance_type"} {
				if s, ok := v1[src].(string); ok && s != "" && s != "unknown" {
					result[dst] = s
				}
			}
		}
		if ds, ok := data["ds"].(map[string]any); ok {
			if meta, ok := ds["meta_data"].(map[string]any); ok {
				for src, dst := range map[string]string{"instance-id": "instance_id", "instance-type": "instance_type",
					"availability-zone": "availability_zone", "region": "region"} {
					if s, ok := meta[src].(string); ok && s != "" && result[dst] == "" {
						result[dst] = s
					}
				}
			}
		}
	}
	if result["instance_id"] == "" {
		result["instance_id"] = readSpecFile("/var/lib/cloud/data/instance-id")
	}
	return result
}
