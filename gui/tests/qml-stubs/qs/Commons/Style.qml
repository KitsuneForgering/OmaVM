pragma Singleton
import QtQuick

QtObject {
    readonly property QtObject bar: QtObject { property int statusSlot: 24 }
}
