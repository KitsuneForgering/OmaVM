import QtQuick
import QtTest

Item {
    id: root
    width: 800
    height: 700

    TestCase {
        name: "CloneDialog"
        when: windowShown

        function open(env) {
            const component = Qt.createComponent("qrc:/CloneDialog.qml")
            compare(component.status, Component.Ready, component.errorString())
            const d = component.createObject(root, { environment: env })
            d.open()
            tryVerify(() => d.opened)
            return d
        }

        // The cost is said before cloning, and a running environment is
        // asked to stop first instead of failing after the click.
        function test_costAndStopFirst() {
            let d = open({ name: "vm", kind: "machine", status: "running" })
            verify(findChild(d, "cloneCost").text.indexOf("free space") >= 0)
            verify(findChild(d, "stopFirst").visible)
            verify(!findChild(d, "cloneButton").enabled)
            d.destroy()
            d = open({ name: "dev", kind: "box", status: "stopped" })
            verify(findChild(d, "cloneCost").text.indexOf("home folder is shared") >= 0)
            verify(!findChild(d, "stopFirst").visible)
            compare(findChild(d, "cloneName").text, "dev copy")
            findChild(d, "cloneButton").clicked()
            compare(backend.lastCall[0], "cloneEnvironment")
            compare(backend.lastCall[2], "dev copy")
            backend.finishAction("clone", false, "name taken")
            verify(d.opened, "a failure keeps the dialog open")
            compare(d.errorText, "name taken")
            d.destroy()
        }
    }
}
