import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Dialogs
import QtQuick.Layouts

Dialog {
    id: dialog
    // The title holds the environment's name: shown as typed, never read
    // as HTML (the header is the style's own Label).
    Binding {
        target: dialog.header
        property: "textFormat"
        value: Text.PlainText
        when: dialog.header !== null && dialog.header.textFormat !== undefined
    }
    property var environment: ({})
    readonly property bool machine: environment.kind === "machine"
    // Must match core.EnvironmentColors (internal/core/environment.go) —
    // that's the source of truth the CLI validates --color against.
    readonly property var palette: ["red", "orange", "yellow", "green", "blue", "purple", "gray"]
    property string selectedColor: ""
    property string errorText: ""
    property bool submitting: false
    // Preparing the guest (omavm prepare): its steps, or why it couldn't run.
    property bool preparing: false
    property var prepareSteps: []
    property string prepareError: ""
    // `environment` (and its `.settings`) comes from `backend.environments`,
    // which is populated straight from `omavm list --json` — the *raw*,
    // persisted settings (internal/core/service.go's List, unlike
    // Settings()/Configure()'s return value, is never resolved through
    // EffectiveSettings()). A Machine whose CPUs/MemoryMiB were never
    // pinned simply omits those JSON keys (`omitempty`), so `settings.cpus`
    // here is `undefined`, not "2" — that's what cpusPinned/memoryPinned
    // below check, to show the user whether a shown number is an actual
    // pin or just today's default (docs/TODO.md P2 "diferenciar
    // preferência configurada de valor efetivo").
    //
    // Regardless of that, saving anything else in this dialog (e.g. just
    // the description) must never resend the *displayed* value as an
    // explicit --cpus/--memory-mib — that would silently pin it, which
    // disables Travel Mode's automatic reduction on battery
    // (internal/backend/machine/qemu/qemu.go's Start only reduces CPUs when the
    // raw, persisted setting is unset). Only actually touching a SpinBox
    // — SpinBox.onValueModified, which fires on user interaction and not
    // on the programmatic `.value = x` assignment in onOpened — marks it
    // as an intentional pin (docs/TODO.md P2 "salvar uma descrição não
    // altera inadvertidamente a intenção de configuração de CPU").
    property bool cpusTouched: false
    property bool memoryTouched: false
    property bool cpusPinned: false
    property bool memoryPinned: false
    property bool hardwareExpanded: false
    title: qsTr("%1 Settings").arg(environment.name || qsTr("Environment"))
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 520)
    height: Math.min(parent.height - 32, 560)
    Overlay.modal: ThemeScrim {}

    onOpened: {
        const settings = environment.settings || {}
        description.text = settings.description || ""
        cpus.value = settings.cpus || 2
        memory.value = settings.memory_mib || 2048
        cpusTouched = false
        memoryTouched = false
        cpusPinned = !!settings.cpus
        memoryPinned = !!settings.memory_mib
        hardwareExpanded = false
        sharedPath.text = settings.shared_path || ""
        sharedFolder.checked = !settings.shared_folder_disabled
        sharedReadOnly.checked = settings.shared_read_only || false
        disconnectISO.checked = settings.disconnect_iso || false
        selectedColor = settings.color || ""
        // Opt-out: absent/false in the JSON means "not disabled", i.e. on.
        shareClipboard.checked = !settings.clipboard_disabled
        clipboardDirection.currentIndex = Math.max(0, ["both", "to-host", "to-guest"].indexOf(settings.clipboard_direction || "both"))
        travelMode.checked = !settings.travel_mode_disabled
        vulkan.checked = !settings.vulkan_disabled
        launcher.checked = !settings.launcher_disabled
        ssh.checked = !settings.ssh_disabled
        openInEmptyWorkspace.checked = !settings.empty_workspace_disabled
        fullscreen.checked = !settings.fullscreen_disabled
        errorText = ""
        submitting = false
        preparing = false
        prepareSteps = []
        prepareError = ""
    }

    Connections {
        target: backend
        function onActionFinished(tag, ok, text) {
            if (tag === "prepare") {
                if (!dialog.preparing) return
                dialog.preparing = false
                if (!ok) {
                    dialog.prepareError = text
                    return
                }
                try {
                    dialog.prepareSteps = JSON.parse(text)
                    dialog.prepareError = ""
                } catch (e) {
                    dialog.prepareError = text
                }
                return
            }
            if (tag !== "configure") return
            dialog.submitting = false
            if (ok) {
                dialog.errorText = ""
                dialog.close()
            } else {
                dialog.errorText = text
            }
        }
    }

    contentItem: ScrollView {
        clip: true
        contentWidth: availableWidth
        ScrollBar.horizontal.policy: ScrollBar.AlwaysOff

    Item {
        width: Math.max(0, dialog.width - dialog.leftPadding - dialog.rightPadding)
        implicitHeight: settingsContent.implicitHeight
    ColumnLayout {
        id: settingsContent
        objectName: "settingsContent"
        width: parent.width
        spacing: 16

        Label { textFormat: Text.PlainText; text: qsTr("General"); font.pixelSize: 18; font.weight: Font.DemiBold }
        TextArea {
            id: description
            Layout.fillWidth: true
            Accessible.name: qsTr("Description")
            Layout.preferredHeight: 82
            placeholderText: qsTr("Description")
            wrapMode: TextEdit.Wrap
            onTextChanged: if (text.length > 500) text = text.slice(0, 500)
        }
        Label { textFormat: Text.PlainText; text: qsTr("Color tag"); color: backend.themeMuted }
        // A Flow, not a row: eight 44 px targets are wider than a narrow
        // dialog, and a row would push every field past its right edge.
        Flow {
            Layout.fillWidth: true
            Layout.topMargin: -12
            spacing: 4
            ToolButton {
                // Keep the visible dot compact while meeting WCAG 2.5.5's
                // 44×44 enhanced pointer target.
                implicitWidth: 44
                implicitHeight: 44
                objectName: "colorSwatch-none"
                Accessible.name: qsTr("No color")
                // One color among several: announced as a radio button, so
                // it reads as checkable on every Qt version.
                Accessible.role: Accessible.RadioButton
                // The selection is only drawn as a thicker border, so the
                // button itself is checkable: with a screen reader active,
                // Qt announces the control's own checked state and ignores
                // an Accessible.checked set by hand.
                checkable: true
                checked: dialog.selectedColor === ""
                // The control stretches its contentItem: the dot sits inside
                // it so it stays a 22 px circle.
                contentItem: Item { Rectangle {
                    width: 22
                    height: 22
                    anchors.centerIn: parent
                    radius: 11
                    color: "transparent"
                    border.width: dialog.selectedColor === "" ? 2 : 1
                    border.color: dialog.selectedColor === "" ? backend.themeAccent : backend.themeMuted
                    Behavior on border.width { NumberAnimation { duration: 120; easing.type: Easing.OutCubic } }
                } }
                onClicked: {
                    dialog.selectedColor = ""
                    checked = Qt.binding(() => dialog.selectedColor === "")
                }
            }
            Repeater {
                model: dialog.palette
                ToolButton {
                    required property string modelData
                    implicitWidth: 44
                    implicitHeight: 44
                    objectName: "colorSwatch-" + modelData
                    Accessible.name: modelData
                    Accessible.role: Accessible.RadioButton
                    checkable: true
                    checked: dialog.selectedColor === modelData
                    contentItem: Item { Rectangle {
                        width: 22
                        height: 22
                        anchors.centerIn: parent
                        radius: 11
                        color: backend.themeTagColors[modelData] || modelData
                        border.width: dialog.selectedColor === modelData ? 3 : 1
                        border.color: dialog.selectedColor === modelData ? backend.themeAccent : backend.themeMuted
                        Behavior on border.width { NumberAnimation { duration: 120; easing.type: Easing.OutCubic } }
                    } }
                    onClicked: {
                        dialog.selectedColor = modelData
                        checked = Qt.binding(() => dialog.selectedColor === modelData)
                    }
                }
            }
        }

        Button {
            objectName: "settingsHardwareButton"
            visible: dialog.machine
            text: dialog.hardwareExpanded ? qsTr("Hide hardware controls") : qsTr("Adjust hardware")
            onClicked: dialog.hardwareExpanded = !dialog.hardwareExpanded
        }
        Label {
            textFormat: Text.PlainText
            visible: dialog.machine
            Layout.fillWidth: true
            text: qsTr("%1 CPUs · %2 memory").arg(cpus.value).arg(memory.displayText)
            color: backend.themeMuted
            wrapMode: Text.Wrap
        }
        Item {
            visible: dialog.machine && dialog.hardwareExpanded
            Layout.fillWidth: true
            Layout.maximumWidth: parent.width
            implicitHeight: hardwareGrid.implicitHeight
            GridLayout {
                id: hardwareGrid
                width: parent.width
                columns: width < 360 ? 1 : 2
                columnSpacing: 16
                rowSpacing: 12
                Label { textFormat: Text.PlainText; text: qsTr("Processors") }
                SpinBox {
                    id: cpus
                    objectName: "settingsCpuSpinBox"
                    Layout.fillWidth: true
                    Accessible.name: qsTr("CPUs")
                    Component.onCompleted: contentItem.Accessible.name = Accessible.name
                    from: 1
                    to: 64
                    onValueModified: dialog.cpusTouched = true
                }
                // Distinguishes "this is an explicit pin" from "this is just
                // today's default preview" (docs/TODO.md P2 "diferenciar
                // preferência configurada de valor efetivo") — otherwise a
                // Machine that has never had its CPUs pinned looks identical
                // to one deliberately pinned at the same number.
                Label {
                    textFormat: Text.PlainText
                    Layout.columnSpan: hardwareGrid.columns
                    Layout.fillWidth: true
                    visible: dialog.machine
                    text: dialog.cpusTouched
                        ? qsTr("Will use this fixed CPU count on save.")
                        : (dialog.cpusPinned
                            ? qsTr("Fixed CPU count — won't change on its own.")
                            : qsTr("Automatic CPU count: Travel Mode may reduce it while on battery."))
                    color: backend.themeMuted
                    font.pixelSize: 12
                    wrapMode: Text.Wrap
                }
                Label { textFormat: Text.PlainText; text: qsTr("Memory") }
                MemorySpinBox {
                    id: memory
                    objectName: "settingsMemorySpinBox"
                    Layout.fillWidth: true
                    onValueModified: dialog.memoryTouched = true
                }
                Label {
                    textFormat: Text.PlainText
                    Layout.columnSpan: hardwareGrid.columns
                    Layout.fillWidth: true
                    visible: dialog.machine
                    text: dialog.memoryTouched
                        ? qsTr("Will use this fixed memory amount on save.")
                        : (dialog.memoryPinned
                            ? qsTr("Fixed memory amount — won't change on its own.")
                            : qsTr("Automatic memory amount: this is today's default."))
                    color: backend.themeMuted
                    font.pixelSize: 12
                    wrapMode: Text.Wrap
                }
            }
        }
        Label {
            textFormat: Text.PlainText
            visible: dialog.machine && dialog.hardwareExpanded
            Layout.fillWidth: true
            text: qsTr("Hardware changes take effect the next time the Machine starts.")
            color: backend.themeMuted
            wrapMode: Text.Wrap
        }

        Label {
            textFormat: Text.PlainText
            visible: dialog.machine
            text: qsTr("Graphics")
            font.pixelSize: 18
            font.weight: Font.DemiBold
        }
        CheckBox {
            id: vulkan
            visible: dialog.machine
            text: qsTr("Vulkan acceleration for 3D apps and games")
            ToolTip.visible: hovered
            ToolTip.text: qsTr("Used only when this computer supports it (see “omavm host”). The guest needs a recent Mesa with the Venus driver. Restart the Machine to apply a change.")
        }

        Label {
            textFormat: Text.PlainText
            text: qsTr("Automation")
            font.pixelSize: 18
            font.weight: Font.DemiBold
        }
        CheckBox {
            id: shareClipboard
            text: dialog.machine
                ? qsTr("Share text clipboard with the guest")
                : qsTr("Let programs in the Box copy to your clipboard")
            ToolTip.visible: hovered
            ToolTip.text: dialog.machine
                ? qsTr("Requires spice-vdagent in the guest desktop. Reopen the Machine's window to apply a change.")
                : qsTr("Programs like Neovim and tmux can copy text straight to the clipboard. They can never read it. Applies to terminals opened after saving.")
        }
        // One way only, when copying in one direction shouldn't happen
        // (a guest that must not see what this computer copies, say).
        ComboBox {
            id: clipboardDirection
            objectName: "clipboardDirection"
            visible: dialog.machine && shareClipboard.checked
            Layout.fillWidth: true
            Layout.leftMargin: 32
            Accessible.name: qsTr("Clipboard direction")
            textRole: "text"
            valueRole: "value"
            model: [
                { value: "both", text: qsTr("Both ways") },
                { value: "to-host", text: qsTr("Only from the guest to this computer") },
                { value: "to-guest", text: qsTr("Only from this computer to the guest") }
            ]
        }
        CheckBox {
            id: travelMode
            text: qsTr("Travel Mode: reduce CPUs automatically on battery")
            ToolTip.visible: hovered
            ToolTip.text: dialog.machine
                ? qsTr("Only applies when you haven't set a custom CPU count.")
                : qsTr("Uses half of this computer's CPUs while on battery, and all of them again once plugged in. Checked each time the Box starts or opens.")
        }
        CheckBox {
            id: ssh
            visible: dialog.machine
            text: qsTr("Allow SSH from this computer")
            ToolTip.visible: hovered
            ToolTip.text: qsTr("Used by “omavm ssh” and “omavm exec”. The guest needs systemd 256 or newer with sshd installed. Restart the Machine to apply a change.")
        }
        // Security Model: the reach of this channel is stated, not hidden
        // in a tooltip.
        Label {
            textFormat: Text.PlainText
            visible: dialog.machine && ssh.checked
            Layout.fillWidth: true
            Layout.leftMargin: 32
            text: qsTr("Any program on this computer can reach it, Boxes included; only the guest's login protects it.")
            color: backend.themeMuted
            font.pixelSize: 12
            wrapMode: Text.Wrap
        }
        CheckBox {
            id: launcher
            objectName: "launcherCheckBox"
            text: qsTr("Show in the app launcher")
            ToolTip.visible: hovered
            ToolTip.text: qsTr("Lists %1 among your applications, so it opens without going through OmaVM first.").arg(environment.name || "")
        }
        CheckBox {
            id: openInEmptyWorkspace
            text: qsTr("Open in an empty workspace")
            ToolTip.visible: hovered
            ToolTip.text: qsTr("If it is already open, switches to it instead. Needs Omarchy's Hyprland; a window rule of your own for OmaVM's viewer takes precedence.")
        }
        CheckBox {
            id: fullscreen
            visible: dialog.machine
            text: qsTr("Open the display fullscreen")
            ToolTip.visible: hovered
            ToolTip.text: qsTr("Leave fullscreen with Hyprland's own fullscreen key. A window rule of your own for OmaVM's viewer takes precedence.")
        }

        // What works with the guest right now, checked on the guest side
        // where OmaVM can (internal/backend/machine/qemu/guestcaps.go), and the
        // next step for the rest — in text, not only in a tooltip.
        Label {
            textFormat: Text.PlainText
            visible: dialog.machine && guestRepeater.count > 0
            text: qsTr("Working with the guest")
            font.pixelSize: 18
            font.weight: Font.DemiBold
        }
        Repeater {
            id: guestRepeater
            model: dialog.machine ? (dialog.environment.guestCapabilities || []) : []
            ColumnLayout {
                id: capabilityRow
                required property var modelData
                objectName: "guestCapability-" + modelData.id
                Layout.fillWidth: true
                spacing: 2
                Label {
                    textFormat: Text.PlainText
                    Layout.fillWidth: true
                    text: capabilityRow.modelData.label + ": " + (
                        capabilityRow.modelData.state === "ready" ? qsTr("connected")
                        : capabilityRow.modelData.state === "needs_guest_component" ? qsTr("needs a step in the guest")
                        : capabilityRow.modelData.state === "needs_restart" ? qsTr("applies after a restart")
                        : capabilityRow.modelData.state === "off" ? qsTr("off")
                        : qsTr("not checked yet"))
                    color: capabilityRow.modelData.state === "ready" ? backend.themeGreen : backend.themeForeground
                    wrapMode: Text.Wrap
                }
                // Selectable: the mount command is meant to be copied.
                TextEdit {
                    Layout.fillWidth: true
                    visible: text !== ""
                    text: capabilityRow.modelData.hint || ""
                    readOnly: true
                    selectByMouse: true
                    wrapMode: Text.Wrap
                    color: backend.themeMuted
                    font.pixelSize: 12
                    Accessible.role: Accessible.StaticText
                    Accessible.name: text
                }
            }
        }

        // Does in the guest what the rows above ask for, through its guest
        // agent: it changes the guest system, so only when asked, and it
        // says so before.
        Label {
            textFormat: Text.PlainText
            visible: prepareButton.visible
            Layout.fillWidth: true
            text: qsTr("Installs the clipboard agent and mounts the shared folder inside the guest, through its guest agent, and checks sound and resolution.")
            wrapMode: Text.Wrap
            color: backend.themeMuted
            font.pixelSize: 12
        }
        Button {
            id: prepareButton
            objectName: "prepareButton"
            visible: dialog.machine && dialog.environment.status === "running"
            enabled: !dialog.preparing && !(backend.busyEnvironments || {})[dialog.environment.name]
            text: dialog.preparing
                  ? ((backend.progress || {})[dialog.environment.name] || qsTr("Preparing…"))
                  : qsTr("Prepare the Guest")
            Accessible.name: qsTr("Prepare the guest")
            onClicked: {
                dialog.preparing = true
                dialog.prepareSteps = []
                dialog.prepareError = ""
                backend.prepareGuest(dialog.environment.name)
            }
        }
        Label {
            textFormat: Text.PlainText
            objectName: "prepareError"
            visible: text !== ""
            Layout.fillWidth: true
            text: dialog.prepareError
            wrapMode: Text.Wrap
            color: backend.themeRed
        }
        Repeater {
            model: dialog.prepareSteps
            ColumnLayout {
                id: stepRow
                required property var modelData
                objectName: "prepareStep-" + modelData.id
                Layout.fillWidth: true
                spacing: 2
                Label {
                    textFormat: Text.PlainText
                    Layout.fillWidth: true
                    text: stepRow.modelData.label + ": " + (
                        stepRow.modelData.result === "done" ? qsTr("set up")
                        : stepRow.modelData.result === "ready" ? qsTr("working")
                        : stepRow.modelData.result === "manual" ? qsTr("one step left for you")
                        : stepRow.modelData.result === "skipped" ? qsTr("skipped")
                        : qsTr("failed"))
                    color: stepRow.modelData.result === "failed" ? backend.themeRed
                           : (stepRow.modelData.result === "done" || stepRow.modelData.result === "ready") ? backend.themeGreen
                           : backend.themeForeground
                    wrapMode: Text.Wrap
                }
                // Selectable: a manual step is a command to copy.
                TextEdit {
                    Layout.fillWidth: true
                    visible: text !== ""
                    text: stepRow.modelData.detail || ""
                    readOnly: true
                    selectByMouse: true
                    wrapMode: Text.Wrap
                    color: backend.themeMuted
                    font.pixelSize: 12
                    Accessible.role: Accessible.StaticText
                    Accessible.name: text
                }
            }
        }

        Label {
            textFormat: Text.PlainText
            visible: dialog.machine && environment.image !== "ready:ubuntu-24.04" && environment.image !== "ready:fedora-44"
            text: qsTr("Installation media")
            font.pixelSize: 18
            font.weight: Font.DemiBold
        }
        CheckBox {
            id: disconnectISO
            visible: dialog.machine && environment.image !== "ready:ubuntu-24.04" && environment.image !== "ready:fedora-44"
            text: qsTr("Boot from disk on next start (disconnect ISO)")
        }

        Label {
            textFormat: Text.PlainText
            visible: dialog.machine
            text: qsTr("Shared folder")
            font.pixelSize: 18
            font.weight: Font.DemiBold
        }
        // On by default, like every host integration; the cost is said
        // here, not hidden (Security Model).
        CheckBox {
            id: sharedFolder
            objectName: "sharedFolderCheck"
            visible: dialog.machine
            checked: true
            text: qsTr("Share a folder with the guest")
        }
        Label {
            textFormat: Text.PlainText
            visible: dialog.machine && sharedFolder.checked
            Layout.fillWidth: true
            text: qsTr("Programs in the guest can read and write everything in it. Files dropped on the Machine's window are copied there.")
            color: backend.themeMuted
            font.pixelSize: 12
            wrapMode: Text.Wrap
        }
        TextField {
            id: sharedPath
            objectName: "sharedPathField"
            visible: dialog.machine && sharedFolder.checked
            Layout.fillWidth: true
            Layout.maximumWidth: parent.width
            Accessible.name: qsTr("Shared folder")
            placeholderText: qsTr("~/OmaVM/Shared (default)")
            selectByMouse: true
        }
        Button {
            objectName: "chooseSharedFolderButton"
            visible: dialog.machine && sharedFolder.checked
            text: qsTr("Choose…")
            onClicked: folderPicker.open()
        }
        CheckBox {
            id: sharedReadOnly
            visible: dialog.machine && sharedFolder.checked
            text: qsTr("Read-only in the guest")
        }
        Label {
            textFormat: Text.PlainText
            visible: dialog.machine && sharedFolder.checked
            Layout.fillWidth: true
            text: qsTr("Mount the tag omavm-share in the guest (Prepare the Guest does it). Changes apply on the next start.")
            color: backend.themeMuted
            wrapMode: Text.Wrap
        }
    }
    }
    }

    FolderDialog {
        id: folderPicker
        title: qsTr("Choose a folder to share")
        onAccepted: sharedPath.text = backend.localPath(selectedFolder)
    }

    footer: ColumnLayout {
        spacing: 0
        Label {
            textFormat: Text.PlainText
            Layout.fillWidth: true
            Layout.leftMargin: 24
            Layout.rightMargin: 24
            opacity: dialog.errorText !== "" ? 1 : 0
            visible: opacity > 0
            text: dialog.errorText
            wrapMode: Text.Wrap
            color: backend.themeRed
            Behavior on opacity { NumberAnimation { duration: 180; easing.type: Easing.OutCubic } }
        }
        DialogButtonBox {
            Layout.fillWidth: true
            Button {
                text: qsTr("Cancel")
                enabled: !dialog.submitting
                DialogButtonBox.buttonRole: DialogButtonBox.RejectRole
                onClicked: dialog.reject()
            }
            Button {
                objectName: "saveButton"
                text: dialog.submitting ? qsTr("Saving…") : qsTr("Save")
                highlighted: true
                enabled: !dialog.submitting
                DialogButtonBox.buttonRole: DialogButtonBox.AcceptRole
                onClicked: {
                    dialog.errorText = ""
                    dialog.submitting = true
                    backend.configure(environment.name, description.text.trim(), cpus.value, dialog.cpusTouched, memory.value, dialog.memoryTouched, dialog.machine, sharedPath.text.trim(), sharedReadOnly.checked, disconnectISO.checked, dialog.selectedColor, shareClipboard.checked, travelMode.checked, vulkan.checked, openInEmptyWorkspace.checked, launcher.checked, ssh.checked, fullscreen.checked, clipboardDirection.currentValue, sharedFolder.checked)
                }
            }
        }
    }
}
