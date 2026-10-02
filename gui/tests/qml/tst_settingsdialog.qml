import QtQuick
import QtTest

Item {
    id: root
    width: 800
    height: 700

    TestCase {
        name: "SettingsDialog"
        when: windowShown

        function saveUntouched(settings) {
            backend.lastCall = []
            const component = Qt.createComponent("qrc:/SettingsDialog.qml")
            compare(component.status, Component.Ready, component.errorString())
            const dialog = component.createObject(root, {
                environment: { name: "desktop", kind: "machine", settings: settings }
            })
            dialog.open()
            tryVerify(() => dialog.opened)
            findChild(dialog, "saveButton").clicked()
            dialog.destroy()
            const call = backend.lastCall
            compare(call[0], "configure")
            return {
                cpusTouched: call[4], memoryTouched: call[6],
                shareClipboard: call[12], travelMode: call[13], vulkan: call[14],
                launcher: call[16], ssh: call[17], fullscreen: call[18],
                clipboardDirection: call[19]
            }
        }

        // Saving without touching CPU/memory must not pin them: a pinned
        // CPU count turns Travel Mode's battery reduction off for good.
        function test_saveDoesNotPinHardware() {
            const saved = saveUntouched({ description: "x" })
            compare(saved.cpusTouched, false)
            compare(saved.memoryTouched, false)
        }

        // Opt-out settings: absent from the JSON means on.
        function test_optOutDefaultsAreOn() {
            const saved = saveUntouched({})
            compare(saved.shareClipboard, true)
            compare(saved.travelMode, true)
            compare(saved.vulkan, true)
            compare(saved.launcher, true)
            compare(saved.ssh, true)
            compare(saved.fullscreen, true)
        }

        // Repeater delegates live in the visual tree, not among QObject
        // children, so findChild() can't see them.
        function findItem(item, name) {
            if (!item)
                return null
            if (item.objectName === name)
                return item
            const kids = [].concat(item.children ? Array.from(item.children) : [],
                                   item.contentItem ? [item.contentItem] : [])
            for (const kid of kids) {
                const found = findItem(kid, name)
                if (found)
                    return found
            }
            return null
        }

        // The selected color is only a thicker border: a screen reader has
        // to be told which swatch is the current one.
        function test_selectedColorIsAnnouncedToScreenReaders() {
            const component = Qt.createComponent("qrc:/SettingsDialog.qml")
            const dialog = component.createObject(root, {
                environment: { name: "desktop", kind: "machine", settings: { color: "blue" } }
            })
            dialog.open()
            tryVerify(() => dialog.opened)
            const blue = findItem(dialog.contentItem, "colorSwatch-blue")
            const red = findItem(dialog.contentItem, "colorSwatch-red")
            const none = findItem(dialog.contentItem, "colorSwatch-none")
            verify(none, "none swatch found")
            verify(blue, "blue swatch found")
            verify(red, "red swatch found")
            for (const swatch of [none, blue, red])
                verify(swatch.width >= 44 && swatch.height >= 44,
                       swatch.objectName + " needs a 44×44 pointer target")
            // As a screen reader gets it, not the QML attached property:
            // with accessibility active Qt announces the control's own
            // state, which older Qt (6.4) and newer disagree on otherwise.
            const state = swatch => backend.accessibleState(swatch)
            verify(state(blue).checkable)
            verify(state(blue).checked, "blue is the selected color")
            verify(!state(red).checked)
            verify(!state(none).checked)
            compare(state(blue).name, "blue")
            dialog.destroy()
        }

        function test_optOutsRoundTrip() {
            const saved = saveUntouched({ clipboard_disabled: true, travel_mode_disabled: true, vulkan_disabled: true, launcher_disabled: true, ssh_disabled: true, fullscreen_disabled: true })
            compare(saved.shareClipboard, false)
            compare(saved.travelMode, false)
            compare(saved.vulkan, false)
            compare(saved.launcher, false)
            compare(saved.ssh, false)
            compare(saved.fullscreen, false)
        }

        // A one-way clipboard survives reopening Settings and saving.
        function test_clipboardDirectionRoundTrips() {
            compare(saveUntouched({ clipboard_direction: "to-host" }).clipboardDirection, "to-host")
            compare(saveUntouched({ clipboard_direction: "to-guest" }).clipboardDirection, "to-guest")
            compare(saveUntouched({}).clipboardDirection, "both")
        }

        // Each guest integration says its state and next step in text.
        function test_guestCapabilitiesAreSpelledOut() {
            const component = Qt.createComponent("qrc:/SettingsDialog.qml")
            const d = component.createObject(root, { environment: { name: "vm", kind: "machine", settings: {},
                guestCapabilities: [
                    { id: "clipboard", label: "Clipboard", state: "needs_guest_component", hint: "Install spice-vdagent in the guest and sign in to its desktop" },
                    { id: "shared_folder", label: "Shared folder", state: "ready", hint: "/home/me is at /mnt/omavm-share in the guest" }
                ] } })
            d.open()
            tryVerify(() => d.opened)
            const clipboard = findChild(d.contentItem, "guestCapability-clipboard")
            verify(clipboard, "clipboard row")
            verify(findChild(d.contentItem, "guestCapability-shared_folder"), "shared folder row")
            d.destroy()
        }

        // Preparing the guest is offered only for a running Machine, and
        // each step comes back in words, a manual one with its command.
        function test_prepareTheGuest() {
            const component = Qt.createComponent("qrc:/SettingsDialog.qml")
            const stopped = component.createObject(root, { environment: { name: "vm", kind: "machine", status: "stopped", settings: {} } })
            stopped.open()
            tryVerify(() => stopped.opened)
            verify(!findChild(stopped.contentItem, "prepareButton").visible, "nothing to prepare while it is stopped")
            stopped.destroy()
            const box = component.createObject(root, { environment: { name: "dev", kind: "box", status: "running", settings: {} } })
            box.open()
            tryVerify(() => box.opened)
            verify(!findChild(box.contentItem, "prepareButton").visible, "a Box has nothing to prepare")
            box.destroy()

            backend.lastCall = []
            const d = component.createObject(root, { environment: { name: "vm", kind: "machine", status: "running", settings: {} } })
            d.open()
            tryVerify(() => d.opened)
            const button = findChild(d.contentItem, "prepareButton")
            verify(button.visible && button.enabled)
            button.clicked()
            compare(backend.lastCall[0], "prepareGuest")
            compare(backend.lastCall[1], "vm")
            verify(!button.enabled, "one preparation at a time")
            backend.finishAction("prepare", true, JSON.stringify([
                { id: "clipboard", label: "Clipboard", result: "manual", detail: "In the guest, run: sudo dnf install spice-vdagent" },
                { id: "shared_folder", label: "Shared folder", result: "done", detail: "/home/me is at /mnt/omavm-share in the guest" }
            ]))
            verify(button.enabled)
            tryVerify(() => findItem(d.contentItem, "prepareStep-clipboard") !== null)
            verify(findItem(d.contentItem, "prepareStep-shared_folder"))

            button.clicked()
            backend.finishAction("prepare", false, "vm has no guest agent answering. Install and start qemu-guest-agent")
            compare(findChild(d.contentItem, "prepareError").text, "vm has no guest agent answering. Install and start qemu-guest-agent")
            verify(findItem(d.contentItem, "prepareStep-clipboard") === null, "old steps are cleared")
            d.destroy()
        }
    }
}
