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

        // A Machine paused because the host's disk filled up says so, and
        // offers Resume; a running one doesn't show its routine detail.
        function test_pausedMachineShowsWhy() {
            const why = "paused: this computer's disk is full (12 MiB free); free up space, then Resume"
            loader.setSource("qrc:/EnvironmentCard.qml", {
                width: 680,
                environment: { name: "vm", kind: "machine", status: "paused", statusDetail: why, settings: {} }
            })
            tryVerify(() => loader.status === Loader.Ready)
            const detail = findChild(loader.item, "statusDetailLabel")
            verify(detail.visible)
            compare(detail.text, why)
            loader.setSource("qrc:/EnvironmentCard.qml", {
                width: 680,
                environment: { name: "vm", kind: "machine", status: "running", statusDetail: "display available", settings: {} }
            })
            tryVerify(() => loader.status === Loader.Ready)
            verify(!findChild(loader.item, "statusDetailLabel").visible)
        }

        // A nearly full host disk is said before the Machine pauses, on a
        // stopped card too (before Start).
        function test_lowSpaceWarning() {
            const why = "only 700 MiB free on this computer: the Machine pauses if it runs out"
            loader.setSource("qrc:/EnvironmentCard.qml", {
                width: 320,
                environment: { name: "vm", kind: "machine", status: "stopped", statusWarning: why, settings: {} }
            })
            tryVerify(() => loader.status === Loader.Ready)
            const label = findChild(loader.item, "statusWarningLabel")
            verify(label.visible)
            compare(label.text, why)
        }

        // A running session says when it differs from its settings.
        function test_sessionAdjustments() {
            loader.setSource("qrc:/EnvironmentCard.qml", {
                width: 320,
                environment: { name: "vm", kind: "machine", status: "running", restartNeeded: true, travelMode: true, settings: {} }
            })
            tryVerify(() => loader.status === Loader.Ready)
            verify(findChild(loader.item, "restartNeededLabel").visible)
            verify(findChild(loader.item, "travelModeLabel").visible)
            loader.setSource("qrc:/EnvironmentCard.qml", {
                width: 320,
                environment: { name: "vm", kind: "machine", status: "stopped", restartNeeded: true, settings: {} }
            })
            tryVerify(() => loader.status === Loader.Ready)
            verify(!findChild(loader.item, "restartNeededLabel").visible, "a stopped Machine has nothing to restart")
        }

        // The card says which system runs inside, in plain words.
        function test_systemName_data() {
            return [
                { tag: "preset", kind: "box", image: "fedora:latest", want: "Fedora" },
                { tag: "toolbox with tag", kind: "box", image: "registry.fedoraproject.org/fedora-toolbox:42", want: "Fedora 42" },
                { tag: "library", kind: "box", image: "docker.io/library/archlinux:latest", want: "Arch Linux" },
                { tag: "registry with port", kind: "box", image: "localhost:5000/team/devbox", want: "devbox" },
                { tag: "digest", kind: "box", image: "quay.io/toolbx/ubuntu-toolbox@sha256:abc", want: "Ubuntu" },
                { tag: "iso", kind: "machine", image: "/home/me/ISOs/Fedora-Workstation-Live-42.iso", want: "Fedora-Workstation-Live-42" },
                { tag: "none", kind: "machine", image: "", want: "" }
            ]
        }
        function test_systemName(data) {
            loader.setSource("qrc:/EnvironmentCard.qml", {
                width: 680,
                environment: { name: "x", kind: data.kind, image: data.image, status: "stopped", settings: {} }
            })
            tryVerify(() => loader.status === Loader.Ready)
            compare(loader.item.systemName(loader.item.environment), data.want)
        }

        // Only a Box offers to update its system from here.
        function test_updateIsOfferedForBoxes() {
            let menu = menuFor("box", "stopped")
            verify(findChild(loader.item, "updateItem").visible)
            menu.close()
            menu = menuFor("machine", "stopped")
            verify(!findChild(loader.item, "updateItem").visible)
            menu.close()
        }

        // A long name at the narrowest width is cut inside the card, never
        // pushing the open action out of it (docs/TODO.md P2).
        function test_longNameStaysInsideANarrowCard() {
            const long = "Fedora de testes do projeto de compiladores com um nome bem comprido"
            loader.setSource("qrc:/EnvironmentCard.qml", {
                width: 320,
                environment: { name: long, kind: "box", status: "stopped", settings: { color: "green" } }
            })
            tryVerify(() => loader.status === Loader.Ready)
            const card = loader.item
            const name = findChild(card, "nameLabel")
            const primary = findChild(card, "primaryAction")
            tryVerify(() => name.width > 0)
            verify(name.truncated, "the name should be cut at 320 px")
            const n = name.mapToItem(card, 0, 0)
            const p = primary.mapToItem(card, 0, 0)
            verify(n.x + name.width <= card.width + 0.5, "name past the card's edge")
            verify(primary.visible && p.x + primary.width <= card.width + 0.5, "open action past the card's edge")
        }

        // A session that won't keep its changes says so while it runs, and
        // only a stopped Desktop offers to start one.
        function test_ephemeralSession() {
            loader.setSource("qrc:/EnvironmentCard.qml", {
                width: 320,
                environment: { name: "vm", kind: "machine", status: "running", ephemeral: true, settings: {} }
            })
            tryVerify(() => loader.status === Loader.Ready)
            verify(findChild(loader.item, "ephemeralLabel").visible)
            verify(!findChild(loader.item, "startEphemeralItem").visible)

            let menu = menuFor("machine", "stopped")
            verify(findChild(loader.item, "startEphemeralItem").visible)
            verify(!findChild(loader.item, "ephemeralLabel").visible)
            menu.close()
            menu = menuFor("box", "stopped")
            verify(!findChild(loader.item, "startEphemeralItem").visible)
            menu.close()
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
        // The label says what the click shows (docs/TODO.md P1), and the
        // button, its hint and the menu agree.
        function test_primaryActionNamesTheResult_data() {
            return [
                { tag: "stopped box", kind: "box", status: "stopped", label: "Open", hint: "Start demo and open its terminal" },
                { tag: "stopped machine", kind: "machine", status: "stopped", label: "Start", hint: "Start demo and show its screen" },
                { tag: "running box", kind: "box", status: "running", label: "Open", hint: "Open a terminal in demo" },
                { tag: "running machine", kind: "machine", status: "running", label: "Open", hint: "Show the screen of demo" },
                { tag: "paused machine", kind: "machine", status: "paused", label: "Resume", hint: "Continue demo where it was paused" }
            ]
        }
        function test_primaryActionNamesTheResult(data) {
            const menu = menuFor(data.kind, data.status)
            const primary = findChild(loader.item, "primaryAction")
            compare(primary.text, data.label)
            compare(primary.Accessible.name, data.label)
            compare(primary.Accessible.description, data.hint)
            verify(menuItem(menu, data.label), "the menu's first action must match the button")
            menu.close()
        }

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
