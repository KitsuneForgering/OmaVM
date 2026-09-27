import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts

Dialog {
    id: dialog
    property var environment: ({})
    title: qsTr("%1 Applications").arg(environment.name || qsTr("Environment"))
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 480)
    height: Math.min(parent.height - 64, 460)
    standardButtons: Dialog.Close
    Overlay.modal: ThemeScrim {}

    onOpened: backend.refreshApps(environment.name)

    ColumnLayout {
        anchors.fill: parent
        spacing: 12

        Label {
            Layout.fillWidth: true
            text: qsTr("Export an application so it appears alongside your other apps.")
            color: backend.themeMuted
            wrapMode: Text.Wrap
        }

        Label {
            Layout.fillWidth: true
            visible: backend.apps.length === 0
            text: qsTr("No exportable applications found.")
            color: backend.themeMuted
        }

        ListView {
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            spacing: 6
            model: backend.apps
            delegate: Pane {
                required property var modelData
                width: ListView.view.width
                Material.elevation: 1
                Material.background: backend.themeSurface
                RowLayout {
                    anchors.fill: parent
                    Label {
                        Layout.fillWidth: true
                        text: modelData.name
                        elide: Text.ElideRight
                    }
                    Button {
                        text: modelData.exported ? qsTr("Remove") : qsTr("Export")
                        highlighted: !modelData.exported
                        onClicked: modelData.exported
                            ? backend.unexportApp(dialog.environment.name, modelData.id)
                            : backend.exportApp(dialog.environment.name, modelData.id)
                    }
                }
            }
        }
    }
}
