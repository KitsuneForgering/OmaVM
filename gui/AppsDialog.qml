import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts

Dialog {
    id: dialog
    property var environment: ({})
    // True once backend.apps actually reflects this dialog's environment —
    // guards against a slower response for a previously open environment
    // landing after the user has switched to another one (docs/TODO.md P0).
    readonly property bool matches: backend.appsEnvironment === environment.name
    property bool requested: false
    title: qsTr("%1 Applications").arg(environment.name || qsTr("Environment"))
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 480)
    height: Math.min(parent.height - 64, 460)
    standardButtons: Dialog.Close
    Overlay.modal: ThemeScrim {}

    onOpened: {
        requested = false
        if (!backend.busy) {
            requested = true
            backend.refreshApps(environment.name)
        }
    }

    Connections {
        target: backend
        function onBusyChanged() {
            if (!dialog.visible || backend.busy || dialog.requested)
                return
            dialog.requested = true
            backend.refreshApps(dialog.environment.name)
        }
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 12

        Label {
            Layout.fillWidth: true
            text: qsTr("Export an application so it appears alongside your other apps.")
            color: backend.themeMuted
            wrapMode: Text.Wrap
        }

        RowLayout {
            Layout.fillWidth: true
            visible: dialog.matches && backend.busy
            spacing: 8
            BusyIndicator { implicitWidth: 18; implicitHeight: 18; running: true }
            Label { text: qsTr("Refreshing…"); color: backend.themeMuted; font.pixelSize: 12 }
        }

        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: !dialog.matches
            spacing: 10
            Item { Layout.fillHeight: true }
            BusyIndicator { Layout.alignment: Qt.AlignHCenter; running: !dialog.matches }
            Label { Layout.alignment: Qt.AlignHCenter; text: qsTr("Loading applications…"); color: backend.themeMuted }
            Item { Layout.fillHeight: true }
        }

        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: dialog.matches && backend.appsError !== ""
            spacing: 10
            Item { Layout.fillHeight: true }
            Icon { Layout.alignment: Qt.AlignHCenter; source: "qrc:/icons/warning.svg"; color: backend.themeRed; iconSize: 36 }
            Label {
                Layout.fillWidth: true
                Layout.alignment: Qt.AlignHCenter
                horizontalAlignment: Text.AlignHCenter
                text: backend.appsError
                color: backend.themeMuted
                wrapMode: Text.Wrap
            }
            Button {
                Layout.alignment: Qt.AlignHCenter
                text: qsTr("Try Again")
                onClicked: backend.refreshApps(dialog.environment.name)
            }
            Item { Layout.fillHeight: true }
        }

        Label {
            Layout.fillWidth: true
            visible: dialog.matches && backend.appsError === "" && backend.apps.length === 0
            text: qsTr("No exportable applications found.")
            color: backend.themeMuted
        }

        ListView {
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            spacing: 6
            visible: dialog.matches && backend.appsError === "" && backend.apps.length > 0
            model: dialog.matches ? backend.apps : []
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
                id: appRow
                required property var modelData
                width: ListView.view.width
                Material.elevation: rowHover.hovered ? 2 : 1
                Material.background: backend.themeSurface

                Behavior on Material.elevation { NumberAnimation { duration: 120; easing.type: Easing.OutCubic } }

                HoverHandler { id: rowHover }

                RowLayout {
                    anchors.fill: parent
                    spacing: 10
                    Icon {
                        source: "qrc:/icons/app.svg"
                        color: modelData.exported ? backend.themeGreen : backend.themeMuted
                        iconSize: 20
                    }
                    Label {
                        Layout.fillWidth: true
                        text: modelData.name
                        elide: Text.ElideRight
                    }
                    Button {
                        // "Remove from menu", never bare "Remove": this only
                        // undoes the launcher shortcut, the app stays
                        // installed in the Box (docs/TODO.md P1).
                        text: modelData.exported ? qsTr("Remove from menu") : qsTr("Export")
                        highlighted: !modelData.exported
                        enabled: !backend.busy
                        onClicked: modelData.exported
                            ? backend.unexportApp(dialog.environment.name, modelData.id)
                            : backend.exportApp(dialog.environment.name, modelData.id)
                    }
                }
            }
        }
    }
}
