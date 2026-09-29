#include "../backend.h"
#include "../colorstoml.h"

#include <QFile>
#include <QDir>
#include <QSignalSpy>
#include <QTemporaryDir>
#include <QTest>

#include <array>
#include <cmath>

class BackendRefreshTest : public QObject {
  Q_OBJECT

private slots:
  void textColorsMeetEnhancedContrast();
  void themeChangeKeepsReadableText();
  void unchangedPollDoesNotFlash();
  void failedPreviewIsNotRetriedEveryPoll();
  void quitWaitsForRunningAction();
  void pickedFilesBecomeLocalPaths();
  void boxTerminalIsTaggedAsTerminal();
  void onlyActiveViewerRulesCount();
};

// Regression: Omarchy's Super+C reached a Box's terminal as Ctrl+C
// (SIGINT) because the window lacked the "terminal" tag.
void BackendRefreshTest::boxTerminalIsTaggedAsTerminal() {
  const QString script = terminalTagScript(4242);
  QVERIFY(script.contains(QStringLiteral("w.pid == 4242")));
  QVERIFY(script.contains(QStringLiteral("w.class == 'dev.omavm.viewer'")));
  QVERIFY(script.contains(QStringLiteral("tag = '+terminal'")));
  QVERIFY(script.contains(QStringLiteral("window = 'address:' .. w.address")));
}

// A personal rule for the viewer takes over workspace placement, but a
// commented-out one must not.
void BackendRefreshTest::onlyActiveViewerRulesCount() {
  QTemporaryDir dir;
  QVERIFY(dir.isValid());
  QVERIFY(QDir().mkpath(dir.filePath(QStringLiteral("sub"))));
  const auto write = [&dir](const QString &name, const QByteArray &text) {
    QFile file(dir.filePath(name));
    QVERIFY(file.open(QIODevice::WriteOnly | QIODevice::Truncate));
    file.write(text);
  };
  write(QStringLiteral("windows.lua"),
        "-- o.window(\"dev.omavm.viewer\", { fullscreen = true })\n"
        "o.window(\"firefox\", {}) -- not dev.omavm.viewer\n");
  write(QStringLiteral("sub/old.conf"),
        "# windowrule = fullscreen, class:dev.omavm.viewer\n");
  QVERIFY(!hasPersonalViewerRule(dir.path()));
  write(QStringLiteral("sub/rules.lua"),
        "o.window(\"dev.omavm.viewer\", { workspace = \"name:omavm\" })\n");
  QVERIFY(hasPersonalViewerRule(dir.path()));
  QVERIFY(!hasPersonalViewerRule(dir.filePath(QStringLiteral("missing"))));
}

void BackendRefreshTest::textColorsMeetEnhancedContrast() {
  const auto ratio = [](QColor a, QColor b) {
    const auto luminance = [](QColor c) {
      const auto channel = [](int value) {
        const double x = value / 255.0;
        return x <= 0.04045 ? x / 12.92 : std::pow((x + 0.055) / 1.055, 2.4);
      };
      return 0.2126 * channel(c.red()) + 0.7152 * channel(c.green()) +
             0.0722 * channel(c.blue());
    };
    const double x = luminance(a), y = luminance(b);
    return (qMax(x, y) + 0.05) / (qMin(x, y) + 0.05);
  };
  for (const auto &colors : {
           std::array<const char *, 3>{"#717a86", "#08090a", "#16181b"},
           {"#808080", "#f7f7f7", "#ffffff"},
           {"#dddddd", "#101010", "#202020"},
       }) {
    const QColor wanted(colors[0]), background(colors[1]), surface(colors[2]);
    const QColor actual = readableTextColor(wanted, background, surface);
    QVERIFY2(ratio(actual, background) >= 7.0, colors[1]);
    QVERIFY2(ratio(actual, surface) >= 7.0, colors[2]);
    if (ratio(wanted, background) >= 7.0 && ratio(wanted, surface) >= 7.0)
      QCOMPARE(actual, wanted);
  }
}

void BackendRefreshTest::themeChangeKeepsReadableText() {
  QTemporaryDir home;
  QVERIFY(home.isValid());
  const QString themeDir = home.filePath(
      QStringLiteral(".local/state/omarchy/current/theme"));
  QVERIFY(QDir().mkpath(themeDir));
  QFile colors(themeDir + QStringLiteral("/colors.toml"));
  const auto writeTheme = [&colors](const QByteArray &contents) {
    if (!colors.open(QIODevice::WriteOnly | QIODevice::Truncate))
      return false;
    const bool ok = colors.write(contents) == contents.size();
    colors.close();
    return ok;
  };
  QVERIFY(writeTheme("mode = \"dark\"\nbackground = \"#08090a\"\n"
                     "lighter_background = \"#16181b\"\n"
                     "foreground = \"#717a86\"\nmuted = \"#717a86\"\n"));

  const QByteArray oldHome = qgetenv("HOME");
  qputenv("HOME", home.path().toUtf8());
  struct RestoreHome {
    QByteArray value;
    ~RestoreHome() {
      if (value.isNull())
        qunsetenv("HOME");
      else
        qputenv("HOME", value);
    }
  } restore{oldHome};

  Backend backend;
  QCOMPARE(backend.themeMode(), QStringLiteral("dark"));
  QCOMPARE(backend.themeForeground(), QStringLiteral("#ffffff"));
  QCOMPARE(backend.themeMuted(), QStringLiteral("#ffffff"));

  QVERIFY(writeTheme("mode = \"light\"\nbackground = \"#f7f7f7\"\n"
                     "lighter_background = \"#ffffff\"\n"
                     "foreground = \"#777777\"\nmuted = \"#777777\"\n"));
  QTRY_COMPARE(backend.themeMode(), QStringLiteral("light"));
  QCOMPARE(backend.themeForeground(), QStringLiteral("#000000"));
  QCOMPARE(backend.themeMuted(), QStringLiteral("#000000"));
}

void BackendRefreshTest::unchangedPollDoesNotFlash() {
  QTemporaryDir dir;
  QVERIFY(dir.isValid());
  const QString marker = dir.filePath(QStringLiteral("polls"));
  QFile cli(dir.filePath(QStringLiteral("omavm")));
  QVERIFY(cli.open(QIODevice::WriteOnly));
  cli.write(
      "#!/bin/sh\n"
      "if [ \"$1\" = list ]; then\n"
      "  printf 'x' >> \"$OMAVM_TEST_POLLS\"\n"
            "  printf '[{\"id\":\"one\",\"name\":\"demo\",\"kind\":\"box\",\"settings\":{}}]'\n"
      "elif [ \"$1\" = status ]; then\n"
      "  printf '{\"state\":\"stopped\"}'\n"
      "fi\n");
  cli.close();
  QVERIFY(cli.setPermissions(QFile::ReadOwner | QFile::WriteOwner |
                             QFile::ExeOwner));
  qputenv("OMAVM_TEST_POLLS", marker.toUtf8());
  qputenv("PATH", dir.path().toUtf8() + ':' + qgetenv("PATH"));

  Backend backend;
  QSignalSpy busy(&backend, &Backend::busyChanged);
  QSignalSpy changed(&backend, &Backend::environmentsChanged);
  backend.poll();
  QTRY_VERIFY(!backend.environments().isEmpty());
  QTRY_COMPARE(
      backend.environments().first().toMap().value("status").toString(),
      QStringLiteral("stopped"));
  QCOMPARE(busy.size(), 0);
  const int firstChanges = changed.size();

  backend.poll();
  QTRY_VERIFY([&] {
    QFile count(marker);
    return count.open(QIODevice::ReadOnly) && count.readAll().size() == 2;
  }());
  QTest::qWait(100);
  QCOMPARE(changed.size(), firstChanges);
  QCOMPARE(busy.size(), 0);
}

void BackendRefreshTest::failedPreviewIsNotRetriedEveryPoll() {
  QTemporaryDir dir;
  QVERIFY(dir.isValid());
  const QString marker = dir.filePath(QStringLiteral("calls"));
  QFile cli(dir.filePath(QStringLiteral("omavm")));
  QVERIFY(cli.open(QIODevice::WriteOnly));
  cli.write(
      "#!/bin/sh\n"
      "printf '%s\\n' \"$1\" >> \"$OMAVM_TEST_CALLS\"\n"
      "case \"$1\" in\n"
      "  list) printf '[{\"id\":\"m\",\"name\":\"vm\",\"kind\":\"machine\","
      "\"settings\":{}}]' ;;\n"
      "  status) printf '{\"state\":\"running\"}' ;;\n"
      "  integration) printf '{\"guest_agent\":\"unavailable\"}' ;;\n"
      "  preview) echo 'no surface' >&2; exit 1 ;;\n"
      "esac\n");
  cli.close();
  QVERIFY(cli.setPermissions(QFile::ReadOwner | QFile::WriteOwner |
                             QFile::ExeOwner));
  qputenv("OMAVM_TEST_CALLS", marker.toUtf8());
  qputenv("PATH", dir.path().toUtf8() + ':' + qgetenv("PATH"));

  const auto count = [&](const QByteArray &command) {
    QFile calls(marker);
    if (!calls.open(QIODevice::ReadOnly))
      return 0;
    return int(calls.readAll().split('\n').count(command));
  };
  Backend backend;
  backend.poll();
  QTRY_COMPARE(count("preview"), 1);
  QTRY_COMPARE(count("integration"), 1);
  backend.poll();
  QTRY_COMPARE(count("integration"), 2);
  QTest::qWait(100);
  QCOMPARE(count("preview"), 1);
}

// Closing the Experience Center mid-action used to destroy the QProcess,
// which SIGKILLs `omavm`: a Restart cut between its Stop and Start left
// the Machine powered off.
void BackendRefreshTest::quitWaitsForRunningAction() {
  QTemporaryDir dir;
  QVERIFY(dir.isValid());
  const QString marker = dir.filePath(QStringLiteral("restarted"));
  QFile cli(dir.filePath(QStringLiteral("omavm")));
  QVERIFY(cli.open(QIODevice::WriteOnly));
  cli.write("#!/bin/sh\n"
            "if [ \"$1\" = restart ]; then sleep 0.5; : > \"$OMAVM_TEST_MARKER\"; fi\n"
            "[ \"$1\" = list ] && printf '[]'\n"
            "exit 0\n");
  cli.close();
  QVERIFY(cli.setPermissions(QFile::ReadOwner | QFile::WriteOwner |
                             QFile::ExeOwner));
  qputenv("OMAVM_TEST_MARKER", marker.toUtf8());
  qputenv("PATH", dir.path().toUtf8() + ':' + qgetenv("PATH"));

  {
    Backend backend;
    QSignalSpy ready(&backend, &Backend::readyToQuit);
    backend.restart(QStringLiteral("vm"));
    QVERIFY(backend.busy());
    backend.requestQuit();
    QTest::qWait(50);
    QCOMPARE(ready.size(), 0);
    QTRY_COMPARE(ready.size(), 1);
    QVERIFY(QFile::exists(marker));
  }

  Backend idle;
  QSignalSpy ready(&idle, &Backend::readyToQuit);
  idle.requestQuit();
  QTRY_COMPARE(ready.size(), 1);
}

// File pickers hand QML a URL; stripping "file://" by hand left spaces and
// accents percent-encoded ("Minhas%20ISOs"), a path QEMU cannot open.
void BackendRefreshTest::pickedFilesBecomeLocalPaths() {
  Backend backend;
  const QString path = QStringLiteral("/home/user/Minhas ISOs/Fedora ção #1.iso");
  QCOMPARE(backend.localPath(QUrl::fromLocalFile(path)), path);
  QCOMPARE(backend.localPath(QUrl(QStringLiteral(
               "file:///home/user/Minhas%20ISOs/a%25b.iso"))),
           QStringLiteral("/home/user/Minhas ISOs/a%b.iso"));
  QCOMPARE(backend.localPath(QUrl()), QString());
}

QTEST_GUILESS_MAIN(BackendRefreshTest)
#include "backend_refresh_test.moc"
