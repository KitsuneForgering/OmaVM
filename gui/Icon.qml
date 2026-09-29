import QtQuick
import QtQuick.Window
import QtQuick.Controls.impl as ControlsImpl

// A monochrome SVG icon tinted to match the Omarchy theme at runtime.
// The source SVGs (gui/icons/*.svg) are plain black shapes on a
// transparent background; recolored via IconImage's own `color`
// property — the same QQuickIconImage type Qt Quick Controls uses
// internally to render a Button/ToolButton's `icon.source`/`icon.color`
// (confirmed working in the real app: the header's add/refresh icons use
// it directly). Two MultiEffect-based approaches (colorization, and
// masking a solid Rectangle with the icon's alpha) both looked correct
// rendered standalone in isolation, but every icon using either one
// rendered as the original, un-tinted black silhouette in the real
// compiled app — reusing Controls' own proven mechanism instead of a
// third shader-based attempt.
Item {
    id: root
    property alias source: img.source
    property alias color: img.color
    property real iconSize: 20
    implicitWidth: iconSize
    implicitHeight: iconSize

    ControlsImpl.IconImage {
        id: img
        anchors.fill: parent
        sourceSize.width: Math.ceil(root.iconSize * Screen.devicePixelRatio)
        sourceSize.height: Math.ceil(root.iconSize * Screen.devicePixelRatio)
        fillMode: Image.PreserveAspectFit
        asynchronous: true
        color: "black"

        Behavior on color {
            ColorAnimation { duration: 150; easing.type: Easing.OutCubic }
        }
    }
}
