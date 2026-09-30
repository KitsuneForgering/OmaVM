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
    property string errorText: ""
    property bool submitting: false
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
    // (internal/backend/qemu/qemu.go's Start only reduces CPUs when the
    // raw, persisted setting is unset). Only actually touching a SpinBox
    // — SpinBox.onValueModified, which fires on user interaction and not
    // on the programmatic `.value = x` assignment in onOpened — marks it
    // as an intentional pin (docs/TODO.md P2 "salvar uma descrição não
    // altera inadvertidamente a intenção de configuração de CPU").
    property bool cpusTouched: false
    property bool memoryTouched: false
    property bool cpusPinned: false
    property bool memoryPinned: false
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
        sharedPath.text = settings.shared_path || ""
        sharedReadOnly.checked = settings.shared_read_only || false
        disconnectISO.checked = settings.disconnect_iso || false
        selectedColor = settings.color || ""
        // Opt-out: absent/false in the JSON means "not disabled", i.e. on.
        shareClipboard.checked = !settings.clipboard_disabled
        travelMode.checked = !settings.travel_mode_disabled
        vulkan.checked = !settings.vulkan_disabled
        launcher.checked = !settings.launcher_disabled
        ssh.checked = !settings.ssh_disabled
        openInEmptyWorkspace.checked = !settings.empty_workspace_disabled
        fullscreen.checked = !settings.fullscreen_disabled
        errorText = ""
        submitting = false
    }

    Connections {
        target: backend
        function onActionFinished(tag, ok, text) {
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

    ColumnLayout {
        width: parent.width
        spacing: 16

        Label { text: qsTr("General"); font.pixelSize: 18; font.weight: Font.DemiBold }
        TextArea {
            id: description
            Layout.fillWidth: true
            Accessible.name: qsTr("Description")
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
                // Keep the visible dot compact while meeting WCAG 2.5.5's
                // 44×44 enhanced pointer target.
                implicitWidth: 44
                implicitHeight: 44
                objectName: "colorSwatch-none"
                Accessible.name: qsTr("No color")
                // The selection is only drawn as a thicker border, so the
                // button itself is checkable: with a screen reader active,
                // Qt announces the control's own checked state and ignores
                // an Accessible.checked set by hand.
                checkable: true
                checked: dialog.selectedColor === ""
                contentItem: Rectangle {
                    width: 22
                    height: 22
                    anchors.centerIn: parent
                    radius: 11
                    color: "transparent"
                    border.width: dialog.selectedColor === "" ? 2 : 1
                    border.color: dialog.selectedColor === "" ? backend.themeAccent : backend.themeMuted
                    Behavior on border.width { NumberAnimation { duration: 120; easing.type: Easing.OutCubic } }
                }
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
                    checkable: true
                    checked: dialog.selectedColor === modelData
                    contentItem: Rectangle {
                        width: 22
                        height: 22
                        anchors.centerIn: parent
                        radius: 11
                        color: modelData
                        border.width: dialog.selectedColor === modelData ? 3 : 1
                        border.color: dialog.selectedColor === modelData ? backend.themeAccent : backend.themeMuted
                        Behavior on border.width { NumberAnimation { duration: 120; easing.type: Easing.OutCubic } }
                    }
                    onClicked: {
                        dialog.selectedColor = modelData
                        checked = Qt.binding(() => dialog.selectedColor === modelData)
                    }
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
            SpinBox {
                id: cpus
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
                Layout.columnSpan: 2
                Layout.fillWidth: true
                visible: dialog.machine
                text: dialog.cpusTouched
                    ? qsTr("Will be pinned to this value on save.")
                    : (dialog.cpusPinned
                        ? qsTr("Pinned — won't change on its own.")
                        : qsTr("Not pinned yet: adapts automatically, and Travel Mode may reduce it while on battery."))
                color: backend.themeMuted
                font.pixelSize: 12
                wrapMode: Text.Wrap
            }
            Label { text: qsTr("Memory (MiB)") }
            SpinBox {
                id: memory
                Layout.fillWidth: true
                Accessible.name: qsTr("Memory in MiB")
                from: 256
                to: 262144
                stepSize: 256
                editable: true
                // Typing goes into the inner text field, which is what a
                // screen reader announces.
                Component.onCompleted: contentItem.Accessible.name = Accessible.name
                onValueModified: dialog.memoryTouched = true
            }
            Label {
                Layout.columnSpan: 2
                Layout.fillWidth: true
                visible: dialog.machine
                text: dialog.memoryTouched
                    ? qsTr("Will be pinned to this value on save.")
                    : (dialog.memoryPinned
                        ? qsTr("Pinned — won't change on its own.")
                        : qsTr("Not pinned yet: this is just today's default."))
                color: backend.themeMuted
                font.pixelSize: 12
                wrapMode: Text.Wrap
            }
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
                ? qsTr("Requires spice-vdagent in the guest desktop. Restart the Machine to apply a change.")
                : qsTr("Programs like Neovim and tmux can copy text straight to the clipboard. They can never read it. Applies to terminals opened after saving.")
        }
        CheckBox {
            id: travelMode
            text: qsTr("Travel Mode: reduce CPUs automatically on battery")
            ToolTip.visible: hovered
            ToolTip.text: dialog.machine
                ? qsTr("Only applies when you haven't pinned a custom CPU count.")
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

        Label {
            visible: dialog.machine
            text: qsTr("Installation media")
            font.pixelSize: 18
            font.weight: Font.DemiBold
        }
        CheckBox {
            id: disconnectISO
            visible: dialog.machine
            text: qsTr("Installation finished — disconnect ISO on next start")
        }

        Label {
            visible: dialog.machine
            text: qsTr("Shared folder")
            font.pixelSize: 18
            font.weight: Font.DemiBold
        }
        RowLayout {
            visible: dialog.machine
            Layout.fillWidth: true
            TextField {
                id: sharedPath
                Layout.fillWidth: true
                Accessible.name: qsTr("Shared folder")
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
    }

    FolderDialog {
        id: folderPicker
        title: qsTr("Choose a folder to share")
        onAccepted: sharedPath.text = backend.localPath(selectedFolder)
    }

    footer: ColumnLayout {
        spacing: 0
        Label {
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
                    backend.configure(environment.name, description.text.trim(), cpus.value, dialog.cpusTouched, memory.value, dialog.memoryTouched, dialog.machine, sharedPath.text.trim(), sharedReadOnly.checked, disconnectISO.checked, dialog.selectedColor, shareClipboard.checked, travelMode.checked, vulkan.checked, openInEmptyWorkspace.checked, launcher.checked, ssh.checked, fullscreen.checked)
                }
            }
        }
    }
}
