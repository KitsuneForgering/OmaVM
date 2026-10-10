import QtQuick

Item {
    objectName: "barIconButton"
    property var bar
    property int slotSize
    property string tooltipText
    property Component iconComponent
    property bool active
    property bool useActiveColor
    property color activeColor
    property color foreground
    signal pressed(var button)
    implicitWidth: 24
    implicitHeight: 24
}
