#include "../terminal.h"

#include <QRandomGenerator>
#include <QSignalSpy>
#include <QtTest>

#include <csignal>

namespace {
QString cellText(const TerminalSession &session, int row, int col) {
  return session.grid().at(row).at(col).ch;
}
} // namespace

class TerminalTest : public QObject {
  Q_OBJECT
private slots:
  // The viewer closes itself only on a clean exit, so the exit code must
  // be real: a failure keeps `omavm open`'s error on screen.
  void finishedReportsTheExitCode() {
    TerminalSession session;
    QSignalSpy finished(&session, &TerminalSession::finished);
    session.startProgram("/bin/sh", {"-c", "echo boom; exit 3"}, 80, 24);
    QTRY_COMPARE(finished.size(), 1);
    QCOMPARE(finished.at(0).at(0).toInt(), 3);
    QCOMPARE(cellText(session, 0, 0), QStringLiteral("b"));
  }

  // "Try Again" in the viewer runs the program again on the same session.
  void sessionCanRunAgainAfterItEnds() {
    TerminalSession session;
    QSignalSpy finished(&session, &TerminalSession::finished);
    session.startProgram("/bin/sh", {"-c", "exit 2"}, 80, 24);
    QTRY_COMPARE(finished.size(), 1);
    QVERIFY(!session.running());
    session.startProgram("/bin/sh", {"-c", "echo again"}, 80, 24);
    QVERIFY(session.running());
    QTRY_COMPARE(finished.size(), 2);
    QCOMPARE(finished.at(1).at(0).toInt(), 0);
    QCOMPARE(cellText(session, 0, 0), QStringLiteral("a"));
  }

  // Killed by a signal is not a success: reported like a shell does,
  // 128 + the signal number.
  void finishedReportsDeathBySignal() {
    TerminalSession session;
    QSignalSpy finished(&session, &TerminalSession::finished);
    session.startProgram("/bin/sh", {"-c", "kill -TERM $$"}, 80, 24);
    QTRY_COMPARE(finished.size(), 1);
    QCOMPARE(finished.at(0).at(0).toInt(), 128 + SIGTERM);
  }

  // ncurses programs (htop, mc) switch the cursor keys to application
  // mode (smkx = \E[?1h for TERM=xterm-256color) and then only recognize
  // the SS3 form the terminfo declares (kcuu1=\EOA).
  void cursorKeysFollowApplicationMode() {
    TerminalSession session;
    session.resize(80, 24);
    QCOMPARE(session.keySequence(Qt::Key_Up, Qt::NoModifier, {}),
             QByteArray("\x1b[A"));
    session.feed(QByteArrayLiteral("\x1b[?1h"));
    QCOMPARE(session.keySequence(Qt::Key_Up, Qt::NoModifier, {}),
             QByteArray("\x1bOA"));
    QCOMPARE(session.keySequence(Qt::Key_Home, Qt::NoModifier, {}),
             QByteArray("\x1bOH"));
    session.feed(QByteArrayLiteral("\x1b[?1l"));
    QCOMPARE(session.keySequence(Qt::Key_Left, Qt::NoModifier, {}),
             QByteArray("\x1b[D"));
  }

  // xterm encodes modifiers as a parameter: Ctrl+Left jumps a word in
  // bash/zsh, which a plain \x1b[D cannot express.
  void modifiedKeysUseXtermParameters() {
    TerminalSession session;
    session.resize(80, 24);
    QCOMPARE(session.keySequence(Qt::Key_Left, Qt::ControlModifier, {}),
             QByteArray("\x1b[1;5D"));
    QCOMPARE(session.keySequence(Qt::Key_Up, Qt::ShiftModifier, {}),
             QByteArray("\x1b[1;2A"));
    QCOMPARE(session.keySequence(Qt::Key_Delete, Qt::ControlModifier, {}),
             QByteArray("\x1b[3;5~"));
    QCOMPARE(session.keySequence(Qt::Key_F1, Qt::ShiftModifier, {}),
             QByteArray("\x1b[1;2P"));
    // Modifiers override application mode, as in xterm.
    session.feed(QByteArrayLiteral("\x1b[?1h"));
    QCOMPARE(session.keySequence(Qt::Key_Right,
                                 Qt::ControlModifier | Qt::AltModifier, {}),
             QByteArray("\x1b[1;7C"));
  }

  void controlAndAltCharacters() {
    TerminalSession session;
    session.resize(80, 24);
    QCOMPARE(session.keySequence(Qt::Key_Backtab, Qt::ShiftModifier, {}),
             QByteArray("\x1b[Z"));
    QCOMPARE(session.keySequence(Qt::Key_C, Qt::ControlModifier,
                                 QStringLiteral("\x03")),
             QByteArray("\x03"));
    QCOMPARE(session.keySequence(Qt::Key_Space, Qt::ControlModifier, {}),
             QByteArray(1, '\0'));
    QCOMPARE(session.keySequence(Qt::Key_BracketLeft, Qt::ControlModifier, {}),
             QByteArray("\x1b"));
    QCOMPARE(session.keySequence(Qt::Key_Underscore, Qt::ControlModifier, {}),
             QByteArray("\x1f"));
    QCOMPARE(session.keySequence(Qt::Key_X, Qt::AltModifier, QStringLiteral("x")),
             QByteArray("\x1bx"));
    QCOMPARE(session.keySequence(Qt::Key_Aacute, Qt::NoModifier,
                                 QStringLiteral("á")),
             QStringLiteral("á").toUtf8());
    QCOMPARE(session.keySequence(Qt::Key_Shift, Qt::ShiftModifier, {}),
             QByteArray());
  }

  // Omarchy's universal paste sends Shift+Insert to terminals; Ctrl+Shift+V
  // is the other common paste chord. Neither may reach the program as keys.
  void pasteShortcuts() {
    QVERIFY(TerminalSession::isPasteShortcut(Qt::Key_Insert, Qt::ShiftModifier));
    QVERIFY(TerminalSession::isPasteShortcut(
        Qt::Key_V, Qt::ControlModifier | Qt::ShiftModifier));
    QVERIFY(!TerminalSession::isPasteShortcut(Qt::Key_V, Qt::ControlModifier));
    QVERIFY(!TerminalSession::isPasteShortcut(Qt::Key_Insert, Qt::NoModifier));
  }

  // Omarchy's universal copy sends Ctrl+Insert; Ctrl+Shift+C is the other
  // common chord. Plain Ctrl+C must still reach the program as SIGINT.
  void copyShortcuts() {
    QVERIFY(TerminalSession::isCopyShortcut(Qt::Key_Insert, Qt::ControlModifier));
    QVERIFY(TerminalSession::isCopyShortcut(
        Qt::Key_C, Qt::ControlModifier | Qt::ShiftModifier));
    QVERIFY(!TerminalSession::isCopyShortcut(Qt::Key_C, Qt::ControlModifier));
    QVERIFY(!TerminalSession::isPasteShortcut(Qt::Key_Insert,
                                              Qt::ControlModifier));
  }

  void selectedTextSpansLinesAndDropsTrailingBlanks() {
    TerminalSession session;
    session.resize(20, 4);
    session.feed(QByteArrayLiteral("hello world\r\nsecond line"));
    const int top = session.bufferLineCount() - 4;
    QCOMPARE(session.text(top, 6, top, 19), QStringLiteral("world"));
    QCOMPARE(session.text(top, 6, top + 1, 5),
             QStringLiteral("world\nsecond"));
    // Dragging backwards selects the same text.
    QCOMPARE(session.text(top + 1, 5, top, 6),
             QStringLiteral("world\nsecond"));
  }

  // A selection never splits a wide character.
  void selectionKeepsWideCharactersWhole() {
    TerminalSession session;
    session.resize(20, 2);
    session.feed(QStringLiteral("a日本b").toUtf8());
    const int top = session.bufferLineCount() - 2;
    // Column 2 is the right half of 日.
    QCOMPARE(session.text(top, 2, top, 4), QStringLiteral("日本"));
  }

  void doubleClickSelectsWordsAndPaths() {
    TerminalSession session;
    session.resize(40, 2);
    session.feed(QByteArrayLiteral("cd /usr/local/bin && ls"));
    const int top = session.bufferLineCount() - 2;
    QCOMPARE(session.wordBounds(top, 8), qMakePair(3, 16));
    QCOMPARE(session.wordBounds(top, 0), qMakePair(0, 1));
    // A blank selects only itself.
    QCOMPARE(session.wordBounds(top, 2), qMakePair(2, 2));
  }

  // Neovim and tmux copy through OSC 52; reading the clipboard back ("?")
  // would let anything in the Box see what the user copied on the host.
  void osc52WritesButNeverReads() {
    TerminalSession session;
    session.resize(80, 24);
    QSignalSpy spy(&session, &TerminalSession::clipboardWriteRequested);
    session.feed("\x1b]52;c;" + QByteArray("olá").toBase64() + "\x07");
    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.at(0).at(0).toString(), QStringLiteral("olá"));
    session.feed(QByteArrayLiteral("\x1b]52;c;?\x07"));
    session.feed(QByteArrayLiteral("\x1b]52;c;not base64!\x1b\\"));
    QCOMPARE(spy.count(), 1);
  }

  // Regression (found by fuzzing under ASan): huge CSI parameters
  // overflowed int, moved the cursor to a negative column, and the next
  // erase wrote outside the row — a crash any program in the Box, or a
  // `cat` of a hostile file, could trigger.
  void hugeParametersKeepTheCursorInside() {
    TerminalSession session;
    session.resize(20, 5);
    // From column 3, 3 + INT_MAX overflowed to a negative column.
    session.feed(QByteArrayLiteral("abc\x1b[2147483647C\x1b[K"));
    QCOMPARE(session.cursorCol(), 19);
    session.feed(QByteArrayLiteral("\x1b[99999999999999999999D\x1b[1K"));
    QCOMPARE(session.cursorCol(), 0);
    session.feed(QByteArrayLiteral("\x1b[2147483647B\x1b[99999999999X"
                                   "\x1b[99999999999@\x1b[99999999999P"
                                   "\x1b[99999999999L\x1b[99999999999M"));
    QCOMPARE(session.cursorRow(), 4);
    session.feed(QByteArrayLiteral("\x1b[99999999999;99999999999Hok"));
    QCOMPARE(session.cursorRow(), 4);
    QVERIFY(session.cursorCol() >= 0 && session.cursorCol() < 20);
  }

  // A short fuzz with a fixed seed: random escape fragments, text and
  // resizes must never crash or leave the cursor outside the screen. (The
  // longer run that found the overflow above used ASan/UBSan.)
  void randomInputKeepsTheScreenConsistent() {
    static const char *const frags[] = {
        "\x1b[", "\x1b]", "\x1b[?1049h", "\x1b[?1049l", "\x1b[5;2r",
        "\x1b[999;999H", "\x1b[99L", "\x1b[99M", "\x1b[99@", "\x1b[99P",
        "\x1b[99X", "\x1b[J", "\x1b[1K", "\x1b[99S", "\x1b[99T", "\x1b" "7",
        "\x1b" "8", "\x1bM", "\x1b" "c", "\x1b[38;5;300m", "\x1b[38;2;1m",
        "\x1b[2147483647C", "\x1b[99999999999D", "\x1b]52;c;?\x07", "\r",
        "\n", "\t", "\b", "a", "日", "e\xcc\x81", "\xcc\x81", "\xe2\x80",
        "\xf0\x9f\x98\x80", "\xff", "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"};
    const int count = int(sizeof(frags) / sizeof(*frags));
    QRandomGenerator rng(20260929);
    for (int it = 0; it < 300; ++it) {
      TerminalSession session;
      session.resize(1 + rng.bounded(90), 1 + rng.bounded(40));
      for (int k = 0; k < 150; ++k) {
        if (rng.bounded(30) == 0)
          session.resize(1 + rng.bounded(90), 1 + rng.bounded(40));
        else
          session.feed(QByteArray(frags[rng.bounded(count)]));
      }
      QVERIFY(session.cursorRow() >= 0 && session.cursorRow() < session.rows());
      QVERIFY(session.cursorCol() >= 0 && session.cursorCol() < session.cols());
      QCOMPARE(session.grid().size(), session.rows());
      const int lines = session.bufferLineCount();
      session.text(0, 0, lines - 1, session.cols() - 1);
      session.wordBounds(lines - 1, rng.bounded(session.cols()));
    }
  }

  // An OSC that never ends must not grow memory without bound, and must
  // not swallow what comes after its terminator.
  void oversizedOscIsDropped() {
    TerminalSession session;
    session.resize(80, 24);
    QSignalSpy spy(&session, &TerminalSession::clipboardWriteRequested);
    session.feed("\x1b]52;c;" +
                 QByteArray(TerminalSession::kMaxOscLength + 10, 'A') +
                 "\x07ok");
    QCOMPARE(spy.count(), 0);
    QCOMPARE(session.grid().at(0).at(0).ch, QStringLiteral("o"));
  }

  // Bash and zsh turn on bracketed paste (\e[?2004h) so a pasted
  // multi-line snippet is inserted, not run line by line.
  void pasteIsBracketedWhenTheProgramAsks() {
    TerminalSession session;
    session.resize(80, 24);
    QCOMPARE(session.pasteSequence(QStringLiteral("ls\nrm x")),
             QByteArray("ls\rrm x"));
    session.feed(QByteArrayLiteral("\x1b[?2004h"));
    QCOMPARE(session.pasteSequence(QStringLiteral("ls\nrm x")),
             QByteArray("\x1b[200~ls\rrm x\x1b[201~"));
    // A pasted end marker must not end the bracket early and let the rest
    // run as typed input.
    QCOMPARE(session.pasteSequence(QStringLiteral("a\x1b[201~b")),
             QByteArray("\x1b[200~ab\x1b[201~"));
    session.feed(QByteArrayLiteral("\x1b[?2004l"));
    QCOMPARE(session.pasteSequence(QStringLiteral("x")), QByteArray("x"));
  }

  // Wide characters take two columns for the program writing them (the
  // shell, vim, a starship prompt with emoji); counting them as one put
  // every following character in the wrong column.
  void wideCharactersTakeTwoColumns() {
    TerminalSession session;
    session.resize(80, 24);
    session.feed(QStringLiteral("日本x").toUtf8());
    QCOMPARE(cellText(session, 0, 0), QStringLiteral("日"));
    QCOMPARE(cellText(session, 0, 1), QString());
    QCOMPARE(cellText(session, 0, 2), QStringLiteral("本"));
    QCOMPARE(cellText(session, 0, 4), QStringLiteral("x"));
    QCOMPARE(session.cursorCol(), 5);

    TerminalSession emoji;
    emoji.resize(80, 24);
    emoji.feed(QStringLiteral("\U0001F600a").toUtf8());
    QCOMPARE(cellText(emoji, 0, 0), QStringLiteral("\U0001F600"));
    QCOMPARE(cellText(emoji, 0, 2), QStringLiteral("a"));
    QCOMPARE(emoji.cursorCol(), 3);
  }

  void wideCharacterAtLastColumnWraps() {
    TerminalSession session;
    session.resize(5, 3);
    session.feed(QStringLiteral("abcd日").toUtf8());
    QCOMPARE(cellText(session, 0, 3), QStringLiteral("d"));
    QCOMPARE(cellText(session, 1, 0), QStringLiteral("日"));
    QCOMPARE(session.cursorRow(), 1);
    QCOMPARE(session.cursorCol(), 2);
  }

  // A combining accent belongs to the previous character's cell.
  void combiningMarksJoinThePreviousCell() {
    TerminalSession session;
    session.resize(80, 24);
    session.feed(QStringLiteral("e\u0301x").toUtf8());
    QCOMPARE(cellText(session, 0, 0), QStringLiteral("e\u0301"));
    QCOMPARE(cellText(session, 0, 1), QStringLiteral("x"));
    QCOMPARE(session.cursorCol(), 2);
  }

  // Overwriting either half of a wide character erases the other half.
  void overwritingHalfAWideCharacterClearsIt() {
    TerminalSession session;
    session.resize(80, 24);
    session.feed(QStringLiteral("日\ra").toUtf8());
    QCOMPARE(cellText(session, 0, 0), QStringLiteral("a"));
    QCOMPARE(cellText(session, 0, 1), QStringLiteral(" "));

    TerminalSession second;
    second.resize(80, 24);
    second.feed(QStringLiteral("日\r\x1b[2Cb").toUtf8());
    second.feed(QStringLiteral("\r\x1b[1Cc").toUtf8());
    QCOMPARE(cellText(second, 0, 0), QStringLiteral(" "));
    QCOMPARE(cellText(second, 0, 1), QStringLiteral("c"));
  }

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
