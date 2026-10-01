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
    signal cloneRequested(var environment)
    // An action of this OmaVM window is running on this environment;
    // others stay usable meanwhile.
    readonly property bool busy: !!(backend.busyEnvironments && backend.busyEnvironments[environment.name])
    readonly property bool running: environment.status === "running"
    readonly property bool paused: environment.status === "paused"
    readonly property bool stopped: environment.status === "stopped"
    readonly property bool transitioning: environment.status === "starting" || environment.status === "stopping"
                                          || environment.status === "creating" || environment.status === "removing"
    readonly property bool failed: environment.status === "error"
    // The process is there but its monitor doesn't answer (a hung QEMU):
    // it still has to be stoppable from here.
    readonly property bool unresponsive: environment.status === "unknown"
    readonly property bool active: running || paused || transitioning || failed || unresponsive
    // The system it runs, readable (UX Principle #5: name, distro, kind,
    // status): a Box's image without registry, "-toolbox" or a "latest"
    // tag, a Desktop's installation image without its path.
    function systemName(env) {
        const image = (env && env.image) || ""
        if (!image)
            return ""
        if (env.kind === "machine") {
            const file = image.split("/").pop()
            return file.replace(/\.iso$/i, "")
        }
        const known = { fedora: "Fedora", ubuntu: "Ubuntu", debian: "Debian", archlinux: "Arch Linux",
                        arch: "Arch Linux", alpine: "Alpine", opensuse: "openSUSE", centos: "CentOS",
                        rockylinux: "Rocky Linux", almalinux: "AlmaLinux" }
        let ref = image.split("@")[0]
        let tag = ""
        const colon = ref.lastIndexOf(":")
        if (colon > ref.lastIndexOf("/")) {
            tag = ref.slice(colon + 1)
            ref = ref.slice(0, colon)
        }
        let repo = ref.split("/").pop().replace(/-toolbox$/, "")
        repo = known[repo] || repo
        return tag && tag !== "latest" ? repo + " " + tag : repo
    }
    readonly property bool machine: environment.kind === "machine"
    // The primary action names what the person will see after the click
    // (docs/TODO.md P1): a stopped Box's click opens its terminal, so it
    // says Open; a stopped Desktop boots first, so Start — and both say
    // the rest in the hint.
    readonly property string primaryLabel: paused ? qsTr("Resume")
                                         : stopped && machine ? qsTr("Start")
                                         : qsTr("Open")
    readonly property string primaryHint: paused ? qsTr("Continue %1 where it was paused").arg(environment.name)
                                        : stopped && machine ? qsTr("Start %1 and show its screen").arg(environment.name)
                                        : stopped ? qsTr("Start %1 and open its terminal").arg(environment.name)
                                        : machine ? qsTr("Show the screen of %1").arg(environment.name)
                                        : qsTr("Open a terminal in %1").arg(environment.name)
    readonly property color statusColor: failed || unresponsive ? backend.themeRed
                                        : running ? backend.themeGreen
                                        : paused || transitioning ? backend.themeAccentText
                                        : backend.themeMuted
    padding: 14
    Material.elevation: hoverHandler.hovered ? 3 : 1
    Material.background: backend.themeSurface

    Behavior on Material.elevation {
        NumberAnimation { duration: 120; easing.type: Easing.OutCubic }
    }

    HoverHandler { id: hoverHandler }

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
                visible: opacity > 0
                opacity: status === Image.Ready ? 1 : 0
                cache: false

                Behavior on opacity { NumberAnimation { duration: 200; easing.type: Easing.OutCubic } }
            }
            Icon {
                anchors.centerIn: parent
                opacity: preview.opacity > 0 ? 0 : 1
                source: card.environment.kind === "machine" ? "qrc:/icons/machine.svg" : "qrc:/icons/box.svg"
                iconSize: card.width < 520 ? 24 : 32
                color: backend.themeAccentText

                Behavior on opacity { NumberAnimation { duration: 200; easing.type: Easing.OutCubic } }
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
                    id: nameLabel
                    objectName: "nameLabel"
                    Layout.fillWidth: true
                    text: card.environment.name
                    font.pixelSize: 19
                    font.weight: Font.DemiBold
                    elide: Text.ElideRight
                    // A long name cut short is still readable in full.
                    HoverHandler { id: nameHover }
                    ToolTip.text: card.environment.name
                    ToolTip.visible: nameHover.hovered && nameLabel.truncated
                    ToolTip.delay: 400
                }
            }
            Label {
                objectName: "kindLabel"
                Layout.fillWidth: true
                visible: card.width >= 430
                text: {
                    const kind = card.environment.kind === "machine" ? qsTr("Desktop") : qsTr("Development Box")
                    const system = card.systemName(card.environment)
                    return system ? kind + " · " + system : kind
                }
                color: backend.themeMuted
                elide: Text.ElideRight
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
                    color: card.statusColor

                    Behavior on color { ColorAnimation { duration: 200 } }

                    SequentialAnimation on scale {
                        running: card.transitioning || card.busy
                        loops: Animation.Infinite
                        NumberAnimation { from: 1; to: 1.5; duration: 500; easing.type: Easing.InOutSine }
                        NumberAnimation { from: 1.5; to: 1; duration: 500; easing.type: Easing.InOutSine }
                    }
                }
                Label {
                    objectName: "statusLabel"
                    text: card.environment.status === "running" ? qsTr("Running")
                          : card.environment.status === "paused" ? qsTr("Paused")
                          : card.environment.status === "starting" ? qsTr("Starting…")
                          : card.environment.status === "stopping" ? qsTr("Shutting down…")
                          : card.environment.status === "creating" ? qsTr("Creating…")
                          : card.environment.status === "removing" ? qsTr("Deleting…")
                          : card.environment.status === "error" ? qsTr("Needs attention")
                          : card.environment.status === "unknown" ? qsTr("Not responding")
                          : card.environment.status === "stopped" ? qsTr("Stopped")
                          : qsTr("Checking…")
                    color: card.statusColor

                    Behavior on color { ColorAnimation { duration: 200 } }
                }
            }
            // Why a Machine is paused or needs attention (a full host
            // disk, a missing container): the state alone doesn't say.
            Label {
                objectName: "statusDetailLabel"
                Layout.fillWidth: true
                visible: (card.paused || card.failed) && !!card.environment.statusDetail
                text: card.environment.statusDetail || ""
                color: card.statusColor
                wrapMode: Text.WordWrap
            }
            // Before it happens: the host's disk is nearly full, and the
            // Machine would pause when it runs out.
            Label {
                objectName: "statusWarningLabel"
                Layout.fillWidth: true
                visible: !card.paused && !!card.environment.statusWarning
                text: card.environment.statusWarning || ""
                color: backend.themeRed
                wrapMode: Text.WordWrap
            }
            // What the running session got differs from its settings: the
            // saved ones wait for a restart, or Travel Mode trimmed CPUs.
            Label {
                objectName: "restartNeededLabel"
                Layout.fillWidth: true
                visible: card.active && !!card.environment.restartNeeded
                text: qsTr("Restart to apply the saved settings")
                color: backend.themeAccentText
                wrapMode: Text.WordWrap
            }
            Label {
                objectName: "travelModeLabel"
                Layout.fillWidth: true
                visible: card.active && !!card.environment.travelMode
                text: qsTr("Travel Mode: started with fewer processors because this computer was on battery")
                color: backend.themeMuted
                wrapMode: Text.WordWrap
            }
            // The stage a long creation reported (which image layer is
            // downloading): a real step, never an invented percentage.
            Label {
                objectName: "progressLabel"
                readonly property string stage: (backend.progress && backend.progress[card.environment.name]) || ""
                Layout.fillWidth: true
                visible: stage !== ""
                text: stage
                color: backend.themeMuted
                wrapMode: Text.WordWrap
                Accessible.role: Accessible.StaticText
                Accessible.name: stage
            }
            // Never hidden by width: it says what shutting down will lose.
            Label {
                objectName: "ephemeralLabel"
                Layout.fillWidth: true
                visible: card.active && !!card.environment.ephemeral
                text: qsTr("Changes in this session are discarded when it shuts down")
                color: backend.themeAccentText
                wrapMode: Text.WordWrap
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
            objectName: "primaryAction"
            // Always reachable, even at 320px: this is the one direct action
            // every card must offer (docs/TODO.md P1). Below 500px it shrinks
            // to an icon-only button instead of disappearing into the menu.
            readonly property bool compact: card.width < 500
            text: card.primaryLabel
            icon.source: "qrc:/icons/play.svg"
            display: compact ? AbstractButton.IconOnly : AbstractButton.TextOnly
            Accessible.name: card.primaryLabel
            Accessible.description: card.primaryHint
            ToolTip.text: card.primaryHint
            ToolTip.visible: hovered
            ToolTip.delay: 600
            highlighted: true
            enabled: !card.transitioning && !card.busy
            onClicked: card.paused ? backend.resume(card.environment.name)
                                   : backend.open(card.environment.name, card.environment.kind)
        }
        ToolButton {
            objectName: "actionsButton"
            icon.source: "qrc:/icons/more.svg"
            icon.color: backend.themeForeground
            Accessible.name: qsTr("Actions for %1").arg(card.environment.name)
            onClicked: actions.open()
            Menu {
                id: actions
                objectName: "actionsMenu"
                y: parent.height
                MenuItem {
                    text: card.primaryLabel
                    Accessible.description: card.primaryHint
                    enabled: !card.transitioning && !card.busy
                    onTriggered: card.paused ? backend.resume(card.environment.name)
                                             : backend.open(card.environment.name, card.environment.kind)
                }
                MenuItem {
                    objectName: "startEphemeralItem"
                    // Try something (an installer, a reproduction) and get
                    // the Desktop back exactly as it was at shutdown.
                    text: qsTr("Start Without Keeping Changes")
                    visible: card.environment.kind === "machine" && !card.active
                    height: visible ? implicitHeight : 0
                    enabled: !card.transitioning && !card.busy
                    onTriggered: backend.openEphemeral(card.environment.name)
                }
                // A hidden MenuItem keeps its height in a Qt Quick Menu, so each
                // kind-specific item collapses itself (see tst_environmentcard.qml).
                MenuItem {
                    visible: card.environment.kind === "machine"
                    height: visible ? implicitHeight : 0
                    text: card.paused ? qsTr("Resume") : qsTr("Pause")
                    enabled: (card.running || card.paused) && !card.busy
                    onTriggered: card.paused ? backend.resume(card.environment.name) : backend.pause(card.environment.name)
                }
                MenuItem { text: qsTr("Restart"); visible: card.environment.kind === "machine"; height: visible ? implicitHeight : 0; enabled: (card.running || card.paused) && !card.busy; onTriggered: backend.restart(card.environment.name) }
                MenuItem { text: qsTr("Shut Down"); enabled: card.active && !card.transitioning && !card.busy; onTriggered: backend.stop(card.environment.name) }
                MenuItem { text: qsTr("Force Stop…"); visible: card.environment.kind === "machine"; height: visible ? implicitHeight : 0; enabled: card.active && !card.busy; onTriggered: card.forceStopRequested(card.environment.name) }
                MenuSeparator {}
                MenuItem { text: qsTr("Snapshots…"); visible: card.environment.kind === "machine"; height: visible ? implicitHeight : 0; onTriggered: card.snapshotsRequested(card.environment) }
                MenuItem { text: qsTr("Applications…"); visible: card.environment.kind !== "machine"; height: visible ? implicitHeight : 0; onTriggered: card.appsRequested(card.environment) }
                // Long (the package manager downloads updates); its lines
                // show on the card as it goes.
                MenuItem {
                    objectName: "updateItem"
                    text: qsTr("Update System")
                    visible: card.environment.kind !== "machine"
                    height: visible ? implicitHeight : 0
                    enabled: !card.busy && !card.transitioning
                    onTriggered: backend.updateEnvironment(card.environment.name)
                }
                MenuItem { text: qsTr("Clone…"); enabled: !card.busy && !card.transitioning; onTriggered: card.cloneRequested(card.environment) }
                MenuItem { text: qsTr("Settings…"); onTriggered: card.settingsRequested(card.environment) }
                MenuItem { text: qsTr("Delete…"); enabled: !card.busy; onTriggered: card.removeRequested(card.environment.name) }
            }
        }
    }
}
