import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts

// Clone: a full, independent copy of a stopped environment. Says what is
// copied and what it costs before anything happens (UX Principle #10).
Dialog {
    id: dialog
    objectName: "cloneDialog"
    property var environment: ({})
    readonly property bool machine: environment.kind === "machine"
    readonly property bool stopped: environment.status === "stopped"
    property bool submitting: false
    property string errorText: ""
    title: qsTr("Clone %1").arg(environment.name || "")
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 460)
    Overlay.modal: ThemeScrim {}

    onOpened: {
        newName.text = qsTr("%1 copy").arg(environment.name || "")
        newName.selectAll()
        newName.forceActiveFocus()
        errorText = ""
        submitting = false
    }

    Connections {
        target: backend
        function onActionFinished(tag, ok, text) {
            if (tag !== "clone" || !dialog.submitting) return
            dialog.submitting = false
            if (ok)
                dialog.close()
            else
                dialog.errorText = text
        }
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 12
        Label {
            objectName: "cloneCost"
            Layout.fillWidth: true
            wrapMode: Text.Wrap
            color: backend.themeMuted
            text: dialog.machine
                ? qsTr("Copies the whole disk of %1, snapshots included. It needs as much free space as that disk uses now (on btrfs the copy shares space until one of them changes). The copy is independent: changing one never touches the other.").arg(dialog.environment.name || "")
                : qsTr("Copies the system of %1: its installed packages and files outside your home folder. Your home folder is shared with this computer, not copied. The copy is independent: changing one never touches the other.").arg(dialog.environment.name || "")
        }
        Label {
            objectName: "stopFirst"
            Layout.fillWidth: true
            visible: !dialog.stopped
            wrapMode: Text.Wrap
            color: backend.themeAccentText
            text: dialog.machine ? qsTr("Shut %1 down first: a running system can't be copied.").arg(dialog.environment.name || "")
                                 : qsTr("Stop %1 first: a running Box can't be copied.").arg(dialog.environment.name || "")
        }
        TextField {
            id: newName
            objectName: "cloneName"
            Layout.fillWidth: true
            Accessible.name: qsTr("Name of the copy")
            placeholderText: qsTr("Name of the copy")
            selectByMouse: true
            enabled: !dialog.submitting
            onAccepted: cloneButton.clicked()
        }
        Label {
            Layout.fillWidth: true
            visible: dialog.errorText !== ""
            text: dialog.errorText
            color: backend.themeRed
            wrapMode: Text.Wrap
        }
        RowLayout {
            Layout.alignment: Qt.AlignRight
            Button {
                text: qsTr("Cancel")
                flat: true
                onClicked: dialog.close()
            }
            Button {
                id: cloneButton
                objectName: "cloneButton"
                text: dialog.submitting ? qsTr("Cloning…") : qsTr("Clone")
                highlighted: true
                enabled: dialog.stopped && !dialog.submitting && newName.text.trim().length > 0
                onClicked: {
                    dialog.errorText = ""
                    dialog.submitting = true
                    backend.cloneEnvironment(dialog.environment.name, newName.text.trim())
                }
            }
        }
    }
}
