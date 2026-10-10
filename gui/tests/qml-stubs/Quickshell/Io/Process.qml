import QtQuick

QtObject {
    property var command: []
    property bool running: false
    property QtObject stdout
    property QtObject stderr
    // Each time the plugin sets running, the test answers through finish().
    property int starts: 0
    onRunningChanged: if (running) starts++
    function finish(out, err) {
        if (stdout) { stdout.text = out; stdout.streamFinished() }
        if (stderr) { stderr.text = err || ""; stderr.streamFinished() }
        running = false
    }
}
