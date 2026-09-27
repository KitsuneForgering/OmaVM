import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts

Pane {
    id: card
    required property var environment
    signal removeRequested(string name)
    signal forceStopRequested(string name)
    signal settingsRequested(var environment)
    signal snapshotsRequested(var environment)
    signal appsRequested(var environment)
    readonly property bool running: environment.status === "running"
    readonly property bool paused: environment.status === "paused"
    readonly property bool stopped: environment.status === "stopped"
    readonly property bool transitioning: environment.status === "starting" || environment.status === "stopping"
    readonly property bool failed: environment.status === "error"
    readonly property bool active: running || paused || transitioning || failed
    padding: 14
    Material.elevation: 1
    Material.background: backend.themeSurface

    RowLayout {
        anchors.fill: parent
        spacing: card.width < 520 ? 10 : 16

        Rectangle {
            Layout.preferredWidth: card.width < 520 ? 92 : 168
            Layout.preferredHeight: card.width < 520 ? 64 : 104
            radius: 8
            clip: true
            color: backend.themeDarkSurface
            border.width: 1
            border.color: backend.themeSelection
            Image {
                id: preview
                anchors.fill: parent
                source: card.environment.preview || ""
                fillMode: Image.PreserveAspectCrop
                visible: status === Image.Ready
                cache: false
            }
            Label {
                anchors.centerIn: parent
                visible: !preview.visible
                text: card.environment.kind === "machine" ? "▣" : ">_"
                font.pixelSize: card.width < 520 ? 24 : 32
                color: backend.themeAccent
            }
        }

        ColumnLayout {
            Layout.fillWidth: true
            spacing: 5
            RowLayout {
                Layout.fillWidth: true
                spacing: 8
                Rectangle {
                    visible: !!(card.environment.settings && card.environment.settings.color)
                    implicitWidth: 10
                    implicitHeight: 10
                    radius: 5
                    color: card.environment.settings ? card.environment.settings.color || "transparent" : "transparent"
                }
                Label {
                    Layout.fillWidth: true
                    text: card.environment.name
                    font.pixelSize: 19
                    font.weight: Font.DemiBold
                    elide: Text.ElideRight
                }
            }
            Label {
                Layout.fillWidth: true
                visible: card.width >= 430
                text: card.environment.kind === "machine" ? qsTr("Desktop") : qsTr("Development Box")
                color: backend.themeMuted
            }
            Label {
                Layout.fillWidth: true
                visible: card.width >= 600 && !!(card.environment.settings && card.environment.settings.description)
                text: card.environment.settings ? card.environment.settings.description || "" : ""
                color: backend.themeMuted
                elide: Text.ElideRight
            }
            RowLayout {
                spacing: 7
                Rectangle {
                    implicitWidth: 8
                    implicitHeight: 8
                    radius: 4
                    color: card.failed ? backend.themeRed
                          : card.running ? backend.themeGreen
                          : card.paused || card.transitioning ? backend.themeAccent
                          : backend.themeMuted
                }
                Label {
                    text: card.environment.status === "running" ? qsTr("Running")
                          : card.environment.status === "paused" ? qsTr("Paused")
                          : card.environment.status === "starting" ? qsTr("Starting…")
                          : card.environment.status === "stopping" ? qsTr("Shutting down…")
                          : card.environment.status === "error" ? qsTr("Needs attention")
                          : card.environment.status === "stopped" ? qsTr("Stopped")
                          : qsTr("Checking…")
                    color: card.failed ? backend.themeRed
                          : card.running ? backend.themeGreen
                          : card.paused || card.transitioning ? backend.themeAccent
                          : backend.themeMuted
                }
            }
            Label {
                Layout.fillWidth: true
                visible: card.environment.kind === "machine" && card.width >= 520 && !!card.environment.guestAgent
                text: card.environment.guestAgent === "connected" ? qsTr("Guest tools connected")
                    : card.environment.guestAgent === "stopped" ? qsTr("Guest tools checked when running")
                    : qsTr("Guest tools not detected")
                color: card.environment.guestAgent === "connected" ? backend.themeGreen : backend.themeMuted
                font.pixelSize: 12
                ToolTip.text: card.environment.integrationHint || ""
                ToolTip.visible: hovered && ToolTip.text !== ""
            }
        }

        Button {
            visible: card.width >= 500
            text: card.running || card.failed ? qsTr("Open") : card.paused ? qsTr("Resume") : qsTr("Start")
            highlighted: true
            enabled: !card.transitioning
            onClicked: card.paused ? backend.resume(card.environment.name)
                                   : backend.open(card.environment.name, card.environment.kind)
        }
        ToolButton {
            text: "⋮"
            Accessible.name: qsTr("Actions for %1").arg(card.environment.name)
            onClicked: actions.open()
            Menu {
                id: actions
                y: parent.height
                MenuItem {
                    text: card.running || card.failed ? qsTr("Open") : card.paused ? qsTr("Resume") : qsTr("Start")
                    enabled: !card.transitioning
                    onTriggered: card.paused ? backend.resume(card.environment.name)
                                             : backend.open(card.environment.name, card.environment.kind)
                }
                MenuItem {
                    visible: card.environment.kind === "machine"
                    text: card.paused ? qsTr("Resume") : qsTr("Pause")
                    enabled: card.running || card.paused
                    onTriggered: card.paused ? backend.resume(card.environment.name) : backend.pause(card.environment.name)
                }
                MenuItem { text: qsTr("Restart"); visible: card.environment.kind === "machine"; enabled: card.running || card.paused; onTriggered: backend.restart(card.environment.name) }
                MenuItem { text: qsTr("Shut Down"); enabled: card.active && !card.transitioning; onTriggered: backend.stop(card.environment.name) }
                MenuItem { text: qsTr("Force Stop…"); visible: card.environment.kind === "machine"; enabled: card.active; onTriggered: card.forceStopRequested(card.environment.name) }
                MenuSeparator {}
                MenuItem { text: qsTr("Snapshots…"); visible: card.environment.kind === "machine"; onTriggered: card.snapshotsRequested(card.environment) }
                MenuItem { text: qsTr("Applications…"); visible: card.environment.kind !== "machine"; onTriggered: card.appsRequested(card.environment) }
                MenuItem { text: qsTr("Settings…"); onTriggered: card.settingsRequested(card.environment) }
                MenuItem { text: qsTr("Delete…"); onTriggered: card.removeRequested(card.environment.name) }
            }
        }
    }
}
