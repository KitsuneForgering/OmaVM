#include "colorstoml.h"

#include <QFile>
#include <QTextStream>

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
