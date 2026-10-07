#include <QAccessible>
#include <QQmlContext>
#include <QQuickItem>
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

signals:
  void actionFinished(const QString &tag, bool ok, const QString &text);

public:
  explicit FakeBackend(QObject *parent)
      : QQmlPropertyMap(this, parent) {}

  Q_INVOKABLE void refreshHost() {}
  Q_INVOKABLE void refreshImages() {}
  Q_INVOKABLE void downloadImage(const QString &os, const QString &release,
                                 const QString &edition) {
    insert(QStringLiteral("lastCall"),
           QVariantList{QStringLiteral("downloadImage"), os, release, edition});
  }
  Q_INVOKABLE void cloneEnvironment(const QString &name, const QString &newName) {
    insert(QStringLiteral("lastCall"),
           QVariantList{QStringLiteral("cloneEnvironment"), name, newName});
  }
  Q_INVOKABLE void refreshApps(const QString &) {}
  Q_INVOKABLE void exportApp(const QString &name, const QString &id) {
    insert(QStringLiteral("lastCall"),
           QVariantList{QStringLiteral("exportApp"), name, id});
  }
  Q_INVOKABLE void prepareGuest(const QString &name) {
    insert(QStringLiteral("lastCall"),
           QVariantList{QStringLiteral("prepareGuest"), name});
  }
  Q_INVOKABLE void unexportApp(const QString &name, const QString &id) {
    insert(QStringLiteral("lastCall"),
           QVariantList{QStringLiteral("unexportApp"), name, id});
  }

  // Lets a test finish an action the way the real Backend reports it.
  Q_INVOKABLE void finishAction(const QString &tag, bool ok,
                                const QString &text) {
    emit actionFinished(tag, ok, text);
  }

  Q_INVOKABLE void configure(const QString &name, const QString &description,
                             int cpus, bool cpusTouched, int memoryMiB,
                             bool memoryTouched, bool machine,
                             const QString &sharedPath, bool sharedReadOnly,
                             bool disconnectISO, const QString &color,
                             bool shareClipboard, bool travelMode, bool vulkan,
                             bool openInEmptyWorkspace, bool launcher,
                             bool ssh, bool fullscreen,
                             const QString &clipboardDirection = QString(),
                             bool sharedFolder = true) {
    insert(QStringLiteral("lastCall"),
           QVariantList{QStringLiteral("configure"), name, description, cpus,
                        cpusTouched, memoryMiB, memoryTouched, machine,
                        sharedPath, sharedReadOnly, disconnectISO, color,
                        shareClipboard, travelMode, vulkan,
                        openInEmptyWorkspace, launcher, ssh, fullscreen,
                        clipboardDirection, sharedFolder});
  }
  // Audits what a screen reader gets from the accessibility tree under
  // item: every visible control someone can act on needs a name, and
  // needs to be reachable with the keyboard. Returns one line per problem.
  Q_INVOKABLE QStringList accessibilityProblems(QQuickItem *item) const {
    QStringList problems;
    QAccessibleInterface *root = QAccessible::queryAccessibleInterface(item);
    if (root)
      audit(root, QString(), problems);
    return problems;
  }

  // What a screen reader reads for item: its name and whether it is
  // checkable and checked.
  Q_INVOKABLE QVariantMap accessibleState(QQuickItem *item) const {
    QAccessibleInterface *node = QAccessible::queryAccessibleInterface(item);
    if (!node)
      return {};
    const QAccessible::State state = node->state();
    return {{QStringLiteral("name"), node->text(QAccessible::Name)},
            {QStringLiteral("checkable"), bool(state.checkable)},
            {QStringLiteral("checked"), bool(state.checked)}};
  }

  static bool focusableWithin(QAccessibleInterface *node) {
    if (node->state().focusable)
      return true;
    for (int i = 0; i < node->childCount(); ++i)
      if (QAccessibleInterface *child = node->child(i))
        if (focusableWithin(child))
          return true;
    return false;
  }

  static void audit(QAccessibleInterface *node, const QString &path,
                    QStringList &problems) {
    const QAccessible::State state = node->state();
    if (state.invisible)
      return;
    const QAccessible::Role role = node->role();
    const QString name = node->text(QAccessible::Name).trimmed();
    const QString here =
        path + QLatin1Char('/') +
        (node->object() ? QString::fromLatin1(node->object()->metaObject()->className())
                        : QStringLiteral("?")) +
        (name.isEmpty() ? QString() : QStringLiteral("(%1)").arg(name));
    switch (role) {
    case QAccessible::Button:
    case QAccessible::CheckBox:
    case QAccessible::RadioButton:
    case QAccessible::EditableText:
    case QAccessible::SpinBox:
    case QAccessible::ComboBox:
    case QAccessible::MenuItem:
    case QAccessible::Slider:
      // A read-only part inside a composite control (a non-editable
      // ComboBox's text field) never takes focus, so nothing lands on it;
      // everything a screen reader can land on needs a name.
      if (name.isEmpty() &&
          (state.focusable || role != QAccessible::EditableText))
        problems << QStringLiteral("no accessible name: %1").arg(here);
      // Focus may land on a part of a composite control (Qt 6.4 puts a
      // SpinBox's focus on its text field and marks only that focusable).
      if (!state.disabled && !focusableWithin(node) &&
          role != QAccessible::MenuItem)
        problems << QStringLiteral("not reachable by keyboard: %1 \"%2\"")
                        .arg(here, name);
      break;
    default:
      break;
    }
    for (int i = 0; i < node->childCount(); ++i)
      if (QAccessibleInterface *child = node->child(i))
        audit(child, here, problems);
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
    // Builds the accessibility tree a screen reader would see.
    QAccessible::setActive(true);
  }

  void qmlEngineAvailable(QQmlEngine *engine) {
    auto *backend = new FakeBackend(engine);
    backend->insert(QStringLiteral("lastCall"), QVariantList());
    backend->insert(QStringLiteral("busy"), false);
    backend->insert(QStringLiteral("busyAction"), QString());
    backend->insert(QStringLiteral("busyEnvironments"), QVariantMap());
    backend->insert(QStringLiteral("progress"), QVariantMap());
    backend->insert(QStringLiteral("hostCapabilities"), QVariantMap());
    backend->insert(QStringLiteral("downloadableImages"), QVariantList());
    backend->insert(QStringLiteral("imagesLoading"), false);
    backend->insert(QStringLiteral("imagesError"), QString());
    for (const char *color :
         {"themeBackground", "themeForeground", "themeAccent", "themeAccentText",
          "themeSelection",
          "themeMuted", "themeSurface", "themeDarkSurface", "themeGreen",
          "themeRed"})
      backend->insert(QString::fromLatin1(color), QStringLiteral("#808080"));
    backend->insert(QStringLiteral("themeMode"), QStringLiteral("dark"));
    backend->insert(QStringLiteral("themeTagColors"),
                    QVariantMap{{QStringLiteral("blue"), QStringLiteral("#4f8dff")}});
    engine->rootContext()->setContextProperty(QStringLiteral("backend"),
                                              backend);
  }
};

QUICK_TEST_MAIN_WITH_SETUP(omavm_qml, Setup)
#include "qml_test.moc"
