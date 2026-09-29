import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts
import QtQuick.Window
import OmaVM 1.0

Window {
    id: win
    width: 1000
    height: 640
    visible: true
    title: terminalTitle
    color: backend.themeBackground
    Material.theme: backend.themeMode === "light" ? Material.Light : Material.Dark
    Material.accent: backend.themeAccent
    Material.primary: backend.themeBackground
    Material.background: backend.themeBackground
    Material.foreground: backend.themeForeground

    // Same reasoning as Viewer.qml: no client-side fullscreen request.
    // The Hyprland window rule (contrib/hypr/omavm-viewer.lua) matches
    // this window's app id — shared with the Machine display viewer — so a
    // Box's terminal and a Machine's display fullscreen the same way on
    // the same dedicated workspace without a second rule to maintain.

    TerminalView {
        id: view
        anchors.fill: parent
        focus: true
        envName: terminalEnvName
        shareClipboard: terminalShareClipboard
        onTitleChanged: title => win.title = title
        // Close only after a clean exit. When `omavm open` fails (the Box
        // can't start, its engine is down) or the session dies, closing
        // would take the error printed above with it.
        onFinished: exitCode => {
            if (exitCode === 0) {
                win.close()
                return
            }
            endedBar.exitCode = exitCode
            endedBar.visible = true
        }
        onErrorOccurred: message => {
            errorLabel.text = message
            errorPanel.visible = true
        }
    }

    Connections {
        target: backend
        function onThemeChanged() { view.reloadPalette() }
    }

    // Once mapped, so Omarchy's Super+C/Super+V treat this window as a
    // terminal (Ctrl+Insert/Shift+Insert) instead of sending Ctrl+C.
    property bool taggedAsTerminal: false
    onActiveChanged: if (active && !taggedAsTerminal) {
        taggedAsTerminal = true
        backend.markAsTerminalWindow()
    }

    Rectangle {
        id: endedBar
        property int exitCode: 0
        visible: false
        anchors { left: parent.left; right: parent.right; bottom: parent.bottom }
        height: endedRow.implicitHeight + 16
        color: backend.themeSurface

        RowLayout {
            id: endedRow
            anchors { fill: parent; leftMargin: 12; rightMargin: 12 }
            Label {
                Layout.fillWidth: true
                color: backend.themeForeground
                wrapMode: Text.Wrap
                text: endedBar.exitCode > 128
                    ? qsTr("The session was terminated (signal %1).").arg(endedBar.exitCode - 128)
                    : qsTr("The session ended with an error (exit code %1).").arg(endedBar.exitCode)
            }
            Button {
                text: qsTr("Try Again")
                onClicked: {
                    endedBar.visible = false
                    view.restart()
                }
            }
            Button {
                text: qsTr("Close")
                onClicked: win.close()
            }
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
