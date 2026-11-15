package agenthttp

import (
	"strings"
	"testing"
)

const lscpuKVM = `Architecture:                       x86_64
CPU op-mode(s):                     32-bit, 64-bit
CPU(s):                             4
On-line CPU(s) list:                0-3
Vendor ID:                          GenuineIntel
Model name:                         Intel(R) Xeon(R) Processor @ 2.30GHz
Thread(s) per core:                 1
Core(s) per socket:                 4
Socket(s):                          1
Hypervisor vendor:                  KVM
L1d cache:                          192 KiB (4 instances)
L2 cache:                           8 MiB (4 instances)
L3 cache:                           504 MiB (1 instance)
NUMA node0 CPU(s):                  0-3`

const lscpuARM = `Architecture:                    aarch64
CPU(s):                          4
Vendor ID:                       ARM
Model name:                      Neoverse-N1
Thread(s) per core:              1
Core(s) per cluster:             4
Socket(s):                       -
Cluster(s):                      1
L2 cache:                        4 MiB (4 instances)`

const lscpuBareMetal = `Architecture:            x86_64
CPU(s):                  16
Vendor ID:               AuthenticAMD
Model name:              AMD Ryzen 7 5800X 8-Core Processor
Thread(s) per core:      2
Core(s) per socket:      8
Socket(s):               1
CPU max MHz:             4850.1948
L3 cache:                32 MiB (1 instance)`

func TestParseCPU(t *testing.T) {
	kvm := parseCPU(lscpuKVM, "", 4)
	if kvm.Cores != 4 || kvm.Threads != 4 || kvm.Sockets != 1 || kvm.CacheSize != "504 MiB" || kvm.MaxMHz != "2300" || kvm.Hypervisor != "KVM" {
		t.Errorf("kvm = %+v", kvm)
	}
	arm := parseCPU(lscpuARM, "", 4)
	if arm.Cores != 4 || arm.Sockets != 1 || arm.CacheSize != "4 MiB" || arm.ModelName != "Neoverse-N1" {
		t.Errorf("arm = %+v", arm)
	}
	bm := parseCPU(lscpuBareMetal, "", 16)
	if bm.Cores != 8 || bm.Threads != 16 || bm.MaxMHz != "4850" || bm.CacheSize != "32 MiB" {
		t.Errorf("bare metal = %+v", bm)
	}
	// No lscpu (minimal image): /proc/cpuinfo with 2 sockets x 2 cores x 2 threads.
	var b strings.Builder
	for i := 0; i < 8; i++ {
		b.WriteString("processor\t: " + string(rune('0'+i)) + "\nvendor_id\t: GenuineIntel\nmodel name\t: Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz\n")
		b.WriteString("cpu MHz\t\t: 2399.998\ncache size\t: 35840 KB\nphysical id\t: " + string(rune('0'+i/4)) + "\ncore id\t\t: " + string(rune('0'+(i/2)%2)) + "\n\n")
	}
	ci := parseCPU("", b.String(), 8)
	if ci.Threads != 8 || ci.Cores != 4 || ci.Sockets != 2 || ci.MaxMHz != "2400" || !strings.Contains(ci.ModelName, "E5-2680") {
		t.Errorf("cpuinfo only = %+v", ci)
	}
}

func TestParseMeminfoAndRounding(t *testing.T) {
	m := parseMeminfo("MemTotal:        3995012 kB\nMemFree:          100000 kB\nMemAvailable:    2995012 kB\nSwapTotal:       1048572 kB\nSwapFree:        1048572 kB\n")
	if m.TotalBytes != 3995012*1024 || m.UsedBytes != 1000000*1024 || m.SwapUsed != 0 {
		t.Errorf("meminfo = %+v", m)
	}
	// A "4 GB" VPS shows ~3.8 GiB usable: installed is reported as 4 GiB.
	if got := roundMemory(3995012 * 1024); got != 4<<30 {
		t.Errorf("round 4G = %d", got)
	}
	if got := roundMemory(1010000 * 1024); got != 1<<30 {
		t.Errorf("round 1G = %d", got)
	}
	if got := roundMemory(470000 * 1024); got != 512<<20 {
		t.Errorf("round 512M = %d", got)
	}
}

func TestParseLspciGPUIgnoresVirtualDisplays(t *testing.T) {
	g := parseLspciGPU("00:02.0 VGA compatible controller: Cirrus Logic GD 5446\n00:03.0 Ethernet controller: Red Hat, Inc. Virtio network device\n")
	if g.Detected || g.Count != 0 || g.Display != "Cirrus Logic GD 5446" {
		t.Errorf("cirrus = %+v", g)
	}
	g = parseLspciGPU("03:00.0 VGA compatible controller: ASPEED Technology, Inc. ASPEED Graphics Family (rev 41)\n")
	if g.Detected {
		t.Errorf("aspeed BMC counted as GPU: %+v", g)
	}
	g = parseLspciGPU("00:02.0 VGA compatible controller: Cirrus Logic GD 5446\n00:05.0 3D controller: NVIDIA Corporation GA100 [A100 PCIe 40GB] (rev a1)\n")
	if !g.Detected || g.Count != 1 || !strings.Contains(g.Name, "A100") {
		t.Errorf("nvidia = %+v", g)
	}
}

func TestClassifyDiskVirtualAndPhysical(t *testing.T) {
	cases := []struct {
		dev, tran, model, vendor, virt string
		rota                           bool
		want                           string
	}{
		{"vda", "", "", "0x1af4", "kvm", true, "Virtual"},            // older lsblk: no TRAN for virtio
		{"xvda", "", "", "", "xen", true, "Virtual"},                 // Xen
		{"sda", "", "QEMU HARDDISK", "QEMU", "kvm", true, "Virtual"}, // virtio-scsi
		{"nvme0n1", "nvme", "Amazon Elastic Block Store", "", "amazon", false, "Virtual"},
		{"sda", "", "PersistentDisk", "Google", "kvm", false, "Virtual"},
		{"sdb", "", "Virtual disk", "VMware", "vmware", true, "Virtual"},
		{"sda", "sata", "Samsung SSD 870 EVO 1TB", "ATA", "none", false, "SSD"},
		{"sda", "sata", "ST4000DM004-2CV104", "ATA", "none", true, "HDD"},
		{"nvme0n1", "nvme", "Samsung SSD 980 PRO 1TB", "", "none", false, "NVMe"},
		{"sdc", "", "", "", "kvm", true, "Virtual"}, // unknown disk inside a VM: do not trust ROTA
	}
	for _, c := range cases {
		got := classifyDiskFull(c.dev, c.tran, c.model, c.vendor, c.rota, true, c.virt, "")
		if got.Type != c.want {
			t.Errorf("%s %q (%s) = %s (%s), want %s", c.dev, c.model, c.virt, got.Type, got.Reason, c.want)
		}
	}
}

func TestBuildDisksPartitionsLVMAndNoise(t *testing.T) {
	prev := statfsFn
	statfsFn = func(path string) (int64, int64, int64, bool) {
		switch path {
		case "/":
			return 40 << 30, 10 << 30, 28 << 30, true
		case "/boot":
			return 1 << 30, 200 << 20, 700 << 20, true
		case "/var/lib/mysql":
			return 100 << 30, 50 << 30, 45 << 30, true
		}
		return 0, 0, 0, false
	}
	t.Cleanup(func() { statfsFn = prev })
	lines := []string{
		`NAME="loop0" KNAME="loop0" PKNAME="" TYPE="loop" SIZE="66789376" ROTA="1" TRAN="" MODEL="" VENDOR="" SERIAL="" MOUNTPOINT="/snap/core20/1" FSTYPE="squashfs"`,
		`NAME="sr0" KNAME="sr0" PKNAME="" TYPE="rom" SIZE="1073741312" ROTA="1" TRAN="ata" MODEL="QEMU DVD-ROM" VENDOR="QEMU" SERIAL="" MOUNTPOINT="" FSTYPE=""`,
		`NAME="vda" KNAME="vda" PKNAME="" TYPE="disk" SIZE="42949672960" ROTA="1" TRAN="" MODEL="" VENDOR="0x1af4" SERIAL="" MOUNTPOINT="" FSTYPE=""`,
		`NAME="vda1" KNAME="vda1" PKNAME="vda" TYPE="part" SIZE="1073741824" ROTA="1" TRAN="" MODEL="" VENDOR="" SERIAL="" MOUNTPOINT="/boot" FSTYPE="ext4"`,
		`NAME="vda2" KNAME="vda2" PKNAME="vda" TYPE="part" SIZE="41875931136" ROTA="1" TRAN="" MODEL="" VENDOR="" SERIAL="" MOUNTPOINT="" FSTYPE="LVM2_member"`,
		`NAME="ubuntu--vg-root" KNAME="dm-0" PKNAME="vda2" TYPE="lvm" SIZE="41875931136" ROTA="1" TRAN="" MODEL="" VENDOR="" SERIAL="" MOUNTPOINT="/" FSTYPE="ext4"`,
		`NAME="vdb" KNAME="vdb" PKNAME="" TYPE="disk" SIZE="107374182400" ROTA="1" TRAN="" MODEL="" VENDOR="0x1af4" SERIAL="" MOUNTPOINT="/var/lib/mysql" FSTYPE="xfs"`,
		`NAME="vdc" KNAME="vdc" PKNAME="" TYPE="disk" SIZE="1048576" ROTA="1" TRAN="" MODEL="" VENDOR="0x1af4" SERIAL="" MOUNTPOINT="[SWAP]" FSTYPE="swap"`,
	}
	var rows []lsblkRow
	for _, l := range lines {
		rows = append(rows, parseLsblkKV(l))
	}
	disks := buildDisks(rows, "/dev/mapper/ubuntu--vg-root / ext4 rw 0 0\n", "kvm")
	if len(disks) != 3 {
		t.Fatalf("disks = %+v", disks)
	}
	vda := disks[0]
	if vda.Name != "/dev/vda" || !vda.Mounted || vda.MountPoint != "/" || len(vda.Mounts) != 2 ||
		vda.SizeBytes != 40<<30 || vda.UsedBytes != (10<<30)+(200<<20) || vda.DiskType != "Virtual" {
		t.Errorf("vda = %+v", vda)
	}
	if disks[1].MountPoint != "/var/lib/mysql" || disks[1].UsedBytes != 50<<30 {
		t.Errorf("vdb = %+v", disks[1])
	}
	if disks[2].Mounted {
		t.Errorf("swap counted as a filesystem: %+v", disks[2])
	}
}

func TestBuildDisksContainerRoot(t *testing.T) {
	prev := statfsFn
	statfsFn = func(path string) (int64, int64, int64, bool) { return 20 << 30, 5 << 30, 15 << 30, path == "/" }
	t.Cleanup(func() { statfsFn = prev })
	disks := buildDisks(nil, "/dev/loop5 / ext4 rw 0 0\n", "lxc")
	if len(disks) != 1 || disks[0].MountPoint != "/" || disks[0].UsedBytes != 5<<30 || !strings.Contains(disks[0].DetectionReason, "container") {
		t.Fatalf("container root = %+v", disks)
	}
}

func TestParseIPJSON(t *testing.T) {
	out := `[{"ifname":"lo","flags":["LOOPBACK","UP","LOWER_UP"],"operstate":"UNKNOWN","link_type":"loopback","addr_info":[{"family":"inet","local":"127.0.0.1","prefixlen":8,"scope":"host"}]},
{"ifname":"docker0","flags":["UP"],"operstate":"DOWN","link_type":"ether","address":"02:42:ac:11:00:01","addr_info":[{"family":"inet","local":"172.17.0.1","prefixlen":16,"scope":"global"}]},
{"ifname":"veth12ab","flags":["UP","LOWER_UP"],"operstate":"UP","link_type":"ether","addr_info":[]},
{"ifname":"venet0","flags":["UP","LOWER_UP"],"operstate":"UNKNOWN","link_type":"void","addr_info":[{"family":"inet","local":"103.1.2.3","prefixlen":32,"scope":"global"}]},
{"ifname":"eth0","flags":["UP","LOWER_UP"],"operstate":"UP","link_type":"ether","address":"52:54:00:12:34:56","addr_info":[{"family":"inet","local":"10.0.0.4","prefixlen":24,"scope":"global"},{"family":"inet","local":"10.0.0.5","prefixlen":24,"scope":"global"},{"family":"inet6","local":"2001:db8::4","prefixlen":64,"scope":"global"},{"family":"inet6","local":"fe80::1","prefixlen":64,"scope":"link"}]}]`
	nets, ok := parseIPJSON(out)
	if !ok || len(nets) != 2 {
		t.Fatalf("nets = %+v", nets)
	}
	eth := nets[0]
	if eth.Interface != "eth0" || eth.IPAddress != "10.0.0.4" || len(eth.IPv4Addrs) != 2 || len(eth.IPv6Addrs) != 1 || eth.Status != "UP" {
		t.Errorf("eth0 = %+v", eth)
	}
	if nets[1].Interface != "venet0" || nets[1].Status != "UP" {
		t.Errorf("venet0 = %+v", nets[1])
	}
}

func TestCloudProvider(t *testing.T) {
	cases := []struct {
		d     dmiInfo
		cloud string
		virt  string
		want  string
	}{
		{dmiInfo{sysVendor: "Tencent Cloud", product: "CVM", bios: "SeaBIOS"}, "", "kvm", "Tencent Cloud"},
		{dmiInfo{sysVendor: "Amazon EC2", product: "t3.micro", bios: "Amazon EC2"}, "aws", "amazon", "AWS"},
		{dmiInfo{sysVendor: "Google", product: "Google Compute Engine"}, "gce", "kvm", "Google Cloud"},
		{dmiInfo{sysVendor: "OpenStack Foundation", product: "OpenStack Nova"}, "openstack", "kvm", "OpenStack"},
		{dmiInfo{sysVendor: "QEMU", product: "Standard PC (i440FX + PIIX, 1996)", bios: "SeaBIOS"}, "", "kvm", "VPS (KVM)"},
		{dmiInfo{sysVendor: "Hetzner", product: "vServer"}, "hetzner", "kvm", "Hetzner"},
		{dmiInfo{sysVendor: "Dell Inc.", product: "PowerEdge R640"}, "", "none", ""},
		{dmiInfo{sysVendor: "Dell Inc.", product: "PowerEdge R640"}, "", "lxc", "Container (LXC)"}, // DMI is the host's
		{dmiInfo{}, "", "openvz", "Container (OpenVZ)"},
	}
	for _, c := range cases {
		if got := cloudProvider(c.d, c.cloud, c.virt); got != c.want {
			t.Errorf("%+v %s = %q, want %q", c.d, c.virt, got, c.want)
		}
	}
}

func TestFormatUptime(t *testing.T) {
	if got := formatUptime((3*24+5)*3600e9 + 7*60e9); got != "up 3 days, 5 hours, 7 minutes" {
		t.Error(got)
	}
	if got := formatUptime(30e9); got != "up 0 minutes" {
		t.Error(got)
	}
}
