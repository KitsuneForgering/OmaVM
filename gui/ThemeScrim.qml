import QtQuick

// The Material style's default modal dim uses a theme-agnostic gray that
// ignores the Omarchy palette. This tints the same dim with the app's own
// dark surface color instead, so opening a dialog darkens the backdrop
// without washing out the accent/background colors it was synced from.
Rectangle {
    readonly property color base: backend.themeDarkSurface
    color: Qt.rgba(base.r, base.g, base.b, 0.72)
    Behavior on opacity { NumberAnimation { duration: 150 } }
}
