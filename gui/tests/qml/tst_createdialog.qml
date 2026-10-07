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

        // The name is suggested from the ISO, without architecture and
        // language noise, unique among existing environments; a Windows 11
        // ISO starts with the 4 GB its installer requires.
        function test_nameAndMemoryAreSuggestedFromTheISO() {
            backend.environments = [{ name: "Win11 24H2" }]
            const dialog = Qt.createComponent("qrc:/CreateDialog.qml").createObject(root)
            dialog.open()
            tryVerify(() => dialog.opened)
            dialog.machine = true
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
