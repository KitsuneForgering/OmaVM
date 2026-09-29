#pragma once

#include <QFileSystemWatcher>
#include <QHash>
#include <QObject>
#include <QProcess>
#include <QUrl>
#include <QVariantList>

class Backend final : public QObject {
  Q_OBJECT
  Q_PROPERTY(
      QVariantList environments READ environments NOTIFY environmentsChanged)
  Q_PROPERTY(bool busy READ busy NOTIFY busyChanged)
  Q_PROPERTY(QString busyAction READ busyAction NOTIFY busyChanged)
  Q_PROPERTY(QString listError READ listError NOTIFY listErrorChanged)
  Q_PROPERTY(QVariantList apps READ apps NOTIFY appsChanged)
  Q_PROPERTY(bool appsLoading READ appsLoading NOTIFY appsChanged)
  Q_PROPERTY(QString appsEnvironment READ appsEnvironment NOTIFY appsChanged)
  Q_PROPERTY(QString appsError READ appsError NOTIFY appsChanged)
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
  QVariantList apps() const { return m_apps; }
  bool busy() const { return m_busy; }
  QString busyAction() const { return m_busyAction; }
  QString listError() const { return m_listError; }
  bool appsLoading() const { return m_appsLoading; }
  QString appsEnvironment() const { return m_appsEnvironment; }
  QString appsError() const { return m_appsError; }
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

  // Quitting while an action runs would destroy its QProcess, which kills
  // `omavm` mid-operation (a Restart cut between Stop and Start leaves the
  // Machine off). readyToQuit fires now if idle, else once the action ends.
  void requestQuit();

  Q_INVOKABLE void refresh();
  Q_INVOKABLE void poll();
  Q_INVOKABLE void createEnvironment(const QString &name, const QString &image,
                                     const QString &kind, int cpus,
                                     bool cpusTouched, int memoryMiB,
                                     bool memoryTouched);
  Q_INVOKABLE void start(const QString &name);
  Q_INVOKABLE void open(const QString &name, const QString &kind);
  Q_INVOKABLE void stop(const QString &name);
  Q_INVOKABLE void restart(const QString &name);
  Q_INVOKABLE void pause(const QString &name);
  Q_INVOKABLE void resume(const QString &name);
  Q_INVOKABLE void forceStop(const QString &name);
  Q_INVOKABLE void configure(const QString &name, const QString &description,
                             int cpus, bool cpusTouched, int memoryMiB,
                             bool memoryTouched, bool machine,
                             const QString &sharedPath, bool sharedReadOnly,
                             bool disconnectISO, const QString &color,
                             bool shareClipboard, bool travelMode, bool vulkan,
                             bool openInEmptyWorkspace, bool launcher,
                             bool ssh);
  Q_INVOKABLE void remove(const QString &name);
  Q_INVOKABLE void createSnapshot(const QString &name, const QString &label);
  Q_INVOKABLE void goToSnapshot(const QString &name, const QString &id);
  Q_INVOKABLE void removeSnapshot(const QString &name, const QString &id);
  Q_INVOKABLE void refreshApps(const QString &name);
  Q_INVOKABLE void exportApp(const QString &name, const QString &id);
  Q_INVOKABLE void unexportApp(const QString &name, const QString &id);
  Q_INVOKABLE void copyToClipboard(const QString &text) const;
  // Tags this process's window as a terminal for Omarchy's universal
  // copy/paste shortcuts (Box terminal mode).
  Q_INVOKABLE void markAsTerminalWindow() const;
  // A file picker's URL as a filesystem path, spaces and accents decoded.
  Q_INVOKABLE QString localPath(const QUrl &url) const {
    return url.toLocalFile();
  }

signals:
  void readyToQuit();
  void environmentsChanged();
  void appsChanged();
  void busyChanged();
  void listErrorChanged();
  void themeChanged();
  void message(const QString &text, bool error);
  // Correlates with the `tag` passed to run()/runForApps() so a dialog that
  // started an action (create, configure, a snapshot or app operation) can
  // tell its own request apart from any other action finishing, and keep
  // itself open with the user's input intact until its own tag reports ok.
  void actionFinished(const QString &tag, bool ok, const QString &text);

private:
  // Returns false (and emits an explanatory message instead of silently
  // no-opping) when another action is already in flight — the codebase
  // deliberately keeps a single in-flight action rather than adding a
  // queue or concurrency (CLAUDE.md, docs/TODO.md P0).
  bool beginBusy(const QString &label);
  void endBusy();
  QString busyMessage() const;
  void run(const QStringList &arguments, const QString &label,
           const QString &tag = QString(), bool refreshAfter = true);
  void runForApps(const QStringList &arguments, const QString &name,
                  const QString &label, const QString &tag);
  void enrichEnvironment(int index, int generation);
  void refreshImpl(bool silent);
  void loadTheme();
  QString cliPath() const;

  QVariantList m_environments;
  // Bumped every time a list response is accepted. Each
  // enrichEnvironment() call (and the status/integration/preview
  // subprocesses it spawns) captures the generation valid when it was
  // spawned; a periodic poll (gui/Main.qml) can start a fresh refresh()
  // while a previous cycle's enrichment calls are still in flight
  // (refresh() itself is single-flight via beginBusy, but its spawned
  // per-environment enrichment isn't), so a stale generation's result
  // landing late must never mutate the current array by raw index —
  // that index may now point at a different environment (docs/TODO.md
  // P1 "impedir resultados atrasados de atualizar o ambiente errado").
  int m_environmentsGeneration = 0;
  // A silent poll never changes busy UI state. A foreground action bumps
  // this token so a list response started before it cannot overwrite it.
  int m_pollGeneration = 0;
  bool m_polling = false;
  bool m_quitRequested = false;
  // Environment id -> earliest time (ms since epoch) to retry a preview
  // that failed. Some guests never expose a surface QEMU can screendump
  // ("no surface"), and without this every 3 s poll re-ran the capture.
  QHash<QString, qint64> m_previewRetryAt;
  QVariantList m_apps;
  QFileSystemWatcher m_themeWatcher;
  bool m_busy = false;
  QString m_busyAction;
  QString m_listError;
  bool m_appsLoading = false;
  QString m_appsEnvironment;
  QString m_appsError;
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

// Hyprland integration, exposed for tests.
bool hasPersonalViewerRule(const QString &hyprConfigDir);
QString terminalTagScript(qint64 pid);
