#include <QQmlContext>
#include <QQmlEngine>
#include <QQmlPropertyMap>
#include <QQuickStyle>
#include <QUrl>
#include <QtQuickTest>

// Stand-in for the C++ Backend context property: the properties the
// components read, and the actions they call recorded in lastCall (name
// followed by the arguments) instead of running the CLI.
class FakeBackend : public QQmlPropertyMap {
  Q_OBJECT

public:
  explicit FakeBackend(QObject *parent)
      : QQmlPropertyMap(this, parent) {}

  Q_INVOKABLE void configure(const QString &name, const QString &description,
                             int cpus, bool cpusTouched, int memoryMiB,
                             bool memoryTouched, bool machine,
                             const QString &sharedPath, bool sharedReadOnly,
                             bool disconnectISO, const QString &color,
                             bool shareClipboard, bool travelMode, bool vulkan,
                             bool openInEmptyWorkspace, bool launcher,
                             bool ssh) {
    insert(QStringLiteral("lastCall"),
           QVariantList{QStringLiteral("configure"), name, description, cpus,
                        cpusTouched, memoryMiB, memoryTouched, machine,
                        sharedPath, sharedReadOnly, disconnectISO, color,
                        shareClipboard, travelMode, vulkan,
                        openInEmptyWorkspace, launcher, ssh});
  }
  Q_INVOKABLE QString localPath(const QUrl &url) const {
    return url.toLocalFile();
  }
};

// Runs the QML UI tests in gui/tests/qml against the real components from
// gui/resources.qrc.
class Setup : public QObject {
  Q_OBJECT

public slots:
  void applicationAvailable() {
    QQuickStyle::setStyle(QStringLiteral("Material"));
  }

  void qmlEngineAvailable(QQmlEngine *engine) {
    auto *backend = new FakeBackend(engine);
    backend->insert(QStringLiteral("lastCall"), QVariantList());
    backend->insert(QStringLiteral("busy"), false);
    backend->insert(QStringLiteral("busyAction"), QString());
    for (const char *color :
         {"themeBackground", "themeForeground", "themeAccent", "themeSelection",
          "themeMuted", "themeSurface", "themeDarkSurface", "themeGreen",
          "themeRed"})
      backend->insert(QString::fromLatin1(color), QStringLiteral("#808080"));
    backend->insert(QStringLiteral("themeMode"), QStringLiteral("dark"));
    engine->rootContext()->setContextProperty(QStringLiteral("backend"),
                                              backend);
  }
};

QUICK_TEST_MAIN_WITH_SETUP(omavm_qml, Setup)
#include "qml_test.moc"
