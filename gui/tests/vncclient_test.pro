QT += core gui network testlib
LIBS += -lz
CONFIG += console testcase c++17
TEMPLATE = app
TARGET = vncclient_test
SOURCES += vncclient_test.cpp ../vncclient.cpp
HEADERS += ../vncclient.h
