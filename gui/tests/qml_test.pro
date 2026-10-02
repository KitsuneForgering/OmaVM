QT += core gui qml quick quickcontrols2 qmltest opengl
CONFIG += console testcase c++17 link_pkgconfig
# The C++ types the viewers import (OmaVM 1.0), so tst_compile.qml checks
# every property they use.
PKGCONFIG += gio-2.0 gio-unix-2.0 egl
TEMPLATE = app
TARGET = qml_test
SOURCES += qml_test.cpp ../displayview.cpp ../displayclient.cpp ../terminal.cpp ../terminalview.cpp ../colorstoml.cpp
HEADERS += ../displayview.h ../displayclient.h ../keymap.h ../terminal.h ../terminalview.h ../colorstoml.h
RESOURCES += ../resources.qrc
DEFINES += QUICK_TEST_SOURCE_DIR=\\\"$$PWD/qml\\\"
