#include "backend.h"
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
              const QString &title) {
  app.setApplicationName(QStringLiteral("dev.omavm.viewer"));
  app.setDesktopFileName(QStringLiteral("dev.omavm.viewer"));

  qmlRegisterType<VncView>("OmaVM", 1, 0, "VncView");

  QQmlApplicationEngine engine;
  engine.rootContext()->setContextProperty(QStringLiteral("vncSocketPath"),
                                            socketPath);
  engine.rootContext()->setContextProperty(QStringLiteral("vncTitle"), title);
  engine.load(QUrl(QStringLiteral("qrc:/Viewer.qml")));
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
  QCommandLineOption viewerOption(QStringLiteral("viewer"),
                                  QStringLiteral("Open a Machine's VNC display"),
                                  QStringLiteral("socket-path"));
  QCommandLineOption titleOption(QStringLiteral("title"),
                                 QStringLiteral("Viewer window title"),
                                 QStringLiteral("title"), QStringLiteral("OmaVM"));
  parser.addOption(viewerOption);
  parser.addOption(titleOption);
  parser.process(app);

  if (parser.isSet(viewerOption))
    return runViewer(app, parser.value(viewerOption), parser.value(titleOption));

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
