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
                launcher: call[16], ssh: call[17]
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
            verify(blue.Accessible.checkable)
            verify(blue.Accessible.checked, "blue is the selected color")
            verify(!red.Accessible.checked)
            verify(!none.Accessible.checked)
            dialog.destroy()
        }

        function test_optOutsRoundTrip() {
            const saved = saveUntouched({ clipboard_disabled: true, travel_mode_disabled: true, vulkan_disabled: true, launcher_disabled: true, ssh_disabled: true })
            compare(saved.shareClipboard, false)
            compare(saved.travelMode, false)
            compare(saved.vulkan, false)
            compare(saved.launcher, false)
            compare(saved.ssh, false)
        }
    }
}
