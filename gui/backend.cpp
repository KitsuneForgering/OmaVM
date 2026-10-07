#include "backend.h"

#include <memory>
#include "colorstoml.h"

#include <QClipboard>
#include <QCoreApplication>
#include <QDateTime>
#include <QDir>
#include <QDirIterator>
#include <QFile>
#include <QGuiApplication>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QStandardPaths>

#include <memory>
#include <utility>

namespace {

// "Open in an empty workspace" (docs/TODO.md P2, CLAUDE.md's validated
// Hyprland policy). This whole block is desktop integration, deliberately
// kept out of the Core/Go backends (CLAUDE.md: "a política pertence à
// integração com o desktop, sem acoplar o Core ao Hyprland").
//
// This Omarchy Hyprland fork evaluates `hyprctl dispatch <ARGS>` as Lua
// source (`hl.dispatch(<ARGS>)`, verbatim, not shell-tokenized) — classic
// Hyprland selector strings like "class:^(...)$" don't parse there. The
// validated pattern (confirmed against a real session, and matching how
// Omarchy's own bar widget QML dispatches workspace switches — see
// /usr/share/omarchy/shell/plugins/bar/widgets/Workspaces.qml) is a
// single Lua expression string, using the structured `hl` API
// (hl.get_workspaces()/hl.get_windows()/hl.dispatch(hl.dsp...)) instead
// of regex window selectors.

// True only when a Hyprland session is actually running — checked via
// the env var alone (no process spawn) before ever touching hyprctl.
bool hyprlandAvailable() {
  return qEnvironmentVariableIsSet("HYPRLAND_INSTANCE_SIGNATURE");
}

// Escapes text for embedding inside a single-quoted Lua string literal.
QString luaQuote(const QString &text) {
  QString escaped = text;
  escaped.replace(QLatin1Char('\\'), QStringLiteral("\\\\"));
  escaped.replace(QLatin1Char('\''), QStringLiteral("\\'"));
  return QLatin1Char('\'') + escaped + QLatin1Char('\'');
}

// Runs a Lua snippet through hyprctl's synchronous "repl" command and
// returns its printed result. Blocking, with a short timeout: this is a
// local socket round-trip that only runs right before opening an
// environment, and the caller needs an answer before deciding where to
// launch — there is no async equivalent of "decide, then launch" here
// without a much larger restructure, so this stays a bounded, occasional
// stall rather than pretending to be non-blocking.
QString hyprctlRepl(const QString &luaCode, bool *ok) {
  QProcess proc;
  proc.start(QStringLiteral("hyprctl"), {QStringLiteral("repl"), luaCode});
  const bool started = proc.waitForStarted(500);
  const bool finished = started && proc.waitForFinished(1500);
  if (!finished) {
    proc.kill();
    proc.waitForFinished(200);
  }
  if (ok)
    *ok = finished && proc.exitCode() == 0;
  return QString::fromUtf8(proc.readAllStandardOutput()).trimmed();
}


} // namespace

// Finds this environment's already-open viewer/terminal window by class
// + title (the shared `dev.omavm.viewer` app id alone can't tell two
// Machines or Boxes apart — both gui/main.cpp's --display-fd/--terminal modes
// title the window "<name> — OmaVM"). If none exists, picks the
// lowest-numbered eligible workspace on the currently active monitor:
// empty (HL.Workspace.is_empty), not special, and not already claimed by
// another monitor — including a workspace ID that doesn't exist yet
// (materializing it is exactly what Hyprland does when you focus an
// unused number). Known, deliberate gaps versus the full P2 policy:
// workspaces "reserved by personal rules" can't be distinguished from any
// other non-empty workspace by this API, so they're simply skipped
// because they're non-empty, not recognized as reserved; and there is no
// queryable "configured workspace range" to respect, so the search is
// capped at a fixed, generous bound (30) instead.
WorkspacePlacement placeInEmptyWorkspace(const QString &title) {
  if (!hyprlandAvailable() || hasPersonalViewerRule(hyprConfigDir()))
    return WorkspacePlacement::NotApplicable;

  const QString script =
      QStringLiteral(
          "local title = %1\n"
          "local existing = hl.get_windows({ class = 'dev.omavm.viewer', title "
          "= title })\n"
          "if #existing > 0 then\n"
          "  hl.dispatch(hl.dsp.focus({ workspace = existing[1].workspace.id "
          "}))\n"
          "  return 'focused'\n"
          "end\n"
          "local mon = hl.get_active_monitor()\n"
          "local byId = {}\n"
          "for _, w in ipairs(hl.get_workspaces()) do\n"
          "  if w.monitor == nil or (mon ~= nil and w.monitor.id == mon.id) "
          "then\n"
          "    byId[w.id] = w\n"
          "  end\n"
          "end\n"
          "for i = 1, 30 do\n"
          "  local w = byId[i]\n"
          "  if w == nil or (w.is_empty and not w.special) then\n"
          "    hl.dispatch(hl.dsp.focus({ workspace = tostring(i) }))\n"
          "    return 'switched'\n"
          "  end\n"
          "end\n"
          "return 'none'\n")
          .arg(luaQuote(title));

  bool ok = false;
  const QString result = hyprctlRepl(script, &ok);
  if (!ok)
    return WorkspacePlacement::Unavailable;
  if (result == QStringLiteral("focused"))
    return WorkspacePlacement::AlreadyFocusedExisting;
  if (result == QStringLiteral("switched"))
    return WorkspacePlacement::LaunchNormally;
  return WorkspacePlacement::Unavailable;
}

// Whether the user's Hyprland config has a window rule of its own for the
// viewer (contrib/hypr/omavm-viewer.lua, or anything else naming
// dev.omavm.viewer). Such a rule moves the window after it opens, so
// switching to an empty workspace first would only leave the user
// looking at an empty workspace. The rule is theirs and wins.
QString hyprConfigDir() {
  return QStandardPaths::writableLocation(
             QStandardPaths::GenericConfigLocation) +
         QStringLiteral("/hypr");
}

bool hasPersonalViewerRule(const QString &hyprConfigDir) {
  QDirIterator it(hyprConfigDir, {QStringLiteral("*.lua"), QStringLiteral("*.conf")},
                  QDir::Files, QDirIterator::Subdirectories);
  while (it.hasNext()) {
    QFile file(it.next());
    if (file.size() > 1024 * 1024 || !file.open(QIODevice::ReadOnly))
      continue;
    for (const QByteArray &raw : file.readAll().split('\n')) {
      // A commented-out rule (Lua "--", hyprlang "#") isn't active.
      QByteArray line = raw;
      const int luaComment = line.indexOf("--");
      if (luaComment >= 0)
        line.truncate(luaComment);
      if (line.trimmed().startsWith('#'))
        continue;
      if (line.contains("dev.omavm.viewer"))
        return true;
    }
  }
  return false;
}

QString terminalTagScript(qint64 pid) {
  // Omarchy's Super+C/Super+V send Ctrl+Insert/Shift+Insert only to windows
  // tagged "terminal" (default/hypr/bindings/clipboard.lua), and tag
  // terminals by app id. The Box terminal shares dev.omavm.viewer with the
  // Machine viewer, where Ctrl+C is right, so its own window is tagged at
  // runtime instead: without the tag, Super+C reached the program in the
  // Box as Ctrl+C and interrupted it. Found by pid: the shell can retitle
  // the window, and the viewer process owns only this one.
  return QStringLiteral(
             "for _, w in ipairs(hl.get_windows()) do\n"
             "  if w.pid == %1 and w.class == 'dev.omavm.viewer' then\n"
             "    hl.dispatch(hl.dsp.window.tag({ tag = '+terminal', window "
             "= 'address:' .. w.address }))\n"
             "    return 'tagged'\n"
             "  end\n"
             "end\n"
             "return 'none'\n")
      .arg(pid);
}

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

QString Backend::busyAction() const {
  if (m_busy.size() == 1)
    return m_busy.first();
  return m_busy.isEmpty() ? QString()
                          : QStringLiteral("%1 actions").arg(m_busy.size());
}

QVariantMap Backend::busyEnvironments() const {
  QVariantMap names;
  for (auto it = m_busy.cbegin(); it != m_busy.cend(); ++it)
    if (!it.key().isEmpty())
      names.insert(it.key(), it.value());
  return names;
}

QString Backend::busyMessage(const QString &key) const {
  const QString action = m_busy.value(key);
  return action.isEmpty()
             ? QStringLiteral("Still working — please wait.")
             : QStringLiteral("Still %1 — please wait.").arg(action);
}

// Actions run concurrently, one per environment: the Core serializes the
// operations on one environment itself (a lock per environment), so a Box
// pulling its image for minutes no longer holds every other card. key is
// the environment's name, or "" for reloading the list.
bool Backend::beginBusy(const QString &key, const QString &label) {
  if (m_busy.contains(key)) {
    // Identify what's already running instead of silently dropping the
    // new request (docs/TODO.md P0: no action may look like it succeeded
    // without executing). Not an error — the user just needs to wait.
    emit message(busyMessage(key), false);
    return false;
  }
  m_busy.insert(key, label);
  ++m_pollGeneration; // Discard a background poll started before this action.
  emit busyChanged();
  return true;
}

void Backend::endBusy(const QString &key) {
  m_busy.remove(key);
  m_progress.remove(key);
  emit busyChanged();
  if (m_quitRequested && m_busy.isEmpty())
    emit readyToQuit();
}

void Backend::requestQuit() {
  m_quitRequested = true;
  if (m_busy.isEmpty())
    QMetaObject::invokeMethod(this, &Backend::readyToQuit,
                              Qt::QueuedConnection);
}

void Backend::refresh() { refreshImpl(false); }

void Backend::poll() {
  if (m_busy.contains(QString()) || m_polling)
    return;
  refreshImpl(true);
}

// After an action: a reload that is already running may have read the
// registry before the action ended, so one more follows it.
void Backend::refreshSoon() {
  if (m_polling || m_busy.contains(QString())) {
    m_refreshPending = true;
    return;
  }
  refreshImpl(true);
}

void Backend::listLoaded() {
  if (m_refreshPending && !m_quitRequested) {
    m_refreshPending = false;
    refreshImpl(true);
  }
}

void Backend::refreshImpl(bool silent) {
  if (silent) {
    m_polling = true;
  } else if (!beginBusy(QString(), QStringLiteral("loading environments"))) {
    return;
  }
  const int pollGeneration = m_pollGeneration;
  auto *process = new QProcess(this);
  connect(
      process, &QProcess::errorOccurred, this,
      [this, process, silent, pollGeneration](QProcess::ProcessError error) {
        if (error != QProcess::FailedToStart)
          return;
        if (silent)
          m_polling = false;
        else
          endBusy(QString());
        QMetaObject::invokeMethod(this, &Backend::listLoaded,
                                  Qt::QueuedConnection);
        if (silent && pollGeneration != m_pollGeneration) {
          process->deleteLater();
          return;
        }
        const QString text =
            QStringLiteral("Could not start omavm: ") + process->errorString();
        if (m_listError != text) {
          m_listError = text;
          emit listErrorChanged();
          emit message(text, true);
        }
        process->deleteLater();
      });
  connect(process, &QProcess::finished, this,
          [this, process, silent, pollGeneration](int code) {
            const QByteArray output = process->readAllStandardOutput();
            const QString errorText =
                QString::fromUtf8(process->readAllStandardError()).trimmed();
            process->deleteLater();
            if (silent)
              m_polling = false;
            else
              endBusy(QString());
            QMetaObject::invokeMethod(this, &Backend::listLoaded,
                                      Qt::QueuedConnection);
            if (silent && pollGeneration != m_pollGeneration)
              return;
            if (code != 0) {
              const QString text =
                  errorText.isEmpty()
                      ? QStringLiteral("Could not load environments")
                      : errorText;
              if (m_listError != text) {
                m_listError = text;
                emit listErrorChanged();
                emit message(text, true);
              }
              return;
            }
            if (!m_listError.isEmpty()) {
              m_listError.clear();
              emit listErrorChanged();
            }
            const QJsonDocument document = QJsonDocument::fromJson(output);
            QVariantList next;
            for (const QJsonValue &value : document.array()) {
              // `list --status` nests what the backend reports; the cards
              // read it as flat fields.
              QVariantMap current = value.toObject().toVariantMap();
              const QVariantMap status =
                  current.take(QStringLiteral("status")).toMap();
              current.insert(QStringLiteral("status"),
                             status.value(QStringLiteral("state")));
              current.insert(QStringLiteral("statusDetail"),
                             status.value(QStringLiteral("detail")));
              current.insert(
                  QStringLiteral("restartNeeded"),
                  status.value(QStringLiteral("restart_needed")).toBool());
              current.insert(
                  QStringLiteral("travelMode"),
                  status.value(QStringLiteral("travel_mode")).toBool());
              current.insert(QStringLiteral("statusWarning"),
                             status.value(QStringLiteral("warning")));
              current.insert(QStringLiteral("ephemeral"),
                             status.value(QStringLiteral("ephemeral")).toBool());
              const QVariantMap integration =
                  current.take(QStringLiteral("integration")).toMap();
              if (!integration.isEmpty()) {
                current.insert(QStringLiteral("guestAgent"),
                               integration.value(QStringLiteral("guest_agent")));
                current.insert(QStringLiteral("integrationHint"),
                               integration.value(QStringLiteral("hint")));
                current.insert(QStringLiteral("guestCapabilities"),
                               integration.value(QStringLiteral("capabilities")));
              }
              // Keep the previous preview while the Machine stays in the
              // same state: recapturing it every poll made cards flash
              // (capturePreview refreshes it on its own schedule). A
              // change of state fetches it again: a Machine that just shut
              // down gets the frame it saved on the way. The list is small
              // (the UI is designed around up to 30 environments), so a
              // lookup here is simpler than another index.
              {
                for (const QVariant &previousItem :
                     std::as_const(m_environments)) {
                  const QVariantMap previous = previousItem.toMap();
                  if (previous.value(QStringLiteral("id")) ==
                          current.value(QStringLiteral("id")) &&
                      previous.value(QStringLiteral("status")) ==
                          current.value(QStringLiteral("status")) &&
                      previous.contains(QStringLiteral("preview"))) {
                    current.insert(QStringLiteral("preview"),
                                   previous.value(QStringLiteral("preview")));
                    break;
                  }
                }
              }
              next.append(current);
            }
            const int generation = ++m_environmentsGeneration;
            if (next != m_environments) {
              m_environments = next;
              emit environmentsChanged();
            }
            for (int i = 0; i < m_environments.size(); ++i)
              capturePreview(i, generation);
          });
  process->start(cliPath(), {QStringLiteral("list"), QStringLiteral("--status"),
                             QStringLiteral("--json")});
}

// capturePreview asks for a thumbnail of a running Machine that has none
// yet. Status and guest tools already came with the list.
void Backend::capturePreview(int index, int generation) {
  if (generation != m_environmentsGeneration || index < 0 ||
      index >= m_environments.size())
    return;
  const QVariantMap environment = m_environments.at(index).toMap();
  const QString id = environment.value(QStringLiteral("id")).toString();
  if (environment.value(QStringLiteral("kind")).toString() !=
      QStringLiteral("machine")) {
    m_previewRetryAt.remove(id);
    return;
  }
  // Running: refreshed every 30 s, like a live thumbnail. Stopped: its
  // last frame, fetched once (it can't change until it runs again).
  const bool running = environment.value(QStringLiteral("status")).toString() ==
                       QStringLiteral("running");
  if ((!running && environment.contains(QStringLiteral("preview"))) ||
      QDateTime::currentMSecsSinceEpoch() < m_previewRetryAt.value(id))
    return;
  auto *preview = new QProcess(this);
  connect(preview, &QProcess::finished, this,
          [this, preview, index, generation, id, running](int previewCode) {
            const QString path =
                QString::fromUtf8(preview->readAllStandardOutput()).trimmed();
            preview->deleteLater();
            const qint64 now = QDateTime::currentMSecsSinceEpoch();
            if (previewCode != 0 || path.isEmpty()) {
              // A stopped Machine that never ran has no frame: ask rarely.
              m_previewRetryAt.insert(id, now + (running ? 60000 : 600000));
              return;
            }
            m_previewRetryAt.insert(id, now + 30000);
            // A poll that landed meanwhile may have reordered the list:
            // never write by a stale index.
            if (generation != m_environmentsGeneration ||
                index >= m_environments.size())
              return;
            QVariantMap current = m_environments.at(index).toMap();
            // The file is rewritten in place: a new query makes the Image
            // load it again (the query is ignored when reading the file).
            QUrl previewUrl = QUrl::fromLocalFile(path);
            previewUrl.setQuery(QStringLiteral("t=%1").arg(now));
            current.insert(QStringLiteral("preview"), previewUrl);
            m_environments[index] = current;
            emit environmentsChanged();
          });
  preview->start(cliPath(), {QStringLiteral("preview"),
                             environment.value(QStringLiteral("name")).toString()});
}

void Backend::run(const QString &key, const QStringList &arguments,
                  const QString &label, const QString &tag,
                  bool refreshAfter, bool toastOutput) {
  if (!beginBusy(key, label)) {
    if (!tag.isEmpty())
      emit actionFinished(tag, false, busyMessage(key));
    return;
  }
  auto *process = new QProcess(this);
  // "progress: STAGE" lines arrive while the command runs; everything else
  // on stdout is its result, shown when it finishes.
  auto result = std::make_shared<QByteArray>();
  connect(process, &QProcess::readyReadStandardOutput, this,
          [this, process, key, result]() {
            while (process->canReadLine()) {
              const QByteArray line = process->readLine();
              if (line.startsWith("progress: ")) {
                m_progress.insert(key, QString::fromUtf8(line.mid(10)).trimmed());
                emit busyChanged();
              } else {
                result->append(line);
              }
            }
          });
  connect(process, &QProcess::errorOccurred, this,
          [this, process, tag, key](QProcess::ProcessError error) {
            if (error != QProcess::FailedToStart)
              return;
            endBusy(key);
            const QString text = QStringLiteral("Could not start omavm: ") +
                                 process->errorString();
            emit message(text, true);
            if (!tag.isEmpty())
              emit actionFinished(tag, false, text);
            process->deleteLater();
          });
  connect(process, &QProcess::finished, this,
          [this, process, refreshAfter, toastOutput, tag, key, result](int code) {
            const QString output =
                QString::fromUtf8(*result + process->readAllStandardOutput())
                    .trimmed();
            const QString errorText =
                QString::fromUtf8(process->readAllStandardError()).trimmed();
            process->deleteLater();
            endBusy(key);
            // Even a failed action may have changed the state (a Start
            // that got halfway).
            if (refreshAfter && !m_quitRequested)
              refreshSoon();
            if (code != 0) {
              const QString text = errorText.isEmpty()
                                       ? QStringLiteral("Action failed")
                                       : errorText;
              emit message(text, true);
              if (!tag.isEmpty())
                emit actionFinished(tag, false, text);
              return;
            }
            if (toastOutput && !output.isEmpty())
              emit message(output, false);
            if (!tag.isEmpty())
              emit actionFinished(tag, true, output);
          });
  process->start(cliPath(), arguments);
}

void Backend::createEnvironment(const QString &name, const QString &image,
                                const QString &kind, int cpus,
                                bool cpusTouched, int memoryMiB,
                                bool memoryTouched) {
  QStringList arguments{QStringLiteral("create"),
                        QStringLiteral("--name"),
                        name,
                        QStringLiteral("--kind"),
                        kind,
                        QStringLiteral("--image"),
                        image};
  // Only pin hardware the user actually adjusted in the summary step —
  // sending the shown default (2 CPUs/2048 MiB) unconditionally would
  // pin every new Machine's hardware from birth, permanently disabling
  // Travel Mode's automatic reduction on battery for it (same bug class
  // as Backend::configure, docs/TODO.md P2).
  if (kind == QStringLiteral("box"))
    arguments << QStringLiteral("--progress");
  if (kind == QStringLiteral("machine")) {
    if (cpusTouched)
      arguments << QStringLiteral("--cpus") << QString::number(cpus);
    if (memoryTouched)
      arguments << QStringLiteral("--memory-mib") << QString::number(memoryMiB);
  }
  run(name, arguments, QStringLiteral("creating %1").arg(name),
      QStringLiteral("create"));
}

void Backend::start(const QString &name) {
  run(name, {QStringLiteral("start"), name}, QStringLiteral("starting %1").arg(name),
      QStringLiteral("start"));
}
void Backend::stop(const QString &name) {
  run(name, {QStringLiteral("stop"), name}, QStringLiteral("stopping %1").arg(name),
      QStringLiteral("stop"));
}
void Backend::restart(const QString &name) {
  run(name, {QStringLiteral("restart"), name},
      QStringLiteral("restarting %1").arg(name), QStringLiteral("restart"));
}
void Backend::pause(const QString &name) {
  run(name, {QStringLiteral("pause"), name}, QStringLiteral("pausing %1").arg(name),
      QStringLiteral("pause"));
}
void Backend::resume(const QString &name) {
  run(name, {QStringLiteral("resume"), name}, QStringLiteral("resuming %1").arg(name),
      QStringLiteral("resume"));
}
void Backend::forceStop(const QString &name) {
  run(name, {QStringLiteral("force-stop"), name},
      QStringLiteral("force stopping %1").arg(name),
      QStringLiteral("force-stop"));
}
void Backend::configure(const QString &name, const QString &description,
                        int cpus, bool cpusTouched, int memoryMiB,
                        bool memoryTouched, bool machine,
                        const QString &sharedPath, bool sharedReadOnly,
                        bool disconnectISO, const QString &color,
                        bool shareClipboard, bool travelMode, bool vulkan,
                        bool openInEmptyWorkspace, bool launcher, bool ssh,
                        bool fullscreen, const QString &clipboardDirection) {
  QStringList arguments{QStringLiteral("settings"),      name,
                        QStringLiteral("--description"), description,
                        QStringLiteral("--color"),       color};
  arguments << (openInEmptyWorkspace
                    ? QStringLiteral("--open-in-empty-workspace=true")
                    : QStringLiteral("--open-in-empty-workspace=false"));
  arguments << (launcher ? QStringLiteral("--launcher=true")
                         : QStringLiteral("--launcher=false"));
  // Both kinds: the guest's clipboard for a Machine, programs copying from
  // the terminal (OSC 52) for a Box.
  arguments << (shareClipboard ? QStringLiteral("--share-clipboard=true")
                               : QStringLiteral("--share-clipboard=false"));
  if (machine && !clipboardDirection.isEmpty())
    arguments << QStringLiteral("--clipboard-direction") << clipboardDirection;
  arguments << (travelMode ? QStringLiteral("--travel-mode=true")
                           : QStringLiteral("--travel-mode=false"));
  if (machine) {
    arguments << (disconnectISO ? QStringLiteral("--disconnect-iso=true")
                                : QStringLiteral("--disconnect-iso=false"));
    // Only send --cpus/--memory-mib when the SpinBox was actually
    // touched: the value shown is Core's *effective* default (2/2048)
    // even when nothing is pinned, so resending it unconditionally on
    // every save (e.g. just editing the description) would silently
    // pin it — permanently turning off Travel Mode's automatic
    // reduction on battery for this Machine (docs/TODO.md P2).
    if (cpusTouched)
      arguments << QStringLiteral("--cpus") << QString::number(cpus);
    if (memoryTouched)
      arguments << QStringLiteral("--memory-mib") << QString::number(memoryMiB);
    arguments << QStringLiteral("--shared-path") << sharedPath;
    arguments << (sharedReadOnly ? QStringLiteral("--shared-read-only")
                                 : QStringLiteral("--shared-writable"));
    arguments << (vulkan ? QStringLiteral("--vulkan=true")
                         : QStringLiteral("--vulkan=false"));
    arguments << (ssh ? QStringLiteral("--ssh=true")
                      : QStringLiteral("--ssh=false"));
    arguments << (fullscreen ? QStringLiteral("--fullscreen=true")
                             : QStringLiteral("--fullscreen=false"));
  }
  run(name, arguments, QStringLiteral("saving settings for %1").arg(name),
      QStringLiteral("configure"));
}
void Backend::remove(const QString &name) {
  run(name, {QStringLiteral("remove"), name}, QStringLiteral("deleting %1").arg(name),
      QStringLiteral("remove"));
}

void Backend::runForApps(const QStringList &arguments, const QString &name,
                         const QString &label, const QString &tag) {
  if (!beginBusy(name, label)) {
    emit actionFinished(tag, false, busyMessage(name));
    return;
  }
  auto *process = new QProcess(this);
  connect(process, &QProcess::errorOccurred, this,
          [this, process, tag, name](QProcess::ProcessError error) {
            if (error != QProcess::FailedToStart)
              return;
            endBusy(name);
            const QString text = QStringLiteral("Could not start omavm: ") +
                                 process->errorString();
            emit message(text, true);
            emit actionFinished(tag, false, text);
            process->deleteLater();
          });
  connect(
      process, &QProcess::finished, this, [this, process, name, tag](int code) {
        const QString errorText =
            QString::fromUtf8(process->readAllStandardError()).trimmed();
        process->deleteLater();
        endBusy(name);
        if (code != 0) {
          const QString text =
              errorText.isEmpty() ? QStringLiteral("Action failed") : errorText;
          emit message(text, true);
          emit actionFinished(tag, false, text);
          return;
        }
        emit actionFinished(tag, true, QString());
        refreshApps(name);
      });
  process->start(cliPath(), arguments);
}

void Backend::reopenDisplay(const QString &name) const {
  QProcess::startDetached(cliPath(), {QStringLiteral("open"), name});
}

void Backend::focusManagerWorkspace() const {
  if (!hyprlandAvailable())
    return;
  hyprctlRepl(QStringLiteral(
                  "local w = hl.get_windows({ class = 'dev.omavm.app' })\n"
                  "if #w > 0 then\n"
                  "  hl.dispatch(hl.dsp.focus({ workspace = w[1].workspace.id }))\n"
                  "end\n"),
              nullptr);
}

void Backend::showManager() const {
  QProcess::startDetached(QCoreApplication::applicationFilePath(), {});
}

void Backend::cloneEnvironment(const QString &name, const QString &newName) {
  // Keyed on the source: it is locked while it is copied, and its card
  // shows the stages.
  run(name, {QStringLiteral("clone"), name, newName, QStringLiteral("--progress")},
      QStringLiteral("cloning %1").arg(name), QStringLiteral("clone"));
}

void Backend::updateEnvironment(const QString &name) {
  run(name, {QStringLiteral("update"), name, QStringLiteral("--progress")},
      QStringLiteral("updating %1").arg(name), QStringLiteral("update"));
}

void Backend::prepareGuest(const QString &name) {
  run(name,
      {QStringLiteral("prepare"), name, QStringLiteral("--json"),
       QStringLiteral("--progress")},
      QStringLiteral("preparing %1").arg(name), QStringLiteral("prepare"),
      true, false);
}

void Backend::refreshHost() {
  // Read-only and quick: no busy state, and a failure leaves the map as it
  // was (the UI then simply says nothing).
  auto *process = new QProcess(this);
  connect(process, &QProcess::finished, this, [this, process](int code) {
    const QByteArray output = process->readAllStandardOutput();
    process->deleteLater();
    if (code != 0)
      return;
    QVariantMap next;
    for (const QJsonValue &value : QJsonDocument::fromJson(output).array()) {
      const QVariantMap capability = value.toObject().toVariantMap();
      next.insert(capability.value(QStringLiteral("id")).toString(),
                  capability);
    }
    if (next != m_hostCapabilities) {
      m_hostCapabilities = next;
      emit hostCapabilitiesChanged();
    }
  });
  connect(process, &QProcess::errorOccurred, process, &QObject::deleteLater);
  process->start(cliPath(), {QStringLiteral("host"), QStringLiteral("--json")});
}

void Backend::refreshApps(const QString &name) {
  if (!beginBusy(name,
                 QStringLiteral("loading applications for %1").arg(name)))
    return;
  auto *process = new QProcess(this);
  connect(process, &QProcess::errorOccurred, this,
          [this, process, name](QProcess::ProcessError error) {
            if (error != QProcess::FailedToStart)
              return;
            endBusy(name);
            const QString text = QStringLiteral("Could not start omavm: ") +
                                 process->errorString();
            m_appsError = text;
            m_appsEnvironment = name;
            emit appsChanged();
            emit message(text, true);
            process->deleteLater();
          });
  connect(process, &QProcess::finished, this, [this, process, name](int code) {
    const QByteArray output = process->readAllStandardOutput();
    const QString errorText =
        QString::fromUtf8(process->readAllStandardError()).trimmed();
    process->deleteLater();
    endBusy(name);
    if (code != 0) {
      m_appsError = errorText.isEmpty()
                        ? QStringLiteral("Could not list applications")
                        : errorText;
      m_appsEnvironment = name;
      emit appsChanged();
      emit message(m_appsError, true);
      return;
    }
    const QJsonDocument document = QJsonDocument::fromJson(output);
    QVariantList next;
    for (const QJsonValue &value : document.array())
      next.append(value.toObject().toVariantMap());
    m_apps = next;
    m_appsError.clear();
    m_appsEnvironment = name;
    emit appsChanged();
  });
  process->start(cliPath(),
                 {QStringLiteral("apps"), name, QStringLiteral("--json")});
}

void Backend::exportApp(const QString &name, const QString &id) {
  runForApps({QStringLiteral("apps"), name, QStringLiteral("--export"), id},
             name, QStringLiteral("exporting an application from %1").arg(name),
             QStringLiteral("apps-export"));
}

void Backend::unexportApp(const QString &name, const QString &id) {
  runForApps(
      {QStringLiteral("apps"), name, QStringLiteral("--unexport"), id}, name,
      QStringLiteral("removing an application shortcut from %1").arg(name),
      QStringLiteral("apps-unexport"));
}

void Backend::createSnapshot(const QString &name, const QString &label) {
  run(name, {QStringLiteral("snapshot"), QStringLiteral("create"), name,
       QStringLiteral("--label"), label},
      QStringLiteral("creating a snapshot of %1").arg(name),
      QStringLiteral("snapshot-create"));
}
void Backend::goToSnapshot(const QString &name, const QString &id) {
  run(name, {QStringLiteral("snapshot"), QStringLiteral("go-to"), name, id},
      QStringLiteral("going to a snapshot of %1").arg(name),
      QStringLiteral("snapshot-goto"));
}
void Backend::removeSnapshot(const QString &name, const QString &id) {
  run(name, {QStringLiteral("snapshot"), QStringLiteral("remove"), name, id},
      QStringLiteral("deleting a snapshot of %1").arg(name),
      QStringLiteral("snapshot-remove"));
}

void Backend::markAsTerminalWindow() const {
  if (!hyprlandAvailable())
    return;
  QProcess::startDetached(
      QStringLiteral("hyprctl"),
      {QStringLiteral("repl"),
       terminalTagScript(QCoreApplication::applicationPid())});
}

void Backend::copyToClipboard(const QString &text) const {
  if (auto *clipboard = QGuiApplication::clipboard())
    clipboard->setText(text);
}

void Backend::open(const QString &name, const QString &kind) {
  // The viewer itself picks the workspace (and goes fullscreen) when it
  // starts, so it happens however the environment is opened: here, from
  // its launcher entry or with `omavm open`.
  bool emptyWorkspace = true;
  bool shareClipboard = true;
  QString color;
  for (const QVariant &item : m_environments) {
    const QVariantMap env = item.toMap();
    if (env.value(QStringLiteral("name")).toString() == name) {
      const QVariantMap settings =
          env.value(QStringLiteral("settings")).toMap();
      emptyWorkspace =
          !settings.value(QStringLiteral("empty_workspace_disabled")).toBool();
      shareClipboard =
          !settings.value(QStringLiteral("clipboard_disabled")).toBool();
      color = settings.value(QStringLiteral("color")).toString();
      break;
    }
  }
  if (kind == QStringLiteral("box")) {
    // Relaunch this same omavm-gui binary in its embedded terminal mode
    // (gui/main.cpp's --terminal, gui/TerminalViewer.qml) — same pattern
    // as a Machine's Open spawning omavm-gui --display-fd
    // (internal/backend/machine/qemu/qemu.go), so a Box opens inside OmaVM's own
    // window instead of whatever external terminal emulator the user has
    // configured.
    const auto flag = [](bool on) {
      return on ? QStringLiteral("true") : QStringLiteral("false");
    };
    QStringList arguments{QStringLiteral("--terminal"), name,
                          QStringLiteral("--title"),
                          name + QStringLiteral(" — OmaVM"),
                          QStringLiteral("--share-clipboard"),
                          flag(shareClipboard),
                          QStringLiteral("--empty-workspace"),
                          flag(emptyWorkspace)};
    if (!color.isEmpty())
      arguments << QStringLiteral("--color") << color;
    QProcess::startDetached(QCoreApplication::applicationFilePath(),
                            arguments);
    return;
  }
  run(name, {QStringLiteral("open"), name}, QStringLiteral("opening %1").arg(name),
      QStringLiteral("open"));
}

void Backend::openEphemeral(const QString &name) {
  const QString tag = QStringLiteral("start-ephemeral:") + name;
  auto *connection = new QMetaObject::Connection;
  *connection = connect(
      this, &Backend::actionFinished, this,
      [this, name, tag, connection](const QString &finished, bool ok,
                                    const QString &) {
        if (finished != tag)
          return;
        disconnect(*connection);
        delete connection;
        if (ok)
          open(name, QStringLiteral("machine"));
      });
  run(name, {QStringLiteral("start"), name, QStringLiteral("--ephemeral")},
      QStringLiteral("starting %1 without keeping changes").arg(name), tag);
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
  // Color tags are drawn in the theme's own hues (a tag is a mark, not
  // text, so they stay as the theme has them); a name the theme lacks
  // keeps the plain color.
  m_themeTagColors.clear();
  for (const auto &[tag, key] :
       {std::pair{"red", "red"}, {"orange", "orange"}, {"yellow", "yellow"},
        {"green", "green"}, {"blue", "blue"}, {"purple", "magenta"},
        {"gray", "muted"}})
    m_themeTagColors.insert(QString::fromLatin1(tag),
                            values.value(QString::fromLatin1(key),
                                         QString::fromLatin1(tag)));

  const QColor background(m_themeBackground), surface(m_themeSurface);
  m_themeForeground = readableTextColor(QColor(m_themeForeground), background,
                                        surface).name();
  m_themeMuted = readableTextColor(QColor(m_themeMuted), background, surface)
                     .name();
  // Status colors are read as text ("Running", errors) and icons: WCAG
  // 1.4.6 (7:1) with their hue kept. The raw accent stays for Material
  // controls, which draw their own text on it.
  m_themeGreen = accessibleColor(QColor(m_themeGreen), background, surface)
                     .name();
  m_themeRed =
      accessibleColor(QColor(m_themeRed), background, surface).name();
  m_themeAccentText =
      accessibleColor(QColor(m_themeAccent), background, surface).name();

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
