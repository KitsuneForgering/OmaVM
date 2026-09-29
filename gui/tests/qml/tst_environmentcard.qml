import QtQuick
import QtTest

Item {
    id: root
    width: 700
    height: 200

    Loader { id: loader }

    TestCase {
        name: "EnvironmentCard"
        when: windowShown

        function menuFor(kind, status) {
            loader.setSource("qrc:/EnvironmentCard.qml", {
                width: 680,
                environment: { name: "demo", kind: kind, status: status || "stopped", settings: {} }
            })
            tryVerify(() => loader.status === Loader.Ready)
            const menu = findChild(loader.item, "actionsMenu")
            verify(menu)
            menu.open()
            tryVerify(() => menu.opened)
            return menu
        }

        // A hidden MenuItem still takes its full height in a Qt Quick Menu,
        // leaving blank rows (a Box's menu had four).
        function test_hiddenActionsLeaveNoGap_data() {
            return [{ tag: "box", kind: "box" }, { tag: "machine", kind: "machine" }]
        }
        function test_hiddenActionsLeaveNoGap(data) {
            const menu = menuFor(data.kind)
            let hidden = 0
            let shownHeight = 0
            for (let i = 0; i < menu.count; ++i) {
                const item = menu.itemAt(i)
                if (item.visible)
                    shownHeight += item.height
                else {
                    ++hidden
                    compare(item.height, 0, item.text + " is hidden but still takes space")
                }
            }
            verify(hidden > 0)
            compare(menu.contentItem.contentHeight, shownHeight)
            menu.close()
        }

        function menuItem(menu, text) {
            for (let i = 0; i < menu.count; ++i)
                if (menu.itemAt(i).text === text)
                    return menu.itemAt(i)
            return null
        }

        // QEMU alive but its monitor silent reports "unknown": that must not
        // read as the initial "Checking…" nor leave the Machine impossible to
        // stop from the GUI.
        function test_unresponsiveMachineCanBeForceStopped() {
            const menu = menuFor("machine", "unknown")
            compare(findChild(loader.item, "statusLabel").text, "Not responding")
            verify(menuItem(menu, "Force Stop…").enabled, "Force Stop must be available")
            verify(menuItem(menu, "Shut Down").enabled, "Shut Down must be available")
            // Its process exists, so "Start" would silently do nothing.
            verify(menuItem(menu, "Open"), "the first action must be Open, not Start")
            menu.close()
        }

        function test_keyboardActionsHaveNamesAndVisibleStatus() {
            loader.setSource("qrc:/EnvironmentCard.qml", {
                width: 680,
                environment: { name: "demo", kind: "box", status: "running", settings: {} }
            })
            tryVerify(() => loader.status === Loader.Ready)
            const primary = findChild(loader.item, "primaryAction")
            const actions = findChild(loader.item, "actionsButton")
            const status = findChild(loader.item, "statusLabel")
            verify(primary && actions && status)
            verify(primary.Accessible.name.length > 0)
            verify(actions.Accessible.name.indexOf("demo") >= 0)
            compare(status.text, "Running")
            verify(primary.width >= 44 && primary.height >= 44,
                   "primary action needs a 44×44 pointer target")
            verify(actions.width >= 44 && actions.height >= 44,
                   "actions menu needs a 44×44 pointer target")
            primary.forceActiveFocus(Qt.TabFocusReason)
            verify(primary.activeFocus)
            keyClick(Qt.Key_Tab)
            tryVerify(() => actions.activeFocus)
            keyClick(Qt.Key_Space)
            const menu = findChild(loader.item, "actionsMenu")
            tryVerify(() => menu.opened)
            menu.close()
        }
    }
}
