import QtQuick
import QtTest

Item {
    id: root
    width: 400
    height: 600

    Loader { id: loader }

    ListView {
        id: view
        anchors.fill: parent
        model: loader.item
        delegate: Item {
            required property var environment
            width: view.width
            height: 40
        }
    }

    TestCase {
        name: "EnvironmentListModel"
        when: windowShown

        function env(name, status) {
            return { name: name, kind: "box", status: status, settings: {} }
        }

        function init() {
            loader.source = ""
            loader.source = "qrc:/EnvironmentListModel.qml"
            tryVerify(() => loader.status === Loader.Ready)
        }

        // A poll that only changes a status used to reset the whole list:
        // every card was destroyed and faded in again (the Experience
        // Center flickered), and an open card menu vanished.
        function test_statusChangeKeepsTheSameCard() {
            const model = loader.item
            model.sync([env("a", "stopped"), env("b", "stopped")])
            tryVerify(() => view.itemAtIndex(1) !== null)
            const card = view.itemAtIndex(0)
            model.sync([env("a", "running"), env("b", "stopped")])
            compare(view.itemAtIndex(0), card, "the card must be updated in place, not recreated")
            compare(card.environment.status, "running")
        }

        function test_insertRemoveAndOrder() {
            const model = loader.item
            model.sync([env("a", "stopped"), env("b", "stopped")])
            model.sync([env("c", "stopped"), env("a", "running")])
            compare(model.count, 2)
            compare(model.get(0).environment.name, "c")
            compare(model.get(1).environment.name, "a")
            compare(model.get(1).environment.status, "running")
            model.sync([])
            compare(model.count, 0)
        }
    }
}
