import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Dialogs
import QtQuick.Layouts

Dialog {
    id: dialog
    title: qsTr("New Environment")
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 680)
    height: Math.min(parent.height - 32, 580)
    padding: 24
    Overlay.modal: ThemeScrim {}

    readonly property int lastStep: 3
    property int step: 0
    property bool machine: true
    property string errorText: ""
    property bool submitting: false
    // Created and listed: the window offers to open it right away.
    signal created(string name, string kind)
    property string nameError: ""
    property bool nameTouched: false
    // Same reasoning as SettingsDialog.qml's cpusTouched/memoryTouched:
    // the summary step shows 2 CPUs/2048 MiB as a helpful default, but
    // sending that unconditionally to `omavm create` would pin every
    // new Machine's hardware from birth — permanently disabling Travel
    // Mode's automatic reduction on battery for it (the same Core bug
    // class documented in docs/TODO.md P2, just hit at creation time
    // instead of a later Settings save). Only actually touching a
    // SpinBox (SpinBox.onValueModified, real user interaction only)
    // marks it as an intentional choice.
    property bool cpusTouched: false
    property bool memoryTouched: false
    property var boxes: [
        { label: "Fedora", image: "fedora:latest" },
        { label: "Ubuntu", image: "ubuntu:latest" },
        { label: "Debian", image: "debian:latest" },
        { label: "Arch Linux", image: "archlinux:latest" },
        { label: "Alpine", image: "alpine:latest" },
        { label: qsTr("Custom image"), image: "" }
    ]

    function selectedImage() {
        return machine ? image.text.trim()
                       : (boxes[boxImage.currentIndex].image || image.text.trim())
    }

    // Mirrors internal/core/service.go's validateEnvironmentName so a bad
    // name is caught here, at the field, instead of only after a round
    // trip to the CLI. Deliberately permissive otherwise: spaces and
    // accented characters are valid names and must stay usable.
    function validateName(candidate) {
        if (!candidate)
            return qsTr("Enter a name for the environment")
        if (candidate === "." || candidate === "..")
            return qsTr("\"%1\" is not a valid environment name").arg(candidate)
        if (candidate.startsWith("-"))
            return qsTr("Environment name cannot start with \"-\"")
        for (let i = 0; i < candidate.length; i++) {
            const ch = candidate.charAt(i)
            const code = candidate.charCodeAt(i)
            // Same as Go's unicode.IsControl: C0, DEL and C1.
            if (ch === "/" || ch === "\\" || code < 32 || (code >= 127 && code < 160))
                return qsTr("Environment name cannot contain \"%1\"").arg(ch)
        }
        return ""
    }

    function resetForm() {
        step = 0
        machine = true
        errorText = ""
        submitting = false
        nameError = ""
        nameTouched = false
        name.clear()
        image.clear()
        boxImage.currentIndex = 0
        cpus.value = 2
        memory.value = 2048
        cpusTouched = false
        memoryTouched = false
    }

    // Tracks the step a transition is animating FROM, so the content can
    // slide in from the direction the user is actually moving (forward
    // = Continue, backward = Back) instead of a plain, direction-blind
    // fade — a small touch that makes the wizard read as a sequence
    // rather than a stack of unrelated screens.
    property int animatingFromStep: 0
    onOpened: {
        resetForm()
        backend.refreshHost()
    }
    // Said before anything is created: without hardware virtualization a
    // Desktop can be created but won't start. Unknown (no answer from the
    // CLI yet) says nothing.
    readonly property var kvm: backend.hostCapabilities ? backend.hostCapabilities["kvm"] : undefined
    readonly property bool kvmMissing: !!kvm && kvm.available === false
    // Desktops still work without OVMF or swtpm, but Windows 11 refuses to
    // install without Secure Boot and a TPM: what to install, said quietly.
    readonly property var windowsHints: {
        const caps = backend.hostCapabilities || {}
        return ["uefi", "tpm"].filter(id => caps[id] && caps[id].available === false && caps[id].hint)
                              .map(id => caps[id].hint)
    }
    readonly property string windowsNote: windowsHints.length > 0
        ? qsTr("Windows 11 won't install yet. %1.").arg(windowsHints.join(". "))
        : ""
    onStepChanged: {
        const forward = step > animatingFromStep
        animatingFromStep = step
        stepStack.opacity = 0
        stepTranslate.x = forward ? 28 : -28
        stepEnter.restart()
    }

    ParallelAnimation {
        id: stepEnter
        NumberAnimation { target: stepStack; property: "opacity"; to: 1; duration: 180; easing.type: Easing.OutCubic }
        NumberAnimation { target: stepTranslate; property: "x"; to: 0; duration: 220; easing.type: Easing.OutCubic }
    }

    Connections {
        target: backend
        function onActionFinished(tag, ok, text) {
            if (tag !== "create") return
            dialog.submitting = false
            if (ok) {
                dialog.errorText = ""
                dialog.close()
                dialog.created(name.text.trim(), dialog.machine ? "machine" : "box")
            } else {
                dialog.errorText = text
            }
        }
    }

    header: ColumnLayout {
        spacing: 8
        Label {
            Layout.leftMargin: 24
            Layout.topMargin: 20
            text: dialog.step === 0 ? qsTr("Choose an environment")
                 : dialog.step === 1 ? qsTr("Choose what to run")
                 : dialog.step === 2 ? qsTr("Name your environment")
                                     : qsTr("Review and create")
            font.pixelSize: 22
            font.weight: Font.DemiBold
        }
        RowLayout {
            Layout.fillWidth: true
            Layout.leftMargin: 24
            Layout.rightMargin: 24
            spacing: 8
            Repeater {
                model: dialog.lastStep + 1
                Rectangle {
                    required property int index
                    Layout.fillWidth: true
                    implicitHeight: 3
                    radius: 2
                    color: index <= dialog.step ? backend.themeAccentText : backend.themeMuted
                    opacity: index <= dialog.step ? 1 : 0.35
                }
            }
        }
    }

    contentItem: StackLayout {
        id: stepStack
        currentIndex: dialog.step
        transform: Translate { id: stepTranslate }

        Item {
            ColumnLayout {
                anchors.fill: parent
                spacing: 14
                Label {
                    Layout.fillWidth: true
                    text: qsTr("How should this environment work?")
                    wrapMode: Text.Wrap
                    color: backend.themeMuted
                }
                Button {
                    id: desktopChoice
                    Layout.fillWidth: true
                    // The card's text is drawn by its own contentItem.
                    Accessible.name: qsTr("Desktop")
                    Accessible.description: dialog.kvmMissing
                        ? qsTr("A complete system with its own kernel, from an installation ISO. This computer can't start one yet: hardware virtualization isn't available.")
                        : qsTr("A complete system with its own kernel, from an installation ISO")
                          + (dialog.windowsNote ? ". " + dialog.windowsNote : "")
                    checkable: true
                    checked: dialog.machine
                    padding: 16
                    scale: desktopChoice.pressed ? 0.97 : 1
                    Behavior on scale { NumberAnimation { duration: 100; easing.type: Easing.OutCubic } }
                    // Height follows the wrapped text below instead of a
                    // fixed guess: a plain multi-line `text:` on a Button
                    // doesn't wrap at all (only explicit \n breaks), so at
                    // the dialog's real ~630px content width the longer
                    // example sentence overflowed past the button and the
                    // dialog edge (confirmed by rendering this in isolation
                    // at that width before fixing it).
                    contentItem: ColumnLayout {
                        spacing: 6
                        Icon {
                            Layout.alignment: Qt.AlignHCenter
                            source: "qrc:/icons/machine.svg"
                            iconSize: 32
                            color: desktopChoice.checked ? backend.themeAccentText : backend.themeMuted
                        }
                        Label {
                            Layout.fillWidth: true
                            text: qsTr("Desktop")
                            font.bold: true
                            horizontalAlignment: Text.AlignHCenter
                            color: desktopChoice.checked ? backend.themeAccentText : backend.themeForeground
                        }
                        Label {
                            Layout.fillWidth: true
                            text: qsTr("A complete computer with its own kernel and graphical display.")
                            wrapMode: Text.Wrap
                            horizontalAlignment: Text.AlignHCenter
                            color: backend.themeMuted
                        }
                        Label {
                            Layout.fillWidth: true
                            text: qsTr("Examples: install Windows, FreeBSD, or an Arch Linux with a custom kernel.")
                            wrapMode: Text.Wrap
                            horizontalAlignment: Text.AlignHCenter
                            font.pixelSize: 12
                            color: backend.themeMuted
                        }
                        Label {
                            objectName: "kvmWarning"
                            Layout.fillWidth: true
                            visible: dialog.kvmMissing
                            text: qsTr("This computer can't start Desktops yet: hardware virtualization isn't available to your user. %1.")
                                  .arg(dialog.kvm && dialog.kvm.hint ? dialog.kvm.hint : qsTr("Enable virtualization in the firmware"))
                            wrapMode: Text.Wrap
                            horizontalAlignment: Text.AlignHCenter
                            color: backend.themeRed
                        }
                        Label {
                            objectName: "windowsNote"
                            Layout.fillWidth: true
                            visible: !dialog.kvmMissing && dialog.windowsHints.length > 0
                            text: dialog.windowsNote
                            wrapMode: Text.Wrap
                            horizontalAlignment: Text.AlignHCenter
                            font.pixelSize: 12
                            color: backend.themeMuted
                        }
                    }
                    onClicked: dialog.machine = true
                }
                Button {
                    id: boxChoice
                    Layout.fillWidth: true
                    Accessible.name: qsTr("Development Box")
                    Accessible.description: qsTr("Linux tools and apps sharing your home folder and this computer's kernel")
                    checkable: true
                    checked: !dialog.machine
                    padding: 16
                    scale: boxChoice.pressed ? 0.97 : 1
                    Behavior on scale { NumberAnimation { duration: 100; easing.type: Easing.OutCubic } }
                    contentItem: ColumnLayout {
                        spacing: 6
                        Icon {
                            Layout.alignment: Qt.AlignHCenter
                            source: "qrc:/icons/box.svg"
                            iconSize: 32
                            color: boxChoice.checked ? backend.themeAccentText : backend.themeMuted
                        }
                        Label {
                            Layout.fillWidth: true
                            text: qsTr("Development Box")
                            font.bold: true
                            horizontalAlignment: Text.AlignHCenter
                            color: boxChoice.checked ? backend.themeAccentText : backend.themeForeground
                        }
                        Label {
                            Layout.fillWidth: true
                            text: qsTr("A fast terminal environment sharing your Omarchy home, files and Wayland session.")
                            wrapMode: Text.Wrap
                            horizontalAlignment: Text.AlignHCenter
                            color: backend.themeMuted
                        }
                        Label {
                            Layout.fillWidth: true
                            text: qsTr("Examples: a Fedora terminal for a project, an Ubuntu toolchain — can also open and export graphical apps to your launcher.")
                            wrapMode: Text.Wrap
                            horizontalAlignment: Text.AlignHCenter
                            font.pixelSize: 12
                            color: backend.themeMuted
                        }
                    }
                    onClicked: dialog.machine = false
                }
                Item { Layout.fillHeight: true }
            }
        }

        Item {
            ColumnLayout {
                anchors.fill: parent
                spacing: 16
                Label {
                    Layout.fillWidth: true
                    text: dialog.machine
                        ? qsTr("Install from an x86_64 ISO. OmaVM will open the graphical installer for you.")
                        : qsTr("Choose a Linux userspace. Distrobox integrates its terminal, files and graphical apps with Omarchy.")
                    wrapMode: Text.Wrap
                    color: backend.themeMuted
                }
                ComboBox {
                    id: boxImage
                    Layout.fillWidth: true
                    visible: !dialog.machine
                    model: dialog.boxes
                    textRole: "label"
                }
                RowLayout {
                    Layout.fillWidth: true
                    visible: dialog.machine || dialog.boxes[boxImage.currentIndex].image === ""
                    TextField {
                        id: image
                        Layout.fillWidth: true
                        Accessible.name: dialog.machine ? qsTr("Installation ISO") : qsTr("Container image")
                        readOnly: dialog.machine
                        placeholderText: dialog.machine ? qsTr("Choose a boot ISO…") : qsTr("Container image, for example opensuse/tumbleweed")
                    }
                    Button {
                        visible: dialog.machine
                        text: qsTr("Choose ISO…")
                        onClicked: isoPicker.open()
                    }
                }
                Rectangle {
                    Layout.fillWidth: true
                    implicitHeight: note.implicitHeight + 24
                    radius: 8
                    color: backend.themeSelection
                    Label {
                        id: note
                        anchors.fill: parent
                        anchors.margins: 12
                        text: dialog.machine
                            ? qsTr("The ISO is installation media. It will not be copied or modified.")
                            : qsTr("Boxes share the host kernel and can still open graphical apps. Choose Desktop instead when you need a separate kernel or full boot.")
                        wrapMode: Text.Wrap
                    }
                }
                Item { Layout.fillHeight: true }
            }
        }

        Item {
            ColumnLayout {
                anchors.fill: parent
                spacing: 16
                Label { text: dialog.machine ? qsTr("Desktop") : qsTr("Development Box"); color: backend.themeAccentText; font.weight: Font.DemiBold }
                Label {
                    Layout.fillWidth: true
                    text: dialog.machine ? image.text : dialog.boxes[boxImage.currentIndex].label
                    elide: Text.ElideMiddle
                    color: backend.themeMuted
                }
                TextField {
                    id: name
                    objectName: "nameField"
                    Layout.fillWidth: true
                    Accessible.name: qsTr("Environment name")
                    placeholderText: qsTr("Environment name")
                    focus: dialog.step === 2
                    onAccepted: createButton.clicked()
                    onEditingFinished: dialog.nameTouched = true
                }
                Label {
                    Layout.fillWidth: true
                    visible: dialog.nameTouched && dialog.nameError !== ""
                    text: dialog.nameError
                    wrapMode: Text.Wrap
                    color: backend.themeRed
                }
                Label {
                    Layout.fillWidth: true
                    text: qsTr("You can start, open and stop it from the same Experience Center as every other environment.")
                    wrapMode: Text.Wrap
                    color: backend.themeMuted
                }
                Item { Layout.fillHeight: true }
            }
        }

        Item {
            ColumnLayout {
                anchors.fill: parent
                spacing: 16
                Label { text: qsTr("Ready to create"); color: backend.themeAccentText; font.weight: Font.DemiBold }
                GridLayout {
                    Layout.fillWidth: true
                    columns: 2
                    columnSpacing: 16
                    rowSpacing: 6
                    Label { text: qsTr("Type:"); color: backend.themeMuted }
                    RowLayout {
                        spacing: 6
                        Icon {
                            source: dialog.machine ? "qrc:/icons/machine.svg" : "qrc:/icons/box.svg"
                            color: backend.themeAccentText
                            iconSize: 16
                        }
                        Label { text: dialog.machine ? qsTr("Desktop") : qsTr("Development Box") }
                    }
                    Label { text: dialog.machine ? qsTr("Installation media:") : qsTr("Image:"); color: backend.themeMuted }
                    Label {
                        Layout.fillWidth: true
                        text: dialog.machine ? image.text : dialog.boxes[boxImage.currentIndex].label
                        elide: Text.ElideMiddle
                    }
                    Label { text: qsTr("Name:"); color: backend.themeMuted }
                    Label {
                        Layout.fillWidth: true
                        text: name.text.trim()
                        elide: Text.ElideRight
                    }
                }
                RowLayout {
                    Layout.fillWidth: true
                    visible: dialog.machine
                    spacing: 16
                    ColumnLayout {
                        Label { text: qsTr("CPUs"); color: backend.themeMuted }
                        SpinBox {
                            id: cpus
                            Accessible.name: qsTr("CPUs")
                            Component.onCompleted: contentItem.Accessible.name = Accessible.name
                            from: 1
                            to: 64
                            value: 2
                            onValueModified: dialog.cpusTouched = true
                        }
                    }
                    ColumnLayout {
                        Label { text: qsTr("Memory (MiB)"); color: backend.themeMuted }
                        SpinBox {
                            id: memory
                            Accessible.name: qsTr("Memory in MiB")
                            from: 256
                            to: 262144
                            stepSize: 256
                            editable: true
                // Typing goes into the inner text field, which is what a
                // screen reader announces.
                Component.onCompleted: contentItem.Accessible.name = Accessible.name
                            value: 2048
                            onValueModified: dialog.memoryTouched = true
                        }
                    }
                    Item { Layout.fillWidth: true }
                }
                Rectangle {
                    Layout.fillWidth: true
                    implicitHeight: closingNote.implicitHeight + 24
                    radius: 8
                    color: backend.themeSelection
                    visible: closingNote.text !== ""
                    Label {
                        id: closingNote
                        anchors.fill: parent
                        anchors.margins: 12
                        wrapMode: Text.Wrap
                        text: dialog.machine && image.text
                            ? qsTr("Starting this Desktop opens the graphical installer from the ISO. Once the guest OS is installed, shut it down, then disconnect the installation media from Settings before starting again from its own disk.")
                            : (!dialog.machine
                                ? qsTr("Graphical applications inside can be exported to your Omarchy launcher anytime from the environment's \"Applications…\" menu.")
                                : "")
                    }
                }
                Item { Layout.fillHeight: true }
            }
        }
    }

    footer: ColumnLayout {
        spacing: 0
        Label {
            Layout.fillWidth: true
            Layout.leftMargin: 24
            Layout.rightMargin: 24
            visible: dialog.errorText !== ""
            text: dialog.errorText
            wrapMode: Text.Wrap
            color: backend.themeRed
        }
        DialogButtonBox {
            Layout.fillWidth: true
            Button {
                text: dialog.step === 0 ? qsTr("Cancel") : qsTr("Back")
                enabled: !dialog.submitting
                DialogButtonBox.buttonRole: DialogButtonBox.ActionRole
                onClicked: dialog.step === 0 ? dialog.reject() : dialog.step--
            }
            Button {
                id: createButton
                text: dialog.submitting ? qsTr("Creating…") : dialog.step === dialog.lastStep ? qsTr("Create Environment") : qsTr("Continue")
                highlighted: true
                enabled: !dialog.submitting
                DialogButtonBox.buttonRole: DialogButtonBox.ActionRole
                onClicked: {
                    if (dialog.step === 1 && !dialog.selectedImage()) {
                        backend.message(dialog.machine ? qsTr("Choose an installation ISO") : qsTr("Enter a container image"), true)
                        return
                    }
                    if (dialog.step === 2) {
                        dialog.nameTouched = true
                        dialog.nameError = dialog.validateName(name.text.trim())
                        if (dialog.nameError !== "") {
                            name.forceActiveFocus()
                            return
                        }
                    }
                    if (dialog.step < dialog.lastStep) {
                        dialog.step++
                        return
                    }
                    dialog.errorText = ""
                    dialog.submitting = true
                    backend.createEnvironment(name.text.trim(), dialog.selectedImage(),
                                              dialog.machine ? "machine" : "box",
                                              cpus.value, dialog.cpusTouched,
                                              memory.value, dialog.memoryTouched)
                }
            }
        }
    }

    FileDialog {
        id: isoPicker
        title: qsTr("Choose a boot ISO")
        nameFilters: [qsTr("ISO images (*.iso)"), qsTr("All files (*)")]
        onAccepted: image.text = backend.localPath(selectedFile)
    }
}
