import QtQuick
import QtTest

Item {
    id: root
    width: 800
    height: 700

    TestCase {
        name: "SnapshotsDialog"
        when: windowShown

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

        function openWith(status) {
            const component = Qt.createComponent("qrc:/SnapshotsDialog.qml")
            compare(component.status, Component.Ready, component.errorString())
            const dialog = component.createObject(root, {
                environment: { name: "desktop", kind: "machine", status: status,
                               snapshots: [{ id: "a-1", label: "Clean", created_at: "2026-09-29T10:00:00Z" }] }
            })
            dialog.open()
            tryVerify(() => dialog.opened)
            return dialog
        }

        // Regression: Go To was offered on a running Machine and failed
        // halfway (QEMU only reverts a disk-only snapshot offline).
        function test_goToNeedsAStoppedMachine() {
            let dialog = openWith("running")
            tryVerify(() => findItem(dialog.contentItem, "goToButton") !== null)
            verify(!findItem(dialog.contentItem, "goToButton").enabled)
            verify(findItem(dialog.contentItem, "runningHint").visible)
            dialog.destroy()

            dialog = openWith("stopped")
            tryVerify(() => findItem(dialog.contentItem, "goToButton") !== null)
            verify(findItem(dialog.contentItem, "goToButton").enabled)
            verify(!findItem(dialog.contentItem, "runningHint").visible)
            dialog.destroy()
        }
    }
}
