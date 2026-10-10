QT += core gui qml quick quickcontrols2 qmltest
CONFIG += console testcase c++17
TEMPLATE = app
TARGET = qml_test
SOURCES += qml_test.cpp
RESOURCES += ../resources.qrc
DEFINES += QUICK_TEST_SOURCE_DIR=\\\"$$PWD/qml\\\"
DEFINES += QML_STUBS_DIR=\\\"$$PWD/qml-stubs\\\" BAR_PLUGIN=\\\"$$PWD/../../contrib/dev.omavm.bar/OmaVM.qml\\\"
