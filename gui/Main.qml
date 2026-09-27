import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts
import QtQuick.Window

ApplicationWindow {
    id: win
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
    Material.foreground: backend.themeForeground

    header: ToolBar {
        Material.background: backend.themeBackground
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: 14
            anchors.rightMargin: 8
            Label { text: qsTr("OmaVM"); font.pixelSize: 20; font.weight: Font.DemiBold }
            Label { text: qsTr("Experience Center"); color: backend.themeMuted; visible: win.width >= 520 }
            Item { Layout.fillWidth: true }
            ToolButton { text: "+"; font.pixelSize: 24; onClicked: createDialog.open(); ToolTip.text: qsTr("New Environment"); ToolTip.visible: hovered }
            ToolButton { text: "↻"; font.pixelSize: 20; onClicked: backend.refresh(); ToolTip.text: qsTr("Refresh"); ToolTip.visible: hovered }
        }
    }

    ListView {
        id: environmentList
        anchors.fill: parent
        model: backend.environments
        spacing: 12
        topMargin: 18
        bottomMargin: 18
        clip: true
        delegate: Item {
            required property var modelData
            width: environmentList.width
            height: card.implicitHeight
            EnvironmentCard {
                id: card
                width: Math.min(parent.width - 28, 860)
                anchors.horizontalCenter: parent.horizontalCenter
                environment: modelData
                onRemoveRequested: name => {
                    confirmDelete.environmentName = name
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
            }
        }
    }

    ColumnLayout {
        anchors.centerIn: parent
        visible: backend.environments.length === 0 && !backend.busy
        spacing: 14
        Label { Layout.alignment: Qt.AlignHCenter; text: "▣"; font.pixelSize: 58; opacity: 0.5 }
        Label { Layout.alignment: Qt.AlignHCenter; text: qsTr("No environments yet"); font.pixelSize: 22; font.weight: Font.DemiBold }
        Label { Layout.alignment: Qt.AlignHCenter; text: qsTr("Create a desktop or development environment to get started."); color: backend.themeMuted }
        Button { Layout.alignment: Qt.AlignHCenter; text: qsTr("New Environment"); highlighted: true; onClicked: createDialog.open() }
    }

    BusyIndicator { anchors.centerIn: parent; running: backend.busy; visible: running }
    CreateDialog { id: createDialog }
    SettingsDialog { id: settingsDialog }
    Dialog {
        id: confirmForceStop
        property string environmentName
        title: qsTr("Force Stop Machine")
        modal: true
        anchors.centerIn: parent
        width: Math.min(parent.width - 32, 420)
        standardButtons: Dialog.Cancel | Dialog.Yes
        contentItem: Label {
            text: qsTr("Force stop “%1”? Unsaved data in the guest may be lost.").arg(confirmForceStop.environmentName)
            wrapMode: Text.Wrap
            color: backend.themeRed
        }
        onAccepted: backend.forceStop(environmentName)
    }
    Dialog {
        id: confirmDelete
        property string environmentName
        title: qsTr("Delete Environment")
        modal: true
        anchors.centerIn: parent
        width: Math.min(parent.width - 32, 420)
        standardButtons: Dialog.Cancel | Dialog.Yes
        contentItem: Label {
            text: qsTr("Delete “%1”? This cannot be undone.").arg(confirmDelete.environmentName)
            wrapMode: Text.Wrap
            color: backend.themeRed
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
        Label { id: toastLabel; wrapMode: Text.Wrap; width: Math.min(420, implicitWidth) }
        Timer { id: toastTimer; interval: 4500; onTriggered: toast.close() }
    }
    Connections {
        target: backend
        function onMessage(text, error) { toast.text = text; toast.open(); toastTimer.restart() }
    }
    Component.onCompleted: backend.refresh()
}
