import QtQuick
import QtQuick.Window
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts
import OmaVM 1.0

Window {
    id: win
    // Tooltips show names and CLI hints: plain text, like every Label.
    Binding {
        target: ToolTip.toolTip.contentItem
        property: "textFormat"
        value: Text.PlainText
        when: ToolTip.toolTip.contentItem !== null && ToolTip.toolTip.contentItem.textFormat !== undefined
    }
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
        if (displayRevealAfterFirmware)
            revealTimer.start()
        else
            reveal()
    }

    // A Machine opened while off stays out of sight during its firmware
    // and boot menu, and appears once its own system has taken over
    // (DisplayView.operatingSystemStarted). A system without a USB tablet
    // driver never says so: the timer shows it anyway.
    property string pendingError: ""
    function showDisconnected() {
        win.reveal()
        closeHint.opacity = 0
        errorLabel.text = win.pendingError
        errorPanel.visible = true
    }
    Connections {
        target: backend
        function onShutDownChecked(poweredOff) {
            if (poweredOff)
                Qt.quit()
            else
                win.showDisconnected()
        }
    }

    function reveal() {
        if (win.visible)
            return
        revealTimer.stop()
        if (displayRevealAfterFirmware && displayEmptyWorkspace)
            backend.placeViewerWindow(displayTitle)
        if (displayFullscreen)
            showFullScreen()
        else
            show()
        closeHint.opacity = 1
        closeHintTimer.start()
    }
    Timer {
        id: revealTimer
        interval: 30000
        onTriggered: win.reveal()
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
        onOperatingSystemStarted: win.reveal()
        // A Machine that powered off (from inside, Shut Down, Stop) takes
        // its viewer with it; only a display lost some other way (QEMU
        // killed or crashed) stays on screen to say so.
        onConnectionFailed: message => {
            win.pendingError = message
            if (displayEnvName !== "")
                backend.checkShutDown(displayEnvName)
            else
                win.showDisconnected()
        }
    }

    // Files dropped on the window are copied into the folder this session
    // shares, as in Parallels; the guest finds them in its shared folder.
    DropArea {
        id: drop
        objectName: "fileDrop"
        anchors.fill: parent
        enabled: displaySharedFolder !== ""
        onEntered: drag => drag.accepted = drag.hasUrls
        onDropped: drop => {
            backend.copyIntoFolder(drop.urls, displaySharedFolder)
            drop.accept(Qt.CopyAction)
            view.forceActiveFocus()
        }
    }
    Rectangle {
        anchors.fill: parent
        visible: drop.containsDrag
        color: Qt.rgba(0, 0, 0, 0.55)
        border.width: 3
        border.color: backend.themeAccent
        Label {
            textFormat: Text.PlainText
            anchors.centerIn: parent
            width: parent.width - 64
            horizontalAlignment: Text.AlignHCenter
            wrapMode: Text.Wrap
            text: qsTr("Drop to copy into %1").arg(displaySharedFolder)
            font.pixelSize: 20
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
                textFormat: Text.PlainText
                Layout.fillWidth: true
                text: qsTr("Disconnected from %1").arg(displayEnvName || qsTr("the Machine"))
                font.pixelSize: 18
                font.weight: Font.DemiBold
                wrapMode: Text.Wrap
            }
            Label {
                textFormat: Text.PlainText
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
                        const shown = win.visibility
                        win.hide()
                        if (backend.reopenDisplay(displayEnvName)) {
                            Qt.quit()
                            return
                        }
                        // Nothing replaces this viewer: keep it, as it was, with why.
                        errorLabel.text = qsTr("Could not run omavm to reconnect. Check that OmaVM is installed, then try again.")
                        win.visibility = shown
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
            textFormat: Text.PlainText
            width: parent.width
            text: qsTr("Point at the top edge or press Ctrl+Alt+M for %1's controls. Ctrl+Alt+Q closes this window; %1 keeps running.").arg(displayEnvName || qsTr("the Machine"))
            wrapMode: Text.Wrap
            horizontalAlignment: Text.AlignHCenter
        }
        Timer {
            id: closeHintTimer
            interval: 6000
            onTriggered: closeHint.opacity = 0
        }
    }

    // The Machine's controls, as in Parallels' fullscreen: pointing at the
    // top edge (or Ctrl+Alt+M) slides them in. Every button leaves the
    // keyboard with the guest (no focus), and an action's result comes back
    // as a short note under the bar.
    property bool controlsShown: false
    function showControls() { controlsShown = true; controlsTimer.restart() }
    function act(note, action) {
        action()
        controlsNote.text = note
        controlsTimer.restart()
        view.forceActiveFocus()
    }
    Item {
        // A thin strip along the top: hovering it doesn't take the guest's
        // pointer (a HoverHandler doesn't accept presses).
        anchors { left: parent.left; right: parent.right; top: parent.top }
        height: 3
        HoverHandler { onHoveredChanged: if (hovered) win.showControls() }
    }
    Timer {
        id: controlsTimer
        interval: 2500
        onTriggered: if (!controlsHover.hovered) { win.controlsShown = false; controlsNote.text = "" }
    }
    Pane {
        id: controls
        objectName: "viewerControls"
        anchors.horizontalCenter: parent.horizontalCenter
        y: win.controlsShown ? 8 : -height - 8
        visible: y > -height - 8
        padding: 6
        Material.background: backend.themeSurface
        Material.elevation: 8
        Behavior on y { NumberAnimation { duration: 180; easing.type: Easing.OutCubic } }
        HoverHandler { id: controlsHover; onHoveredChanged: if (!hovered) controlsTimer.restart() }

        ColumnLayout {
            spacing: 2
            RowLayout {
                spacing: 2
                Label {
                    textFormat: Text.PlainText
                    text: displayEnvName
                    font.weight: Font.DemiBold
                    leftPadding: 10
                    rightPadding: 10
                }
                ToolButton {
                    objectName: "ctrlAltDelButton"
                    text: qsTr("Ctrl+Alt+Del")
                    focusPolicy: Qt.NoFocus
                    onClicked: win.act(qsTr("Sent Ctrl+Alt+Del"), () => view.sendCtrlAltDel())
                }
                ToolButton {
                    objectName: "snapshotButton"
                    text: qsTr("Take Snapshot")
                    focusPolicy: Qt.NoFocus
                    onClicked: win.act(qsTr("Taking a snapshot…"), () => backend.createSnapshot(displayEnvName,
                        qsTr("From the window, %1").arg(Qt.formatDateTime(new Date(), "yyyy-MM-dd hh:mm"))))
                }
                ToolButton {
                    objectName: "fullscreenButton"
                    text: win.visibility === Window.FullScreen ? qsTr("Exit Full Screen") : qsTr("Full Screen")
                    focusPolicy: Qt.NoFocus
                    onClicked: win.act("", () => win.visibility === Window.FullScreen ? win.showNormal() : win.showFullScreen())
                }
                ToolButton {
                    text: qsTr("Open OmaVM")
                    focusPolicy: Qt.NoFocus
                    onClicked: win.act("", () => backend.showManager())
                }
                ToolButton {
                    objectName: "shutDownButton"
                    // An ordinary shutdown: the guest is asked to power off
                    // and saves its work; never a power cut.
                    text: qsTr("Shut Down")
                    focusPolicy: Qt.NoFocus
                    onClicked: win.act(qsTr("Asked %1 to shut down…").arg(displayEnvName), () => backend.stop(displayEnvName))
                }
            }
            Label {
                textFormat: Text.PlainText
                id: controlsNote
                objectName: "controlsNote"
                Layout.fillWidth: true
                visible: text !== ""
                color: backend.themeMuted
                horizontalAlignment: Text.AlignHCenter
                wrapMode: Text.Wrap
                bottomPadding: 4
            }
        }
    }
    Connections {
        target: backend
        function onActionFinished(tag, ok, text) {
            if (text === "")
                return
            controlsNote.text = text
            win.showControls()
        }
    }

    Shortcut {
        sequence: "Ctrl+Alt+M"
        onActivated: win.controlsShown ? win.controlsShown = false : win.showControls()
    }
    Shortcut {
        sequence: "Ctrl+Alt+Q"
        onActivated: win.close()
    }
}
