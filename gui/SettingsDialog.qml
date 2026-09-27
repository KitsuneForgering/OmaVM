import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Dialogs
import QtQuick.Layouts

Dialog {
    id: dialog
    property var environment: ({})
    readonly property bool machine: environment.kind === "machine"
    title: qsTr("%1 Settings").arg(environment.name || qsTr("Environment"))
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 520)
    standardButtons: Dialog.Cancel | Dialog.Save

    onOpened: {
        const settings = environment.settings || {}
        description.text = settings.description || ""
        cpus.value = settings.cpus || 2
        memory.value = settings.memory_mib || 2048
        sharedPath.text = settings.shared_path || ""
        sharedReadOnly.checked = settings.shared_read_only || false
    }

    ColumnLayout {
        width: parent.width
        spacing: 16

        Label { text: qsTr("General"); font.pixelSize: 18; font.weight: Font.DemiBold }
        TextArea {
            id: description
            Layout.fillWidth: true
            Layout.preferredHeight: 82
            placeholderText: qsTr("Description")
            wrapMode: TextEdit.Wrap
            onTextChanged: if (text.length > 500) text = text.slice(0, 500)
        }

        Label {
            visible: dialog.machine
            text: qsTr("Hardware")
            font.pixelSize: 18
            font.weight: Font.DemiBold
        }
        GridLayout {
            visible: dialog.machine
            Layout.fillWidth: true
            columns: 2
            columnSpacing: 16
            rowSpacing: 12
            Label { text: qsTr("Processors") }
            SpinBox { id: cpus; Layout.fillWidth: true; from: 1; to: 64 }
            Label { text: qsTr("Memory (MiB)") }
            SpinBox { id: memory; Layout.fillWidth: true; from: 256; to: 262144; stepSize: 256; editable: true }
        }
        Label {
            visible: dialog.machine
            Layout.fillWidth: true
            text: qsTr("Hardware changes take effect the next time the Machine starts.")
            color: backend.themeMuted
            wrapMode: Text.Wrap
        }

        Label {
            visible: dialog.machine
            text: qsTr("Shared folder")
            font.pixelSize: 18
            font.weight: Font.DemiBold
        }
        RowLayout {
            visible: dialog.machine
            Layout.fillWidth: true
            TextField {
                id: sharedPath
                Layout.fillWidth: true
                placeholderText: qsTr("Host folder (optional)")
                selectByMouse: true
            }
            Button {
                text: qsTr("Choose…")
                onClicked: folderPicker.open()
            }
        }
        CheckBox {
            id: sharedReadOnly
            visible: dialog.machine && sharedPath.text.length > 0
            text: qsTr("Read-only in the guest")
        }
        Label {
            visible: dialog.machine && sharedPath.text.length > 0
            Layout.fillWidth: true
            text: qsTr("Mount the tag omavm-share in the guest. Changes apply on the next start.")
            color: backend.themeMuted
            wrapMode: Text.Wrap
        }
    }

    FolderDialog {
        id: folderPicker
        title: qsTr("Choose a folder to share")
        onAccepted: sharedPath.text = selectedFolder.toString().replace(/^file:\/\//, "")
    }

    onAccepted: backend.configure(environment.name, description.text.trim(), cpus.value, memory.value, dialog.machine, sharedPath.text.trim(), sharedReadOnly.checked)
}
