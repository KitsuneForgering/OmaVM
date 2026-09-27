import QtQuick
import QtQuick.Window
import OmaVM 1.0

Window {
    id: win
    width: 1000
    height: 640
    visible: true
    title: terminalTitle
    color: "black"

    // Same reasoning as Viewer.qml: no client-side fullscreen request.
    // The Hyprland window rule (contrib/hypr/omavm-viewer.lua) matches
    // this window's app id — shared with the Machine VNC viewer — so a
    // Box's terminal and a Machine's display fullscreen the same way on
    // the same dedicated workspace without a second rule to maintain.

    TerminalView {
        id: view
        anchors.fill: parent
        focus: true
        envName: terminalEnvName
        onTitleChanged: title => win.title = title
        onFinished: exitCode => win.close()
        onErrorOccurred: message => {
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
