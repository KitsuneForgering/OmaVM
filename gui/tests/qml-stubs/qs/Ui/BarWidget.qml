import QtQuick

Item {
    property string moduleName
    property var bar: null
    property var settings: ({})
    function setting(key, fallback) { return settings[key] !== undefined ? settings[key] : fallback }
    function broadcast(message) {}
}
