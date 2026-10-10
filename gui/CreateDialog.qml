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
    property bool readyUbuntu: true
    property string readySystem: "ubuntu"
    property bool windowsGuided: false
    property bool autoDownloading: false
    closePolicy: autoDownloading ? Popup.NoAutoClose : Popup.CloseOnEscape
    property string errorText: ""
    property bool submitting: false
    // Created and listed: the window offers to open it right away.
    signal created(string name, string kind, bool prepared)
    property string nameError: ""
    property bool nameTouched: false
    // Same reasoning as SettingsDialog.qml's cpusTouched/memoryTouched:
    // The Core chooses half the host by default. Only manual edits pin
    // CPU or memory and override that dynamic default.
    property bool cpusTouched: false
    property bool memoryTouched: false
    property bool windowsMinimumApplied: false
    property bool hardwareExpanded: false
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

    readonly property var recommendedResources: backend.hostCapabilities ? backend.hostCapabilities["machine-resources"] : undefined
    readonly property int defaultCPUs: recommendedResources && recommendedResources.cpus ? recommendedResources.cpus : 2
    readonly property int defaultMemoryMiB: recommendedResources && recommendedResources.memory_mib ? recommendedResources.memory_mib : 2048
    function updateDefaultResources() {
        if (!cpusTouched) cpus.value = defaultCPUs
        if (!memoryTouched) memory.value = defaultMemoryMiB
    }
    function clearWindowsMinimum() {
        if (windowsMinimumApplied) {
            windowsMinimumApplied = false
            memoryTouched = false
            memory.value = defaultMemoryMiB
        }
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

    // A name suggested from what was chosen, as Parallels names a VM after
    // its system: the ISO's file name without architecture, language and
    // edition noise ("Win11_24H2_English_x64.iso" -> "Win11 24H2"), or
    // the Box's distribution; " 2", " 3"… when it is taken.
    property string suggestedName: ""
    function suggestName() {
        let base
        if (machine) {
            const noise = ["x64", "x86", "x86_64", "amd64", "i386", "i686", "english", "international",
                           "dvd", "dvd1", "live", "desktop", "netinst", "install", "iso", "boot", "anyboot", "multi", "release"]
            base = readyUbuntu ? (readySystem === "fedora" ? qsTr("Fedora 44") : qsTr("Ubuntu 24.04 LTS"))
                : windowsGuided ? qsTr("Windows 11") : image.text.trim().split("/").pop().replace(/\.iso$/i, "")
                .split(/[\s_-]+/).filter(t => t && noise.indexOf(t.toLowerCase()) < 0).join(" ")
        } else {
            base = boxes[boxImage.currentIndex].image ? boxes[boxImage.currentIndex].label
                 : image.text.trim().split("/").pop().split(":")[0]
        }
        base = base ? base.charAt(0).toUpperCase() + base.slice(1) : (machine ? qsTr("Desktop") : qsTr("Box"))
        const taken = (backend.environments || []).map(e => e.name)
        let candidate = base
        for (let n = 2; taken.indexOf(candidate) >= 0; n++)
            candidate = base + " " + n
        return candidate
    }
    // Windows 11's installer refuses less than 4 GB of memory. Recognized
    // by the ISO's name only: a recommendation shown and editable, never
    // a claim about what the ISO is.
    readonly property bool windowsElevenIso: machine && !readyUbuntu && (windowsGuided || /win(dows)?[\s_-]*11/i.test(image.text.split("/").pop()))

    function resetForm() {
        step = 0
        machine = true
        readyUbuntu = true
        readySystem = "ubuntu"
        windowsGuided = false
        errorText = ""
        submitting = false
        nameError = ""
        nameTouched = false
        name.clear()
        suggestedName = ""
        image.clear()
        boxImage.currentIndex = 0
        cpusTouched = false
        memoryTouched = false
        windowsMinimumApplied = false
        updateDefaultResources()
        autoDownloading = false
        hardwareExpanded = false
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
    Connections {
        target: backend
        function onHostCapabilitiesChanged() { dialog.updateDefaultResources() }
    }
    // Said before anything is created: without hardware virtualization a
    // Desktop can be created but won't start. Unknown (no answer from the
    // CLI yet) says nothing.
    readonly property var kvm: backend.hostCapabilities ? backend.hostCapabilities["kvm"] : undefined
    readonly property bool kvmMissing: !!kvm && kvm.available === false
    readonly property var boxTools: {
        const caps = backend.hostCapabilities || {}
        return ["distrobox", "container-engine"].filter(id => caps[id] && caps[id].available === false && caps[id].hint)
                                               .map(id => caps[id].hint)
    }
    readonly property string boxNote: boxTools.join(". ")
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
            if (tag === "download" && dialog.autoDownloading) {
                dialog.autoDownloading = false
                if (ok) {
                    image.text = text.trim().split("\n").pop()
                    createButton.clicked()
                } else {
                    dialog.errorText = text
                }
                return
            }
            if (tag !== "create") return
            dialog.submitting = false
            if (ok) {
                dialog.errorText = ""
                dialog.close()
                dialog.created(name.text.trim(), dialog.machine ? "machine" : "box",
                               false)
            } else {
                dialog.errorText = text
            }
        }
    }

    header: ColumnLayout {
        spacing: 8
        Label {
            textFormat: Text.PlainText
            objectName: "createTitle"
            Layout.fillWidth: true
            Layout.leftMargin: 24
            Layout.rightMargin: 24
            Layout.topMargin: 20
            text: dialog.step === 0 ? qsTr("Choose an environment")
                 : dialog.step === 1 ? qsTr("Choose what to run")
                 : dialog.step === 2 ? qsTr("Name your environment")
                                     : qsTr("Review and create")
            font.pixelSize: 22
            font.weight: Font.DemiBold
            wrapMode: Text.Wrap
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

        ScrollView {
            objectName: "createTypeScroll"
            clip: true
            ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
            ColumnLayout {
                width: parent.width
                spacing: 14
                Label {
                    textFormat: Text.PlainText
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
                            textFormat: Text.PlainText
                            Layout.fillWidth: true
                            text: qsTr("Desktop")
                            font.bold: true
                            horizontalAlignment: Text.AlignHCenter
                            color: desktopChoice.checked ? backend.themeAccentText : backend.themeForeground
                        }
                        Label {
                            textFormat: Text.PlainText
                            Layout.fillWidth: true
                            text: qsTr("A complete computer with its own kernel and graphical display.")
                            wrapMode: Text.Wrap
                            horizontalAlignment: Text.AlignHCenter
                            color: backend.themeMuted
                        }
                        Label {
                            textFormat: Text.PlainText
                            Layout.fillWidth: true
                            text: qsTr("Examples: install Windows, FreeBSD, or an Arch Linux with a custom kernel.")
                            wrapMode: Text.Wrap
                            horizontalAlignment: Text.AlignHCenter
                            font.pixelSize: 12
                            color: backend.themeMuted
                        }
                        Label {
                            textFormat: Text.PlainText
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
                            textFormat: Text.PlainText
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
                          + (dialog.boxNote ? ". " + dialog.boxNote : "")
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
                            textFormat: Text.PlainText
                            Layout.fillWidth: true
                            text: qsTr("Development Box")
                            font.bold: true
                            horizontalAlignment: Text.AlignHCenter
                            color: boxChoice.checked ? backend.themeAccentText : backend.themeForeground
                        }
                        Label {
                            textFormat: Text.PlainText
                            Layout.fillWidth: true
                            text: qsTr("A fast terminal environment sharing your Omarchy home, files and Wayland session.")
                            wrapMode: Text.Wrap
                            horizontalAlignment: Text.AlignHCenter
                            color: backend.themeMuted
                        }
                        Label {
                            textFormat: Text.PlainText
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
            }
        }

        ScrollView {
            objectName: "createSourceScroll"
            clip: true
            ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
            ColumnLayout {
                width: parent.width
                spacing: 16
                Label {
                    textFormat: Text.PlainText
                    Layout.fillWidth: true
                    text: dialog.machine
                        ? qsTr("Choose Ubuntu, Fedora or Windows 11 to download its ISO automatically, then follow the graphical installer. You can also choose your own x86_64 ISO.")
                        : qsTr("Choose a Linux userspace. Distrobox integrates its terminal, files and graphical apps with Omarchy.")
                    wrapMode: Text.Wrap
                    color: backend.themeMuted
                }
                Label {
                    textFormat: Text.PlainText
                    objectName: "boxWarning"
                    Layout.fillWidth: true
                    visible: !dialog.machine && dialog.boxNote !== ""
                    text: qsTr("Development Boxes need a few host tools. %1.").arg(dialog.boxNote)
                    wrapMode: Text.Wrap
                    color: backend.themeRed
                }
                ComboBox {
                    id: boxImage
                    Layout.fillWidth: true
                    visible: !dialog.machine
                    model: dialog.boxes
                    textRole: "label"
                }
                RadioButton {
                    objectName: "readyUbuntuChoice"
                    Layout.fillWidth: true
                    visible: dialog.machine
                    text: qsTr("Ubuntu 24.04 LTS · download ISO automatically")
                    checked: dialog.readyUbuntu && dialog.readySystem === "ubuntu"
                    onClicked: { dialog.readyUbuntu = true; dialog.readySystem = "ubuntu"; dialog.windowsGuided = false; dialog.clearWindowsMinimum(); image.clear() }
                }
                RadioButton {
                    objectName: "readyFedoraChoice"
                    Layout.fillWidth: true
                    visible: dialog.machine
                    text: qsTr("Fedora 44 · download ISO automatically")
                    checked: dialog.readyUbuntu && dialog.readySystem === "fedora"
                    onClicked: { dialog.readyUbuntu = true; dialog.readySystem = "fedora"; dialog.windowsGuided = false; dialog.clearWindowsMinimum(); image.clear() }
                }
                RadioButton {
                    objectName: "windowsGuidedChoice"
                    Layout.fillWidth: true
                    visible: dialog.machine
                    text: qsTr("Windows 11 · download ISO automatically")
                    checked: dialog.windowsGuided
                    onClicked: {
                        const needsMinimum = memory.value < 4096
                        dialog.readyUbuntu = false
                        dialog.windowsGuided = true
                        image.clear()
                        if (needsMinimum) {
                            memory.value = 4096
                            dialog.memoryTouched = true
                            dialog.windowsMinimumApplied = true
                        }
                    }
                }
                RadioButton {
                    objectName: "manualIsoChoice"
                    Layout.fillWidth: true
                    visible: dialog.machine
                    text: qsTr("Install from an ISO")
                    checked: !dialog.readyUbuntu && !dialog.windowsGuided
                    onClicked: { dialog.readyUbuntu = false; dialog.windowsGuided = false; dialog.clearWindowsMinimum(); image.clear() }
                }
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: (dialog.machine && !dialog.readyUbuntu) ||
                             (!dialog.machine && dialog.boxes[boxImage.currentIndex].image === "")
                    TextField {
                        id: image
                        objectName: "imageField"
                        Layout.fillWidth: true
                        Accessible.name: dialog.machine ? qsTr("Installation ISO") : qsTr("Container image")
                        readOnly: dialog.machine
                        placeholderText: dialog.machine ? (dialog.windowsGuided ? qsTr("Choose a Windows 11 ISO…") : qsTr("Choose a boot ISO…")) : qsTr("Container image, for example opensuse/tumbleweed")
                    }
                    RowLayout {
                        visible: dialog.machine && !dialog.readyUbuntu
                        Button {
                            objectName: "chooseIsoButton"
                            text: qsTr("Choose ISO…")
                            onClicked: isoPicker.open()
                        }
                        Button {
                            objectName: "downloadSystemButton"
                            visible: dialog.machine && !!backend.hostCapabilities
                                     && !!backend.hostCapabilities["quickget"]
                                     && backend.hostCapabilities["quickget"].available === true
                            text: dialog.windowsGuided ? qsTr("Download Windows 11…") : qsTr("Download…")
                            onClicked: {
                                downloadDialog.initialQuery = dialog.windowsGuided ? "Windows 11" : ""
                                downloadDialog.open()
                            }
                        }
                    }
                }
                Rectangle {
                    Layout.fillWidth: true
                    implicitHeight: note.implicitHeight + 24
                    radius: 8
                    color: backend.themeSelection
                    Label {
                        textFormat: Text.PlainText
                        id: note
                        anchors.fill: parent
                        anchors.margins: 12
                        text: dialog.machine && dialog.readyUbuntu
                            ? qsTr("OmaVM downloads the official %1 installer ISO and checks its SHA-256 checksum. Start the Desktop after creation and follow the graphical installer.").arg(dialog.readySystem === "fedora" ? qsTr("Fedora 44") : qsTr("Ubuntu 24.04 LTS"))
                            : dialog.machine && dialog.windowsGuided
                            ? qsTr("OmaVM requests the Windows 11 ISO from Microsoft. It provides UEFI, Secure Boot, TPM 2.0 and a disk Windows can see without extra drivers. Follow Windows Setup after the first boot.")
                            : dialog.machine
                            ? qsTr("The ISO is installation media. It will not be copied or modified.")
                            : qsTr("Boxes share the host kernel and can still open graphical apps. Choose Desktop instead when you need a separate kernel or full boot.")
                        wrapMode: Text.Wrap
                    }
                }
            }
        }

        ScrollView {
            clip: true
            ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
            ColumnLayout {
                width: parent.width
                spacing: 16
                Label { textFormat: Text.PlainText; text: dialog.machine ? qsTr("Desktop") : qsTr("Development Box"); color: backend.themeAccentText; font.weight: Font.DemiBold }
                Label {
                    textFormat: Text.PlainText
                    Layout.fillWidth: true
                    text: dialog.machine && dialog.readyUbuntu ? (dialog.readySystem === "fedora" ? qsTr("Fedora 44 installer ISO") : qsTr("Ubuntu 24.04 LTS installer ISO"))
                        : dialog.machine ? image.text : dialog.boxes[boxImage.currentIndex].label
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
                    textFormat: Text.PlainText
                    Layout.fillWidth: true
                    visible: dialog.nameTouched && dialog.nameError !== ""
                    text: dialog.nameError
                    wrapMode: Text.Wrap
                    color: backend.themeRed
                }
                Label {
                    textFormat: Text.PlainText
                    Layout.fillWidth: true
                    text: qsTr("You can start, open and stop it from the same Experience Center as every other environment.")
                    wrapMode: Text.Wrap
                    color: backend.themeMuted
                }
            }
        }

        ScrollView {
            objectName: "createSummaryScroll"
            clip: true
            ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
            ColumnLayout {
                width: parent.width
                spacing: 16
                Label { textFormat: Text.PlainText; text: qsTr("Ready to create"); color: backend.themeAccentText; font.weight: Font.DemiBold }
                GridLayout {
                    Layout.fillWidth: true
                    columns: 2
                    columnSpacing: 16
                    rowSpacing: 6
                    Label { textFormat: Text.PlainText; text: qsTr("Type:"); color: backend.themeMuted }
                    RowLayout {
                        spacing: 6
                        Icon {
                            source: dialog.machine ? "qrc:/icons/machine.svg" : "qrc:/icons/box.svg"
                            color: backend.themeAccentText
                            iconSize: 16
                        }
                        Label { textFormat: Text.PlainText; text: dialog.machine ? qsTr("Desktop") : qsTr("Development Box") }
                    }
                    Label { textFormat: Text.PlainText; text: dialog.machine ? qsTr("Installation media:") : qsTr("Image:"); color: backend.themeMuted }
                    Label {
                        textFormat: Text.PlainText
                        Layout.fillWidth: true
                        text: dialog.machine && dialog.readyUbuntu ? (dialog.readySystem === "fedora" ? qsTr("Fedora 44 installer ISO") : qsTr("Ubuntu 24.04 LTS installer ISO"))
                            : dialog.machine ? image.text : dialog.boxes[boxImage.currentIndex].label
                        elide: Text.ElideMiddle
                    }
                    Label { textFormat: Text.PlainText; text: qsTr("Name:"); color: backend.themeMuted }
                    Label {
                        textFormat: Text.PlainText
                        Layout.fillWidth: true
                        text: name.text.trim()
                        elide: Text.ElideRight
                    }
                    // The qcow2 is sparse (qemu.diskSize): nothing is
                    // reserved up front, which is what people ask first.
                    Label { textFormat: Text.PlainText; visible: dialog.machine; text: qsTr("Disk:"); color: backend.themeMuted }
                    Label {
                        textFormat: Text.PlainText
                        visible: dialog.machine
                        Layout.fillWidth: true
                        text: qsTr("grows as it's used, up to 1 TB")
                        wrapMode: Text.Wrap
                    }
                }
                Label {
                    textFormat: Text.PlainText
                    Layout.fillWidth: true
                    visible: dialog.machine
                    text: qsTr("%1 CPUs · %2 memory").arg(cpus.value).arg(memory.displayText)
                    color: backend.themeMuted
                }
                Button {
                    objectName: "createHardwareButton"
                    visible: dialog.machine
                    text: dialog.hardwareExpanded ? qsTr("Hide hardware controls") : qsTr("Adjust hardware")
                    onClicked: dialog.hardwareExpanded = !dialog.hardwareExpanded
                }
                GridLayout {
                    Layout.fillWidth: true
                    visible: dialog.machine && dialog.hardwareExpanded
                    columns: width < 330 ? 1 : 2
                    columnSpacing: 16
                    rowSpacing: 16
                    ColumnLayout {
                        Label { textFormat: Text.PlainText; text: qsTr("CPUs"); color: backend.themeMuted }
                        SpinBox {
                            id: cpus
                            objectName: "createCPUs"
                            Accessible.name: qsTr("CPUs")
                            Component.onCompleted: contentItem.Accessible.name = Accessible.name
                            from: 1
                            to: 64
                            value: 2
                            onValueModified: dialog.cpusTouched = true
                        }
                    }
                    ColumnLayout {
                        Label { textFormat: Text.PlainText; text: qsTr("Memory"); color: backend.themeMuted }
                        MemorySpinBox {
                            id: memory
                            objectName: "createMemory"
                            from: dialog.windowsGuided ? 4096 : 256
                            value: 2048
                            onValueModified: { dialog.memoryTouched = true; dialog.windowsMinimumApplied = false }
                        }
                    }
                }
                Label {
                    textFormat: Text.PlainText
                    objectName: "windowsMemoryNote"
                    Layout.fillWidth: true
                    visible: dialog.windowsElevenIso && memory.value >= 4096
                    text: qsTr("Windows 11 needs at least 4 GB of memory; this Desktop uses %1.").arg(memory.displayText)
                    wrapMode: Text.Wrap
                    color: backend.themeMuted
                    font.pixelSize: 12
                }
                Rectangle {
                    Layout.fillWidth: true
                    implicitHeight: closingNote.implicitHeight + 24
                    radius: 8
                    color: backend.themeSelection
                    visible: closingNote.text !== ""
                    Label {
                        textFormat: Text.PlainText
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
            }
        }
    }

    footer: ColumnLayout {
        spacing: 0
        Label {
            textFormat: Text.PlainText
            Layout.fillWidth: true
            Layout.leftMargin: 24
            Layout.rightMargin: 24
            visible: dialog.autoDownloading
            text: (backend.progress && backend.progress["download"]) || qsTr("Downloading installation ISO…")
            wrapMode: Text.Wrap
            color: backend.themeMuted
        }
        Label {
            textFormat: Text.PlainText
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
                enabled: !dialog.submitting && !dialog.autoDownloading
                DialogButtonBox.buttonRole: DialogButtonBox.ActionRole
                onClicked: dialog.step === 0 ? dialog.reject() : dialog.step--
            }
            Button {
                id: createButton
                objectName: "createButton"
                text: dialog.autoDownloading ? qsTr("Downloading…") : dialog.submitting ? qsTr("Creating…") : dialog.step === dialog.lastStep
                      ? (dialog.width < 360 ? qsTr("Create") : qsTr("Create Environment")) : qsTr("Continue")
                Accessible.name: dialog.step === dialog.lastStep ? qsTr("Create Environment") : text
                highlighted: true
                enabled: !dialog.submitting && !dialog.autoDownloading
                DialogButtonBox.buttonRole: DialogButtonBox.ActionRole
                onClicked: {
                    if (dialog.step === 1 && dialog.windowsGuided && (dialog.kvmMissing || dialog.windowsHints.length > 0)) {
                        dialog.errorText = dialog.kvmMissing
                            ? qsTr("Windows 11 needs hardware virtualization before you continue.")
                            : dialog.windowsNote
                        return
                    }
                    dialog.errorText = ""
                    if (dialog.step === 1 && !dialog.selectedImage()) {
                        if (dialog.machine && (dialog.readyUbuntu || dialog.windowsGuided)) {
                            dialog.autoDownloading = true
                            backend.downloadImage(dialog.readyUbuntu ? dialog.readySystem : "windows",
                                                  dialog.readyUbuntu ? (dialog.readySystem === "fedora" ? "44" : "24.04") : "11",
                                                  dialog.readySystem === "fedora" && dialog.readyUbuntu ? "Workstation" : "")
                        } else {
                            backend.message(dialog.machine ? qsTr("Choose an installation ISO") : qsTr("Enter a container image"), true)
                        }
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
                    if (dialog.step === 1) {
                        // Suggested again only while the person hasn't
                        // typed a name of their own.
                        if (name.text === "" || name.text === dialog.suggestedName) {
                            dialog.suggestedName = dialog.suggestName()
                            name.text = dialog.suggestedName
                            name.selectAll()
                        }
                        if (dialog.windowsElevenIso && !dialog.memoryTouched && memory.value < 4096) {
                            memory.value = 4096
                            dialog.memoryTouched = true
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
    DownloadDialog {
        id: downloadDialog
        objectName: "downloadDialog"
        parent: Overlay.overlay
        onDownloaded: path => image.text = path
    }
}
