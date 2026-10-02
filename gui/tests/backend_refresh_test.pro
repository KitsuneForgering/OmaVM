QT += core gui network testlib
CONFIG += c++17 testcase
TARGET = backend_refresh_test
SOURCES += backend_refresh_test.cpp ../backend.cpp ../colorstoml.cpp ../singleinstance.cpp
HEADERS += ../backend.h ../colorstoml.h ../singleinstance.h
