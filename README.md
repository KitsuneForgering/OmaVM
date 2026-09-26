# OmaVM

A highly inspired on parallels, virtual machine control to Omarchy.

OmaVM manages **Environments** for Omarchy: **Boxes** (userspace Linux via
OmaVM's own container engine, on Podman or Docker) and **Machines**
(independent kernel via QEMU/KVM). See [CLAUDE.md](CLAUDE.md) for the
full product/architecture model.

## Status

Phase 1 vertical slice: Core domain + CLI + GUI, with a container-based
Box backend and a QEMU/KVM Machine backend implementing
Create/Start/Open/Stop/Status/Remove (and Exec for Boxes). The GUI
(Experience Center) is a GTK4 + libadwaita front end calling the same
Core as the CLI, matching Omarchy's native GNOME/Adwaita look.

## Build

Requires Go, and `libgtk-4-dev`/`libadwaita-1-dev` (or your distro's
equivalent) for the GUI, since it's built with CGO bindings.

```bash
make check   # gofmt -l . && go vet ./... && go test ./... && go build ./...
```

Or invoke the pieces directly: `make build`, `make test`, `make vet`,
`make fmt`, `make fmt-check`. See the [Makefile](Makefile).

CI (`.github/workflows/ci.yml`) runs the same checks on every push/PR.

## Usage

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
```

Requires `podman` (preferred) or `docker` on `PATH` for Boxes, and
`qemu-img`/`qemu-system-x86_64` with `/dev/kvm` access for Machines.
Opening a Machine's graphical display additionally needs a VNC client on
`PATH` — `remote-viewer` (`virt-viewer`), `vncviewer` (`tigervnc`), or
`gvncviewer`. Without one, `open` fails with a clear message and the
`vnc://host:port` you can connect to manually instead of silently no-op'ing.

`list` and `status` accept `--json` (in any position, e.g. both
`omavm list --json` and `omavm status radic --json` work) for
structured output aimed at agents, scripts, and the Quickshell bar
widget below.

## GUI (Experience Center)

```bash
make run-gui
```

Environments appear as cards (name, kind, image, status) with Start,
Open, Stop and Delete actions — no backend/infrastructure detail is
exposed. Creating one asks "what do you want to run?", not "which
backend?": pick a distro (or a custom Linux image, or a boot ISO for a
non-Linux system) and, for Linux, whether you want a Development
Environment (Box) or a Virtual Machine (Machine).

Install the `omavm` CLI binary alongside `omavm-gui` (same directory or
on `PATH`) so a Box's "Open" can attach an interactive shell in your
terminal.

A running Machine's card shows a live screenshot of its display
(captured via QEMU's QMP `screendump`, refreshed on every action or
manual Refresh); a Box's card honestly says it has no display to
preview instead of showing a placeholder that pretends otherwise. The
window itself is responsive — the card grid drops to 2 columns, then 1,
as it narrows (`AdwBreakpoint`), not just clipping.

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

```bash
make install-quickshell-plugin
```

This only copies the plugin into `~/.config/omarchy/plugins/`; it does
**not** auto-enable it. Per Omarchy's own plugin trust model (plugins run
unsandboxed inside the already-live shell), review the QML yourself and
then run the two commands the target prints
(`omarchy-shell shell rescanPlugins && omarchy plugin enable dev.omavm.bar`).
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
