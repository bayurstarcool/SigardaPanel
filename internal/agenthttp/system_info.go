package agenthttp

// Machine details beyond CPU, memory and disks: the system board and
// firmware (DMI), memory modules (dmidecode), boot time, load and process
// count, time zone, and per-interface link speed / driver / MTU.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SystemInfo describes the machine.
type SystemInfo struct {
	Manufacturer string     `json:"manufacturer,omitempty"`
	Product      string     `json:"product,omitempty"`
	Version      string     `json:"version,omitempty"`
	BoardVendor  string     `json:"board_vendor,omitempty"`
	BoardName    string     `json:"board_name,omitempty"`
	ChassisType  string     `json:"chassis_type,omitempty"`
	BIOSVendor   string     `json:"bios_vendor,omitempty"`
	BIOSVersion  string     `json:"bios_version,omitempty"`
	BIOSDate     string     `json:"bios_date,omitempty"`
	BootTime     string     `json:"boot_time,omitempty"` // RFC 3339
	LoadAvg      [3]float64 `json:"load_avg"`
	Processes    int        `json:"processes,omitempty"`
	Timezone     string     `json:"timezone,omitempty"`
}

// MemoryModule is one populated memory slot.
type MemoryModule struct {
	Locator      string `json:"locator,omitempty"`
	Size         string `json:"size"`
	Type         string `json:"type,omitempty"`
	Speed        string `json:"speed,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
}

var chassisTypes = map[string]string{"1": "Other", "2": "Unknown", "3": "Desktop", "4": "Low-profile desktop", "6": "Mini tower", "7": "Tower",
	"8": "Portable", "9": "Laptop", "10": "Notebook", "13": "All in one", "17": "Main server chassis", "23": "Rack mount", "24": "Sealed-case PC",
	"28": "Blade", "29": "Blade enclosure", "30": "Tablet", "31": "Convertible", "35": "Mini PC", "36": "Stick PC"}

func dmiClean(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "to be filled by o.e.m.", "default string", "not specified", "none", "n/a", "system product name", "system manufacturer", "0", "o.e.m.":
		return ""
	}
	return strings.TrimSpace(v)
}

func getSystemInfo() SystemInfo {
	d := func(f string) string { return dmiClean(sysRead("/sys/class/dmi/id/" + f)) }
	s := SystemInfo{
		Manufacturer: d("sys_vendor"), Product: d("product_name"), Version: d("product_version"),
		BoardVendor: d("board_vendor"), BoardName: d("board_name"),
		BIOSVendor: d("bios_vendor"), BIOSVersion: d("bios_version"), BIOSDate: d("bios_date"),
	}
	if t := sysRead("/sys/class/dmi/id/chassis_type"); t != "" {
		s.ChassisType = chassisTypes[t]
	}
	for _, line := range strings.Split(sysRead("/proc/stat"), "\n") {
		if v, ok := strings.CutPrefix(line, "btime "); ok {
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				s.BootTime = time.Unix(n, 0).UTC().Format(time.RFC3339)
			}
		}
	}
	if f := strings.Fields(sysRead("/proc/loadavg")); len(f) >= 4 {
		for i := 0; i < 3; i++ {
			s.LoadAvg[i] = parseFloat(f[i])
		}
		if _, total, ok := strings.Cut(f[3], "/"); ok {
			s.Processes, _ = strconv.Atoi(total) // scheduling entities (threads included)
		}
	}
	if pids := sysGlob("/proc/[0-9]*"); len(pids) > 0 {
		s.Processes = len(pids)
	}
	s.Timezone = strings.TrimSpace(readSpecFile("/etc/timezone"))
	if s.Timezone == "" {
		if l, err := os.Readlink("/etc/localtime"); err == nil {
			if _, z, ok := strings.Cut(l, "zoneinfo/"); ok {
				s.Timezone = z
			}
		}
	}
	return s
}

// memoryModules lists populated slots from dmidecode (root; absent in many
// containers). Virtual machines show their configured memory as modules.
func memoryModules() []MemoryModule {
	out, err := runSpecCmd(10*time.Second, "dmidecode", "-t", "17")
	if err != nil {
		return nil
	}
	return parseDmidecodeMemory(out)
}

func parseDmidecodeMemory(out string) []MemoryModule {
	var mods []MemoryModule
	var cur *MemoryModule
	flush := func() {
		if cur != nil && cur.Size != "" {
			mods = append(mods, *cur)
		}
		cur = nil
	}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "Memory Device" {
			flush()
			cur = &MemoryModule{}
			continue
		}
		if cur == nil {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "Size":
			if !strings.HasPrefix(v, "No Module") && v != "Unknown" && v != "0" {
				cur.Size = v
			}
		case "Locator":
			cur.Locator = dmiClean(v)
		case "Type":
			if t := dmiClean(v); t != "Unknown" && t != "Other" {
				cur.Type = t
			}
		case "Configured Memory Speed", "Configured Clock Speed":
			if s := dmiClean(v); s != "" && s != "Unknown" {
				cur.Speed = s
			}
		case "Speed":
			if s := dmiClean(v); s != "" && s != "Unknown" && cur.Speed == "" {
				cur.Speed = s
			}
		case "Manufacturer":
			if m := dmiClean(v); m != "Unknown" && !strings.HasPrefix(strings.ToLower(m), "manufacturer") {
				cur.Manufacturer = m
			}
		}
	}
	flush()
	return mods
}

// linkDetails adds speed, duplex, driver and MTU to the interfaces.
func linkDetails(nets []NetworkInfo) {
	for i := range nets {
		base := "/sys/class/net/" + nets[i].Interface
		if n, err := strconv.Atoi(sysRead(base + "/speed")); err == nil && n > 0 {
			nets[i].SpeedMbps = n
		}
		if dup := sysRead(base + "/duplex"); dup == "full" || dup == "half" {
			nets[i].Duplex = dup
		}
		if m, err := strconv.Atoi(sysRead(base + "/mtu")); err == nil {
			nets[i].MTU = m
		}
		if l, err := os.Readlink(sysRoot + base + "/device/driver"); err == nil {
			nets[i].Driver = filepath.Base(l)
		}
		switch {
		case nets[i].Driver == "virtio_net" || strings.HasPrefix(nets[i].Driver, "vmxnet") || nets[i].Driver == "hv_netvsc" || nets[i].Driver == "xen-netfront" || nets[i].Driver == "ena" || nets[i].Driver == "gve":
			nets[i].Virtual = true
		case sysRead(base+"/device/vendor") == "" && nets[i].Driver == "":
			nets[i].Virtual = true // venet, tun, bridges
		}
	}
}
