import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts

Dialog {
    id: dialog
    property var environment: ({})
    readonly property bool busy: !!(backend.busyEnvironments && backend.busyEnvironments[environment.name])
    // True once backend.apps actually reflects this dialog's environment —
    // guards against a slower response for a previously open environment
    // landing after the user has switched to another one (docs/TODO.md P0).
    readonly property bool matches: backend.appsEnvironment === environment.name
    property bool requested: false
    // The application an export or removal is running for, and how the
    // last one ended: each row says what is happening to it, and a failure
    // keeps the row and offers the same action again.
    property string pendingId: ""
    property string pendingName: ""
    property bool pendingExport: false
    property string resultText: ""
    property bool resultFailed: false

    function act(app) {
        pendingId = app.id
        pendingName = app.name
        pendingExport = !app.exported
        resultText = ""
        if (pendingExport)
            backend.exportApp(environment.name, app.id)
        else
            backend.unexportApp(environment.name, app.id)
    }
    title: qsTr("%1 Applications").arg(environment.name || qsTr("Environment"))
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 480)
    height: Math.min(parent.height - 64, 460)
    standardButtons: Dialog.Close
    Overlay.modal: ThemeScrim {}

    onOpened: {
        pendingId = ""
        resultText = ""
        requested = false
        if (!dialog.busy) {
            requested = true
            backend.refreshApps(environment.name)
        }
    }

    Connections {
        target: backend
        function onActionFinished(tag, ok, text) {
            if ((tag !== "apps-export" && tag !== "apps-unexport") || dialog.pendingId === "")
                return
            dialog.resultFailed = !ok
            dialog.resultText = !ok ? text
                : dialog.pendingExport ? qsTr("“%1” is in the app menu.").arg(dialog.pendingName)
                : qsTr("“%1” was removed from the app menu; it is still installed in %2.").arg(dialog.pendingName).arg(dialog.environment.name)
            dialog.pendingId = ""
        }
        function onBusyChanged() {
            if (!dialog.visible || dialog.busy || dialog.requested)
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

        Label {
            objectName: "resultLabel"
            Layout.fillWidth: true
            visible: dialog.resultText !== ""
            text: dialog.resultText
            color: dialog.resultFailed ? backend.themeRed : backend.themeGreen
            wrapMode: Text.Wrap
        }

        RowLayout {
            Layout.fillWidth: true
            visible: dialog.matches && dialog.busy && dialog.pendingId === ""
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
                objectName: "appRow-" + modelData.name
                required property var modelData
                readonly property bool pending: dialog.pendingId === modelData.id
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
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: 0
                        Label {
                            Layout.fillWidth: true
                            text: modelData.name
                            elide: Text.ElideRight
                        }
                        // In words, not only the icon's color.
                        Label {
                            Layout.fillWidth: true
                            visible: modelData.exported || appRow.pending
                            text: appRow.pending
                                  ? (dialog.pendingExport ? qsTr("Adding to the app menu…") : qsTr("Removing from the app menu…"))
                                  : qsTr("In the app menu")
                            color: backend.themeMuted
                            font.pixelSize: 12
                            elide: Text.ElideRight
                        }
                    }
                    BusyIndicator {
                        visible: appRow.pending
                        running: visible
                        implicitWidth: 24
                        implicitHeight: 24
                    }
                    Button {
                        objectName: "appAction"
                        visible: !appRow.pending
                        // "Remove from menu", never bare "Remove": this only
                        // undoes the launcher shortcut, the app stays
                        // installed in the Box (docs/TODO.md P1).
                        text: modelData.exported ? qsTr("Remove from menu") : qsTr("Export")
                        highlighted: !modelData.exported
                        Accessible.name: (modelData.exported ? qsTr("Remove %1 from the app menu") : qsTr("Export %1")).arg(modelData.name)
                        enabled: !dialog.busy
                        onClicked: dialog.act(modelData)
                    }
                }
            }
        }
    }
}
