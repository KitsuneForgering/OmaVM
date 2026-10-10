import QtQuick
import QtTest

Item {
    id: root
    width: 800
    height: 700

    TestCase {
        name: "CreateDialog"

        // The field repeats the Core's name rules (internal/core's
        // validateEnvironmentName) so a bad name is caught before the CLI.
        function test_nameRulesMatchTheCore() {
            const component = Qt.createComponent("qrc:/CreateDialog.qml")
            compare(component.status, Component.Ready, component.errorString())
            const dialog = component.createObject(root)
            verify(dialog.validateName("") !== "")
            verify(dialog.validateName("..") !== "")
            verify(dialog.validateName("a/b") !== "")
            // Regression: a leading dash made the name read as an option.
            verify(dialog.validateName("--json") !== "")
            verify(dialog.validateName("-x") !== "")
            verify(dialog.validateName("a\u0085b") !== "", "C1 control")
            compare(dialog.validateName("a-b"), "")
            compare(dialog.validateName("Fedora de Teste"), "")
            compare(dialog.validateName("Ação"), "")
            dialog.destroy()
        }

        // Creation leads to first use: the window learns which environment
        // was created and of which kind, only when it succeeded.
        function test_successfulCreationIsAnnounced() {
            const component = Qt.createComponent("qrc:/CreateDialog.qml")
            const dialog = component.createObject(root)
            const spy = createTemporaryObject(signalSpyComponent, root, { target: dialog, signalName: "created" })
            dialog.machine = false
            findChild(dialog, "nameField").text = " dev "
            backend.finishAction("create", false, "boom")
            compare(spy.count, 0)
            backend.finishAction("create", true, "")
            compare(spy.count, 1)
            compare(spy.signalArguments[0][0], "dev")
            compare(spy.signalArguments[0][1], "box")
            backend.finishAction("start", true, "")
            compare(spy.count, 1)
            dialog.destroy()
        }
    }

    TestCase {
        name: "CreateDialogHost"
        when: windowShown

        function test_ubuntuDownloadsISOByDefault() {
            const dialog = Qt.createComponent("qrc:/CreateDialog.qml").createObject(root)
            dialog.open()
            tryVerify(() => dialog.opened)
            dialog.step = 1
            verify(dialog.readyUbuntu)
            compare(dialog.selectedImage(), "")
            verify(findChild(dialog, "readyUbuntuChoice").checked)
            backend.lastCall = []
            mouseClick(findChild(dialog, "createButton"))
            compare(backend.lastCall[0], "downloadImage")
            compare(backend.lastCall[1], "ubuntu")
            compare(backend.lastCall[2], "24.04")
            compare(dialog.step, 1)
            backend.finishAction("download", true, "/tmp/ubuntu-24.04.iso")
            compare(dialog.step, 2)
            compare(dialog.selectedImage(), "/tmp/ubuntu-24.04.iso")
            compare(findChild(dialog, "nameField").text, "Ubuntu 24.04 LTS")
            dialog.destroy()
        }

        function test_failedISODownloadCanBeRetried() {
            const dialog = Qt.createComponent("qrc:/CreateDialog.qml").createObject(root)
            dialog.open()
            tryVerify(() => dialog.opened)
            dialog.step = 1
            mouseClick(findChild(dialog, "createButton"))
            verify(dialog.autoDownloading)
            verify(!findChild(dialog, "createButton").enabled)
            backend.finishAction("download", false, "network error")
            verify(!dialog.autoDownloading)
            compare(dialog.errorText, "network error")
            compare(dialog.step, 1)
            mouseClick(findChild(dialog, "createButton"))
            verify(dialog.autoDownloading)
            backend.finishAction("download", true, "/tmp/ubuntu.iso")
            compare(dialog.step, 2)
            dialog.destroy()
        }

        function test_fedoraAndWindowsGuidedChoices() {
            const dialog = Qt.createComponent("qrc:/CreateDialog.qml").createObject(root)
            dialog.open()
            tryVerify(() => dialog.opened)
            dialog.step = 1
            mouseClick(findChild(dialog, "readyFedoraChoice"))
            compare(dialog.selectedImage(), "")
            mouseClick(findChild(dialog, "createButton"))
            compare(backend.lastCall[1], "fedora")
            compare(backend.lastCall[2], "44")
            compare(backend.lastCall[3], "Workstation")
            backend.finishAction("download", true, "/tmp/fedora-44.iso")
            compare(findChild(dialog, "nameField").text, "Fedora 44")
            dialog.step = 1
            mouseClick(findChild(dialog, "windowsGuidedChoice"))
            compare(dialog.selectedImage(), "")
            backend.hostCapabilities = { kvm: { available: true }, uefi: { available: true },
                                         tpm: { available: false, hint: "Install swtpm" } }
            mouseClick(findChild(dialog, "createButton"))
            compare(dialog.step, 1)
            verify(dialog.errorText.indexOf("Install swtpm") >= 0)
            backend.hostCapabilities = { kvm: { available: true }, uefi: { available: true }, tpm: { available: true } }
            mouseClick(findChild(dialog, "createButton"))
            compare(backend.lastCall[1], "windows")
            compare(backend.lastCall[2], "11")
            backend.finishAction("download", true, "/home/u/Win11.iso")
            compare(dialog.selectedImage(), "/home/u/Win11.iso")
            verify(dialog.windowsElevenIso)
            compare(findChild(dialog, "nameField").text, "Windows 11")
            verify(dialog.memoryTouched)
            backend.hostCapabilities = {}
            dialog.destroy()
        }

        // The name is suggested from the ISO, without architecture and
        // language noise, unique among existing environments; a Windows 11
        // ISO starts with the 4 GB its installer requires.
        function test_nameAndMemoryAreSuggestedFromTheISO() {
            backend.environments = [{ name: "Win11 24H2" }]
            const dialog = Qt.createComponent("qrc:/CreateDialog.qml").createObject(root)
            dialog.open()
            tryVerify(() => dialog.opened)
            dialog.machine = true
            dialog.readyUbuntu = false
            dialog.step = 1
            findChild(dialog, "imageField").text = "/home/u/Downloads/Win11_24H2_English_x64.iso"
            mouseClick(findChild(dialog, "createButton"))
            compare(dialog.step, 2)
            compare(findChild(dialog, "nameField").text, "Win11 24H2 2")
            verify(dialog.memoryTouched)
            verify(dialog.windowsElevenIso)
            // A name typed by the person is kept on the way back and forth.
            findChild(dialog, "nameField").text = "work"
            dialog.step = 1
            mouseClick(findChild(dialog, "createButton"))
            compare(findChild(dialog, "nameField").text, "work")
            backend.environments = []
            dialog.destroy()
        }

        function test_wizardCanScrollInANarrowWindow() {
            const oldWidth = root.width
            const oldHeight = root.height
            root.width = 320
            root.height = 420
            const dialog = Qt.createComponent("qrc:/CreateDialog.qml").createObject(root)
            dialog.open()
            tryVerify(() => dialog.opened)
            compare(dialog.width, 288)
            const typeScroll = findChild(dialog, "createTypeScroll")
            verify(typeScroll.contentItem.contentHeight > typeScroll.height,
                   "both environment choices must remain scrollable")
            typeScroll.contentItem.contentY = typeScroll.contentItem.contentHeight - typeScroll.height
            verify(typeScroll.contentItem.contentY > 0)

            dialog.step = 1
            dialog.readyUbuntu = false
            backend.hostCapabilities = { quickget: { available: true } }
            const choose = findChild(dialog, "chooseIsoButton")
            const download = findChild(dialog, "downloadSystemButton")
            verify(choose.visible && download.visible)
            const right = download.mapToItem(root, download.width, 0).x
            verify(right <= dialog.x + dialog.width - dialog.padding,
                   "ISO actions must fit inside the narrow dialog")

            dialog.step = 3
            findChild(dialog, "imageField").text = "/home/u/Downloads/Win11_24H2_English_x64.iso"
            const create = findChild(dialog, "createButton")
            compare(create.text, "Create")
            verify(create.mapToItem(root, create.width, 0).x <= dialog.x + dialog.width,
                   "the final action must fit inside the narrow dialog")
            const summaryScroll = findChild(dialog, "createSummaryScroll")
            verify(summaryScroll.contentItem.contentHeight > summaryScroll.height,
                   "the summary and its installation note must remain scrollable")
            summaryScroll.contentItem.contentY = summaryScroll.contentItem.contentHeight - summaryScroll.height
            verify(summaryScroll.contentItem.contentY > 0)
            backend.hostCapabilities = {}
            dialog.destroy()
            root.width = oldWidth
            root.height = oldHeight
        }

        function test_hardwareIsSecondaryButAdjustable() {
            const dialog = Qt.createComponent("qrc:/CreateDialog.qml").createObject(root)
            dialog.open()
            tryVerify(() => dialog.opened)
            dialog.step = 3
            const toggle = findChild(dialog, "createHardwareButton")
            verify(!dialog.hardwareExpanded)
            verify(!dialog.cpusTouched && !dialog.memoryTouched)
            compare(toggle.text, "Adjust hardware")
            mouseClick(toggle)
            verify(dialog.hardwareExpanded)
            verify(!dialog.cpusTouched && !dialog.memoryTouched)
            compare(toggle.text, "Hide hardware controls")
            dialog.destroy()
        }

        function test_hostResourcesSetUntouchedMachineDefaults() {
            backend.hostCapabilities = { "machine-resources": { cpus: 6, memory_mib: 8192 } }
            const dialog = Qt.createComponent("qrc:/CreateDialog.qml").createObject(root)
            dialog.open()
            tryVerify(() => dialog.opened)
            dialog.step = 3
            compare(dialog.defaultCPUs, 6)
            compare(dialog.defaultMemoryMiB, 8192)
            compare(findChild(dialog, "createCPUs").value, 6)
            compare(findChild(dialog, "createMemory").value, 8192)
            verify(!dialog.cpusTouched && !dialog.memoryTouched)
            mouseClick(findChild(dialog, "createHardwareButton"))
            backend.hostCapabilities = { "machine-resources": { cpus: 8, memory_mib: 12288 } }
            compare(dialog.defaultCPUs, 8)
            compare(dialog.defaultMemoryMiB, 12288)
            compare(findChild(dialog, "createCPUs").value, 8)
            compare(findChild(dialog, "createMemory").value, 12288)
            verify(!dialog.cpusTouched && !dialog.memoryTouched)
            backend.hostCapabilities = {}
            dialog.destroy()
        }

        function test_windowsMinimumDoesNotChangeLinuxDefault() {
            backend.hostCapabilities = { "machine-resources": { cpus: 4, memory_mib: 2048 } }
            const dialog = Qt.createComponent("qrc:/CreateDialog.qml").createObject(root)
            dialog.open()
            tryVerify(() => dialog.opened)
            dialog.step = 1
            mouseClick(findChild(dialog, "windowsGuidedChoice"))
            compare(findChild(dialog, "createMemory").value, 4096)
            verify(dialog.memoryTouched)
            mouseClick(findChild(dialog, "readyFedoraChoice"))
            compare(findChild(dialog, "createMemory").value, 2048)
            verify(!dialog.memoryTouched)
            backend.hostCapabilities = {}
            dialog.destroy()
        }

        // With quickget installed, a Desktop's system can be downloaded;
        // the ISO it produces fills the field. Without it, no button.
        function test_systemCanBeDownloadedWithQuickget() {
            const dialog = Qt.createComponent("qrc:/CreateDialog.qml").createObject(root)
            dialog.open()
            tryVerify(() => dialog.opened)
            dialog.machine = true
            dialog.readyUbuntu = false
            dialog.step = 1
            const button = findChild(dialog, "downloadSystemButton")
            verify(!button.visible)
            backend.hostCapabilities = { quickget: { available: true } }
            verify(button.visible)
            backend.downloadableImages = [
                { name: "Fedora", os: "fedora", release: "41", edition: "Workstation" },
                { name: "Ubuntu", os: "ubuntu", release: "24.04" }
            ]
            mouseClick(button)
            const download = findChild(dialog, "downloadDialog") || dialog.children.find(c => c.objectName === "downloadDialog")
            tryVerify(() => download && download.opened)
            findChild(download, "imageSearch").text = "ubuntu"
            compare(download.shown.length, 1)
            download.chosen = download.shown[0]
            backend.lastCall = []
            mouseClick(findChild(download, "downloadButton"))
            compare(backend.lastCall[0], "downloadImage")
            compare(backend.lastCall[1], "ubuntu")
            compare(backend.lastCall[2], "24.04")
            backend.finishAction("download", true, "/home/u/OmaVM/Images/ubuntu-24.04.iso")
            tryVerify(() => !download.opened)
            compare(findChild(dialog, "imageField").text, "/home/u/OmaVM/Images/ubuntu-24.04.iso")
            backend.hostCapabilities = {}
            backend.downloadableImages = []
            dialog.destroy()
        }

        function test_missingBoxToolsAreShownBeforeCreating() {
            const component = Qt.createComponent("qrc:/CreateDialog.qml")
            const dialog = component.createObject(root)
            const warning = findChild(dialog, "boxWarning")
            dialog.open()
            tryVerify(() => dialog.opened)
            dialog.machine = false
            dialog.step = 1
            verify(!warning.visible, "unknown must say nothing")
            backend.hostCapabilities = {
                distrobox: { available: false, hint: "Install Distrobox" },
                "container-engine": { available: false, hint: "Install Podman" }
            }
            verify(warning.visible)
            verify(warning.text.indexOf("Install Distrobox") >= 0)
            verify(warning.text.indexOf("Install Podman") >= 0)
            backend.hostCapabilities = {
                distrobox: { available: true },
                "container-engine": { available: true }
            }
            verify(!warning.visible)
            backend.hostCapabilities = {}
            dialog.destroy()
        }

        // Without KVM a Desktop can be created but never starts: the
        // choice says so before anything is created, with the fix.
        function test_missingKVMIsSaidBeforeCreating() {
            const component = Qt.createComponent("qrc:/CreateDialog.qml")
            const dialog = component.createObject(root)
            const warning = findChild(dialog, "kvmWarning")
            verify(!dialog.kvmMissing, "unknown must say nothing")
            backend.hostCapabilities = { kvm: { id: "kvm", available: false, hint: "Add your user to the kvm group" } }
            verify(dialog.kvmMissing)
            verify(warning.text.indexOf("Add your user to the kvm group") >= 0)
            backend.hostCapabilities = { kvm: { id: "kvm", available: true } }
            verify(!dialog.kvmMissing)
            backend.hostCapabilities = {}
            dialog.destroy()
        }

        // Windows 11 needs Secure Boot and a TPM: what's missing is said,
        // and nothing when the host has both or hasn't answered yet.
        function test_windowsNeedsUEFIAndATPM() {
            const component = Qt.createComponent("qrc:/CreateDialog.qml")
            const dialog = component.createObject(root)
            const note = findChild(dialog, "windowsNote")
            dialog.open()
            tryVerify(() => dialog.opened)
            verify(!note.visible, "unknown must say nothing")
            backend.hostCapabilities = { kvm: { available: true },
                                         uefi: { available: true },
                                         tpm: { available: false, hint: "Install swtpm" } }
            verify(note.visible)
            compare(note.text, "Windows 11 won't install yet. Install swtpm.")
            backend.hostCapabilities = { kvm: { available: true },
                                         uefi: { available: false, hint: "Install edk2-ovmf" },
                                         tpm: { available: false, hint: "Install swtpm" } }
            compare(note.text, "Windows 11 won't install yet. Install edk2-ovmf. Install swtpm.")
            backend.hostCapabilities = { kvm: { available: true }, uefi: { available: true }, tpm: { available: true } }
            verify(!note.visible)
            // Without KVM nothing starts at all; that warning is enough.
            backend.hostCapabilities = { kvm: { available: false }, tpm: { available: false, hint: "Install swtpm" } }
            verify(!note.visible)
            backend.hostCapabilities = {}
            dialog.destroy()
        }
    }

    Component { id: signalSpyComponent; SignalSpy {} }
}
