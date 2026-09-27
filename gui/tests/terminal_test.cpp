#include "../terminal.h"

#include <QSignalSpy>
#include <QtTest>

namespace {
QString cellText(const TerminalSession &session, int row, int col) {
  return session.grid().at(row).at(col).ch;
}
} // namespace

class TerminalTest : public QObject {
  Q_OBJECT
private slots:
  void plainTextAndNewline() {
    TerminalSession session;
    session.resize(80, 24);
    session.feed(QByteArrayLiteral("Hello\r\nWorld"));
    QCOMPARE(cellText(session, 0, 0), QStringLiteral("H"));
    QCOMPARE(cellText(session, 0, 4), QStringLiteral("o"));
    QCOMPARE(cellText(session, 1, 0), QStringLiteral("W"));
    QCOMPARE(session.cursorRow(), 1);
    QCOMPARE(session.cursorCol(), 5);
  }

  void sgrBasicColorAndReset() {
    TerminalSession session;
    session.resize(80, 24);
    session.feed(QByteArrayLiteral("\x1b[31mR\x1b[0mN"));
    const TerminalCell red = session.grid().at(0).at(0);
    QCOMPARE(int(red.fg.kind), int(TerminalColorKind::Palette));
    QCOMPARE(int(red.fg.index), 1);
    const TerminalCell normal = session.grid().at(0).at(1);
    QCOMPARE(int(normal.fg.kind), int(TerminalColorKind::Default));
  }

  void sgr256AndTruecolor() {
    TerminalSession session;
    session.resize(80, 24);
    session.feed(QByteArrayLiteral("\x1b[38;5;196mA\x1b[38;2;10;20;30mB"));
    const TerminalCell a = session.grid().at(0).at(0);
    QCOMPARE(int(a.fg.kind), int(TerminalColorKind::Palette));
    QCOMPARE(int(a.fg.index), 196);
    const TerminalCell b = session.grid().at(0).at(1);
    QCOMPARE(int(b.fg.kind), int(TerminalColorKind::True));
    QCOMPARE(b.fg.rgb, qRgb(10, 20, 30));
  }

  void cursorAddressing() {
    TerminalSession session;
    session.resize(80, 24);
    session.feed(QByteArrayLiteral("\x1b[5;10HX"));
    QCOMPARE(cellText(session, 4, 9), QStringLiteral("X"));
  }

  void alternateScreenBuffer() {
    TerminalSession session;
    session.resize(80, 24);
    session.feed(QByteArrayLiteral("primary"));
    QVERIFY(!session.alternateScreen());
    session.feed(QByteArrayLiteral("\x1b[?1049h"));
    QVERIFY(session.alternateScreen());
    // Entering the alt screen doesn't imply "cursor home" (matches real
    // xterm/vte: DEC 1049 saves+clears but leaves cursor coordinates
    // alone) — a real full-screen app always repositions before drawing,
    // same as this test does.
    session.feed(QByteArrayLiteral("\x1b[H"));
    session.feed(QByteArrayLiteral("alt"));
    QCOMPARE(cellText(session, 0, 0), QStringLiteral("a"));
    session.feed(QByteArrayLiteral("\x1b[?1049l"));
    QVERIFY(!session.alternateScreen());
    // Primary screen content survives a round trip through the alt screen.
    QCOMPARE(cellText(session, 0, 0), QStringLiteral("p"));
  }

  void scrollbackAccumulates() {
    TerminalSession session;
    session.resize(20, 5);
    for (int i = 0; i < 20; ++i)
      session.feed(QByteArrayLiteral("line\r\n"));
    QVERIFY(session.scrollback().size() > 0);
  }

  void titleOsc() {
    TerminalSession session;
    session.resize(80, 24);
    QSignalSpy titles(&session, &TerminalSession::titleChanged);
    session.feed(QByteArrayLiteral("\x1b]0;My Title\x07"));
    QCOMPARE(titles.count(), 1);
    QCOMPARE(titles.at(0).at(0).toString(), QStringLiteral("My Title"));
  }

  void splitEscapeSequenceAcrossFeeds() {
    TerminalSession session;
    session.resize(80, 24);
    session.feed(QByteArrayLiteral("\x1b[3"));
    session.feed(QByteArrayLiteral("1mR"));
    const TerminalCell red = session.grid().at(0).at(0);
    QCOMPARE(int(red.fg.kind), int(TerminalColorKind::Palette));
    QCOMPARE(int(red.fg.index), 1);
  }

  void splitUtf8CharacterAcrossFeeds() {
    TerminalSession session;
    session.resize(80, 24);
    // "é" (U+00E9) as UTF-8: 0xC3 0xA9, split across two feed() calls.
    session.feed(QByteArray("\xc3", 1));
    session.feed(QByteArray("\xa9", 1));
    QCOMPARE(cellText(session, 0, 0), QString::fromUtf8("\xc3\xa9"));
  }

  void realPtyEchoAndExit() {
    TerminalSession session;
    QSignalSpy updates(&session, &TerminalSession::updated);
    QSignalSpy finished(&session, &TerminalSession::finished);
    session.startProgram("/bin/sh", {"-c", "printf 'hi'"}, 80, 24);
    QTRY_VERIFY_WITH_TIMEOUT(finished.count() > 0, 5000);
    QCOMPARE(finished.at(0).at(0).toInt(), 0);
    QVERIFY(updates.count() > 0);
    QCOMPARE(cellText(session, 0, 0), QStringLiteral("h"));
    QCOMPARE(cellText(session, 0, 1), QStringLiteral("i"));
  }

  // Exercises TerminalSession::write() — the same call TerminalView's
  // keyPressEvent makes — against a real PTY: /bin/cat echoes whatever
  // it reads on stdin straight back to stdout, so bytes written in
  // should come back out through the parser into the grid.
  void realPtyWriteIsEchoedBack() {
    TerminalSession session;
    QSignalSpy updates(&session, &TerminalSession::updated);
    session.startProgram("/bin/cat", {}, 80, 24);
    session.write(QByteArrayLiteral("typed\n"));
    QTRY_VERIFY_WITH_TIMEOUT(updates.count() > 0 &&
                                 cellText(session, 0, 0) == QStringLiteral("t"),
                             5000);
    QCOMPARE(cellText(session, 0, 0), QStringLiteral("t"));
    QCOMPARE(cellText(session, 0, 4), QStringLiteral("d"));
    session.write(QByteArrayLiteral("\x04")); // EOF (Ctrl+D) to end cat
  }
};

QTEST_GUILESS_MAIN(TerminalTest)
#include "terminal_test.moc"
