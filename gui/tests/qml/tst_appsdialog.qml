import QtQuick
import QtTest

Item {
    id: root
    width: 800
    height: 700

    TestCase {
        name: "AppsDialog"
        when: windowShown

        // The row being exported says so, the result is said in words, and
        // a failure keeps the row with the same action to try again.
        function test_exportShowsProgressAndResult() {
            backend.appsEnvironment = "dev"
            backend.appsError = ""
            backend.apps = [{ id: "/usr/share/applications/gimp.desktop", name: "Gimp", exported: false }]
            const component = Qt.createComponent("qrc:/AppsDialog.qml")
            compare(component.status, Component.Ready, component.errorString())
            const dialog = component.createObject(root, { environment: { name: "dev", kind: "box" } })
            dialog.open()
            tryVerify(() => dialog.opened)

            dialog.act(backend.apps[0])
            compare(backend.lastCall[0], "exportApp")
            compare(backend.lastCall[2], "/usr/share/applications/gimp.desktop")
            const row = findChild(dialog.contentItem, "appRow-Gimp")
            verify(row.pending)
            verify(!findChild(row, "appAction").visible, "no second click while it runs")

            backend.finishAction("apps-export", false, "distrobox-export failed")
            verify(!row.pending)
            compare(dialog.resultText, "distrobox-export failed")
            verify(dialog.resultFailed)
            verify(findChild(row, "appAction").visible, "the row offers the action again")

            dialog.act(backend.apps[0])
            backend.finishAction("apps-export", true, "")
            compare(dialog.resultText, "“Gimp” is in the app menu.")

            backend.apps = [{ id: "/usr/share/applications/gimp.desktop", name: "Gimp", exported: true }]
            dialog.act(backend.apps[0])
            compare(backend.lastCall[0], "unexportApp")
            backend.finishAction("apps-unexport", true, "")
            compare(dialog.resultText, "“Gimp” was removed from the app menu; it is still installed in dev.")
            dialog.destroy()
            backend.apps = []
        }
    }
}
