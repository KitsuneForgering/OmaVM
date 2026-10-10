// Package core is the OmaVM Core: it models environments and delegates
// their lifecycle to a Backend, without depending on any specific
// virtualization or container tool.
package core

import (
	"encoding/json"
	"fmt"
	"time"
)

// EnvironmentKind distinguishes a userspace-only Box from a Machine with
// an independent kernel. It is a semantic distinction, not a distro
// distinction: Linux can be a Machine when it needs its own kernel.
type EnvironmentKind int

const (
	Box EnvironmentKind = iota
	Machine
)

func (k EnvironmentKind) String() string {
	switch k {
	case Box:
		return "box"
	case Machine:
		return "machine"
	default:
		return "unknown"
	}
}

// ParseEnvironmentKind parses the CLI/GUI-facing spelling of a kind.
func ParseEnvironmentKind(s string) (EnvironmentKind, error) {
	switch s {
	case "box":
		return Box, nil
	case "machine":
		return Machine, nil
	default:
		return 0, &InvalidKindError{Value: s}
	}
}

// MarshalJSON renders as "box"/"machine" rather than a raw int, so both
// the on-disk state file and CLI --json output stay human-readable and
// stable across a future reordering of the const block.
func (k EnvironmentKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

func (k *EnvironmentKind) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		parsed, err := ParseEnvironmentKind(s)
		if err != nil {
			return err
		}
		*k = parsed
		return nil
	}

	// Accept the pre-MarshalJSON raw-int encoding too, so an
	// environments.json written before this format existed still loads.
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("unmarshal environment kind: %w", err)
	}
	if n != int(Box) && n != int(Machine) {
		return fmt.Errorf("unmarshal environment kind: out of range: %d", n)
	}
	*k = EnvironmentKind(n)
	return nil
}

// Environment is a computational environment the user creates, runs and
// manages. Backend is the name of the Backend that owns it (e.g.
// "distrobox", "qemu"); it is set by the Service on Create and is an
// implementation detail that higher layers may display but never branch
// product behavior on beyond what Kind already implies.
type Environment struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Image     string              `json:"image"`
	Backend   string              `json:"backend"`
	Kind      EnvironmentKind     `json:"kind"`
	Settings  EnvironmentSettings `json:"settings,omitempty"`
	Snapshots []Snapshot          `json:"snapshots,omitempty"`
	// Operation is set while the environment is being created or removed:
	// the registry keeps it for the whole backend call (so the name is
	// taken and other clients can show it) without holding the registry
	// locked meanwhile.
	Operation string `json:"operation,omitempty"`
}

// ReadyUbuntuImage identifies OmaVM's supported, preconfigured Ubuntu
// Desktop. It is a system choice, not a local installation ISO path.
const (
	ReadyUbuntuImage = "ready:ubuntu-24.04"
	ReadyFedoraImage = "ready:fedora-44"
)

func IsReadyImage(image string) bool {
	return image == ReadyUbuntuImage || image == ReadyFedoraImage
}

// Operations that take an environment out of normal use while they run.
const (
	OperationCreating = "creating"
	OperationRemoving = "removing"
)

// Snapshot is a point-in-time state of an Environment the user can return
// to. Label is the significant, user-facing text (UX Principle #9:
// snapshots are never presented by internal ID); ID is the technical tag
// the owning Backend uses to address it and is exposed in JSON output for
// automation, never as the primary way a human identifies a snapshot.
type Snapshot struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
	// CrashConsistent marks a snapshot of a running environment whose
	// guest couldn't flush its disks first (no guest agent): going to it
	// is like booting after a power cut.
	CrashConsistent bool `json:"crash_consistent,omitempty"`
	// WithoutFirmwareState marks a snapshot of a running Machine with UEFI
	// variables or a TPM: only its disk was kept, so going to it keeps the
	// boot entries and TPM keys it has at that moment.
	WithoutFirmwareState bool `json:"without_firmware_state,omitempty"`
}

// defaultSnapshotLimit caps automatic snapshot history per Environment
// when SnapshotLimit isn't set, so history doesn't grow unbounded (UX
// Principle #9).
const defaultSnapshotLimit = 10

type EnvironmentSettings struct {
	DisconnectISO  bool   `json:"disconnect_iso,omitempty"`
	Description    string `json:"description,omitempty"`
	CPUs           int    `json:"cpus,omitempty"`
	MemoryMiB      int    `json:"memory_mib,omitempty"`
	SharedPath     string `json:"shared_path,omitempty"`
	SharedReadOnly bool   `json:"shared_read_only,omitempty"`
	// SharedFolderDisabled turns off the Machine's shared folder, which is
	// on by default: SharedPath when set, otherwise the Machine backend's
	// own default folder (~/OmaVM/Shared). Stored inverted like the other
	// opt-outs, so an older registry keeps sharing on.
	SharedFolderDisabled bool   `json:"shared_folder_disabled,omitempty"`
	SnapshotLimit        int    `json:"snapshot_limit,omitempty"`
	Color                string `json:"color,omitempty"`
	// ClipboardDisabled and TravelModeDisabled are stored inverted so the
	// Go zero value (false) means "enabled" — both are opt-out behaviors:
	// on by default, persisted per environment like every other setting
	// here, never a per-session UI checkbox the user has to remember to
	// re-enable. For a Machine, the clipboard is shared with the guest;
	// for a Box, programs in its terminal may copy to it (OSC 52).
	ClipboardDisabled bool `json:"clipboard_disabled,omitempty"`
	// ClipboardDirection limits a shared clipboard to one way: "" (both,
	// the default), ClipboardToHost or ClipboardToGuest. A Box only has
	// the way to the host (OSC 52); pasting into its terminal is always a
	// deliberate keystroke.
	ClipboardDirection string `json:"clipboard_direction,omitempty"`
	TravelModeDisabled bool   `json:"travel_mode_disabled,omitempty"`
	// VulkanDisabled turns off Vulkan acceleration for a Machine, which is
	// otherwise on whenever the host supports it. Same inverted storage as
	// the two above.
	VulkanDisabled bool `json:"vulkan_disabled,omitempty"`
	// SSHDisabled leaves out the channel `omavm ssh` and `omavm exec` use to
	// reach a Machine (AF_VSOCK), which is otherwise added whenever the host
	// supports it. Any process on the host can reach that channel, Boxes
	// included; only the guest's login protects it.
	SSHDisabled bool `json:"ssh_disabled,omitempty"`
	// LauncherDisabled hides the environment from the host's application
	// launcher, where it is listed by default. Applies to both Kinds.
	LauncherDisabled bool `json:"launcher_disabled,omitempty"`
	// EmptyWorkspaceDisabled opens the environment on the current
	// workspace instead of an empty one, which is the default for both
	// Kinds. It replaces the earlier opt-in "open_in_empty_workspace"
	// key: whoever set that wanted it on, which is now the default, so
	// the old key needs no migration.
	EmptyWorkspaceDisabled bool `json:"empty_workspace_disabled,omitempty"`
	// FullscreenDisabled opens a Machine's display in a window instead of
	// fullscreen, which is the default.
	FullscreenDisabled bool `json:"fullscreen_disabled,omitempty"`
}

func (e Environment) EffectiveSettings() EnvironmentSettings {
	settings := e.Settings
	if e.Kind == Machine {
		cpus, memory := DefaultMachineResources()
		if settings.CPUs == 0 {
			settings.CPUs = cpus
		}
		if settings.MemoryMiB == 0 {
			settings.MemoryMiB = memory
		}
	}
	if settings.SnapshotLimit == 0 {
		settings.SnapshotLimit = defaultSnapshotLimit
	}
	return settings
}

// EnvironmentColors is the fixed palette Color must come from — an open
// hex value would fight the Omarchy theme instead of complementing it
// (CLAUDE.md: "não crie um sistema de temas próprio").
var EnvironmentColors = []string{"red", "orange", "yellow", "green", "blue", "purple", "gray"}

func validColor(c string) bool {
	for _, v := range EnvironmentColors {
		if v == c {
			return true
		}
	}
	return false
}

type SettingsPatch struct {
	DisconnectISO        *bool
	Description          *string
	CPUs                 *int
	MemoryMiB            *int
	SharedPath           *string
	SharedReadOnly       *bool
	SharedFolder         *bool
	SnapshotLimit        *int
	Color                *string
	ShareClipboard       *bool
	ClipboardDirection   *string
	TravelMode           *bool
	Vulkan               *bool
	Launcher             *bool
	SSH                  *bool
	OpenInEmptyWorkspace *bool
	Fullscreen           *bool
}

// Clipboard directions (EnvironmentSettings.ClipboardDirection).
const (
	ClipboardBoth    = "both"
	ClipboardToHost  = "to-host"
	ClipboardToGuest = "to-guest"
)

// ClipboardToHost reports whether text copied in the environment may
// reach this computer's clipboard.
func (s EnvironmentSettings) ClipboardToHost() bool {
	return !s.ClipboardDisabled && s.ClipboardDirection != ClipboardToGuest
}

// ClipboardToGuest reports whether text copied on this computer may reach
// the environment's clipboard (Machines only).
func (s EnvironmentSettings) ClipboardToGuest() bool {
	return !s.ClipboardDisabled && s.ClipboardDirection != ClipboardToHost
}

// ClipboardMode is the clipboard setting as one word: "false" (off),
// "true" (both ways), ClipboardToHost or ClipboardToGuest — the value of
// omavm-gui's --share-clipboard.
func (s EnvironmentSettings) ClipboardMode() string {
	switch {
	case s.ClipboardDisabled:
		return "false"
	case s.ClipboardDirection == ClipboardToHost || s.ClipboardDirection == ClipboardToGuest:
		return s.ClipboardDirection
	}
	return "true"
}
