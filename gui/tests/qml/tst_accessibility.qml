import QtQuick
import QtTest

// Every control a person can act on has a name a screen reader reads and
// can be reached with the keyboard (WCAG 4.1.2 and 2.1.1), checked on the
// accessibility tree Qt gives assistive technologies.
Item {
    id: root
    width: 900
    height: 800

    TestCase {
        name: "Accessibility"
        when: windowShown

        function audit(item) {
            const problems = backend.accessibilityProblems(item)
            compare(problems.join("\n"), "", "accessibility problems")
        }

        function test_cards_data() {
            return [
                { tag: "stopped box", kind: "box", status: "stopped" },
                { tag: "running machine", kind: "machine", status: "running" },
                { tag: "paused machine", kind: "machine", status: "paused" },
                { tag: "failed machine", kind: "machine", status: "error" },
                { tag: "narrow card", kind: "machine", status: "running", width: 320 }
            ]
        }
        function test_cards(data) {
            const component = Qt.createComponent("qrc:/EnvironmentCard.qml")
            compare(component.status, Component.Ready, component.errorString())
            const card = component.createObject(root, {
                width: data.width || 680,
                environment: { name: "demo", kind: data.kind, status: data.status, settings: {} }
            })
            audit(card)
            const menu = findChild(card, "actionsMenu")
            menu.open()
            tryVerify(() => menu.opened)
            audit(menu.contentItem)
            menu.close()
            card.destroy()
        }

        function dialog(url, properties) {
            const component = Qt.createComponent(url)
            compare(component.status, Component.Ready, component.errorString())
            const d = component.createObject(root, properties || {})
            d.open()
            tryVerify(() => d.opened)
            return d
        }

        function test_settings_data() {
            return [{ tag: "machine", kind: "machine" }, { tag: "box", kind: "box" }]
        }
        function test_settings(data) {
            const d = dialog("qrc:/SettingsDialog.qml", { environment: { name: "demo", kind: data.kind, settings: {} } })
            audit(d.contentItem.parent)
            d.destroy()
        }

        function test_create_data() {
            return [
                { tag: "kind", step: 0, machine: true },
                { tag: "iso", step: 1, machine: true },
                { tag: "image", step: 1, machine: false },
                { tag: "name", step: 2, machine: true },
                { tag: "summary", step: 3, machine: true }
            ]
        }
        function test_create(data) {
            const d = dialog("qrc:/CreateDialog.qml")
            d.machine = data.machine
            d.step = data.step
            wait(300) // the step transition
            audit(d.contentItem.parent)
            d.destroy()
        }

        function test_apps() {
            backend.appsEnvironment = "dev"
            backend.appsError = ""
            backend.appsLoading = false
            backend.apps = [{ id: "/usr/share/applications/a.desktop", name: "Gimp", exported: true },
                            { id: "/usr/share/applications/b.desktop", name: "Inkscape", exported: false }]
            const d = dialog("qrc:/AppsDialog.qml", { environment: { name: "dev", kind: "box", settings: {} } })
            audit(d.contentItem.parent)
            d.destroy()
            backend.apps = []
        }

        function test_snapshots() {
            const d = dialog("qrc:/SnapshotsDialog.qml", {
                environment: { name: "demo", kind: "machine", status: "stopped", settings: {},
                               snapshots: [{ id: "a-1", label: "Clean installation", created_at: "2026-09-29T12:00:00Z" }] }
            })
            audit(d.contentItem.parent)
            d.destroy()
        }
    }
}
