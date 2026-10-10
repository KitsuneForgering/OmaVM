package qemu

import (
	"strings"
	"testing"
)

// splitQEMUOpts reads an option list the way QEMU's QemuOpts parser does:
// commas separate options and ",," is a literal comma.
func splitQEMUOpts(s string) []string {
	var opts []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != ',' {
			cur.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && s[i+1] == ',' {
			cur.WriteByte(',')
			i++
			continue
		}
		opts = append(opts, cur.String())
		cur.Reset()
	}
	return append(opts, cur.String())
}

// Whatever a path or Machine name holds, optValue keeps it one option
// value: QEMU reads back exactly it, and nothing after it becomes an
// option of its own (",process=x" in a name must not set -name's process).
func FuzzOptValueStaysOneValue(f *testing.F) {
	for _, seed := range []string{"Windows 11, trabalho", "/home/a,b/disk.qcow2", ",", ",,", "x,process=evil", "trailing,", ",leading", "plain"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		got := splitQEMUOpts("file=" + optValue(value) + ",if=virtio")
		if len(got) != 2 || got[0] != "file="+value || got[1] != "if=virtio" {
			t.Fatalf("value %q read back as options %q", value, got)
		}
	})
}

// Values read back from a running QEMU's /proc cmdline or the guest list
// are untrusted shapes: parsing them must never panic.
func FuzzHostParsersNeverPanic(f *testing.F) {
	f.Add([]byte("qemu-system-x86_64\x00-smp\x004\x00-m\x002048\x00-device\x00vhost-vsock-pci,guest-cid=70000\x00-cdrom\x00x.iso"))
	f.Add([]byte("Display Name,OS,Release,Option,Downloader,PNG,SVG\nUbuntu,ubuntu,24.04,,wget,,\nmac,macos,14,,,,\n\"broken"))
	f.Add([]byte("-smp\x00"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = parseApplied(strings.Split(string(data), "\x00"))
		_, _ = parseCID(data)
		images, err := parseQuickgetList(data)
		for _, image := range images {
			if err == nil && image.OS == "macos" {
				t.Fatalf("macOS offered from %q", data)
			}
		}
	})
}
