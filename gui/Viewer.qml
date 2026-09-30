import QtQuick
import QtQuick.Window
import QtQuick.Controls
import QtQuick.Controls.Material
import OmaVM 1.0

Window {
    id: win
    width: 1280
    height: 800
    visible: false
    title: displayTitle
    color: backend.themeBackground
    Material.theme: backend.themeMode === "light" ? Material.Light : Material.Dark
    Material.accent: backend.themeAccent
    Material.primary: backend.themeBackground
    Material.background: backend.themeBackground
    Material.foreground: backend.themeForeground

    // Fullscreen is a Machine setting (on by default). main.cpp turns it
    // off here when a personal Hyprland rule for the viewer
    // (contrib/hypr/omavm-viewer.lua) already makes it fullscreen: two
    // independent requests act as a toggle and cancel each other.
    Component.onCompleted: {
        if (displayFullscreen)
            showFullScreen()
        else
            show()
    }

    // Clipboard sharing is a Machine Settings toggle (opt-out, on by
    // default — see gui/SettingsDialog.qml), not a per-window checkbox:
    // the user shouldn't have to remember to re-enable it every time
    // they open the viewer. main.cpp passes the Machine's current
    // setting in as displayShareClipboard.
    DisplayView {
        id: view
        anchors.fill: parent
        focus: true
        shareClipboard: displayShareClipboard
        connectionFd: displayConnectionFd
        onConnectionFailed: message => {
            errorLabel.text = message
            errorPanel.visible = true
        }
    }

    Rectangle {
        id: errorPanel
        anchors.centerIn: parent
        visible: false
        width: Math.min(parent.width - 80, 480)
        height: errorLabel.implicitHeight + 32
        radius: 8
        color: backend.themeSurface
        border.color: backend.themeRed

        Text {
            id: errorLabel
            anchors { left: parent.left; right: parent.right; verticalCenter: parent.verticalCenter; margins: 16 }
            color: backend.themeForeground
            wrapMode: Text.Wrap
            horizontalAlignment: Text.AlignHCenter
        }
    }

    Shortcut {
        sequence: "Ctrl+Alt+Q"
        onActivated: win.close()
    }
}
