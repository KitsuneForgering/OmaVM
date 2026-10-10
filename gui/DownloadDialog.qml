import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts

// Systems to download for a new Desktop, through quickget (installed by
// the person; OmaVM keeps no catalog). The download lands in
// ~/OmaVM/Images and its ISO goes back to the creation wizard.
Dialog {
    id: dialog
    signal downloaded(string path)
    readonly property bool busy: !!(backend.busyEnvironments && backend.busyEnvironments["download"])
    property string query: ""
    property string initialQuery: ""
    property var chosen: null
    property string errorText: ""
    // Compared by key: each read of the list hands out new objects.
    function key(img) { return img ? img.os + "|" + img.release + "|" + (img.edition || "") : "" }
    readonly property var shown: (backend.downloadableImages || []).filter(img =>
        (img.name + " " + img.release + " " + (img.edition || "")).toLowerCase().indexOf(query.toLowerCase()) >= 0)

    title: qsTr("Download a System")
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 520)
    height: Math.min(parent.height - 64, 560)
    closePolicy: busy ? Popup.NoAutoClose : Popup.CloseOnEscape | Popup.CloseOnPressOutside
    Overlay.modal: ThemeScrim {}

    onOpened: {
        search.text = initialQuery
        query = initialQuery
        chosen = null
        errorText = ""
        if ((backend.downloadableImages || []).length === 0)
            backend.refreshImages()
        search.forceActiveFocus()
    }

    Connections {
        target: backend
        function onActionFinished(tag, ok, text) {
            if (tag !== "download" || !dialog.opened)
                return
            if (ok) {
                dialog.downloaded(text.split("\n").pop())
                dialog.close()
            } else {
                dialog.errorText = text
            }
        }
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 12
        Label {
            textFormat: Text.PlainText
            Layout.fillWidth: true
            text: qsTr("Downloaded by quickget from each system's own site, into ~/OmaVM/Images.")
            color: backend.themeMuted
            wrapMode: Text.Wrap
        }
        TextField {
            id: search
            objectName: "imageSearch"
            Layout.fillWidth: true
            enabled: !dialog.busy
            placeholderText: qsTr("Search: Fedora, Ubuntu 24.04, Windows…")
            Accessible.name: qsTr("Search systems")
            onTextChanged: dialog.query = text
        }
        BusyIndicator {
            Layout.alignment: Qt.AlignHCenter
            visible: backend.imagesLoading
            running: visible
        }
        Label {
            textFormat: Text.PlainText
            Layout.fillWidth: true
            visible: backend.imagesLoading
            text: qsTr("Asking quickget what it can download… the first time takes a while.")
            color: backend.themeMuted
            horizontalAlignment: Text.AlignHCenter
            wrapMode: Text.Wrap
        }
        Label {
            textFormat: Text.PlainText
            Layout.fillWidth: true
            visible: backend.imagesError !== "" || dialog.errorText !== ""
            text: dialog.errorText || backend.imagesError
            color: backend.themeRed
            wrapMode: Text.Wrap
        }
        ListView {
            id: list
            objectName: "imageList"
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            enabled: !dialog.busy
            model: dialog.shown
            ScrollBar.vertical: ScrollBar {}
            delegate: ItemDelegate {
                required property var modelData
                width: ListView.view.width
                checkable: true
                checked: dialog.key(dialog.chosen) === dialog.key(modelData)
                highlighted: checked
                text: modelData.name + " " + modelData.release + (modelData.edition ? " · " + modelData.edition : "")
                onClicked: dialog.chosen = modelData
                onDoubleClicked: { dialog.chosen = modelData; downloadButton.clicked() }
            }
        }
        Label {
            textFormat: Text.PlainText
            Layout.fillWidth: true
            visible: dialog.busy
            text: (backend.progress && backend.progress["download"]) || qsTr("Starting the download…")
            wrapMode: Text.Wrap
        }
        RowLayout {
            Layout.alignment: Qt.AlignRight
            Button {
                text: qsTr("Cancel")
                flat: true
                enabled: !dialog.busy
                onClicked: dialog.close()
            }
            Button {
                id: downloadButton
                objectName: "downloadButton"
                text: dialog.busy ? qsTr("Downloading…") : qsTr("Download")
                highlighted: true
                enabled: !!dialog.chosen && !dialog.busy
                onClicked: {
                    dialog.errorText = ""
                    backend.downloadImage(dialog.chosen.os, dialog.chosen.release, dialog.chosen.edition || "")
                }
            }
        }
    }
}
