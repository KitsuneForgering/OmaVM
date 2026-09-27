#include "backend.h"
#include "colorstoml.h"

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QStandardPaths>

Backend::Backend(QObject *parent) : QObject(parent) {
  connect(&m_themeWatcher, &QFileSystemWatcher::fileChanged, this,
          &Backend::loadTheme);
  connect(&m_themeWatcher, &QFileSystemWatcher::directoryChanged, this,
          &Backend::loadTheme);
  loadTheme();
}

QString Backend::cliPath() const {
  const QString sibling = QDir(QCoreApplication::applicationDirPath())
                              .filePath(QStringLiteral("omavm"));
  if (QFileInfo::exists(sibling))
    return sibling;
  const QString installed =
      QStandardPaths::findExecutable(QStringLiteral("omavm"));
  if (!installed.isEmpty())
    return installed;
  return sibling;
}

void Backend::setBusy(bool busy) {
  if (m_busy == busy)
    return;
  m_busy = busy;
  emit busyChanged();
}

void Backend::refresh() {
  if (m_busy)
    return;
  setBusy(true);
  auto *process = new QProcess(this);
  connect(process, &QProcess::errorOccurred, this,
          [this, process](QProcess::ProcessError error) {
            if (error != QProcess::FailedToStart)
              return;
            setBusy(false);
            emit message(QStringLiteral("Could not start omavm: ") +
                             process->errorString(),
                         true);
            process->deleteLater();
          });
  connect(process, &QProcess::finished, this, [this, process](int code) {
    const QByteArray output = process->readAllStandardOutput();
    const QString errorText =
        QString::fromUtf8(process->readAllStandardError()).trimmed();
    process->deleteLater();
    setBusy(false);
    if (code != 0) {
      emit message(errorText.isEmpty()
                       ? QStringLiteral("Could not load environments")
                       : errorText,
                   true);
      return;
    }
    const QJsonDocument document = QJsonDocument::fromJson(output);
    QVariantList next;
    for (const QJsonValue &value : document.array())
      next.append(value.toObject().toVariantMap());
    m_environments = next;
    emit environmentsChanged();
    for (int i = 0; i < m_environments.size(); ++i)
      enrichEnvironment(i);
  });
  process->start(cliPath(), {QStringLiteral("list"), QStringLiteral("--json")});
}

void Backend::enrichEnvironment(int index) {
  if (index < 0 || index >= m_environments.size())
    return;
  const QString name =
      m_environments.at(index).toMap().value("name").toString();
  auto *status = new QProcess(this);
  connect(
      status, &QProcess::finished, this, [this, status, index, name](int code) {
        const QJsonDocument document =
            QJsonDocument::fromJson(status->readAllStandardOutput());
        status->deleteLater();
        if (code != 0 || index >= m_environments.size())
          return;
        QVariantMap environment = m_environments.at(index).toMap();
        const QString state = document.object().value("state").toString();
        environment.insert(QStringLiteral("status"), state);
        m_environments[index] = environment;
        emit environmentsChanged();

        if (environment.value("kind").toString() != QStringLiteral("machine"))
          return;
        auto *integration = new QProcess(this);
        connect(integration, &QProcess::finished, this,
                [this, integration, index](int integrationCode) {
                  const QJsonDocument report = QJsonDocument::fromJson(
                      integration->readAllStandardOutput());
                  integration->deleteLater();
                  if (integrationCode != 0 || index >= m_environments.size())
                    return;
                  QVariantMap current = m_environments.at(index).toMap();
                  current.insert(QStringLiteral("guestAgent"),
                                 report.object()
                                     .value(QStringLiteral("guest_agent"))
                                     .toString());
                  current.insert(
                      QStringLiteral("integrationHint"),
                      report.object().value(QStringLiteral("hint")).toString());
                  m_environments[index] = current;
                  emit environmentsChanged();
                });
        integration->start(cliPath(), {QStringLiteral("integration"), name,
                                       QStringLiteral("--json")});

        if (state != QStringLiteral("running"))
          return;
        auto *preview = new QProcess(this);
        connect(
            preview, &QProcess::finished, this,
            [this, preview, index](int previewCode) {
              const QString path =
                  QString::fromUtf8(preview->readAllStandardOutput()).trimmed();
              preview->deleteLater();
              if (previewCode != 0 || path.isEmpty() ||
                  index >= m_environments.size())
                return;
              QVariantMap current = m_environments.at(index).toMap();
              current.insert(QStringLiteral("preview"),
                             QUrl::fromLocalFile(path));
              m_environments[index] = current;
              emit environmentsChanged();
            });
        preview->start(cliPath(), {QStringLiteral("preview"), name});
      });
  status->start(cliPath(),
                {QStringLiteral("status"), name, QStringLiteral("--json")});
}

void Backend::run(const QStringList &arguments, bool refreshAfter) {
  if (m_busy)
    return;
  setBusy(true);
  auto *process = new QProcess(this);
  connect(process, &QProcess::errorOccurred, this,
          [this, process](QProcess::ProcessError error) {
            if (error != QProcess::FailedToStart)
              return;
            setBusy(false);
            emit message(QStringLiteral("Could not start omavm: ") +
                             process->errorString(),
                         true);
            process->deleteLater();
          });
  connect(process, &QProcess::finished, this,
          [this, process, refreshAfter](int code) {
            const QString output =
                QString::fromUtf8(process->readAllStandardOutput()).trimmed();
            const QString errorText =
                QString::fromUtf8(process->readAllStandardError()).trimmed();
            process->deleteLater();
            setBusy(false);
            if (code != 0) {
              emit message(errorText.isEmpty() ? QStringLiteral("Action failed")
                                               : errorText,
                           true);
              return;
            }
            if (!output.isEmpty())
              emit message(output, false);
            if (refreshAfter)
              refresh();
          });
  process->start(cliPath(), arguments);
}

void Backend::createEnvironment(const QString &name, const QString &image,
                                const QString &kind) {
  run({QStringLiteral("create"), QStringLiteral("--name"), name,
       QStringLiteral("--kind"), kind, QStringLiteral("--image"), image});
}

void Backend::start(const QString &name) {
  run({QStringLiteral("start"), name});
}
void Backend::stop(const QString &name) { run({QStringLiteral("stop"), name}); }
void Backend::restart(const QString &name) {
  run({QStringLiteral("restart"), name});
}
void Backend::pause(const QString &name) {
  run({QStringLiteral("pause"), name});
}
void Backend::resume(const QString &name) {
  run({QStringLiteral("resume"), name});
}
void Backend::forceStop(const QString &name) {
  run({QStringLiteral("force-stop"), name});
}
void Backend::configure(const QString &name, const QString &description,
                        int cpus, int memoryMiB, bool machine,
                        const QString &sharedPath, bool sharedReadOnly,
                        bool disconnectISO, const QString &color,
                        bool shareClipboard, bool travelMode) {
  QStringList arguments{QStringLiteral("settings"),      name,
                        QStringLiteral("--description"), description,
                        QStringLiteral("--color"),       color};
  if (machine) {
    arguments << (disconnectISO ? QStringLiteral("--disconnect-iso=true")
                                : QStringLiteral("--disconnect-iso=false"));
    arguments << QStringLiteral("--cpus") << QString::number(cpus)
              << QStringLiteral("--memory-mib") << QString::number(memoryMiB)
              << QStringLiteral("--shared-path") << sharedPath;
    arguments << (sharedReadOnly ? QStringLiteral("--shared-read-only")
                                 : QStringLiteral("--shared-writable"));
    arguments << (shareClipboard ? QStringLiteral("--share-clipboard=true")
                                 : QStringLiteral("--share-clipboard=false"));
    arguments << (travelMode ? QStringLiteral("--travel-mode=true")
                             : QStringLiteral("--travel-mode=false"));
  }
  run(arguments);
}
void Backend::remove(const QString &name) {
  run({QStringLiteral("remove"), name});
}

void Backend::runForApps(const QStringList &arguments, const QString &name) {
  if (m_busy)
    return;
  setBusy(true);
  auto *process = new QProcess(this);
  connect(process, &QProcess::errorOccurred, this,
          [this, process](QProcess::ProcessError error) {
            if (error != QProcess::FailedToStart)
              return;
            setBusy(false);
            emit message(QStringLiteral("Could not start omavm: ") +
                             process->errorString(),
                         true);
            process->deleteLater();
          });
  connect(process, &QProcess::finished, this, [this, process, name](int code) {
    const QString errorText =
        QString::fromUtf8(process->readAllStandardError()).trimmed();
    process->deleteLater();
    setBusy(false);
    if (code != 0) {
      emit message(errorText.isEmpty() ? QStringLiteral("Action failed")
                                       : errorText,
                   true);
      return;
    }
    refreshApps(name);
  });
  process->start(cliPath(), arguments);
}

void Backend::refreshApps(const QString &name) {
  if (m_busy)
    return;
  setBusy(true);
  auto *process = new QProcess(this);
  connect(process, &QProcess::errorOccurred, this,
          [this, process](QProcess::ProcessError error) {
            if (error != QProcess::FailedToStart)
              return;
            setBusy(false);
            emit message(QStringLiteral("Could not start omavm: ") +
                             process->errorString(),
                         true);
            process->deleteLater();
          });
  connect(process, &QProcess::finished, this, [this, process](int code) {
    const QByteArray output = process->readAllStandardOutput();
    const QString errorText =
        QString::fromUtf8(process->readAllStandardError()).trimmed();
    process->deleteLater();
    setBusy(false);
    if (code != 0) {
      emit message(errorText.isEmpty()
                       ? QStringLiteral("Could not list applications")
                       : errorText,
                   true);
      return;
    }
    const QJsonDocument document = QJsonDocument::fromJson(output);
    QVariantList next;
    for (const QJsonValue &value : document.array())
      next.append(value.toObject().toVariantMap());
    m_apps = next;
    emit appsChanged();
  });
  process->start(cliPath(),
                 {QStringLiteral("apps"), name, QStringLiteral("--json")});
}

void Backend::exportApp(const QString &name, const QString &id) {
  runForApps({QStringLiteral("apps"), name, QStringLiteral("--export"), id},
             name);
}

void Backend::unexportApp(const QString &name, const QString &id) {
  runForApps({QStringLiteral("apps"), name, QStringLiteral("--unexport"), id},
             name);
}

void Backend::createSnapshot(const QString &name, const QString &label) {
  run({QStringLiteral("snapshot"), QStringLiteral("create"), name,
       QStringLiteral("--label"), label});
}
void Backend::goToSnapshot(const QString &name, const QString &id) {
  run({QStringLiteral("snapshot"), QStringLiteral("go-to"), name, id});
}
void Backend::removeSnapshot(const QString &name, const QString &id) {
  run({QStringLiteral("snapshot"), QStringLiteral("remove"), name, id});
}

void Backend::open(const QString &name, const QString &kind) {
  if (kind == QStringLiteral("box")) {
    // Relaunch this same omavm-gui binary in its embedded terminal mode
    // (gui/main.cpp's --terminal, gui/TerminalViewer.qml) — same pattern
    // as a Machine's Open spawning omavm-gui --viewer
    // (internal/backend/qemu/qemu.go), so a Box opens inside OmaVM's own
    // window instead of whatever external terminal emulator the user has
    // configured.
    QProcess::startDetached(QCoreApplication::applicationFilePath(),
                            {QStringLiteral("--terminal"), name,
                             QStringLiteral("--title"),
                             name + QStringLiteral(" — OmaVM")});
    return;
  }
  run({QStringLiteral("open"), name});
}

void Backend::loadTheme() {
  const QString currentDir =
      QDir::homePath() + QStringLiteral("/.local/state/omarchy/current");
  const QString themeDir = currentDir + QStringLiteral("/theme");
  const QString path = themeDir + QStringLiteral("/colors.toml");
  m_themeMode = QStringLiteral("dark");
  m_themeBackground = QStringLiteral("#101010");
  m_themeForeground = QStringLiteral("#eeeeee");
  m_themeAccent = QStringLiteral("#5584aa");
  m_themeSelection = QStringLiteral("#186a9a");
  m_themeMuted = QStringLiteral("#777777");
  m_themeSurface = QStringLiteral("#202020");
  m_themeDarkSurface = QStringLiteral("#080808");
  m_themeGreen = QStringLiteral("#65a765");
  m_themeRed = QStringLiteral("#d35f5f");

  const QHash<QString, QString> values = loadColorsToml(path);
  auto take = [&values](const QString &key, QString &target) {
    const auto it = values.constFind(key);
    if (it != values.constEnd())
      target = it.value();
  };
  take(QStringLiteral("mode"), m_themeMode);
  take(QStringLiteral("background"), m_themeBackground);
  take(QStringLiteral("foreground"), m_themeForeground);
  take(QStringLiteral("accent"), m_themeAccent);
  take(QStringLiteral("selection"), m_themeSelection);
  take(QStringLiteral("muted"), m_themeMuted);
  take(QStringLiteral("lighter_background"), m_themeSurface);
  take(QStringLiteral("dark_background"), m_themeDarkSurface);
  take(QStringLiteral("green"), m_themeGreen);
  take(QStringLiteral("red"), m_themeRed);

  const QStringList watched =
      m_themeWatcher.files() + m_themeWatcher.directories();
  if (!watched.isEmpty())
    m_themeWatcher.removePaths(watched);
  if (QDir(currentDir).exists())
    m_themeWatcher.addPath(currentDir);
  if (QDir(themeDir).exists())
    m_themeWatcher.addPath(themeDir);
  if (QFile::exists(path))
    m_themeWatcher.addPath(path);
  emit themeChanged();
}
