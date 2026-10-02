import QtQuick
import QtQuick.Window
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts
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
        closeHint.opacity = 1
        closeHintTimer.start()
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
        clipboardDirection: displayClipboardDirection
        connectionFd: displayConnectionFd
        onConnectionFailed: message => {
            closeHint.opacity = 0
            errorLabel.text = message
            errorPanel.visible = true
        }
    }

    // The connection to the display ended (the guest shut down, QEMU went
    // away, the connection failed): say so and offer the ways back, since
    // in fullscreen there is no title bar to close or switch from.
    Pane {
        id: errorPanel
        objectName: "disconnectedPanel"
        anchors.centerIn: parent
        visible: false
        width: Math.min(parent.width - 80, 480)
        padding: 20
        Material.background: backend.themeSurface
        Material.elevation: 6

        ColumnLayout {
            anchors.fill: parent
            spacing: 12
            Label {
                Layout.fillWidth: true
                text: qsTr("Disconnected from %1").arg(displayEnvName || qsTr("the Machine"))
                font.pixelSize: 18
                font.weight: Font.DemiBold
                wrapMode: Text.Wrap
            }
            Label {
                id: errorLabel
                Layout.fillWidth: true
                color: backend.themeMuted
                wrapMode: Text.Wrap
            }
            RowLayout {
                Layout.alignment: Qt.AlignRight
                spacing: 8
                Button {
                    text: qsTr("Open OmaVM")
                    flat: true
                    onClicked: backend.showManager()
                }
                Button {
                    text: qsTr("Close")
                    flat: true
                    onClicked: win.close()
                }
                Button {
                    objectName: "reconnectButton"
                    // `omavm open` starts the Machine if it shut down and
                    // opens a new viewer; this one goes away first so the
                    // new one doesn't find it and step aside.
                    visible: displayEnvName !== ""
                    text: qsTr("Reconnect")
                    highlighted: true
                    onClicked: {
                        win.hide()
                        backend.reopenDisplay(displayEnvName)
                        Qt.quit()
                    }
                }
            }
        }
    }

    // Shown for a few seconds when the viewer opens: fullscreen hides the
    // title bar, and closing the window must not read as shutting down.
    Pane {
        id: closeHint
        objectName: "closeHint"
        anchors { horizontalCenter: parent.horizontalCenter; top: parent.top; topMargin: 24 }
        width: Math.min(parent.width - 48, 560)
        padding: 14
        Material.background: backend.themeSurface
        Material.elevation: 6
        opacity: 0
        visible: opacity > 0
        Behavior on opacity { NumberAnimation { duration: 250; easing.type: Easing.OutCubic } }
        Label {
            width: parent.width
            text: qsTr("Ctrl+Alt+Q closes this window. %1 keeps running; shut it down from OmaVM.").arg(displayEnvName || qsTr("The Machine"))
            wrapMode: Text.Wrap
            horizontalAlignment: Text.AlignHCenter
        }
        Timer {
            id: closeHintTimer
            interval: 6000
            onTriggered: closeHint.opacity = 0
        }
    }

    Shortcut {
        sequence: "Ctrl+Alt+Q"
        onActivated: win.close()
    }
}
