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

// Like readableTextColor, for colors that carry meaning (a red error, a
// green "Running", the accent): keeps the hue and moves it toward white
// or black only as far as needed for `ratio` against both surfaces,
// instead of replacing it with plain black or white.
QColor accessibleColor(const QColor &preferred, const QColor &background,
                       const QColor &surface, double ratio = 7.0);

// WCAG contrast ratio of the lower of text-on-background and
// text-on-surface. Exposed for tests.
double minimumContrastRatio(const QColor &text, const QColor &background,
                            const QColor &surface);
