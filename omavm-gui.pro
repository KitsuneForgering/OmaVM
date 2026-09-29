QT += core gui qml quick quickcontrols2 network opengl
CONFIG += link_pkgconfig
PKGCONFIG += gio-2.0 gio-unix-2.0 egl
CONFIG += c++17 release
TARGET = omavm-gui
TEMPLATE = app

HEADERS += gui/backend.h gui/displayclient.h gui/displayview.h gui/keymap.h gui/colorstoml.h gui/terminal.h gui/terminalview.h
SOURCES += gui/main.cpp gui/backend.cpp gui/displayclient.cpp gui/displayview.cpp gui/colorstoml.cpp gui/terminal.cpp gui/terminalview.cpp
RESOURCES += gui/resources.qrc
