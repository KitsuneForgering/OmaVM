import QtQuick
import QtQuick.Controls

// Memory for a Machine. The value stays in MiB, what the CLI takes;
// people read and type gigabytes, or megabytes with an M ("768 MB").
SpinBox {
    from: 256
    to: 262144
    stepSize: 512
    editable: true
    Accessible.name: qsTr("Memory")
    textFromValue: (v, locale) => v >= 1024 && v % 512 === 0 || v >= 10240
        ? qsTr("%1 GB").arg(Number(v / 1024).toLocaleString(locale, "f", v % 1024 === 0 ? 0 : 1))
        : qsTr("%1 MB").arg(v)
    valueFromText: (text, locale) => {
        const n = parseFloat(text.replace(",", ".")) || 0
        return Math.round(/m/i.test(text) ? n : n * 1024)
    }
    validator: RegularExpressionValidator { regularExpression: /\s*[0-9]+([.,][0-9]+)?\s*([GgMm][Bb]?)?\s*/ }
    // Typing goes into the inner text field, which is what a screen
    // reader announces.
    Component.onCompleted: contentItem.Accessible.name = Accessible.name
}
