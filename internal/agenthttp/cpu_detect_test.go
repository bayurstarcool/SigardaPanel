package agenthttp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSys builds a /sys tree: sockets × cores × threads, with caches.
func fakeSys(t *testing.T, sockets, cores, threads int, offline []int, cpufreq bool) {
	t.Helper()
	root := t.TempDir()
	w := func(p, v string) {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(v+"\n"), 0o644)
	}
	n := 0
	total := sockets * cores * threads
	for s := 0; s < sockets; s++ {
		for c := 0; c < cores; c++ {
			for th := 0; th < threads; th++ {
				// Linux numbers siblings far apart: cpu = th*(sockets*cores) + s*cores + c.
				cpu := th*(sockets*cores) + s*cores + c
				d := fmt.Sprintf("/sys/devices/system/cpu/cpu%d", cpu)
				w(d+"/topology/physical_package_id", fmt.Sprint(s))
				w(d+"/topology/core_id", fmt.Sprint(c))
				w(d+"/topology/die_id", "0")
				sib := []string{}
				for x := 0; x < threads; x++ {
					sib = append(sib, fmt.Sprint(x*(sockets*cores)+s*cores+c))
				}
				w(d+"/topology/thread_siblings_list", strings.Join(sib, ","))
				for i, cache := range []struct{ level, typ, size, shared string }{
					{"1", "Data", "48K", strings.Join(sib, ",")},
					{"1", "Instruction", "32K", strings.Join(sib, ",")},
					{"2", "Unified", "1280K", strings.Join(sib, ",")},
					{"3", "Unified", "36864K", fmt.Sprintf("socket%d", s)},
				} {
					idx := fmt.Sprintf("%s/cache/index%d", d, i)
					w(idx+"/level", cache.level)
					w(idx+"/type", cache.typ)
					w(idx+"/size", cache.size)
					w(idx+"/shared_cpu_list", cache.shared)
				}
				n++
			}
		}
	}
	for _, o := range offline {
		w(fmt.Sprintf("/sys/devices/system/cpu/cpu%d/online", o), "0")
	}
	for s := 0; s < sockets; s++ {
		w(fmt.Sprintf("/sys/devices/system/node/node%d/cpulist", s), "x")
	}
	if cpufreq {
		w("/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq", "3900000")
		w("/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_min_freq", "800000")
		w("/sys/devices/system/cpu/cpu0/cpufreq/base_frequency", "2400000")
		w("/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq", "3100000")
		w("/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor", "performance")
	}
	_ = total
	prevRoot, prevRead := sysRoot, readSpecFile
	sysRoot = root
	readSpecFile = func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	t.Cleanup(func() { sysRoot, readSpecFile = prevRoot, prevRead })
}

func TestSysfsTopologyDualSocketHyperthreading(t *testing.T) {
	fakeSys(t, 2, 4, 2, nil, true)
	info := CPUInfo{Threads: 2, Cores: 1} // what a wrong lscpu might say
	applySysfsCPU(&info)
	finishCPU(&info)
	if info.Threads != 16 || info.Cores != 8 || info.Sockets != 2 || info.ThreadsPerCore != 2 || info.CoresPerSocket != 4 || info.NUMANodes != 2 {
		t.Fatalf("topology = %+v", info)
	}
	want := map[string]string{"L1d": "48 KiB×8=384 KiB", "L1i": "32 KiB×8=256 KiB", "L2": "1.2 MiB×8=10 MiB", "L3": "36 MiB×2=72 MiB"}
	for _, c := range info.Caches {
		if got := fmt.Sprintf("%s×%d=%s", c.Size, c.Instances, c.Total); got != want[c.Level] {
			t.Errorf("%s = %s, want %s", c.Level, got, want[c.Level])
		}
	}
	if info.CacheSize != "72 MiB" || info.MaxTurboMHz != "3900" || info.BaseMHz != "2400" || info.CurrentMHz != "3100" || info.MinMHz != "800" ||
		info.Governor != "performance" || info.MaxMHz != "3900" {
		t.Fatalf("clocks/caches = %+v", info)
	}
}

func TestSysfsOfflineCPUs(t *testing.T) {
	fakeSys(t, 1, 4, 1, []int{3}, false)
	info := CPUInfo{}
	applySysfsCPU(&info)
	finishCPU(&info)
	if info.Threads != 3 || info.OfflineCPUs != 1 || info.Cores != 3 {
		t.Fatalf("offline = %+v", info)
	}
}

// util-linux ≥ 2.37 tree format (what lscpu prints on a terminal).
const lscpuTree = `Architecture:            x86_64
  CPU op-mode(s):        32-bit, 64-bit
CPU(s):                  4
  On-line CPU(s) list:   0-3
Vendor ID:               GenuineIntel
  BIOS Vendor ID:        Smdbmds
  Model name:            Intel(R) Xeon(R) Platinum 8255C CPU @ 2.50GHz
    BIOS Model name:     3.0
    CPU family:          6
    Model:               85
    Thread(s) per core:  1
    Core(s) per socket:  4
    Socket(s):           1
    Stepping:            5
    BogoMIPS:            4988.27
    Flags:               fpu vme sse4_2 avx avx2 avx512f aes hypervisor
Virtualization features:
  Hypervisor vendor:     KVM
  Virtualization type:   full
Caches (sum of all):
  L1d:                   128 KiB (4 instances)
  L1i:                   128 KiB (4 instances)
  L2:                    16 MiB (4 instances)
  L3:                    35.8 MiB (1 instance)
NUMA:
  NUMA node(s):          1`

// util-linux 2.23 (CentOS 7): per-cache sizes, "CPU MHz".
const lscpuOld = `Architecture:          x86_64
CPU(s):                2
Thread(s) per core:    1
Core(s) per socket:    2
Socket(s):             1
NUMA node(s):          1
Vendor ID:             AuthenticAMD
CPU family:            23
Model:                 49
Model name:            AMD EPYC 7K62 48-Core Processor
Stepping:              0
CPU MHz:               2595.124
BogoMIPS:              5190.24
Hypervisor vendor:     KVM
L1d cache:             32K
L1i cache:             32K
L2 cache:              4096K
L3 cache:              16384K`

const lscpuJSONNested = `{"lscpu":[{"field":"Architecture:","data":"aarch64","children":[{"field":"CPU op-mode(s):","data":"64-bit"}]},
{"field":"CPU(s):","data":"4"},{"field":"Vendor ID:","data":"ARM","children":[{"field":"Model name:","data":"Neoverse-N1","children":[
{"field":"Thread(s) per core:","data":"1"},{"field":"Core(s) per cluster:","data":"4"},{"field":"Socket(s):","data":"-"},{"field":"Cluster(s):","data":"1"},{"field":"BogoMIPS:","data":"50.00"}]}]},
{"field":"Caches (sum of all):","data":null,"children":[{"field":"L1d:","data":"256 KiB (4 instances)"},{"field":"L1i:","data":"256 KiB (4 instances)"},{"field":"L2:","data":"4 MiB (4 instances)"},{"field":"L3:","data":"32 MiB (1 instance)"}]}]}`

func TestLscpuFormats(t *testing.T) {
	tree := buildCPU(parseLscpuText(lscpuTree), "", 4)
	finishCPU(&tree)
	if tree.ModelName != "Intel(R) Xeon(R) Platinum 8255C CPU @ 2.50GHz" || tree.Cores != 4 || tree.CacheSize != "35.8 MiB" || tree.BaseMHz != "2500" ||
		tree.Family != "6" || tree.Model != "85" || tree.Hypervisor != "KVM" || len(tree.Caches) != 4 {
		t.Errorf("tree = %+v", tree)
	}
	if tree.Caches[2].Level != "L2" || tree.Caches[2].Size != "4 MiB" || tree.Caches[2].Instances != 4 {
		t.Errorf("tree L2 = %+v", tree.Caches[2])
	}

	old := buildCPU(parseLscpuText(lscpuOld), "", 2)
	finishCPU(&old)
	if old.Cores != 2 || old.CacheSize != "16 MiB" || old.CurrentMHz != "2595" || old.MaxMHz != "2595" || old.ModelName != "AMD EPYC 7K62 48-Core Processor" {
		t.Errorf("old = %+v", old)
	}

	js := buildCPU(parseLscpuJSON(lscpuJSONNested), "", 4)
	finishCPU(&js)
	if js.ModelName != "Neoverse-N1" || js.Cores != 4 || js.Sockets != 1 || js.CacheSize != "32 MiB" || js.Architecture != "aarch64" || js.BogoMIPS != "50.00" {
		t.Errorf("json = %+v", js)
	}
}

func TestProcCPUInfoBoards(t *testing.T) {
	// Raspberry Pi 4: no lscpu model, ARM implementer code.
	pi := `processor	: 0
BogoMIPS	: 108.00
Features	: fp asimd evtstrm crc32 cpuid
CPU implementer	: 0x41
CPU architecture: 8
CPU part	: 0xd08
CPU revision	: 3

processor	: 1
CPU implementer	: 0x41

Hardware	: BCM2835
Model		: Raspberry Pi 4 Model B Rev 1.4`
	info := buildCPU(map[string]string{}, pi, 4)
	finishCPU(&info)
	if info.VendorID != "ARM" || info.ModelName != "BCM2835" || info.Threads != 2 || info.Cores != 2 || len(info.Features) == 0 || info.Features[0] != "NEON" {
		t.Errorf("pi = %+v", info)
	}
	// x86 with VT-x exposed (nested virtualization or bare metal).
	x := buildCPU(map[string]string{"Model name": "X"}, "processor : 0\nflags : sse4_2 avx2 vmx aes\n", 1)
	if x.VirtSupport != "VT-x" {
		t.Errorf("virt = %q", x.VirtSupport)
	}
}

func TestCgroupQuotaAndHelpers(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "sys/fs/cgroup"), 0o755)
	os.WriteFile(filepath.Join(root, "sys/fs/cgroup/cpu.max"), []byte("150000 100000\n"), 0o644)
	prevRoot, prevRead := sysRoot, readSpecFile
	sysRoot = root
	readSpecFile = func(p string) string { b, _ := os.ReadFile(p); return strings.TrimSpace(string(b)) }
	defer func() { sysRoot, readSpecFile = prevRoot, prevRead }()
	if q := cgroupCPUQuota(); q != "1.5 CPUs" {
		t.Errorf("quota = %q", q)
	}
	if n := countCPUList("0-3,8,10-11"); n != 7 {
		t.Errorf("cpulist = %d", n)
	}
	if b := parseSizeIEC("35.8 MiB"); fmtIEC(b) != "35.8 MiB" {
		t.Errorf("size = %s", fmtIEC(b))
	}
}
