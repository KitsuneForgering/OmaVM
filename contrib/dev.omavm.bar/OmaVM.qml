import QtQuick
import QtQuick.Effects
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

  // Self-hides at zero, same convention as omarchy.system-update and
  // omarchy.weather: a machine with no OmaVM environments draws nothing
  // rather than sitting in the tray with a "0". Loading (-1) also stays
  // hidden so the icon doesn't flash in on shell startup only to vanish
  // a moment later on a fresh machine.
  visible: root.envCount > 0
  implicitWidth: visible ? button.implicitWidth : 0
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
    var envs
    try {
      envs = JSON.parse(String(output || "[]"))
    } catch (e) {
      // omavm not installed, or --json not supported by this version —
      // fail quiet rather than spam the shell log every refresh tick.
      root.envCount = 0
      return
    }
    if (Array.isArray(envs)) {
      root.envCount = envs.length
      return
    }
    // omavm answered with an error ({"error": {...}}, e.g. a damaged
    // registry): the environments still exist, so keep showing the last
    // count instead of vanishing as if there were none.
    if (envs && envs.error)
      console.warn("omavm-bar", envs.error.message || envs.error.code)
  }

  readonly property int refreshIntervalSec: Math.max(5, Number(setting("refreshIntervalSec", 30)))

  Timer {
    interval: root.refreshIntervalSec * 1000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: root.refresh()
  }

  // The OmaVM window glyph (data/icons/dev.omavm.app.svg, flattened to a
  // single-color line icon as icon.svg here) instead of the bare ▣ font
  // glyph, so this reads as the app's own identity rather than a random
  // Unicode box — matches docs/TODO.md P2 "Unificar a identidade do
  // widget com o aplicativo". Recolored via BarIconButton's own
  // iconComponent slot (see /usr/share/omarchy/shell/Ui/BarIconButton.qml)
  // rather than a fixed color, so it follows the bar's foreground/active
  // colors like every built-in icon does.
  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    slotSize: Style.bar.statusSlot
    tooltipText: "OmaVM — " + root.envCount + " environment" + (root.envCount === 1 ? "" : "s") + ", click to open"
    iconComponent: Component {
      Item {
        // Same as the shell's own symbolic tray icons
        // (/usr/share/omarchy/shell/plugins/bar/widgets/Tray.qml): a hidden
        // image kept as a layer, so the effect has a texture to sample, and
        // decoded at physical pixels for HiDPI.
        Image {
          id: glyphImage
          anchors.fill: parent
          source: Qt.resolvedUrl("icon.svg")
          sourceSize.width: Math.round(Math.min(width, height) * Screen.devicePixelRatio)
          sourceSize.height: Math.round(Math.min(width, height) * Screen.devicePixelRatio)
          fillMode: Image.PreserveAspectFit
          visible: false
          layer.enabled: true
        }
        MultiEffect {
          anchors.fill: glyphImage
          source: glyphImage
          colorization: 1
          colorizationColor: button.active && button.useActiveColor ? button.activeColor : button.foreground
        }
      }
    }
    onPressed: function(b) {
      if (b === Qt.RightButton) root.refresh()
      else root.openExperienceCenter()
    }
  }
}
