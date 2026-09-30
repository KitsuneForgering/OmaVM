#include "backend.h"
#include "terminalview.h"
#include "displayview.h"

#include <QCommandLineParser>
#include <QGuiApplication>
#include <QIcon>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickStyle>
#include <QQuickWindow>

namespace {
// Placement shared by both viewer modes, done before the window exists so
// it opens on the workspace switched to. Returns false when a window for
// this environment is already open (and was focused): no second one.
bool placeViewer(const QString &title, bool emptyWorkspace) {
  if (!emptyWorkspace)
    return true;
  switch (placeInEmptyWorkspace(title)) {
  case WorkspacePlacement::AlreadyFocusedExisting:
    return false;
  case WorkspacePlacement::Unavailable:
    qWarning("omavm: no empty workspace found; opening on the current one");
    break;
  case WorkspacePlacement::NotApplicable:
  case WorkspacePlacement::LaunchNormally:
    break;
  }
  return true;
}

// A Machine's display: a single view of QEMU's D-Bus display, launched by
// the qemu backend's Open() rather than by the user picking it from the
// Experience Center. Kept as a mode of this same binary so there's one
// Qt app to build and package instead of two. connectionFd is a socket
// QEMU already accepted (the backend hands it over as an inherited fd).
int runViewer(QGuiApplication &app, int connectionFd, const QString &title,
              bool shareClipboard, bool emptyWorkspace, bool fullscreen) {
  if (!placeViewer(title, emptyWorkspace))
    return 0;
  app.setApplicationName(QStringLiteral("dev.omavm.viewer"));
  app.setDesktopFileName(QStringLiteral("dev.omavm.viewer"));
  // Guest frames are imported as dma-bufs through EGL into GL textures.
  QQuickWindow::setGraphicsApi(QSGRendererInterface::OpenGL);

  qmlRegisterType<DisplayView>("OmaVM", 1, 0, "DisplayView");

  Backend backend;
  QQmlApplicationEngine engine;
  engine.rootContext()->setContextProperty(QStringLiteral("backend"), &backend);
  engine.rootContext()->setContextProperty(
      QStringLiteral("displayConnectionFd"), connectionFd);
  engine.rootContext()->setContextProperty(QStringLiteral("displayTitle"),
                                           title);
  // The Machine's own Settings (opt-out, on by default) decide this —
  // not a per-window checkbox the user has to remember to re-check.
  engine.rootContext()->setContextProperty(
      QStringLiteral("displayShareClipboard"), shareClipboard);
  // A personal window rule for the viewer (contrib/hypr/omavm-viewer.lua)
  // already makes it fullscreen: asking too would toggle it back off.
  engine.rootContext()->setContextProperty(
      QStringLiteral("displayFullscreen"),
      fullscreen && !hasPersonalViewerRule(hyprConfigDir()));
  engine.load(QUrl(QStringLiteral("qrc:/Viewer.qml")));
  if (engine.rootObjects().isEmpty())
    return -1;
  return app.exec();
}

// A Box's terminal: opened the same way as a Machine's display — this
// binary relaunching itself in a dedicated mode — instead of an external
// terminal emulator running `omavm open <name>` (gui/backend.cpp used to
// shell out to xdg-terminal-exec for this). Deliberately reuses the
// "dev.omavm.viewer" app id so the same opt-in Hyprland window rule
// (contrib/hypr/omavm-viewer.lua) covers both.
int runTerminal(QGuiApplication &app, const QString &envName,
                const QString &title, bool shareClipboard,
                bool emptyWorkspace) {
  if (!placeViewer(title, emptyWorkspace))
    return 0;
  app.setApplicationName(QStringLiteral("dev.omavm.viewer"));
  app.setDesktopFileName(QStringLiteral("dev.omavm.viewer"));

  qmlRegisterType<TerminalView>("OmaVM", 1, 0, "TerminalView");

  Backend backend;
  QQmlApplicationEngine engine;
  engine.rootContext()->setContextProperty(QStringLiteral("backend"), &backend);
  engine.rootContext()->setContextProperty(QStringLiteral("terminalEnvName"),
                                           envName);
  engine.rootContext()->setContextProperty(QStringLiteral("terminalTitle"),
                                           title);
  // Lets programs in the Box copy to the clipboard (OSC 52); the Box's
  // own opt-out setting.
  engine.rootContext()->setContextProperty(
      QStringLiteral("terminalShareClipboard"), shareClipboard);
  engine.load(QUrl(QStringLiteral("qrc:/TerminalViewer.qml")));
  if (engine.rootObjects().isEmpty())
    return -1;
  return app.exec();
}
} // namespace

int main(int argc, char *argv[]) {
  QGuiApplication app(argc, argv);
  app.setApplicationDisplayName(QStringLiteral("OmaVM"));
  app.setOrganizationName(QStringLiteral("OmaVM"));
  QQuickStyle::setStyle(QStringLiteral("Material"));

  QCommandLineParser parser;
  QCommandLineOption viewerOption(
      QStringLiteral("display-fd"),
      QStringLiteral("Show a Machine's display over an inherited connection"),
      QStringLiteral("fd"));
  QCommandLineOption terminalOption(
      QStringLiteral("terminal"),
      QStringLiteral("Open a Development Box's terminal"),
      QStringLiteral("environment-name"));
  QCommandLineOption titleOption(
      QStringLiteral("title"), QStringLiteral("Viewer window title"),
      QStringLiteral("title"), QStringLiteral("OmaVM"));
  QCommandLineOption shareClipboardOption(
      QStringLiteral("share-clipboard"),
      QStringLiteral("Whether to share the text clipboard with the guest, "
                     "or let programs in a Box copy to it"),
      QStringLiteral("bool"), QStringLiteral("true"));
  parser.addOption(viewerOption);
  parser.addOption(terminalOption);
  parser.addOption(titleOption);
  QCommandLineOption emptyWorkspaceOption(
      QStringLiteral("empty-workspace"),
      QStringLiteral("Open in an empty Hyprland workspace"),
      QStringLiteral("bool"), QStringLiteral("true"));
  QCommandLineOption fullscreenOption(
      QStringLiteral("fullscreen"),
      QStringLiteral("Open a Machine's display fullscreen"),
      QStringLiteral("bool"), QStringLiteral("true"));
  parser.addOption(shareClipboardOption);
  parser.addOption(emptyWorkspaceOption);
  parser.addOption(fullscreenOption);
  parser.process(app);

  const auto on = [&parser](const QCommandLineOption &option) {
    return parser.value(option) != QStringLiteral("false");
  };
  if (parser.isSet(viewerOption))
    return runViewer(app, parser.value(viewerOption).toInt(),
                     parser.value(titleOption), on(shareClipboardOption),
                     on(emptyWorkspaceOption), on(fullscreenOption));
  if (parser.isSet(terminalOption))
    return runTerminal(app, parser.value(terminalOption),
                       parser.value(titleOption), on(shareClipboardOption),
                       on(emptyWorkspaceOption));

  app.setApplicationName(QStringLiteral("dev.omavm.app"));
  app.setDesktopFileName(QStringLiteral("dev.omavm.app"));
  app.setWindowIcon(QIcon::fromTheme(QStringLiteral("dev.omavm.app")));

  Backend backend;
  // Closing the window hides it, but an in-flight action (Restart,
  // Create, ...) still finishes before the process exits.
  app.setQuitOnLastWindowClosed(false);
  QObject::connect(&app, &QGuiApplication::lastWindowClosed, &backend,
                   &Backend::requestQuit);
  QObject::connect(&backend, &Backend::readyToQuit, &app,
                   &QCoreApplication::quit);
  QQmlApplicationEngine engine;
  engine.rootContext()->setContextProperty(QStringLiteral("backend"), &backend);
  engine.load(QUrl(QStringLiteral("qrc:/Main.qml")));
  if (engine.rootObjects().isEmpty())
    return -1;
  return app.exec();
}
