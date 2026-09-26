# OmaVM bar widget

One bar entry showing how many OmaVM environments (Boxes + Machines)
exist, and a click-through into the Experience Center GUI.

## Data

The widget shells out to `omavm list --json` on a timer (default every 30s,
`refreshIntervalSec` in settings) and counts the array length. It never
manages environments itself and never talks to Podman/Docker/QEMU
directly — that stays inside the `omavm` Core, per OmaVM's own
CLI/GUI Contract (see [CLAUDE.md](../../CLAUDE.md) in the OmaVM repo).
If `omavm` isn't on `PATH` or the call fails, the widget just shows 0
rather than spamming the shell log.

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
