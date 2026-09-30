#include "colorstoml.h"

#include <QFile>
#include <QTextStream>

#include <algorithm>
#include <cmath>

namespace {
double luminance(const QColor &color) {
  const auto channel = [](int value) {
    const double s = value / 255.0;
    return s <= 0.04045 ? s / 12.92 : std::pow((s + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * channel(color.red()) +
         0.7152 * channel(color.green()) +
         0.0722 * channel(color.blue());
}

double contrast(const QColor &a, const QColor &b) {
  const double x = luminance(a), y = luminance(b);
  return (std::max(x, y) + 0.05) / (std::min(x, y) + 0.05);
}

double minimumContrast(const QColor &text, const QColor &background,
                       const QColor &surface) {
  return std::min(contrast(text, background), contrast(text, surface));
}
} // namespace

QColor readableTextColor(const QColor &preferred, const QColor &background,
                         const QColor &surface) {
  if (preferred.isValid() &&
      minimumContrast(preferred, background, surface) >= 7.0)
    return preferred;
  const QColor black(Qt::black), white(Qt::white);
  return minimumContrast(black, background, surface) >=
                 minimumContrast(white, background, surface)
             ? black
             : white;
}

double minimumContrastRatio(const QColor &text, const QColor &background,
                            const QColor &surface) {
  return minimumContrast(text, background, surface);
}

QColor accessibleColor(const QColor &preferred, const QColor &background,
                       const QColor &surface, double ratio) {
  if (!preferred.isValid())
    return readableTextColor(preferred, background, surface);
  if (minimumContrast(preferred, background, surface) >= ratio)
    return preferred;
  // Toward whichever extreme contrasts more with both surfaces. Mixing in
  // more of it only increases the contrast, so the smallest mix that is
  // enough can be found by bisection.
  const QColor black(Qt::black), white(Qt::white);
  const QColor target = minimumContrast(white, background, surface) >=
                                minimumContrast(black, background, surface)
                            ? white
                            : black;
  const auto mix = [&](double t) {
    return QColor::fromRgbF(
        float(preferred.redF() + (target.redF() - preferred.redF()) * t),
        float(preferred.greenF() + (target.greenF() - preferred.greenF()) * t),
        float(preferred.blueF() + (target.blueF() - preferred.blueF()) * t));
  };
  if (minimumContrast(target, background, surface) < ratio)
    return target; // the best there is
  double low = 0, high = 1;
  for (int i = 0; i < 24; ++i) {
    const double middle = (low + high) / 2;
    (minimumContrast(mix(middle), background, surface) >= ratio ? high : low) =
        middle;
  }
  // Rounding to 8-bit channels can land just under the ratio.
  QColor result = mix(high);
  for (double t = high; minimumContrast(result, background, surface) < ratio &&
                        t < 1;
       t = std::min(1.0, t + 0.01))
    result = mix(t);
  return result;
}

QHash<QString, QString> loadColorsToml(const QString &path) {
  QHash<QString, QString> values;
  QFile file(path);
  if (!file.open(QIODevice::ReadOnly | QIODevice::Text))
    return values;

  QTextStream in(&file);
  while (!in.atEnd()) {
    const QString line = in.readLine().trimmed();
    const qsizetype equals = line.indexOf(QLatin1Char('='));
    if (line.isEmpty() || line.startsWith(QLatin1Char('#')) || equals < 0)
      continue;
    const QString key = line.left(equals).trimmed();
    QString value = line.mid(equals + 1).trimmed();
    if (value.size() >= 2 && (value.front() == QLatin1Char('"') ||
                              value.front() == QLatin1Char('\'')))
      value = value.mid(1, value.size() - 2);
    values.insert(key, value);
  }
  return values;
}
