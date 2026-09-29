import QtQuick
import QtTest

Item {
    id: root
    width: 800
    height: 700

    TestCase {
        name: "CreateDialog"

        // The field repeats the Core's name rules (internal/core's
        // validateEnvironmentName) so a bad name is caught before the CLI.
        function test_nameRulesMatchTheCore() {
            const component = Qt.createComponent("qrc:/CreateDialog.qml")
            compare(component.status, Component.Ready, component.errorString())
            const dialog = component.createObject(root)
            verify(dialog.validateName("") !== "")
            verify(dialog.validateName("..") !== "")
            verify(dialog.validateName("a/b") !== "")
            // Regression: a leading dash made the name read as an option.
            verify(dialog.validateName("--json") !== "")
            verify(dialog.validateName("-x") !== "")
            verify(dialog.validateName("a\u0085b") !== "", "C1 control")
            compare(dialog.validateName("a-b"), "")
            compare(dialog.validateName("Fedora de Teste"), "")
            compare(dialog.validateName("Ação"), "")
            dialog.destroy()
        }
    }
}
