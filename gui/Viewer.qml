import QtQuick
import QtQuick.Window
import QtQuick.Controls
import OmaVM 1.0

Window {
    id: win
    width: 1280
    height: 800
    visible: true
    title: vncTitle
    color: "black"

    // No client-side fullscreen request here on purpose: a Hyprland
    // window rule (contrib/hypr/omavm-viewer.lua) is the intended way to
    // fullscreen this on its own workspace. If both the client and the
    // compositor's rule request fullscreen independently, Wayland treats
    // the second request as a toggle, so the two fight and the window can
    // end up NOT fullscreen. Without that opt-in rule, this just opens as
    // an ordinary window — see README.md.

    // Clipboard sharing is a Machine Settings toggle (opt-out, on by
    // default — see gui/SettingsDialog.qml), not a per-window checkbox:
    // the user shouldn't have to remember to re-enable it every time
    // they open the viewer. main.cpp passes the Machine's current
    // setting in as vncShareClipboard.
    VncView {
        id: view
        anchors.fill: parent
        focus: true
        shareClipboard: vncShareClipboard
        onClipboardWarning: message => { clipboardNote.text = message; clipboardNote.visible = true }
        socketPath: vncSocketPath
        onConnectionFailed: message => {
            errorLabel.text = message
            errorLabel.visible = true
        }
    }

    Label {
        id: clipboardNote
        visible: false
        anchors { top: parent.top; horizontalCenter: parent.horizontalCenter; topMargin: 8 }
        width: Math.min(parent.width - 32, 600)
        wrapMode: Text.Wrap
        color: "white"
        background: Rectangle { color: "#333333" }
    }

    Text {
        id: errorLabel
        anchors.centerIn: parent
        color: "white"
        visible: false
        wrapMode: Text.Wrap
        width: Math.min(parent.width - 80, 480)
        horizontalAlignment: Text.AlignHCenter
    }

    Shortcut {
        sequence: "Ctrl+Alt+Q"
        onActivated: win.close()
    }
}
