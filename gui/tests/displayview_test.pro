QT += core gui quick opengl testlib
CONFIG += console testcase c++17 link_pkgconfig
PKGCONFIG += gio-2.0 gio-unix-2.0 egl
TEMPLATE = app
TARGET = displayview_test
SOURCES += displayview_test.cpp ../displayview.cpp ../displayclient.cpp
HEADERS += ../displayview.h ../displayclient.h ../keymap.h
