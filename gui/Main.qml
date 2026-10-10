import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts
import QtQuick.Window

ApplicationWindow {
    id: win
    // Tooltips show names and CLI hints: plain text, like every Label.
    Binding {
        target: ToolTip.toolTip.contentItem
        property: "textFormat"
        value: Text.PlainText
        when: ToolTip.toolTip.contentItem !== null && ToolTip.toolTip.contentItem.textFormat !== undefined
    }
    width: 960
    height: 640
    minimumWidth: 320
    minimumHeight: 420
    visible: true
    title: qsTr("OmaVM")
    color: backend.themeBackground
    Material.theme: backend.themeMode === "light" ? Material.Light : Material.Dark
    Material.accent: backend.themeAccent
    Material.primary: backend.themeBackground
    Material.background: backend.themeBackground
    Material.foreground: backend.themeForeground

    header: ToolBar {
        Material.background: backend.themeBackground
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: 14
            anchors.rightMargin: 8
            Label { textFormat: Text.PlainText; text: qsTr("OmaVM"); font.pixelSize: 20; font.weight: Font.DemiBold }
            Label { textFormat: Text.PlainText; text: qsTr("Experience Center"); color: backend.themeMuted; visible: win.width >= 520 }
            Label {
                textFormat: Text.PlainText
                visible: backend.busy && win.width >= 420
                text: backend.busyAction !== "" ? qsTr("Working: %1…").arg(backend.busyAction) : qsTr("Working…")
                color: backend.themeMuted
                elide: Text.ElideRight
                Layout.maximumWidth: 260
            }
            Item { Layout.fillWidth: true }
            ToolButton {
                icon.source: "qrc:/icons/add.svg"
                icon.color: backend.themeForeground
                onClicked: createDialog.open()
                ToolTip.text: qsTr("New Environment")
                ToolTip.visible: hovered
                Accessible.name: qsTr("New Environment")
            }
            ToolButton {
                id: refreshButton
                icon.source: "qrc:/icons/refresh.svg"
                icon.color: backend.themeForeground
                // Spins once on a manual click only — not on the silent
                // background poll (Timer below), which never calls this
                // handler — so clicking Refresh gets visible feedback
                // without the icon spinning every few seconds on its own.
                onClicked: { refreshSpin.restart(); backend.refresh() }
                ToolTip.text: qsTr("Refresh")
                ToolTip.visible: hovered
                Accessible.name: qsTr("Refresh")
                RotationAnimation {
                    id: refreshSpin
                    target: refreshButton
                    property: "rotation"
                    from: 0
                    to: 360
                    duration: 500
                    easing.type: Easing.InOutCubic
                }
            }
        }
    }

    // Synced in place, not backend.environments directly: see
    // EnvironmentListModel.qml (a status change used to recreate every
    // card, replaying the fade-in and closing any open card menu).
    EnvironmentListModel { id: environmentModel }
    Connections {
        target: backend
        function onEnvironmentsChanged() { environmentModel.sync(backend.environments) }
    }

    ListView {
        id: environmentList
        anchors.fill: parent
        model: environmentModel
        spacing: 12
        topMargin: 18
        bottomMargin: 18
        clip: true
        populate: Transition {
            NumberAnimation { property: "opacity"; from: 0; to: 1; duration: 220; easing.type: Easing.OutCubic }
        }
        add: Transition {
            NumberAnimation { property: "opacity"; from: 0; to: 1; duration: 220; easing.type: Easing.OutCubic }
            NumberAnimation { property: "y"; from: 24; duration: 220; easing.type: Easing.OutCubic }
        }
        remove: Transition {
            NumberAnimation { property: "opacity"; to: 0; duration: 160; easing.type: Easing.InCubic }
        }
        displaced: Transition {
            NumberAnimation { properties: "x,y"; duration: 200; easing.type: Easing.OutCubic }
        }
        delegate: Item {
            required property var environment
            width: environmentList.width
            height: card.implicitHeight
            EnvironmentCard {
                id: card
                width: Math.min(parent.width - 28, 860)
                anchors.horizontalCenter: parent.horizontalCenter
                environment: parent.environment
                onRemoveRequested: name => {
                    confirmDelete.environmentName = name
                    confirmDelete.environmentKind = environment.kind
                    confirmDelete.open()
                }
                onForceStopRequested: name => {
                    confirmForceStop.environmentName = name
                    confirmForceStop.open()
                }
                onSettingsRequested: environment => {
                    settingsDialog.environment = environment
                    settingsDialog.open()
                }
                onSnapshotsRequested: environment => {
                    snapshotsDialog.environment = environment
                    snapshotsDialog.open()
                }
                onAppsRequested: environment => {
                    appsDialog.environment = environment
                    appsDialog.open()
                }
                onCloneRequested: environment => {
                    cloneDialog.environment = environment
                    cloneDialog.open()
                }
            }
        }
    }

    ColumnLayout {
        anchors.centerIn: parent
        anchors.margins: 24
        width: Math.min(parent.width - 48, 420)
        visible: backend.environments.length === 0 && backend.listError === ""
        spacing: 14
        Icon { Layout.alignment: Qt.AlignHCenter; source: "qrc:/icons/app-mono.svg"; color: backend.themeMuted; iconSize: 58; opacity: 0.5 }
        Label { textFormat: Text.PlainText; Layout.alignment: Qt.AlignHCenter; text: qsTr("No environments yet"); font.pixelSize: 22; font.weight: Font.DemiBold }
        Label {
            textFormat: Text.PlainText
            Layout.fillWidth: true
            Layout.alignment: Qt.AlignHCenter
            horizontalAlignment: Text.AlignHCenter
            text: qsTr("Create a desktop or development environment to get started.")
            color: backend.themeMuted
            wrapMode: Text.Wrap
        }
        Button { Layout.alignment: Qt.AlignHCenter; text: qsTr("New Environment"); highlighted: true; onClicked: createDialog.open() }
    }

    ColumnLayout {
        anchors.centerIn: parent
        anchors.margins: 24
        width: Math.min(parent.width - 48, 420)
        visible: backend.environments.length === 0 && backend.listError !== ""
        spacing: 14
        Icon { Layout.alignment: Qt.AlignHCenter; source: "qrc:/icons/warning.svg"; color: backend.themeRed; iconSize: 48 }
        Label { textFormat: Text.PlainText; Layout.alignment: Qt.AlignHCenter; text: qsTr("Could not load environments"); font.pixelSize: 20; font.weight: Font.DemiBold }
        Label {
            textFormat: Text.PlainText
            Layout.fillWidth: true
            Layout.alignment: Qt.AlignHCenter
            horizontalAlignment: Text.AlignHCenter
            text: backend.listError
            color: backend.themeMuted
            wrapMode: Text.Wrap
        }
        Button { Layout.alignment: Qt.AlignHCenter; text: qsTr("Try Again"); onClicked: backend.refresh() }
    }

    BusyIndicator {
        anchors.centerIn: parent
        // Loading the list; a card shows its own action.
        running: backend.busy && backend.environments.length === 0
        opacity: running ? 1 : 0
        visible: opacity > 0
        Behavior on opacity { NumberAnimation { duration: 150; easing.type: Easing.OutCubic } }
    }
    CreateDialog {
        id: createDialog
        onCreated: (name, kind, prepared) => {
            firstUse.environmentName = name
            firstUse.environmentKind = kind
            firstUse.prepared = prepared
            firstUse.login = ""
            firstUse.open()
            if (prepared)
                backend.readCredentials(name)
        }
    }
    // Creation leads to first use (docs/TODO.md P1): the terminal or the
    // installer is one click away, and "Later" leaves the new card ready.
    // backend.open refuses a second click while the first is running, so
    // this never starts a second copy.
    Dialog {
        id: firstUse
        objectName: "firstUseDialog"
        // The title holds the environment's name: shown as typed, never read
        // as HTML (the header is the style's own Label).
        Binding {
            target: firstUse.header
            property: "textFormat"
            value: Text.PlainText
            when: firstUse.header !== null && firstUse.header.textFormat !== undefined
        }
        property string environmentName
        property string environmentKind
        property bool prepared: false
        property string login: ""
        readonly property bool machine: environmentKind === "machine"
        title: qsTr("“%1” is ready").arg(environmentName)
        modal: true
        anchors.centerIn: parent
        width: Math.min(parent.width - 32, 420)
        standardButtons: Dialog.Cancel | Dialog.Ok
        Component.onCompleted: {
            standardButton(Dialog.Cancel).text = qsTr("Later")
        }
        onAboutToShow: standardButton(Dialog.Ok).text = prepared ? qsTr("Start Setup")
            : machine ? qsTr("Start Installer") : qsTr("Open Terminal")
        Connections {
            target: backend
            function onActionFinished(tag, ok, text) {
                if (tag === "credentials:" + firstUse.environmentName && ok)
                    firstUse.login = text.trim()
            }
        }
        Overlay.modal: ThemeScrim {}
        // In a layout: as the contentItem itself, the label's unwrapped
        // width fed back into the dialog's size (binding loop).
        contentItem: ColumnLayout {
            Label {
                Layout.fillWidth: true
                textFormat: Text.PlainText
                text: firstUse.prepared
                    ? qsTr("Start the first setup. It downloads the graphical desktop and restarts automatically. Keep the Machine running and connected to the internet.")
                    : firstUse.machine
                    ? qsTr("Start it now to boot the installation media. When the installation is done, shut it down and disconnect the media in Settings.")
                    : qsTr("Open its terminal now, or later from its card or the app launcher.")
                wrapMode: Text.Wrap
            }
            TextArea {
                Layout.fillWidth: true
                visible: firstUse.prepared
                readOnly: true
                selectByMouse: true
                wrapMode: TextEdit.Wrap
                text: firstUse.login || qsTr("Reading the initial login…")
                Accessible.name: qsTr("Initial Ubuntu login")
            }
        }
        onAccepted: backend.open(environmentName, environmentKind)
    }
    SettingsDialog { id: settingsDialog }
    CloneDialog { id: cloneDialog }
    SnapshotsDialog { id: snapshotsDialog }
    AppsDialog { id: appsDialog }
    Dialog {
        id: confirmForceStop
        property string environmentName
        title: qsTr("Force Stop Machine")
        modal: true
        anchors.centerIn: parent
        width: Math.min(parent.width - 32, 420)
        standardButtons: Dialog.Cancel | Dialog.Yes
        // A generic "Yes" button doesn't say what's about to happen — the
        // title already names the destructive action, so the button
        // should too (docs/TODO.md P1 "verbos específicos nas
        // confirmações").
        Component.onCompleted: {
            const yesButton = standardButton(Dialog.Yes)
            if (yesButton) yesButton.text = qsTr("Force Stop")
        }
        Overlay.modal: ThemeScrim {}
        // In a layout: as the contentItem itself, the label's unwrapped
        // width fed back into the dialog's size (binding loop).
        contentItem: ColumnLayout {
            Label {
                Layout.fillWidth: true
                textFormat: Text.PlainText
                text: qsTr("Force stop “%1”? Unsaved data in the guest may be lost.").arg(confirmForceStop.environmentName)
                wrapMode: Text.Wrap
                color: backend.themeRed
            }
        }
        onAccepted: backend.forceStop(environmentName)
    }
    Dialog {
        id: confirmDelete
        property string environmentName
        property string environmentKind
        title: qsTr("Delete Environment")
        modal: true
        anchors.centerIn: parent
        width: Math.min(parent.width - 32, 420)
        standardButtons: Dialog.Cancel | Dialog.Yes
        Component.onCompleted: {
            const yesButton = standardButton(Dialog.Yes)
            if (yesButton) yesButton.text = qsTr("Delete")
        }
        Overlay.modal: ThemeScrim {}
        // In a layout: as the contentItem itself, the label's unwrapped
        // width fed back into the dialog's size (binding loop).
        contentItem: ColumnLayout {
            Label {
                Layout.fillWidth: true
                textFormat: Text.PlainText
                text: confirmDelete.environmentKind === "machine"
                    ? qsTr("Delete “%1”? Its virtual disk and every snapshot are removed. This cannot be undone.").arg(confirmDelete.environmentName)
                    : qsTr("Delete “%1”? Its container is removed; files under your Omarchy home are not touched. This cannot be undone.").arg(confirmDelete.environmentName)
                wrapMode: Text.Wrap
                color: backend.themeRed
            }
        }
        onAccepted: backend.remove(environmentName)
    }
    Popup {
        id: toast
        x: Math.round((parent.width - width) / 2)
        y: parent.height - height - 24
        padding: 14
        Material.background: backend.themeSurface
        closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutside
        property alias text: toastLabel.text
        Label { textFormat: Text.PlainText; id: toastLabel; wrapMode: Text.Wrap; width: Math.min(420, implicitWidth) }
        Timer { id: toastTimer; interval: 4500; onTriggered: toast.close() }
    }

    // Errors stay on screen until the user dismisses them (or a later
    // action succeeds): docs/TODO.md P0 wants the cause and next step to
    // remain readable past the 4.5s a toast lives, with a copyable detail.
    Pane {
        id: errorBanner
        property string text: ""
        // The action the error came from: only its own later success
        // clears it, never another environment's.
        property string source: ""
        anchors.top: parent.top
        anchors.left: parent.left
        anchors.right: parent.right
        z: 20
        padding: 12
        opacity: text !== "" ? 1 : 0
        visible: opacity > 0
        transform: Translate { y: (1 - errorBanner.opacity) * -24 }
        Material.background: backend.themeSurface
        Material.elevation: 4

        Behavior on opacity { NumberAnimation { duration: 180; easing.type: Easing.OutCubic } }

        RowLayout {
            anchors.fill: parent
            spacing: 10
            Icon { source: "qrc:/icons/warning.svg"; color: backend.themeRed; iconSize: 18 }
            Label {
                textFormat: Text.PlainText
                Layout.fillWidth: true
                text: errorBanner.text
                color: backend.themeRed
                wrapMode: Text.Wrap
            }
            Button {
                text: qsTr("Copy details")
                flat: true
                onClicked: backend.copyToClipboard(errorBanner.text)
            }
            ToolButton {
                icon.source: "qrc:/icons/close.svg"
                icon.color: backend.themeRed
                Accessible.name: qsTr("Dismiss")
                onClicked: errorBanner.text = ""
            }
        }
    }

    Connections {
        target: backend
        function onMessage(text, error, source) {
            if (error) {
                errorBanner.text = text
                errorBanner.source = source
            } else {
                toast.text = text; toast.open(); toastTimer.restart()
            }
        }
        function onSucceeded(source) {
            if (source === errorBanner.source) errorBanner.text = ""
        }
    }
    // Converges on the real state without the user pressing Refresh
    // (docs/TODO.md P1): a light poll while the window has focus, not a
    // background daemon — stops the moment focus is lost. The silent poll
    // does not flash loading UI or disable controls every three seconds.
    Timer {
        interval: 3000
        running: win.active
        repeat: true
        onTriggered: backend.poll()
    }

    Component.onCompleted: backend.refresh()
}
