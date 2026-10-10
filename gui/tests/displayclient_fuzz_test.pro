QT += core gui testlib
CONFIG += console testcase c++17 link_pkgconfig
PKGCONFIG += gio-2.0 gio-unix-2.0
TEMPLATE = app
TARGET = displayclient_fuzz_test
SOURCES += displayclient_fuzz_test.cpp ../displayclient.cpp
HEADERS += ../displayclient.h
