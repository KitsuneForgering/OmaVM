package qemu

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeHost builds a minimal /sys and /dev tree and points the package's
// host paths at it for the duration of the test.
type fakeHost struct {
	t   *testing.T
	sys string
	dev string
	icd string
	lib string
}

func newFakeHost(t *testing.T) *fakeHost {
	root := t.TempDir()
	h := &fakeHost{
		t:   t,
		sys: filepath.Join(root, "sys"),
		dev: filepath.Join(root, "dev"),
		icd: filepath.Join(root, "icd.d"),
		lib: filepath.Join(root, "lib"),
	}
	server := filepath.Join(root, "virgl_render_server")
	for _, dir := range []string{h.sys, h.dev, h.icd, h.lib} {
		h.mkdir(dir)
	}
	h.write(server, "")
	oldSys, oldDev, oldICD, oldLib, oldServer, oldHelp := sysRoot, devRoot, vulkanICDDirs, libDirs, renderServerPaths, qemuDeviceHelp
	sysRoot, devRoot, vulkanICDDirs, libDirs, renderServerPaths = h.sys, h.dev, []string{h.icd}, []string{h.lib}, []string{server}
	qemuDeviceHelp = func(context.Context, string) (string, error) { return "  venus=<bool>\n  blob=<bool>\n", nil }
	t.Cleanup(func() {
		sysRoot, devRoot, vulkanICDDirs, libDirs, renderServerPaths, qemuDeviceHelp = oldSys, oldDev, oldICD, oldLib, oldServer, oldHelp
	})
	return h
}

func (h *fakeHost) mkdir(path string) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		h.t.Fatal(err)
	}
}

func (h *fakeHost) write(path, content string) {
	h.mkdir(filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// gpu adds a PCI display device; group lists its IOMMU group members
// (nil means the IOMMU is off).
func (h *fakeHost) gpu(addr, vendor string, boot bool, group []string) {
	dev := filepath.Join(h.sys, "bus/pci/devices", addr)
	h.write(filepath.Join(dev, "class"), "0x030000\n")
	h.write(filepath.Join(dev, "vendor"), vendor+"\n")
	if boot {
		h.write(filepath.Join(dev, "boot_vga"), "1\n")
	}
	if group != nil {
		for _, m := range group {
			h.mkdir(filepath.Join(dev, "iommu_group/devices", m))
		}
		h.mkdir(filepath.Join(h.sys, "kernel/iommu_groups/1"))
	}
}

func (h *fakeHost) renderNode(vendor string) {
	h.write(filepath.Join(h.sys, "class/drm/renderD128/device/vendor"), vendor+"\n")
	h.write(filepath.Join(h.dev, "dri/renderD128"), "")
}

func (h *fakeHost) vulkanDriver(icdName, library string) {
	h.write(filepath.Join(h.icd, icdName), `{"ICD":{"library_path":"`+library+`"}}`)
	h.write(filepath.Join(h.lib, library), "")
}

func TestDetectGraphicsVenusOnIntel(t *testing.T) {
	h := newFakeHost(t)
	h.renderNode("0x8086")
	h.vulkanDriver("intel_icd.x86_64.json", "libvulkan_intel.so")
	h.write(filepath.Join(h.dev, "udmabuf"), "")

	g := detectGraphics(context.Background())
	if !g.openGL || !g.vulkan {
		t.Fatalf("want OpenGL and Vulkan, got %+v", g)
	}
}

func TestDetectGraphicsExplainsMissingVulkan(t *testing.T) {
	tests := map[string]struct {
		setup func(*fakeHost)
		want  string
	}{
		"nvidia": {func(h *fakeHost) {
			h.renderNode("0x10de")
			h.write(filepath.Join(h.dev, "udmabuf"), "")
		}, "only Intel and AMD"},
		"no mesa driver": {func(h *fakeHost) {
			h.renderNode("0x1002")
			h.write(filepath.Join(h.dev, "udmabuf"), "")
		}, "vulkan-radeon"},
		"icd without library": {func(h *fakeHost) {
			h.renderNode("0x8086")
			h.write(filepath.Join(h.icd, "intel_icd.json"), `{"ICD":{"library_path":"libvulkan_intel.so"}}`)
			h.write(filepath.Join(h.dev, "udmabuf"), "")
		}, "vulkan-intel"},
		"no udmabuf": {func(h *fakeHost) {
			h.renderNode("0x8086")
			h.vulkanDriver("intel_icd.json", "libvulkan_intel.so")
		}, "udmabuf"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			h := newFakeHost(t)
			tt.setup(h)
			g := detectGraphics(context.Background())
			if !g.openGL {
				t.Fatalf("OpenGL should still be available: %+v", g)
			}
			if g.vulkan || !strings.Contains(g.vulkanDetail, tt.want) {
				t.Fatalf("want Vulkan unavailable mentioning %q, got %+v", tt.want, g)
			}
		})
	}
}

func TestDetectGraphicsWithoutRenderNode(t *testing.T) {
	newFakeHost(t)
	g := detectGraphics(context.Background())
	if g.openGL || g.vulkan {
		t.Fatalf("no render node must disable both, got %+v", g)
	}
}

func TestPassthroughCapability(t *testing.T) {
	tests := map[string]struct {
		setup     func(*fakeHost)
		available bool
		want      string
	}{
		"single gpu": {func(h *fakeHost) {
			h.gpu("0000:00:02.0", "0x8086", true, []string{"0000:00:02.0"})
		}, false, "single GPU"},
		"iommu off": {func(h *fakeHost) {
			h.gpu("0000:00:02.0", "0x8086", true, nil)
			h.gpu("0000:01:00.0", "0x10de", false, nil)
		}, false, "IOMMU is disabled"},
		"isolated secondary gpu": {func(h *fakeHost) {
			h.gpu("0000:00:02.0", "0x8086", true, []string{"0000:00:02.0"})
			h.gpu("0000:01:00.0", "0x1002", false, []string{"0000:01:00.0", "0000:01:00.1"})
		}, true, "own IOMMU group"},
		"shared group": {func(h *fakeHost) {
			h.gpu("0000:00:02.0", "0x8086", true, []string{"0000:00:02.0"})
			h.gpu("0000:01:00.0", "0x1002", false, []string{"0000:00:01.0", "0000:01:00.0"})
		}, false, "shares its IOMMU group"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			h := newFakeHost(t)
			tt.setup(h)
			c := passthroughCapability()
			if c.Available != tt.available || !strings.Contains(c.Detail, tt.want) {
				t.Fatalf("want available=%t mentioning %q, got %+v", tt.available, tt.want, c)
			}
		})
	}
}
