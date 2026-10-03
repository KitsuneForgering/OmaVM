import QtQuick
import QtTest

// Compiles the top-level QML files no other test loads, without creating
// them: CI runs Qt 6.4, development runs Arch's Qt, and a construct only
// the newer one accepts (a variable named long) must fail here, not on a
// user's machine. The dialogs and cards are compiled by their own tests.
TestCase {
    name: "Compile"

    function test_compiles_data() {
        return ["Main.qml", "Viewer.qml", "TerminalViewer.qml", "ThemeScrim.qml", "Icon.qml"]
            .map(file => ({ tag: file, file: file }))
    }

    function test_compiles(data) {
        const component = Qt.createComponent("qrc:/" + data.file)
        compare(component.status, Component.Ready, component.errorString())
    }
}
