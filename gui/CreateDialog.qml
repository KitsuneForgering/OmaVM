import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Dialogs
import QtQuick.Layouts

Dialog {
    id: dialog
    title: qsTr("New Environment")
    modal: true
    anchors.centerIn: parent
    width: Math.min(parent.width - 32, 680)
    height: Math.min(parent.height - 32, 540)
    padding: 24
    Overlay.modal: ThemeScrim {}

    property int step: 0
    property bool machine: true
    property var boxes: [
        { label: "Fedora", image: "fedora:latest" },
        { label: "Ubuntu", image: "ubuntu:latest" },
        { label: "Debian", image: "debian:latest" },
        { label: "Arch Linux", image: "archlinux:latest" },
        { label: "Alpine", image: "alpine:latest" },
        { label: qsTr("Custom image"), image: "" }
    ]

    function selectedImage() {
        return machine ? image.text.trim()
                       : (boxes[boxImage.currentIndex].image || image.text.trim())
    }

    function resetForm() {
        step = 0
        machine = true
        name.clear()
        image.clear()
        boxImage.currentIndex = 0
    }

    onOpened: resetForm()

    header: ColumnLayout {
        spacing: 8
        Label {
            Layout.leftMargin: 24
            Layout.topMargin: 20
            text: dialog.step === 0 ? qsTr("Choose an environment")
                 : dialog.step === 1 ? qsTr("Choose what to run")
                                     : qsTr("Name your environment")
            font.pixelSize: 22
            font.weight: Font.DemiBold
        }
        RowLayout {
            Layout.fillWidth: true
            Layout.leftMargin: 24
            Layout.rightMargin: 24
            spacing: 8
            Repeater {
                model: 3
                Rectangle {
                    required property int index
                    Layout.fillWidth: true
                    implicitHeight: 3
                    radius: 2
                    color: index <= dialog.step ? backend.themeAccent : backend.themeMuted
                    opacity: index <= dialog.step ? 1 : 0.35
                }
            }
        }
    }

    contentItem: StackLayout {
        currentIndex: dialog.step

        Item {
            ColumnLayout {
                anchors.fill: parent
                spacing: 14
                Label {
                    Layout.fillWidth: true
                    text: qsTr("How should this environment work?")
                    wrapMode: Text.Wrap
                    color: backend.themeMuted
                }
                Button {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 112
                    checkable: true
                    checked: dialog.machine
                    text: qsTr("Desktop\nA complete computer with its own kernel and graphical display")
                    onClicked: dialog.machine = true
                }
                Button {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 112
                    checkable: true
                    checked: !dialog.machine
                    text: qsTr("Development Box\nA fast terminal environment integrated with your Omarchy home")
                    onClicked: dialog.machine = false
                }
                Item { Layout.fillHeight: true }
            }
        }

        Item {
            ColumnLayout {
                anchors.fill: parent
                spacing: 16
                Label {
                    Layout.fillWidth: true
                    text: dialog.machine
                        ? qsTr("Install from an x86_64 ISO. OmaVM will open the graphical installer for you.")
                        : qsTr("Choose a Linux userspace. Distrobox integrates its terminal, files and graphical apps with Omarchy.")
                    wrapMode: Text.Wrap
                    color: backend.themeMuted
                }
                ComboBox {
                    id: boxImage
                    Layout.fillWidth: true
                    visible: !dialog.machine
                    model: dialog.boxes
                    textRole: "label"
                }
                RowLayout {
                    Layout.fillWidth: true
                    visible: dialog.machine || dialog.boxes[boxImage.currentIndex].image === ""
                    TextField {
                        id: image
                        Layout.fillWidth: true
                        readOnly: dialog.machine
                        placeholderText: dialog.machine ? qsTr("Choose a boot ISO…") : qsTr("Container image, for example opensuse/tumbleweed")
                    }
                    Button {
                        visible: dialog.machine
                        text: qsTr("Choose ISO…")
                        onClicked: isoPicker.open()
                    }
                }
                Rectangle {
                    Layout.fillWidth: true
                    implicitHeight: note.implicitHeight + 24
                    radius: 8
                    color: backend.themeSelection
                    Label {
                        id: note
                        anchors.fill: parent
                        anchors.margins: 12
                        text: dialog.machine
                            ? qsTr("The ISO is installation media. It will not be copied or modified.")
                            : qsTr("Boxes share the host kernel. Choose Desktop instead when you need a separate kernel or full boot.")
                        wrapMode: Text.Wrap
                    }
                }
                Item { Layout.fillHeight: true }
            }
        }

        Item {
            ColumnLayout {
                anchors.fill: parent
                spacing: 16
                Label { text: dialog.machine ? qsTr("Desktop") : qsTr("Development Box"); color: backend.themeAccent; font.weight: Font.DemiBold }
                Label {
                    Layout.fillWidth: true
                    text: dialog.machine ? image.text : dialog.boxes[boxImage.currentIndex].label
                    elide: Text.ElideMiddle
                    color: backend.themeMuted
                }
                TextField {
                    id: name
                    Layout.fillWidth: true
                    placeholderText: qsTr("Environment name")
                    focus: dialog.step === 2
                    onAccepted: createButton.clicked()
                }
                Label {
                    Layout.fillWidth: true
                    text: qsTr("You can start, open and stop it from the same Experience Center as every other environment.")
                    wrapMode: Text.Wrap
                    color: backend.themeMuted
                }
                Item { Layout.fillHeight: true }
            }
        }
    }

    footer: DialogButtonBox {
        Button {
            text: dialog.step === 0 ? qsTr("Cancel") : qsTr("Back")
            DialogButtonBox.buttonRole: DialogButtonBox.ActionRole
            onClicked: dialog.step === 0 ? dialog.reject() : dialog.step--
        }
        Button {
            id: createButton
            text: dialog.step === 2 ? qsTr("Create Environment") : qsTr("Continue")
            highlighted: true
            DialogButtonBox.buttonRole: DialogButtonBox.ActionRole
            onClicked: {
                if (dialog.step === 1 && !dialog.selectedImage()) {
                    backend.message(dialog.machine ? qsTr("Choose an installation ISO") : qsTr("Enter a container image"), true)
                    return
                }
                if (dialog.step < 2) {
                    dialog.step++
                    return
                }
                if (!name.text.trim()) {
                    backend.message(qsTr("Enter a name for the environment"), true)
                    return
                }
                backend.createEnvironment(name.text.trim(), dialog.selectedImage(), dialog.machine ? "machine" : "box")
                dialog.accept()
            }
        }
    }

    FileDialog {
        id: isoPicker
        title: qsTr("Choose a boot ISO")
        nameFilters: [qsTr("ISO images (*.iso)"), qsTr("All files (*)")]
        onAccepted: image.text = selectedFile.toString().replace(/^file:\/\//, "")
    }
}
