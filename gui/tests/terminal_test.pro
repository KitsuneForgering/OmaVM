QT += core gui testlib
CONFIG += console testcase c++17
TEMPLATE = app
TARGET = terminal_test
SOURCES += terminal_test.cpp ../terminal.cpp
HEADERS += ../terminal.h
