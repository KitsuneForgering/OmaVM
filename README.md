<p align="center">
  <img src="data/icons/dev.omavm.app.svg" alt="OmaVM icon" width="96" height="96">
</p>

<h1 align="center">OmaVM</h1>

<p align="center">
  <strong>Linux development environments and virtual machines, at home on Omarchy.</strong>
</p>

<p align="center">
  <a href="https://github.com/KitsuneSemCalda/OmaVM/actions/workflows/ci.yml"><img src="https://github.com/KitsuneSemCalda/OmaVM/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/go-1.27.1%2B-00ADD8?logo=go" alt="Go 1.27.1+"></a>
</p>

<p align="center">
  <a href="#two-kinds-of-environment">Box or Machine?</a> ·
  <a href="#quick-start">Quick start</a> ·
  <a href="#requirements">Requirements</a> ·
  <a href="#usage-reference">CLI reference</a> ·
  <a href="#status">Project status</a>
</p>

OmaVM is a desktop app for creating and managing environments on
[Omarchy](https://omarchy.org). Work with another Linux distribution's tools
in a **Box**, or install and boot a complete operating system in a **Machine**.
Start, open, and stop both from the same window, with a built-in terminal for
Boxes and a graphical viewer for Machines.

<p align="center">
  <a href="docs/experience-center.png">
    <img src="docs/experience-center.png" alt="OmaVM Experience Center with arch-desktop running and kernel-lab stopped, showing a display preview and Open and Start actions" width="944">
  </a>
  <br>
  <em>The Experience Center: your environments, their status, and the next action.</em>
</p>

The interface follows your live Omarchy theme and fits Hyprland's tiling
layout. Management operations are also available through the `omavm` CLI
for scripts and automation.

## What can I use it for?

- **Develop with another Linux distribution.** Keep a project's tools and
  packages in a Fedora, Ubuntu, Debian, Arch, or Alpine Box while working
  with files in your Omarchy home directory.
- **Try a complete operating system.** Boot an installation ISO in a Machine
  and use its desktop in an OmaVM window.
- **Test boot and kernel changes.** Give Linux its own kernel and virtual disk
  in a Machine, with snapshots to return to an earlier disk state.
- **Bring Box applications into your desktop.** Export graphical applications
  installed in a Box so they appear in your host app launcher.
- **Automate environment management.** Create environments, run commands in
  Boxes, and query status through the `omavm` CLI, with JSON output for scripts.

## Two kinds of environment

An **environment** is a named system you create and manage in OmaVM.
Choose its kind according to what you need:

| | Development Box | Desktop / Machine |
|---|---|---|
| Best for | Development tools and Linux applications | A complete OS, desktop, or kernel testing |
| Runs | Linux userspace sharing the host kernel | An OS with its own kernel and boot process |
| Created from | A Linux container image, such as `fedora:latest` | An x86_64 installation ISO |
| Opens in | OmaVM's built-in terminal | OmaVM's built-in graphical viewer |
| Host integration | Shared home directory and graphical applications through Distrobox | Configurable shared folder, audio, and text clipboard with guest support |
| Powered by | Distrobox with Podman or Docker | QEMU/KVM |

A Box is closely integrated with your host: it shares the kernel and home
directory, so it should not be treated as an isolated sandbox. Choose a Machine
when you need a separate kernel or full OS boot. Linux can run as either kind.
In the creation dialog, **Desktop** creates a Machine and **Development Box**
creates a Box.

## From the Experience Center

| Open your environment | Manage it | Connect it to Omarchy |
|---|---|---|
| Use a Box's built-in terminal or a Machine's graphical viewer. | Adjust settings, assign a color, and manage Machine snapshots. | Export Box apps to the launcher or configure a Machine's shared folder and clipboard. |

OmaVM coordinates Distrobox and QEMU/KVM behind these actions. Its product
direction takes inspiration from Parallels Desktop: make creating and using
another system approachable, with advanced settings available when needed.
An optional [Quickshell bar widget](contrib/dev.omavm.bar) gives you a shortcut
to the app and an environment count in its tooltip.

## Status

OmaVM is actively developed and **not yet 1.0**. Both Boxes and Machines support
creation, start, open, stop, status, settings, and removal. Boxes also support
command execution and application export; Machines support pause/resume,
restart, snapshots, shared folders, and text clipboard integration.

Guest integration depends on the guest OS, drivers, and services; see
[Machines](#machines) for requirements and behavior. Remote/cloud hosts,
GPU passthrough, disposable environments, and seamless Machine application
windows are outside the current feature set.

See [CLAUDE.md](CLAUDE.md) for the product model, architecture, and development
scope.

## Requirements

| For | Needs |
|---|---|
| Build | Go, Qt 6 Base/Declarative, `qmake6`, zlib development files |
| Boxes | `distrobox` + `podman` (preferred) or `docker` |
| Machines | `qemu-img`/`qemu-system-x86_64` with `/dev/kvm` access |
| Shared folders (Machines) | `virtiofsd` |

Opening a Machine's display needs nothing extra — the viewer is built
into `omavm-gui`. See [Machines](#machines) below for what that gets you.

## Quick start

```bash
git clone https://github.com/KitsuneSemCalda/OmaVM.git
cd OmaVM
make check      # build, vet, test everything
make install    # omavm + omavm-gui on PATH, .desktop entry, no root needed
```

Launch **OmaVM** from your app launcher or run `omavm-gui`:

1. Choose **Development Box** for Linux tools, or **Desktop** for a complete OS.
2. Select a distribution for a Box, or a local x86_64 installation ISO for a Machine.
3. Name and create the environment, then select **Start** and **Open**.

You can also run the GUI from the source tree with `make run-gui`.

<details>
<summary>Prefer the CLI? Create your first environment from the terminal.</summary>

```bash
# A Development Box: Linux tools with access to your home directory.
omavm create --name radic --kind box --image fedora:latest
omavm start radic
omavm open radic

# A Machine: install an OS from a local ISO.
omavm create --name kernels --kind machine --image /path/to/install.iso
omavm start kernels
omavm open kernels
```

</details>

## Build

```bash
make check   # gofmt, go vet, go test, Go CLI build, and Qt GUI build
```

Or invoke the pieces directly: `make build`, `make test`, `make vet`,
`make fmt`, `make fmt-check`. See the [Makefile](Makefile). CI
(`.github/workflows/ci.yml`) runs the same checks on every push/PR.

## Usage reference

```bash
bin/omavm create --name radic --kind box --image fedora:latest
bin/omavm start radic
bin/omavm exec radic -- go test ./...
bin/omavm status radic
bin/omavm stop radic
bin/omavm rm radic

bin/omavm create --name kernels --kind machine --image /path/to/install.iso
bin/omavm start kernels
bin/omavm status kernels
bin/omavm pause kernels
bin/omavm resume kernels
bin/omavm restart kernels
bin/omavm integration kernels
bin/omavm settings kernels --description "Kernel lab" --cpus 4 --memory-mib 4096
bin/omavm settings kernels --shared-path "$HOME/Projects" --shared-read-only
```

Machines expose the folder to the guest as the virtiofs tag `omavm-share`;
inside a Linux guest, mount it with `mount -t virtiofs omavm-share /mnt/omavm-share`.

`list` and `status` accept `--json` (in any position, e.g. both
`omavm list --json` and `omavm status radic --json` work) for
structured output aimed at agents, scripts, and the Quickshell bar
widget below.

## Machines

Machines try the installed disk before the ISO, falling back to installation
media while the disk is not bootable. After installation, enable **Installation
finished — disconnect ISO on next start** in Settings. Shut down and start the
Machine to apply it; the ISO file can then be moved or deleted. The equivalent
CLI command is `omavm settings kernels --disconnect-iso`. Use
`--disconnect-iso=false` to reconnect the original ISO on a later start.

**Shut Down** asks the guest to shut down normally. If it has not exited after
ten seconds, OmaVM reports that it is still waiting and leaves the Machine
running. **Restart** also requires a successful normal shutdown before starting
again. **Force Stop** is the explicit emergency action and may lose unsaved
work. Deleting a running Machine also requires shutdown to succeed.

Registry changes are serialized across CLI and GUI processes so concurrent
creation, deletion and settings updates do not overwrite each other's records.

Opening a Machine's graphical display launches `omavm-gui` in its built-in
VNC viewer mode (`omavm-gui --viewer <socket> --title <name>`) — no
external VNC/SPICE client to install. Machines run fully headless
(`-display egl-headless`, GPU-accelerated `virtio-vga-gl`) and expose the
display over a local-only VNC Unix socket, plus PipeWire audio and virtiofs
shared folders. Text clipboard sharing is a Machine setting (**Settings →
Automation → Share text clipboard**, or `omavm settings NAME --share-clipboard`),
on by default and applied automatically whenever you open the viewer — it is
not a per-window checkbox you need to re-enable each time. Turn it off per
Machine if you don't want it. Only UTF-8 text is exchanged, limited to
approximately 1 MiB; images and files are not transferred. The guest desktop
must have a running `spice-vdagent` compatible with its graphical session. The
QEMU guest agent shown in the environment card is a different component and
does not prove clipboard readiness. Restart an existing Machine to attach the
clipboard channel. This uses QEMU's `qemu-vdagent` bridge with VNC; it does not
require a SPICE display or a new OmaVM guest daemon. Travel Mode (also in
Settings → Automation, on by default) halves the Machine's default CPU
allocation for that session while the host is running on battery, unless
you've pinned a custom CPU count.

By default the viewer opens as an ordinary window wherever Hyprland
would place it. Resizing that window requests a matching guest resolution
through VNC after a short delay. This requires a guest display driver and
desktop that honor virtio GPU resize requests. During boot, or when the guest
does not resize, the viewer preserves the image proportions with black margins
and maps pointer input to the displayed image.

The protocol tests (`make test-viewer`, also part of `make test` and CI) cover
fragmented resize/clipboard messages, bounded clipboard decompression,
framebuffer bounds and rejected/forwarded resize requests.
When QEMU is installed they also check negotiation against a temporary QEMU
process without guest disks, including bidirectional Unicode clipboard between
two clients; this is not a guest desktop compatibility test.

The Box terminal's parser has its own suite (`make test-terminal`, also part
of `make test` and CI): cursor addressing, SGR colors (16/256/truecolor),
the alternate screen buffer, scrollback, OSC window-title handling, escape
sequences and UTF-8 characters split across separate reads, and a real-PTY
round trip (spawn a process, write to it, confirm the echoed output lands
in the grid) exercising the same code path keyboard input takes.

To always have the viewer open fullscreen on its own dedicated workspace,
add this line to your own `~/.config/hypr/windows.lua`
(same file, same pattern as any other personal window rule there):

```lua
o.window("dev.omavm.viewer", { workspace = "name:omavm", fullscreen = true })
```

A plain named workspace, not a Hyprland "special" one — special
workspaces are scratchpad overlays that stay hidden until explicitly
toggled, so a window rule alone never makes them visible. This is
optional and never enabled automatically — see
[contrib/hypr/omavm-viewer.lua](contrib/hypr/omavm-viewer.lua). The same
rule applies to a Box's terminal viewer too, since it shares the same
`dev.omavm.viewer` app id — no separate rule needed for Boxes.

## GUI (Experience Center)

```bash
make run-gui
```

Environments appear as cards (name, kind, image, status) with Start,
Open, Stop and Delete actions — no backend/infrastructure detail is
exposed. Creating one starts with a choice between **Desktop** (Machine) and
**Development Box** (Box). Then select a local x86_64 installation ISO for a
Machine, or a Linux distribution or custom container image for a Box.

Install the `omavm` CLI binary alongside `omavm-gui` (same directory or
on `PATH`) so a Box's "Open" can attach an interactive shell. Opening a
Box launches `omavm-gui` in a built-in terminal mode
(`omavm-gui --terminal <name> --title <title>`) — a small VT100/ANSI
terminal emulator implemented from scratch (PTY + a Ground/Escape/CSI/OSC
parser covering cursor addressing, SGR colors including 256-color and
truecolor, the alternate screen buffer used by vim/htop/less/tmux, and
scrollback), not an external terminal emulator. It shares the same
`dev.omavm.viewer` app id as the Machine VNC viewer below, so the same
optional Hyprland window rule covers both.

A running Machine's card shows a live screenshot of its display
(captured via QEMU's QMP `screendump`, refreshed on every action or
manual Refresh); a Box's card honestly says it has no display to
preview instead of showing a placeholder that pretends otherwise. The
window itself is responsive — the card grid adapts as it narrows instead
of clipping.

Errors and lifecycle events (created/removed) also surface as native
desktop notifications, not just in-window toasts, so they're visible
even if the window isn't focused.

## Install (app launcher entry, icon, PATH)

```bash
make install     # installs to ~/.local/{bin,share/...}, no root needed
make uninstall
```

This puts `omavm`/`omavm-gui` on `PATH` and registers a `.desktop` entry
+ icon so OmaVM shows up in the app launcher and taskbar like any other
Omarchy app. Set `PREFIX=/usr/local` (with `sudo`) for a system-wide
install instead.

## Logs

Both binaries log structured (JSON) events. `/var/log` is root-owned by
default, and OmaVM never escalates privileges silently to write there
(see CLAUDE.md's Security Model), so logging goes to
`/var/log/omavm/<component>.log` only if that directory already exists
and is writable; otherwise it falls back to
`$XDG_STATE_HOME/omavm/logs/<component>.log` (usually
`~/.local/state/omavm/logs/`). The very first log line always records
which path is in effect.

To opt into the system location instead, provision it once yourself:

```bash
sudo install -d -o "$USER" -g "$USER" -m 0755 /var/log/omavm
```

## Omarchy desktop shell (Quickshell bar widget)

Omarchy's desktop (`omarchy-shell`) is a plugin-based Quickshell
instance — see `/usr/share/omarchy/shell/README.md` on an Omarchy
machine. [`contrib/dev.omavm.bar`](contrib/dev.omavm.bar) is a bar-widget
plugin: it shells out to `omavm list --json` to show how many
environments exist, and clicking it launches `omavm-gui`.

`make install` already copies it into `~/.config/omarchy/plugins/` for
you (also available standalone as `make install-quickshell-plugin`). This
only stages the files; it does **not** auto-enable the plugin. Per
Omarchy's own plugin trust model (plugins run unsandboxed inside the
already-live shell), review the QML yourself and then run:

```bash
omarchy-shell shell rescanPlugins && omarchy plugin enable dev.omavm.bar
```

See [contrib/dev.omavm.bar/README.md](contrib/dev.omavm.bar/README.md)
for interactions and settings.

## Hyprland

No window rule is shipped for the main window on purpose: it tiles like
any other Omarchy app, which is the native experience on a tiling
compositor — floating it would work against Hyprland, not with it. If
you want a keybinding to open the Experience Center, add one yourself
(not applied automatically — it's your config):

```conf
bind = SUPER, V, exec, omavm-gui
```

## Contributing

Issues and PRs welcome. [CLAUDE.md](CLAUDE.md) is this project's technical
and product constitution — read it first, especially the Non-Goals and
"Rules for AI Agents" sections, before proposing new backends,
abstractions, or big features. Small, focused changes that fit the
existing domain model are the fastest path to a merge.

## License

[MIT](LICENSE)
