import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui

// Bar entry point for OmaVM. Deliberately minimal and read-only: it never
// manages environments itself (CLAUDE.md's CLI/GUI Contract — every real
// operation goes through the Core, never a UI shelling out to
// infrastructure directly), it only shells out to `omavm list --json`
// (the same Core the CLI/GUI use) to show a count, and launches
// `omavm-gui` on click.
BarWidget {
  id: root
  moduleName: "dev.omavm.bar"

  // -1 = not loaded yet, so the label can say "…" instead of a wrong "0"
  // for the first tick after the shell starts.
  property int envCount: -1

  function refresh() {
    if (!listProc.running) listProc.running = true
  }

  function openExperienceCenter() {
    if (root.bar) root.bar.run("omavm-gui")
  }

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  IpcHandler {
    target: "dev.omavm.bar"
    function refresh(): void { root.broadcast("refresh") }
  }

  Process {
    id: listProc
    command: ["omavm", "list", "--json"]

    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: root.applyListing(text)
    }

    stderr: StdioCollector {
      waitForEnd: true
      onStreamFinished: if (text.trim() !== "") console.warn("omavm-bar", text.trim())
    }
  }

  function applyListing(output) {
    try {
      var envs = JSON.parse(String(output || "[]"))
      root.envCount = Array.isArray(envs) ? envs.length : 0
    } catch (e) {
      // omavm not installed, or --json not supported by this version —
      // fail quiet rather than spam the shell log every refresh tick.
      root.envCount = 0
    }
  }

  readonly property int refreshIntervalSec: Math.max(5, Number(setting("refreshIntervalSec", 30)))

  Timer {
    interval: root.refreshIntervalSec * 1000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: root.refresh()
  }

  readonly property string label: root.envCount < 0 ? "VM …" : "VM " + root.envCount

  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: root.label
    slotSize: Style.bar.statusSlot
    fontSize: Style.font.caption
    tooltipText: root.envCount < 0
      ? "OmaVM — loading…"
      : root.envCount === 0
        ? "OmaVM — no environments yet, click to create one"
        : "OmaVM — " + root.envCount + " environment" + (root.envCount === 1 ? "" : "s") + ", click to open"
    onPressed: function(b) {
      if (b === Qt.RightButton) root.refresh()
      else root.openExperienceCenter()
    }
  }
}
