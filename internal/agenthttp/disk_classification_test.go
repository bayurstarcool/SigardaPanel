package agenthttp

import "testing"

func TestClassifyDiskVirtualBackendUnknown(t *testing.T) {
	got := classifyDisk("vda", "virtio", "", "0x1af4", true, true)
	if got.Type != "Virtual" || !got.Virtual || got.Confidence != "unknown" {
		t.Fatalf("unexpected virtual classification: %#v", got)
	}
}

func TestClassifyDiskNVMe(t *testing.T) {
	got := classifyDisk("nvme0n1", "", "", "", false, true)
	if got.Type != "NVMe" || got.Confidence != "high" || got.Virtual {
		t.Fatalf("unexpected NVMe classification: %#v", got)
	}
}

func TestClassifyDiskRotationalPhysicalLowConfidence(t *testing.T) {
	got := classifyDisk("sda", "sata", "", "ATA", true, true)
	if got.Type != "HDD" || got.Confidence != "low" || got.Virtual {
		t.Fatalf("unexpected rotational classification: %#v", got)
	}
}

func TestParseLsblkKVPreservesSpaces(t *testing.T) {
	got := parseLsblkKV(`NAME="vda" SIZE="64424509440" TYPE="disk" ROTA="1" TRAN="virtio" MODEL="" VENDOR="0x1af4" SERIAL="abc"`)
	if got["NAME"] != "vda" || got["TRAN"] != "virtio" || got["VENDOR"] != "0x1af4" || got["SERIAL"] != "abc" {
		t.Fatalf("unexpected lsblk parse: %#v", got)
	}
}

func TestParseLsblkKVModelWithSpaces(t *testing.T) {
	got := parseLsblkKV(`NAME="sda" SIZE="171261820928" TYPE="disk" ROTA="0" TRAN="" MODEL="QEMU HARDDISK" VENDOR="QEMU"`)
	if got["MODEL"] != "QEMU HARDDISK" || got["VENDOR"] != "QEMU" {
		t.Fatalf("unexpected model parse: %#v", got)
	}
}
