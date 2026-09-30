import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts

Dialog {
    id: dialog
    property var environment: ({})
    readonly property bool busy: !!(backend.busyEnvironments && backend.busyEnvironments[environment.name])
    readonly property var snapshots: environment.snapshots || []
    // Mirrors internal/core/environment.go's defaultSnapshotLimit: the CLI
    // only emits snapshot_limit in JSON when it was explicitly customized
    // (omitempty), so an absent value means the Core-side default applies.
    readonly property int retentionLimit: (environment.settings && environment.settings.snapshot_limit) || 10
    // Going to a snapshot replaces the disk, which QEMU only does with
    // the Machine shut down.
    readonly property bool canGoTo: environment.status === "stopped"
    property string createError: ""
    property bool creating: false
    title: qsTr("%1 Snapshots").arg(environment.name || qsTr("Environment"))
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 480)
    height: Math.min(parent.height - 64, 460)
    standardButtons: Dialog.Close
    Overlay.modal: ThemeScrim {}

    onOpened: {
        newLabel.text = ""
        createError = ""
        creating = false
    }

    // Keeps the list current without closing the dialog after create/go-to/
    // delete (docs/TODO.md P0): backend.environments refreshes after every
    // action, so re-find this same environment by name (its stable identity)
    // and re-bind to the fresh snapshot list.
    Connections {
        target: backend
        function onEnvironmentsChanged() {
            if (!dialog.visible)
                return
            for (const env of backend.environments) {
                if (env.name === dialog.environment.name) {
                    dialog.environment = env
                    return
                }
            }
        }
        function onActionFinished(tag, ok, text) {
            if (tag !== "snapshot-create")
                return
            dialog.creating = false
            if (ok) {
                dialog.createError = ""
                newLabel.text = ""
            } else {
                dialog.createError = text
            }
        }
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 12

        RowLayout {
            Layout.fillWidth: true
            TextField {
                id: newLabel
                Layout.fillWidth: true
                Accessible.name: qsTr("Snapshot label")
                placeholderText: qsTr("Label (e.g. Before system upgrade)")
                selectByMouse: true
                enabled: !dialog.creating
                onAccepted: createButton.clicked()
            }
            Button {
                id: createButton
                text: dialog.creating ? qsTr("Creating…") : qsTr("Create")
                highlighted: true
                enabled: !dialog.creating && !dialog.busy && newLabel.text.trim().length > 0
                onClicked: {
                    dialog.createError = ""
                    dialog.creating = true
                    backend.createSnapshot(dialog.environment.name, newLabel.text.trim())
                }
            }
        }

        Label {
            Layout.fillWidth: true
            opacity: dialog.createError !== "" ? 1 : 0
            visible: opacity > 0
            text: dialog.createError
            color: backend.themeRed
            wrapMode: Text.Wrap
            Behavior on opacity { NumberAnimation { duration: 180; easing.type: Easing.OutCubic } }
        }

        Label {
            Layout.fillWidth: true
            text: dialog.snapshots.length >= dialog.retentionLimit
                ? qsTr("Keeping the most recent %1 snapshots. Creating another one will remove the oldest: “%2”.")
                      .arg(dialog.retentionLimit).arg(dialog.snapshots.length > 0 ? dialog.snapshots[0].label : "")
                : qsTr("Keeping up to %1 snapshots; the oldest is removed automatically after that.").arg(dialog.retentionLimit)
            color: backend.themeMuted
            wrapMode: Text.Wrap
            font.pixelSize: 12
        }

        Label {
            Layout.fillWidth: true
            visible: !dialog.canGoTo
            objectName: "runningHint"
            text: qsTr("While %1 is running, a snapshot saves its disk as if the power had been cut, without open windows or memory. Shut it down to go to a snapshot.").arg(dialog.environment.name || "")
            color: backend.themeMuted
            wrapMode: Text.Wrap
            font.pixelSize: 12
        }

        Label {
            Layout.fillWidth: true
            visible: dialog.snapshots.length === 0
            text: qsTr("No snapshots yet. Snapshots let you go back to a saved point in time.")
            color: backend.themeMuted
            wrapMode: Text.Wrap
        }

        ListView {
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            spacing: 6
            model: dialog.snapshots
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
            delegate: Pane {
                id: snapshotRow
                required property var modelData
                width: ListView.view.width
                Material.elevation: rowHover.hovered ? 2 : 1
                Material.background: backend.themeSurface

                Behavior on Material.elevation { NumberAnimation { duration: 120; easing.type: Easing.OutCubic } }

                HoverHandler { id: rowHover }

                RowLayout {
                    anchors.fill: parent
                    spacing: 10
                    Icon { source: "qrc:/icons/history.svg"; color: backend.themeAccentText; iconSize: 20 }
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: 2
                        Label {
                            Layout.fillWidth: true
                            text: modelData.label
                            font.weight: Font.DemiBold
                            elide: Text.ElideRight
                        }
                        Label {
                            Layout.fillWidth: true
                            // Taken while running without guest tools: the
                            // guest couldn't flush its disks first.
                            text: Qt.formatDateTime(new Date(modelData.created_at), "yyyy-MM-dd hh:mm")
                                  + (modelData.crash_consistent
                                     ? qsTr(" · taken while running without guest tools: going to it is like restarting after a power cut")
                                     : "")
                            color: backend.themeMuted
                            font.pixelSize: 12
                            wrapMode: Text.WordWrap
                        }
                    }
                    Button {
                        objectName: "goToButton"
                        text: qsTr("Go To")
                        enabled: !dialog.busy && dialog.canGoTo
                        onClicked: {
                            confirmGoTo.snapshotId = modelData.id
                            confirmGoTo.snapshotLabel = modelData.label
                            confirmGoTo.open()
                        }
                    }
                    ToolButton {
                        text: qsTr("Delete")
                        enabled: !dialog.busy
                        onClicked: {
                            confirmDeleteSnapshot.snapshotId = modelData.id
                            confirmDeleteSnapshot.snapshotLabel = modelData.label
                            confirmDeleteSnapshot.open()
                        }
                    }
                }
            }
        }
    }

    Dialog {
        id: confirmGoTo
        property string snapshotId
        property string snapshotLabel
        title: qsTr("Go To Snapshot")
        modal: true
        anchors.centerIn: parent
        width: Math.min(parent.width - 32, 380)
        standardButtons: Dialog.Cancel | Dialog.Yes
        // Specific verb instead of a generic "Yes" (docs/TODO.md P1).
        Component.onCompleted: {
            const yesButton = standardButton(Dialog.Yes)
            if (yesButton) yesButton.text = qsTr("Go To")
        }
        Overlay.modal: ThemeScrim {}
        contentItem: Label {
            text: qsTr("Go to “%1” for %2? Anything that happened after this snapshot in %2 will be lost.")
                .arg(confirmGoTo.snapshotLabel).arg(dialog.environment.name)
            wrapMode: Text.Wrap
            color: backend.themeRed
        }
        onAccepted: backend.goToSnapshot(dialog.environment.name, snapshotId)
    }

    Dialog {
        id: confirmDeleteSnapshot
        property string snapshotId
        property string snapshotLabel
        title: qsTr("Delete Snapshot")
        modal: true
        anchors.centerIn: parent
        width: Math.min(parent.width - 32, 380)
        standardButtons: Dialog.Cancel | Dialog.Yes
        Component.onCompleted: {
            const yesButton = standardButton(Dialog.Yes)
            if (yesButton) yesButton.text = qsTr("Delete")
        }
        Overlay.modal: ThemeScrim {}
        contentItem: Label {
            text: qsTr("Delete “%1”? This cannot be undone.").arg(confirmDeleteSnapshot.snapshotLabel)
            wrapMode: Text.Wrap
            color: backend.themeRed
        }
        onAccepted: backend.removeSnapshot(dialog.environment.name, snapshotId)
    }
}
