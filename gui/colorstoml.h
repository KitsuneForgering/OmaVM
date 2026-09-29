#pragma once

#include <QHash>
#include <QColor>
#include <QString>

// Parses Omarchy's flat `key = "value"` colors.toml (no sections, no
// nesting) into a lookup table. Shared by Backend::loadTheme() (the app's
// Material palette) and the embedded terminal's ANSI palette — both just
// need a handful of named colors out of the same file.
QHash<QString, QString> loadColorsToml(const QString &path);

// Reach WCAG 1.4.6's 7:1 target for normal UI text when either black or
// white can contrast with both app surfaces.
QColor readableTextColor(const QColor &preferred, const QColor &background,
                         const QColor &surface);
