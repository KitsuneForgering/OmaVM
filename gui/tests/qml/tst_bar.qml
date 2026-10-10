import QtQuick
import QtTest

// The Quickshell bar plugin (contrib/dev.omavm.bar), the real file, on
// stand-in shell modules (gui/tests/qml-stubs): what it shows for each
// thing `omavm list --json` can answer.
Item {
    id: root
    width: 200
    height: 40

    Loader { id: loader }

    TestCase {
        name: "BarPlugin"
        when: windowShown

        function findByProperty(item, property) {
            if (!item)
                return null
            if (item[property] !== undefined && item.objectName !== "barIconButton")
                return item
            for (const kid of (item.data ? Array.from(item.data) : [])) {
                const found = findByProperty(kid, property)
                if (found)
                    return found
            }
            return null
        }

        function load() {
            loader.source = ""
            loader.source = barPluginUrl
            tryCompare(loader, "status", Loader.Ready)
            const proc = findByProperty(loader.item, "command")
            verify(proc, "the plugin's Process")
            tryVerify(() => proc.running, 2000, "the plugin lists environments on start")
            return proc
        }

        function answer(proc, output) {
            proc.finish(output, "")
        }

        function test_listsWithTheCLIsJSON() {
            const proc = load()
            compare(proc.command.slice(-2), ["list", "--json"])
            verify(String(proc.command[0]).endsWith("omavm"))
        }

        function test_countAndTooltip() {
            const proc = load()
            answer(proc, '[{"id":"a","name":"x"},{"id":"b","name":"y"}]\n')
            compare(loader.item.envCount, 2)
            verify(loader.item.visible)
            const button = findChild(loader.item, "barIconButton")
            compare(button.tooltipText, "OmaVM — 2 environments, click to open")
        }

        // No environments: the entry hides, like the shell's own widgets.
        function test_hidesAtZero() {
            const proc = load()
            answer(proc, "[]\n")
            compare(loader.item.envCount, 0)
            verify(!loader.item.visible)
        }

        // omavm missing or too old: no output, nothing to show.
        function test_noOutputHides() {
            const proc = load()
            answer(proc, "")
            compare(loader.item.envCount, 0)
            verify(!loader.item.visible)
        }

        // Regression: an error ({"error": ...}, a damaged registry) read
        // as "no environments" and the entry vanished. The environments
        // are still there: the last count stays.
        function test_errorKeepsTheLastCount() {
            const proc = load()
            answer(proc, '[{"id":"a"},{"id":"b"},{"id":"c"}]')
            compare(loader.item.envCount, 3)
            loader.item.refresh()
            tryVerify(() => proc.running)
            ignoreWarning(/omavm-bar.*registry is damaged/)
            answer(proc, '{"error":{"code":"failed","message":"the environment registry is damaged"}}\n')
            compare(loader.item.envCount, 3)
            verify(loader.item.visible)
        }

        // Anything else on stdout never breaks it.
        function test_garbageNeverThrows() {
            failOnWarning(/TypeError|ReferenceError/)
            for (const output of ["not json", "{", "null", "42", '"text"', "{}", "[1,2]"]) {
                const proc = load()
                answer(proc, output)
                // Shown only for a real list; otherwise as before.
                verify(output === "[1,2]" ? loader.item.envCount === 2 : loader.item.envCount <= 0, output)
            }
        }
    }
}
