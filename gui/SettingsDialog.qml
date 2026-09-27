import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Dialogs
import QtQuick.Layouts

Dialog {
    id: dialog
    property var environment: ({})
    readonly property bool machine: environment.kind === "machine"
    // Must match core.EnvironmentColors (internal/core/environment.go) —
    // that's the source of truth the CLI validates --color against.
    readonly property var palette: ["red", "orange", "yellow", "green", "blue", "purple", "gray"]
    property string selectedColor: ""
    title: qsTr("%1 Settings").arg(environment.name || qsTr("Environment"))
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 520)
    standardButtons: Dialog.Cancel | Dialog.Save
    Overlay.modal: ThemeScrim {}

    onOpened: {
        const settings = environment.settings || {}
        description.text = settings.description || ""
        cpus.value = settings.cpus || 2
        memory.value = settings.memory_mib || 2048
        sharedPath.text = settings.shared_path || ""
        sharedReadOnly.checked = settings.shared_read_only || false
        disconnectISO.checked = settings.disconnect_iso || false
        selectedColor = settings.color || ""
        // Opt-out: absent/false in the JSON means "not disabled", i.e. on.
        shareClipboard.checked = !settings.clipboard_disabled
        travelMode.checked = !settings.travel_mode_disabled
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
        RowLayout {
            Layout.fillWidth: true
            spacing: 8
            Label { text: qsTr("Color tag:"); color: backend.themeMuted }
            ToolButton {
                implicitWidth: 22
                implicitHeight: 22
                Accessible.name: qsTr("No color")
                contentItem: Rectangle {
                    radius: 11
                    color: "transparent"
                    border.width: dialog.selectedColor === "" ? 2 : 1
                    border.color: dialog.selectedColor === "" ? backend.themeAccent : backend.themeMuted
                }
                onClicked: dialog.selectedColor = ""
            }
            Repeater {
                model: dialog.palette
                ToolButton {
                    required property string modelData
                    implicitWidth: 22
                    implicitHeight: 22
                    Accessible.name: modelData
                    contentItem: Rectangle {
                        radius: 11
                        color: modelData
                        border.width: dialog.selectedColor === modelData ? 3 : 1
                        border.color: dialog.selectedColor === modelData ? backend.themeAccent : backend.themeMuted
                    }
                    onClicked: dialog.selectedColor = modelData
                }
            }
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
            text: qsTr("Automation")
            font.pixelSize: 18
            font.weight: Font.DemiBold
        }
        CheckBox {
            id: shareClipboard
            visible: dialog.machine
            text: qsTr("Share text clipboard with the guest")
            ToolTip.visible: hovered
            ToolTip.text: qsTr("Requires spice-vdagent in the guest desktop. Restart the Machine to apply a change.")
        }
        CheckBox {
            id: travelMode
            visible: dialog.machine
            text: qsTr("Travel Mode: reduce CPUs automatically on battery")
            ToolTip.visible: hovered
            ToolTip.text: qsTr("Only applies when you haven't pinned a custom CPU count.")
        }

        Label {
            visible: dialog.machine
            text: qsTr("Shared folder")
            font.pixelSize: 18
            font.weight: Font.DemiBold
        }
        CheckBox {
            id: disconnectISO
            visible: dialog.machine
            text: qsTr("Installation finished — disconnect ISO on next start")
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

    onAccepted: backend.configure(environment.name, description.text.trim(), cpus.value, memory.value, dialog.machine, sharedPath.text.trim(), sharedReadOnly.checked, disconnectISO.checked, dialog.selectedColor, shareClipboard.checked, travelMode.checked)
}
