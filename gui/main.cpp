#include "backend.h"
#include "terminalview.h"
#include "vncview.h"

#include <QCommandLineParser>
#include <QGuiApplication>
#include <QIcon>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickStyle>

namespace {
// A Machine's display: a single fullscreen VNC view, launched by the
// qemu backend's Open() rather than by the user picking it from the
// Experience Center. Kept as a mode of this same binary so there's one
// Qt app to build and package instead of two.
int runViewer(QGuiApplication &app, const QString &socketPath,
              const QString &title, bool shareClipboard) {
  app.setApplicationName(QStringLiteral("dev.omavm.viewer"));
  app.setDesktopFileName(QStringLiteral("dev.omavm.viewer"));

  qmlRegisterType<VncView>("OmaVM", 1, 0, "VncView");

  QQmlApplicationEngine engine;
  engine.rootContext()->setContextProperty(QStringLiteral("vncSocketPath"),
                                           socketPath);
  engine.rootContext()->setContextProperty(QStringLiteral("vncTitle"), title);
  // The Machine's own Settings (opt-out, on by default) decide this —
  // not a per-window checkbox the user has to remember to re-check.
  engine.rootContext()->setContextProperty(QStringLiteral("vncShareClipboard"),
                                           shareClipboard);
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
                const QString &title) {
  app.setApplicationName(QStringLiteral("dev.omavm.viewer"));
  app.setDesktopFileName(QStringLiteral("dev.omavm.viewer"));

  qmlRegisterType<TerminalView>("OmaVM", 1, 0, "TerminalView");

  QQmlApplicationEngine engine;
  engine.rootContext()->setContextProperty(QStringLiteral("terminalEnvName"),
                                           envName);
  engine.rootContext()->setContextProperty(QStringLiteral("terminalTitle"),
                                           title);
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
      QStringLiteral("viewer"), QStringLiteral("Open a Machine's VNC display"),
      QStringLiteral("socket-path"));
  QCommandLineOption terminalOption(
      QStringLiteral("terminal"),
      QStringLiteral("Open a Development Box's terminal"),
      QStringLiteral("environment-name"));
  QCommandLineOption titleOption(
      QStringLiteral("title"), QStringLiteral("Viewer window title"),
      QStringLiteral("title"), QStringLiteral("OmaVM"));
  QCommandLineOption shareClipboardOption(
      QStringLiteral("share-clipboard"),
      QStringLiteral("Whether to share the text clipboard with the guest"),
      QStringLiteral("bool"), QStringLiteral("true"));
  parser.addOption(viewerOption);
  parser.addOption(terminalOption);
  parser.addOption(titleOption);
  parser.addOption(shareClipboardOption);
  parser.process(app);

  if (parser.isSet(viewerOption))
    return runViewer(app, parser.value(viewerOption), parser.value(titleOption),
                     parser.value(shareClipboardOption) !=
                         QStringLiteral("false"));
  if (parser.isSet(terminalOption))
    return runTerminal(app, parser.value(terminalOption),
                       parser.value(titleOption));

  app.setApplicationName(QStringLiteral("dev.omavm.app"));
  app.setDesktopFileName(QStringLiteral("dev.omavm.app"));
  app.setWindowIcon(QIcon::fromTheme(QStringLiteral("dev.omavm.app")));

  Backend backend;
  QQmlApplicationEngine engine;
  engine.rootContext()->setContextProperty(QStringLiteral("backend"), &backend);
  engine.load(QUrl(QStringLiteral("qrc:/Main.qml")));
  if (engine.rootObjects().isEmpty())
    return -1;
  return app.exec();
}
