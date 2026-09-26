# OmaVM bar widget

A single icon in the tray cluster — no "VM N" text — that appears only
when at least one OmaVM environment (Box or Machine) exists, and opens
the Experience Center GUI on click. Same self-hiding convention as
`omarchy.system-update` and `omarchy.weather`: nothing to report, nothing
drawn.

## Data

The widget shells out to `omavm list --json` on a timer (default every 30s,
`refreshIntervalSec` in settings) and counts the array length. It never
manages environments itself and never talks to Podman/Docker/QEMU
directly — that stays inside the `omavm` Core, per OmaVM's own
CLI/GUI Contract (see [CLAUDE.md](../../CLAUDE.md) in the OmaVM repo).
If `omavm` isn't on `PATH` or the call fails, the widget just shows 0
(hidden) rather than spamming the shell log. The count itself only
appears in the tooltip on hover, matching how the other tray icons
(network, bluetooth, volume) work.

## Interactions

- Left click: launch `omavm-gui` (the Experience Center).
- Right click: refresh immediately instead of waiting for the timer.
- IPC: `omarchy-shell dev.omavm.bar refresh` (verified working; `shell` is
  Omarchy's own reserved meta-target — e.g. `omarchy-shell shell ping` —
  not a prefix, so it must not appear before the plugin id here).

## Requirements

- The `omavm` CLI on `PATH` (installed via OmaVM's `make install`, which
  installs to `~/.local/bin`).
- `omavm-gui` on `PATH` for the click-through to actually open something.

## Installing

This plugin isn't a separate git repo (it ships inside the OmaVM
monorepo), so install it by hand rather than `omarchy plugin add`:

```bash
cp -r contrib/dev.omavm.bar ~/.config/omarchy/plugins/dev.omavm.bar
omarchy-shell shell rescanPlugins
omarchy plugin enable dev.omavm.bar
```

Per Omarchy's own plugin trust model, review `OmaVM.qml` before enabling —
plugins run unsandboxed inside `omarchy-shell`.

Editing the QML after it's already enabled needs more than
`rescanPlugins` or a disable/enable cycle to actually take effect —
neither reloads a widget's source once loaded (verified: both left the
old behavior running). `omarchy-restart-shell` does reload it, at the
cost of restarting the whole shell (bar, panels, notifications) for a
moment; it refuses to run while the session is locked.
