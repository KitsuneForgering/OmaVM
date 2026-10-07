#include "../backend.h"
#include "../singleinstance.h"
#include "../colorstoml.h"

#include <QFile>
#include <QDir>
#include <QDirIterator>
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
  void actionsOnDifferentEnvironmentsRunTogether();
  void creationReportsProgressStages();
  void secondLaunchActivatesTheFirst();
  void everyThemeTextMeetsAAA();
};

// Regression: status colors were used as text straight from the theme —
// red errors, green "Running", accent labels — and most Omarchy themes
// put them under WCAG 1.4.6's 7:1 (red under it in 20 of the 22 themes
// shipped on 2026-09-29, 4 of them under even 3:1). Every theme
// installed here is checked, plus a fixed set for machines without
// Omarchy, the colors taken from its nord, catppuccin-latte and miasma.
void BackendRefreshTest::everyThemeTextMeetsAAA() {
  QStringList themes;
  QDirIterator it(QStringLiteral("/usr/share/omarchy/themes"),
                  {QStringLiteral("colors.toml")}, QDir::Files,
                  QDirIterator::Subdirectories);
  while (it.hasNext())
    themes << QString::fromUtf8(
        [&] {
          QFile f(it.next());
          return f.open(QIODevice::ReadOnly) ? f.readAll() : QByteArray();
        }());
  themes << QStringLiteral(
                "mode = \"dark\"\naccent = \"#81a1c1\"\nmuted = \"#4c566a\"\n"
                "background = \"#2e3440\"\nlighter_background = \"#3b4252\"\n"
                "foreground = \"#d8dee9\"\nred = \"#bf616a\"\ngreen = \"#a3be8c\"\n")
         << QStringLiteral(
                "mode = \"light\"\naccent = \"#1e66f5\"\nmuted = \"#9ca0b0\"\n"
                "background = \"#eff1f5\"\nlighter_background = \"#e6e9ef\"\n"
                "foreground = \"#4c4f69\"\nred = \"#d20f39\"\ngreen = \"#40a02b\"\n")
         << QStringLiteral(
                "mode = \"dark\"\naccent = \"#78824b\"\nmuted = \"#666666\"\n"
                "background = \"#222222\"\nlighter_background = \"#333333\"\n"
                "foreground = \"#c2c2b0\"\nred = \"#685742\"\ngreen = \"#5f875f\"\n");

  const QByteArray oldHome = qgetenv("HOME");
  for (const QString &theme : std::as_const(themes)) {
    QTemporaryDir home;
    QVERIFY(home.isValid());
    const QString dir =
        home.filePath(QStringLiteral(".local/state/omarchy/current/theme"));
    QVERIFY(QDir().mkpath(dir));
    QFile colors(dir + QStringLiteral("/colors.toml"));
    QVERIFY(colors.open(QIODevice::WriteOnly));
    colors.write(theme.toUtf8());
    colors.close();
    qputenv("HOME", home.path().toUtf8());
    Backend backend;
    const QColor background(backend.themeBackground()),
        surface(backend.themeSurface());
    for (const QString &text :
         {backend.themeForeground(), backend.themeMuted(),
          backend.themeAccentText(), backend.themeGreen(),
          backend.themeRed()}) {
      const double ratio =
          minimumContrastRatio(QColor(text), background, surface);
      QVERIFY2(ratio >= 7.0, qPrintable(QStringLiteral("%1 at %2:1 in\n%3")
                                            .arg(text)
                                            .arg(ratio, 0, 'f', 2)
                                            .arg(theme)));
    }
  }
  qputenv("HOME", oldHome);
}

// Actions used to run one at a time across the whole window: a Box
// pulling its image for minutes disabled every other card. Different
// environments now run together; a second action on the same one is
// refused with what is running, and quitting waits for all of them.
void BackendRefreshTest::actionsOnDifferentEnvironmentsRunTogether() {
  QTemporaryDir dir;
  QVERIFY(dir.isValid());
  const QString log = dir.filePath(QStringLiteral("log"));
  QFile cli(dir.filePath(QStringLiteral("omavm")));
  QVERIFY(cli.open(QIODevice::WriteOnly));
  cli.write("#!/bin/sh\n"
            "case \"$1\" in\n"
            "  start) echo \"begin $2\" >> \"$OMAVM_TEST_LOG\"; sleep 0.4;\n"
            "         echo \"end $2\" >> \"$OMAVM_TEST_LOG\" ;;\n"
            "  list) printf '[]' ;;\n"
            "esac\n");
  cli.close();
  QVERIFY(cli.setPermissions(QFile::ReadOwner | QFile::WriteOwner |
                             QFile::ExeOwner));
  qputenv("OMAVM_TEST_LOG", log.toUtf8());
  qputenv("PATH", dir.path().toUtf8() + ':' + qgetenv("PATH"));

  Backend backend;
  QSignalSpy finished(&backend, &Backend::actionFinished);
  QSignalSpy ready(&backend, &Backend::readyToQuit);
  backend.start(QStringLiteral("a"));
  backend.start(QStringLiteral("b"));
  QCOMPARE(finished.size(), 0); // neither refused
  QVERIFY(backend.busyEnvironments().contains(QStringLiteral("a")));
  QVERIFY(backend.busyEnvironments().contains(QStringLiteral("b")));

  backend.start(QStringLiteral("a"));
  QCOMPARE(finished.size(), 1);
  QCOMPARE(finished.at(0).at(1).toBool(), false);
  QVERIFY(finished.at(0).at(2).toString().contains(QStringLiteral("starting a")));

  backend.requestQuit();
  QTRY_COMPARE_WITH_TIMEOUT(ready.size(), 1, 5000);
  QFile out(log);
  QVERIFY(out.open(QIODevice::ReadOnly));
  const QList<QByteArray> lines = out.readAll().trimmed().split('\n');
  QCOMPARE(lines.size(), 4);
  // Both began before either ended.
  QVERIFY(lines.at(0).startsWith("begin") && lines.at(1).startsWith("begin"));
  QVERIFY(backend.busyEnvironments().isEmpty());
}

// A Box's image download used to show only "Creating…" for minutes. The
// stages `create --progress` prints reach the card while it runs, and are
// neither shown as the command's result nor left behind once it ends.
void BackendRefreshTest::creationReportsProgressStages() {
  QTemporaryDir dir;
  QVERIFY(dir.isValid());
  const QString args = dir.filePath(QStringLiteral("args"));
  QFile cli(dir.filePath(QStringLiteral("omavm")));
  QVERIFY(cli.open(QIODevice::WriteOnly));
  cli.write("#!/bin/sh\n"
            "case \"$1\" in\n"
            "  create) echo \"$@\" > \"$OMAVM_TEST_ARGS\";\n"
            "          echo 'progress: Downloading fedora: layer 1'; sleep 0.5;\n"
            "          echo 'created demo (box, backend=distrobox)' ;;\n"
            "  list) printf '[]' ;;\n"
            "esac\n");
  cli.close();
  QVERIFY(cli.setPermissions(QFile::ReadOwner | QFile::WriteOwner |
                             QFile::ExeOwner));
  qputenv("OMAVM_TEST_ARGS", args.toUtf8());
  qputenv("PATH", dir.path().toUtf8() + ':' + qgetenv("PATH"));

  Backend backend;
  QSignalSpy finished(&backend, &Backend::actionFinished);
  backend.createEnvironment(QStringLiteral("demo"), QStringLiteral("fedora"),
                            QStringLiteral("box"), 2, false, 2048, false);
  QTRY_COMPARE_WITH_TIMEOUT(
      backend.progress().value(QStringLiteral("demo")).toString(),
      QStringLiteral("Downloading fedora: layer 1"), 3000);
  QTRY_COMPARE_WITH_TIMEOUT(finished.size(), 1, 5000);
  QCOMPARE(finished.at(0).at(2).toString(),
           QStringLiteral("created demo (box, backend=distrobox)"));
  QVERIFY(backend.progress().isEmpty());
  QFile recorded(args);
  QVERIFY(recorded.open(QIODevice::ReadOnly));
  QVERIFY(recorded.readAll().contains("--progress"));
}

// The launcher clicked twice, or "Open OmaVM" from a viewer, brings the
// running Experience Center forward instead of opening a second one; a
// socket file left by a crash doesn't block the next launch.
void BackendRefreshTest::secondLaunchActivatesTheFirst() {
  QTemporaryDir dir;
  QVERIFY(dir.isValid());
  const QString path = dir.filePath(QStringLiteral("omavm-gui.sock"));

  QFile stale(path);
  QVERIFY(stale.open(QIODevice::WriteOnly));
  stale.close();
  SingleInstance first(path);
  QVERIFY(!first.notifyRunning());
  QVERIFY(first.listen());

  QSignalSpy activated(&first, &SingleInstance::activated);
  SingleInstance second(path);
  QVERIFY(second.notifyRunning());
  QTRY_COMPARE_WITH_TIMEOUT(activated.size(), 1, 2000);
}

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
  // A color with meaning keeps its hue: a red error stays red.
  const QColor red = accessibleColor(QColor("#bf616a"), QColor("#2e3440"),
                                     QColor("#3b4252"));
  QVERIFY(minimumContrastRatio(red, QColor("#2e3440"), QColor("#3b4252")) >=
          7.0);
  QVERIFY(red.red() > red.green() && red.red() > red.blue());

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
                     "foreground = \"#717a86\"\nmuted = \"#717a86\"\n"
                     "magenta = \"#c77dff\"\n"));

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
  // Color tags take the theme's hues; one the theme lacks stays plain.
  QCOMPARE(backend.themeTagColors().value(QStringLiteral("purple")).toString(),
           QStringLiteral("#c77dff"));
  QCOMPARE(backend.themeTagColors().value(QStringLiteral("orange")).toString(),
           QStringLiteral("orange"));

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
      "  printf '[{\"id\":\"one\",\"name\":\"demo\",\"kind\":\"box\","
      "\"settings\":{},\"status\":{\"state\":\"stopped\"}}]'\n"
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
      "\"settings\":{},\"status\":{\"state\":\"running\"},"
      "\"integration\":{\"guest_agent\":\"unavailable\"}}]' ;;\n"
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
  QTRY_COMPARE(backend.environments().first().toMap().value("guestAgent"),
               QVariant(QStringLiteral("unavailable")));
  backend.poll();
  QTRY_COMPARE(count("list"), 2);
  QTest::qWait(100);
  QCOMPARE(count("preview"), 1);
  // One process per poll: status and guest tools come with the list.
  QCOMPARE(count("status") + count("integration"), 0);
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
