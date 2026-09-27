#pragma once

#include <QHash>
#include <QString>

// Parses Omarchy's flat `key = "value"` colors.toml (no sections, no
// nesting) into a lookup table. Shared by Backend::loadTheme() (the app's
// Material palette) and the embedded terminal's ANSI palette — both just
// need a handful of named colors out of the same file.
QHash<QString, QString> loadColorsToml(const QString &path);
