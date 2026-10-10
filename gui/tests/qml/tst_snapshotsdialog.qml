import QtQuick
import QtTest

Item {
    id: root
    width: 800
    height: 700
    // How a label shows text when nothing in it is read as markup.
    Text { id: plainReference; textFormat: Text.PlainText; visible: false }

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

        // The title carries the environment's name, and the list each
        // snapshot's label: both shown as typed, never as HTML.
        function test_namesAndLabelsAreNeverRichText() {
            const html = "<b>desk</b> <i>top</i>"
            const component = Qt.createComponent("qrc:/SnapshotsDialog.qml")
            const dialog = component.createObject(root, {
                environment: { name: html, kind: "machine", status: "stopped",
                               snapshots: [{ id: "a-1", label: html, created_at: "2026-09-29T10:00:00Z" }] }
            })
            dialog.open()
            tryVerify(() => dialog.opened)
            verify(dialog.header, "the dialog has a header")
            tryCompare(dialog.header, "textFormat", Text.PlainText)
            plainReference.font = dialog.header.font
            plainReference.text = dialog.header.text
            verify(dialog.header.text.indexOf(html) >= 0)
            fuzzyCompare(dialog.header.contentWidth, Math.min(plainReference.contentWidth, dialog.header.width), 1,
                         "the title was rendered as rich text")
            dialog.destroy()
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

        function test_createControlsFitANarrowWindow() {
            const oldWidth = root.width
            const oldHeight = root.height
            root.width = 320
            root.height = 420
            const dialog = openWith("stopped")
            const field = findItem(dialog.contentItem, "snapshotLabelField")
            const button = findItem(dialog.contentItem, "snapshotCreateButton")
            verify(field && button)
            verify(field.mapToItem(root, field.width, 0).x <= dialog.x + dialog.width,
                   "the label field must fit")
            verify(button.mapToItem(root, button.width, 0).x <= dialog.x + dialog.width,
                   "the create action must fit")
            verify(button.mapToItem(root, 0, 0).y >= field.mapToItem(root, 0, field.height).y,
                   "the create action follows the label field")
            dialog.destroy()
            root.width = oldWidth
            root.height = oldHeight
        }
    }
}
