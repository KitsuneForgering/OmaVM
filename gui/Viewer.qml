import QtQuick
import QtQuick.Window
import OmaVM 1.0

Window {
    id: win
    width: 1280
    height: 800
    visible: true
    visibility: Window.FullScreen
    title: vncTitle
    color: "black"

    VncView {
        id: view
        anchors.fill: parent
        focus: true
        socketPath: vncSocketPath
        onConnectionFailed: message => {
            errorLabel.text = message
            errorLabel.visible = true
        }
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
