QT += core gui qml quick quickcontrols2 network
LIBS += -lz
CONFIG += c++17 release
TARGET = omavm-gui
TEMPLATE = app

HEADERS += gui/backend.h gui/vncclient.h gui/vncview.h gui/colorstoml.h gui/terminal.h gui/terminalview.h
SOURCES += gui/main.cpp gui/backend.cpp gui/vncclient.cpp gui/vncview.cpp gui/colorstoml.cpp gui/terminal.cpp gui/terminalview.cpp
RESOURCES += gui/resources.qrc
