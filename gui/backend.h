#pragma once

#include <QFileSystemWatcher>
#include <QObject>
#include <QProcess>
#include <QVariantList>

class Backend final : public QObject {
  Q_OBJECT
  Q_PROPERTY(
      QVariantList environments READ environments NOTIFY environmentsChanged)
  Q_PROPERTY(bool busy READ busy NOTIFY busyChanged)
  Q_PROPERTY(QString themeMode READ themeMode NOTIFY themeChanged)
  Q_PROPERTY(QString themeBackground READ themeBackground NOTIFY themeChanged)
  Q_PROPERTY(QString themeForeground READ themeForeground NOTIFY themeChanged)
  Q_PROPERTY(QString themeAccent READ themeAccent NOTIFY themeChanged)
  Q_PROPERTY(QString themeSelection READ themeSelection NOTIFY themeChanged)
  Q_PROPERTY(QString themeMuted READ themeMuted NOTIFY themeChanged)
  Q_PROPERTY(QString themeSurface READ themeSurface NOTIFY themeChanged)
  Q_PROPERTY(QString themeDarkSurface READ themeDarkSurface NOTIFY themeChanged)
  Q_PROPERTY(QString themeGreen READ themeGreen NOTIFY themeChanged)
  Q_PROPERTY(QString themeRed READ themeRed NOTIFY themeChanged)

public:
  explicit Backend(QObject *parent = nullptr);

  QVariantList environments() const { return m_environments; }
  bool busy() const { return m_busy; }
  QString themeMode() const { return m_themeMode; }
  QString themeBackground() const { return m_themeBackground; }
  QString themeForeground() const { return m_themeForeground; }
  QString themeAccent() const { return m_themeAccent; }
  QString themeSelection() const { return m_themeSelection; }
  QString themeMuted() const { return m_themeMuted; }
  QString themeSurface() const { return m_themeSurface; }
  QString themeDarkSurface() const { return m_themeDarkSurface; }
  QString themeGreen() const { return m_themeGreen; }
  QString themeRed() const { return m_themeRed; }

  Q_INVOKABLE void refresh();
  Q_INVOKABLE void createEnvironment(const QString &name, const QString &image,
                                     const QString &kind);
  Q_INVOKABLE void start(const QString &name);
  Q_INVOKABLE void open(const QString &name, const QString &kind);
  Q_INVOKABLE void stop(const QString &name);
  Q_INVOKABLE void restart(const QString &name);
  Q_INVOKABLE void pause(const QString &name);
  Q_INVOKABLE void resume(const QString &name);
  Q_INVOKABLE void forceStop(const QString &name);
  Q_INVOKABLE void configure(const QString &name, const QString &description,
                             int cpus, int memoryMiB, bool machine,
                             const QString &sharedPath, bool sharedReadOnly);
  Q_INVOKABLE void remove(const QString &name);

signals:
  void environmentsChanged();
  void busyChanged();
  void themeChanged();
  void message(const QString &text, bool error);

private:
  void run(const QStringList &arguments, bool refreshAfter = true);
  void enrichEnvironment(int index);
  void loadTheme();
  QString cliPath() const;
  void setBusy(bool busy);

  QVariantList m_environments;
  QFileSystemWatcher m_themeWatcher;
  bool m_busy = false;
  QString m_themeMode = QStringLiteral("dark");
  QString m_themeBackground = QStringLiteral("#101010");
  QString m_themeForeground = QStringLiteral("#eeeeee");
  QString m_themeAccent = QStringLiteral("#5584aa");
  QString m_themeSelection = QStringLiteral("#186a9a");
  QString m_themeMuted = QStringLiteral("#777777");
  QString m_themeSurface = QStringLiteral("#202020");
  QString m_themeDarkSurface = QStringLiteral("#080808");
  QString m_themeGreen = QStringLiteral("#65a765");
  QString m_themeRed = QStringLiteral("#d35f5f");
};
