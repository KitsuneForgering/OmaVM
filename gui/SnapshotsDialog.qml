import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts

Dialog {
    id: dialog
    property var environment: ({})
    readonly property var snapshots: environment.snapshots || []
    title: qsTr("%1 Snapshots").arg(environment.name || qsTr("Environment"))
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 480)
    height: Math.min(parent.height - 64, 460)
    standardButtons: Dialog.Close
    Overlay.modal: ThemeScrim {}

    onOpened: newLabel.text = ""

    ColumnLayout {
        anchors.fill: parent
        spacing: 12

        RowLayout {
            Layout.fillWidth: true
            TextField {
                id: newLabel
                Layout.fillWidth: true
                placeholderText: qsTr("Label (e.g. Before system upgrade)")
                selectByMouse: true
                onAccepted: createButton.clicked()
            }
            Button {
                id: createButton
                text: qsTr("Create")
                highlighted: true
                enabled: newLabel.text.trim().length > 0
                onClicked: {
                    backend.createSnapshot(dialog.environment.name, newLabel.text.trim())
                    newLabel.text = ""
                }
            }
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
            delegate: Pane {
                required property var modelData
                width: ListView.view.width
                Material.elevation: 1
                Material.background: backend.themeSurface
                RowLayout {
                    anchors.fill: parent
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
                            text: Qt.formatDateTime(new Date(modelData.created_at), "yyyy-MM-dd hh:mm")
                            color: backend.themeMuted
                            font.pixelSize: 12
                        }
                    }
                    Button {
                        text: qsTr("Go To")
                        onClicked: backend.goToSnapshot(dialog.environment.name, modelData.id)
                    }
                    ToolButton {
                        text: qsTr("Delete")
                        onClicked: backend.removeSnapshot(dialog.environment.name, modelData.id)
                    }
                }
            }
        }
    }
}
