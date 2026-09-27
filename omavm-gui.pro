QT += core gui qml quick quickcontrols2 network
CONFIG += c++17 release
TARGET = omavm-gui
TEMPLATE = app

HEADERS += gui/backend.h gui/vncclient.h gui/vncview.h
SOURCES += gui/main.cpp gui/backend.cpp gui/vncclient.cpp gui/vncview.cpp
RESOURCES += gui/resources.qrc
