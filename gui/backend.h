#pragma once

#include <QFileSystemWatcher>
#include <QHash>
#include <QMap>
#include <QVariantMap>
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
  // Environment name -> what is running on it right now.
  Q_PROPERTY(
      QVariantMap busyEnvironments READ busyEnvironments NOTIFY busyChanged)
  // Environment name -> the stage a long operation reported last
  // (`create --progress`), such as which layer of an image is downloading.
  Q_PROPERTY(QVariantMap progress READ progress NOTIFY busyChanged)
  // `omavm host --json`, keyed by capability id ("kvm", ...): what this
  // computer offers Desktops, so the UI can say it before creating one.
  Q_PROPERTY(QVariantMap hostCapabilities READ hostCapabilities NOTIFY
                 hostCapabilitiesChanged)
  Q_PROPERTY(QString listError READ listError NOTIFY listErrorChanged)
  Q_PROPERTY(QVariantList apps READ apps NOTIFY appsChanged)
  Q_PROPERTY(bool appsLoading READ appsLoading NOTIFY appsChanged)
  Q_PROPERTY(QString appsEnvironment READ appsEnvironment NOTIFY appsChanged)
  Q_PROPERTY(QString appsError READ appsError NOTIFY appsChanged)
  Q_PROPERTY(QString themeMode READ themeMode NOTIFY themeChanged)
  Q_PROPERTY(QString themeBackground READ themeBackground NOTIFY themeChanged)
  Q_PROPERTY(QString themeForeground READ themeForeground NOTIFY themeChanged)
  Q_PROPERTY(QString themeAccent READ themeAccent NOTIFY themeChanged)
  // The accent where it is text or an icon, at 7:1 (WCAG AAA).
  Q_PROPERTY(
      QString themeAccentText READ themeAccentText NOTIFY themeChanged)
  Q_PROPERTY(QString themeSelection READ themeSelection NOTIFY themeChanged)
  Q_PROPERTY(QString themeMuted READ themeMuted NOTIFY themeChanged)
  Q_PROPERTY(QString themeSurface READ themeSurface NOTIFY themeChanged)
  Q_PROPERTY(QString themeDarkSurface READ themeDarkSurface NOTIFY themeChanged)
  Q_PROPERTY(QString themeGreen READ themeGreen NOTIFY themeChanged)
  Q_PROPERTY(QString themeRed READ themeRed NOTIFY themeChanged)
  // Color tag name (core.EnvironmentColors) -> the theme's own hue for it.
  Q_PROPERTY(QVariantMap themeTagColors READ themeTagColors NOTIFY themeChanged)

public:
  explicit Backend(QObject *parent = nullptr);

  QVariantList environments() const { return m_environments; }
  QVariantList apps() const { return m_apps; }
  bool busy() const { return !m_busy.isEmpty(); }
  QString busyAction() const;
  QVariantMap busyEnvironments() const;
  QVariantMap progress() const { return m_progress; }
  QVariantMap hostCapabilities() const { return m_hostCapabilities; }
  QString listError() const { return m_listError; }
  bool appsLoading() const { return m_appsLoading; }
  QString appsEnvironment() const { return m_appsEnvironment; }
  QString appsError() const { return m_appsError; }
  QString themeMode() const { return m_themeMode; }
  QString themeBackground() const { return m_themeBackground; }
  QString themeForeground() const { return m_themeForeground; }
  QString themeAccent() const { return m_themeAccent; }
  QString themeAccentText() const { return m_themeAccentText; }
  QString themeSelection() const { return m_themeSelection; }
  QString themeMuted() const { return m_themeMuted; }
  QString themeSurface() const { return m_themeSurface; }
  QString themeDarkSurface() const { return m_themeDarkSurface; }
  QString themeGreen() const { return m_themeGreen; }
  QString themeRed() const { return m_themeRed; }
  QVariantMap themeTagColors() const { return m_themeTagColors; }

  // Quitting while an action runs would destroy its QProcess, which kills
  // `omavm` mid-operation (a Restart cut between Stop and Start leaves the
  // Machine off). readyToQuit fires now if idle, else once the action ends.
  void requestQuit();
  // The window was shown again before the last action finished.
  void cancelQuit() { m_quitRequested = false; }

  Q_INVOKABLE void refresh();
  Q_INVOKABLE void poll();
  Q_INVOKABLE void createEnvironment(const QString &name, const QString &image,
                                     const QString &kind, int cpus,
                                     bool cpusTouched, int memoryMiB,
                                     bool memoryTouched);
  Q_INVOKABLE void start(const QString &name);
  Q_INVOKABLE void open(const QString &name, const QString &kind);
  // Starts a Machine without keeping this session's changes, then opens it.
  Q_INVOKABLE void openEphemeral(const QString &name);
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
                             bool ssh, bool fullscreen,
                             const QString &clipboardDirection = QString(),
                             bool sharedFolder = true);
  Q_INVOKABLE void remove(const QString &name);
  Q_INVOKABLE void createSnapshot(const QString &name, const QString &label);
  Q_INVOKABLE void goToSnapshot(const QString &name, const QString &id);
  Q_INVOKABLE void removeSnapshot(const QString &name, const QString &id);
  Q_INVOKABLE void refreshApps(const QString &name);
  Q_INVOKABLE void refreshHost();
  Q_INVOKABLE void cloneEnvironment(const QString &name, const QString &newName);
  Q_INVOKABLE void updateEnvironment(const QString &name);
  // Sets up a running Machine's guest; its steps come back as JSON in
  // actionFinished("prepare", ...), for the Settings dialog to show.
  Q_INVOKABLE void prepareGuest(const QString &name);
  // From a viewer: open the Machine again (a new display connection, in a
  // new viewer), or bring up the Experience Center.
  Q_INVOKABLE void reopenDisplay(const QString &name) const;
  Q_INVOKABLE void showManager() const;
  // Brings the Experience Center's workspace into view, for a second
  // launch: Wayland doesn't let a window focus itself unasked.
  Q_INVOKABLE void focusManagerWorkspace() const;
  Q_INVOKABLE void exportApp(const QString &name, const QString &id);
  Q_INVOKABLE void unexportApp(const QString &name, const QString &id);
  Q_INVOKABLE void copyToClipboard(const QString &text) const;
  // Copies dropped files (local URLs) into folder without overwriting
  // anything: a name already there gets " (2)". Each copy runs in the
  // background; actionFinished("drop", ...) says how it went.
  Q_INVOKABLE void copyIntoFolder(const QVariantList &urls,
                                  const QString &folder);
  // The path a copy of `source` takes in `folder`: its own name, or
  // "name (2).ext", "name (3).ext"... when taken. Exposed for tests.
  static QString freeDestination(const QString &folder, const QString &source);
  // Tags this process's window as a terminal for Omarchy's universal
  // copy/paste shortcuts (Box terminal mode).
  Q_INVOKABLE void markAsTerminalWindow() const;
  // A file picker's URL as a filesystem path, spaces and accents decoded.
  Q_INVOKABLE QString localPath(const QUrl &url) const {
    return url.toLocalFile();
  }

signals:
  void hostCapabilitiesChanged();
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
  // no-opping) when an action on the same key (environment name, or ""
  // for reloading the list) is already in flight. Different environments
  // run concurrently; the Core serializes one environment's operations.
  bool beginBusy(const QString &key, const QString &label);
  void endBusy(const QString &key);
  QString busyMessage(const QString &key) const;
  void run(const QString &key, const QStringList &arguments,
           const QString &label, const QString &tag = QString(),
           bool refreshAfter = true, bool toastOutput = true);
  void refreshSoon();
  void listLoaded();
  void runForApps(const QStringList &arguments, const QString &name,
                  const QString &label, const QString &tag);
  void capturePreview(int index, int generation);
  void refreshImpl(bool silent);
  void loadTheme();
  QString cliPath() const;

  QVariantList m_environments;
  // Bumped every time a list response is accepted. Each capturePreview()
  // call captures the generation valid when it was spawned; a periodic
  // poll (gui/Main.qml) can start a fresh refresh() while a previous
  // cycle's preview is still in flight, so a stale result landing late
  // must never mutate the current array by raw index — that index may now
  // point at a different environment (docs/TODO.md P1 "impedir resultados
  // atrasados de atualizar o ambiente errado").
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
  // Key (environment name, "" for the list) -> action label.
  QMap<QString, QString> m_busy;
  QVariantMap m_progress;
  QVariantMap m_hostCapabilities;
  bool m_refreshPending = false;
  QString m_listError;
  bool m_appsLoading = false;
  QString m_appsEnvironment;
  QString m_appsError;
  QString m_themeMode = QStringLiteral("dark");
  QString m_themeBackground = QStringLiteral("#101010");
  QString m_themeForeground = QStringLiteral("#eeeeee");
  QString m_themeAccent = QStringLiteral("#5584aa");
  QString m_themeAccentText = QStringLiteral("#5584aa");
  QString m_themeSelection = QStringLiteral("#186a9a");
  QString m_themeMuted = QStringLiteral("#777777");
  QString m_themeSurface = QStringLiteral("#202020");
  QString m_themeDarkSurface = QStringLiteral("#080808");
  QString m_themeGreen = QStringLiteral("#65a765");
  QString m_themeRed = QStringLiteral("#d35f5f");
  QVariantMap m_themeTagColors;
};

// Hyprland integration, exposed for tests.
bool hasPersonalViewerRule(const QString &hyprConfigDir);

enum class WorkspacePlacement {
  NotApplicable,          // no Hyprland session, or the user's own window
                          // rule places OmaVM's viewer: open as usual,
                          // silently
  LaunchNormally,         // switched to an empty workspace; go ahead and open
  AlreadyFocusedExisting, // an existing window for this environment was
                          // found and focused; do not open another
  Unavailable             // no eligible workspace, or the dispatch failed:
                          // open on the current workspace
};
// Run by a viewer (Machine display or Box terminal) before its window
// exists, so the window opens on the workspace it switched to.
WorkspacePlacement placeInEmptyWorkspace(const QString &title);
// The user's Hyprland config directory.
QString hyprConfigDir();
QString terminalTagScript(qint64 pid);
