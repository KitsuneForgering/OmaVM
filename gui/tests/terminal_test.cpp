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
  // Dropped files become shell words: spaces and quotes survive.
  void droppedFilesAreTypedAsQuotedPaths() {
    QCOMPARE(TerminalSession::droppedPaths(
                 {QStringLiteral("/home/u/My Notes.txt"),
                  QStringLiteral("/home/u/it's.png")}),
             QStringLiteral("'/home/u/My Notes.txt' '/home/u/it'\\''s.png' "));
  }

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

  // Every key the terminfo entry for TERM=xterm-256color declares, read
  // from the system's own database (infocmp), with the keypad in the mode
  // that entry's smkx sets: what ncurses programs in a Box (vim, htop,
  // less, tmux) will recognize, checked key by key.
  void everyKeyMatchesTheTerminfoEntry() {
    QProcess infocmp;
    infocmp.start(QStringLiteral("infocmp"),
                  {QStringLiteral("-1"), QStringLiteral("-x"),
                   QStringLiteral("xterm-256color")});
    if (!infocmp.waitForFinished(5000) || infocmp.exitCode() != 0)
      QSKIP("needs infocmp with the xterm-256color entry");
    QHash<QString, QByteArray> caps;
    const auto unescape = [](const QByteArray &v) {
      QByteArray out;
      for (qsizetype i = 0; i < v.size(); ++i) {
        if (v[i] == '\\' && i + 1 < v.size()) {
          const char c = v[++i];
          out += c == 'E' || c == 'e' ? '\x1b' : c;
        } else if (v[i] == '^' && i + 1 < v.size()) {
          const char c = v[++i];
          out += c == '?' ? '\x7f' : char(c & 0x1f);
        } else {
          out += v[i];
        }
      }
      return out;
    };
    for (QByteArray line : infocmp.readAllStandardOutput().split('\n')) {
      line = line.trimmed();
      if (line.endsWith(','))
        line.chop(1);
      const qsizetype eq = line.indexOf('=');
      if (eq > 0)
        caps.insert(QString::fromLatin1(line.left(eq)), unescape(line.mid(eq + 1)));
    }
    QVERIFY(caps.contains(QStringLiteral("smkx")));

    TerminalSession session;
    session.resize(80, 24);
    session.feed(caps.value(QStringLiteral("smkx")));

    struct Key {
      int key;
      const char *base; // terminfo name without modifiers
      const char *shift = nullptr;
    };
    const Key plain[] = {
        {Qt::Key_Up, "kcuu1"},    {Qt::Key_Down, "kcud1"},
        {Qt::Key_Right, "kcuf1"}, {Qt::Key_Left, "kcub1"},
        {Qt::Key_Home, "khome"},  {Qt::Key_End, "kend"},
        {Qt::Key_Insert, "kich1"}, {Qt::Key_Delete, "kdch1"},
        {Qt::Key_PageUp, "kpp"},  {Qt::Key_PageDown, "knp"},
    };
    int checked = 0;
    QStringList wrong;
    const auto check = [&](const QString &cap, int key,
                           Qt::KeyboardModifiers mods, const QString &text) {
      if (!caps.contains(cap))
        return;
      ++checked;
      const QByteArray got = session.keySequence(key, mods, text);
      if (got != caps.value(cap))
        wrong << QStringLiteral("%1: sends %2, terminfo says %3")
                     .arg(cap, QString::fromLatin1(got.toPercentEncoding()),
                          QString::fromLatin1(caps.value(cap).toPercentEncoding()));
    };
    for (const Key &k : plain)
      check(QString::fromLatin1(k.base), k.key, Qt::NoModifier, {});
    check(QStringLiteral("kbs"), Qt::Key_Backspace, Qt::NoModifier,
          QStringLiteral("\b"));
    check(QStringLiteral("kcbt"), Qt::Key_Backtab, Qt::ShiftModifier, {});
    // F1-F12, then the same with Shift (kf13-24), Ctrl (kf25-36),
    // Ctrl+Shift (kf37-48) and Alt (kf49-60).
    const Qt::KeyboardModifiers fmods[] = {
        Qt::NoModifier, Qt::ShiftModifier, Qt::ControlModifier,
        Qt::ControlModifier | Qt::ShiftModifier, Qt::AltModifier};
    for (int group = 0; group < 5; ++group)
      for (int f = 0; f < 12; ++f)
        check(QStringLiteral("kf%1").arg(group * 12 + f + 1), Qt::Key_F1 + f,
              fmods[group], {});
    // Arrows and Home/End with modifiers, xterm's extended names: the
    // number is 1 + Shift(1) + Alt(2) + Ctrl(4).
    const struct {
      int key;
      const char *name;
    } extended[] = {{Qt::Key_Up, "kUP"},   {Qt::Key_Down, "kDN"},
                    {Qt::Key_Left, "kLFT"}, {Qt::Key_Right, "kRIT"},
                    {Qt::Key_Home, "kHOM"}, {Qt::Key_End, "kEND"},
                    {Qt::Key_Delete, "kDC"}, {Qt::Key_Insert, "kIC"},
                    {Qt::Key_PageUp, "kPRV"}, {Qt::Key_PageDown, "kNXT"}};
    for (const auto &e : extended) {
      for (int n = 2; n <= 7; ++n) {
        Qt::KeyboardModifiers mods;
        if ((n - 1) & 1)
          mods |= Qt::ShiftModifier;
        if ((n - 1) & 2)
          mods |= Qt::AltModifier;
        if ((n - 1) & 4)
          mods |= Qt::ControlModifier;
        // Shift alone has the bare name (kUP, kDC); the others a suffix.
        const QString cap = QString::fromLatin1(e.name) +
                            (n == 2 ? QString() : QString::number(n));
        check(cap, e.key, mods, {});
      }
    }
    QVERIFY2(checked > 100, qPrintable(QStringLiteral("only %1 keys checked").arg(checked)));
    QVERIFY2(wrong.isEmpty(), qPrintable(wrong.join(QLatin1Char('\n'))));
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
  // The window keeps Alacritty's scrollback and font-size keys; everything
  // else, Page Up alone and Ctrl+Shift+0 included, still reaches the program.
  void viewShortcuts() {
    using VS = TerminalSession::ViewShortcut;
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_PageUp, Qt::ShiftModifier), VS::PageUp);
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_PageDown, Qt::ShiftModifier), VS::PageDown);
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_Home, Qt::ShiftModifier), VS::Top);
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_End, Qt::ShiftModifier), VS::Bottom);
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_Equal, Qt::ControlModifier), VS::ZoomIn);
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_Plus, Qt::ControlModifier | Qt::ShiftModifier), VS::ZoomIn);
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_Minus, Qt::ControlModifier), VS::ZoomOut);
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_0, Qt::ControlModifier), VS::ZoomReset);
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_PageUp, Qt::NoModifier), VS::None);
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_0, Qt::ControlModifier | Qt::ShiftModifier), VS::None);
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_Minus, Qt::ControlModifier | Qt::ShiftModifier), VS::None);
    QCOMPARE(TerminalSession::viewShortcut(Qt::Key_C, Qt::ControlModifier), VS::None);
  }

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

  // Regression: ESC ( B was read as ESC plus a printed "B", and sgr0 is
  // ESC ( B ESC [ m in xterm-256color — every attribute reset in vim,
  // htop or less left a "B" behind. ESC ( 0 is ncurses' line drawing
  // (smacs): htop's and tmux's borders came out as "lqqk".
  void lineDrawingAndCharsetSwitches() {
    TerminalSession session;
    session.resize(20, 4);
    session.feed(QByteArrayLiteral("\x1b(0lqk\r\nx x\r\nmqj\x1b(B ok\x1b(B\x1b[m"));
    QStringList rows;
    for (int row = 0; row < 3; ++row) {
      QString line;
      for (int col = 0; col < 6; ++col)
        line += cellText(session, row, col);
      rows << line.trimmed();
    }
    QCOMPARE(rows, (QStringList{QStringLiteral("┌─┐"), QStringLiteral("│ │"),
                                QStringLiteral("└─┘ ok")}));
    // SO/SI switch to G1 and back.
    TerminalSession shifted;
    shifted.resize(20, 2);
    shifted.feed(QByteArrayLiteral("\x1b)0a\x0eq\x0fq"));
    QCOMPARE(cellText(shifted, 0, 0) + cellText(shifted, 0, 1) + cellText(shifted, 0, 2),
             QStringLiteral("a─q"));
  }

  // Everything the xterm-256color entry says this terminal understands,
  // as tput writes it: none of it may leave text on the screen (an
  // unknown sequence typically shows up as "[3m" or "?1049h" garbage).
  void everyTerminfoCapabilityIsUnderstood() {
    const QList<QStringList> caps = {
        {"bel"}, {"blink"}, {"bold"}, {"cbt"}, {"civis"}, {"clear"},
        {"cnorm"}, {"cr"}, {"csr", "2", "20"}, {"cub", "3"}, {"cub1"},
        {"cud", "2"}, {"cud1"}, {"cuf", "4"}, {"cuf1"}, {"cup", "5", "10"},
        {"cuu", "1"}, {"cuu1"}, {"cvvis"}, {"dch", "2"}, {"dch1"}, {"dim"},
        {"dl", "1"}, {"dl1"}, {"ech", "3"}, {"ed"}, {"el"}, {"el1"},
        {"flash"}, {"home"}, {"hpa", "10"}, {"ht"}, {"hts"}, {"ich", "2"},
        {"il", "1"}, {"il1"}, {"ind"}, {"indn", "2"}, {"invis"}, {"nel"},
        {"oc"}, {"op"}, {"rc"}, {"rev"}, {"ri"}, {"rin", "2"}, {"ritm"},
        {"rmacs"}, {"rmam"}, {"rmcup"}, {"rmir"}, {"rmkx"}, {"rmm"},
        {"rmso"}, {"rmul"}, {"sc"}, {"setab", "21"}, {"setaf", "196"},
        {"sgr", "1", "1", "1", "1", "0", "1", "0", "0", "0"}, {"sgr0"},
        {"sitm"}, {"smacs"}, {"smam"}, {"smcup"}, {"smir"}, {"smkx"},
        {"smm"}, {"smso"}, {"smul"}, {"tbc"}, {"vpa", "5"},
        {"smxx"}, {"rmxx"}, {"E3"}, {"BD"}, {"BE"}, {"fd"}, {"fe"},
        {"Ss", "2"}, {"Se"}, {"rs1"}, {"rs2"}, {"is2"}};
    int checked = 0;
    QStringList garbage;
    for (const QStringList &cap : caps) {
      QProcess tput;
      tput.start(QStringLiteral("tput"),
                 QStringList{QStringLiteral("-T"), QStringLiteral("xterm-256color")} + cap);
      if (!tput.waitForFinished(5000))
        QSKIP("needs tput");
      const QByteArray bytes = tput.readAllStandardOutput();
      if (tput.exitCode() != 0 || bytes.isEmpty())
        continue; // not in this ncurses' entry
      ++checked;
      TerminalSession session;
      session.resize(80, 24);
      session.feed(bytes);
      for (int row = 0; row < 24; ++row)
        for (int col = 0; col < 80; ++col)
          if (!cellText(session, row, col).trimmed().isEmpty()) {
            garbage << QStringLiteral("%1 left \"%2\" at %3,%4 (sent %5)")
                           .arg(cap.join(QLatin1Char(' ')), cellText(session, row, col))
                           .arg(row).arg(col)
                           .arg(QString::fromLatin1(bytes.toPercentEncoding()));
            row = 24;
            break;
          }
    }
    QVERIFY2(checked > 60, qPrintable(QStringLiteral("only %1 capabilities checked").arg(checked)));
    QVERIFY2(garbage.isEmpty(), qPrintable(garbage.join(QLatin1Char('\n'))));
  }

  // What a program in the terminal learns about it from its environment:
  // the terminfo entry it follows, and that it draws 24-bit color.
  void programsSeeTheTerminalsCapabilities() {
    TerminalSession session;
    QSignalSpy finished(&session, &TerminalSession::finished);
    session.startProgram("/bin/sh", {"-c", "printf '%s %s' \"$TERM\" \"$COLORTERM\""}, 80, 24);
    QTRY_VERIFY_WITH_TIMEOUT(finished.count() > 0, 5000);
    QString line;
    for (int col = 0; col < 80; ++col)
      line += cellText(session, 0, col);
    QCOMPARE(line.trimmed(), QStringLiteral("xterm-256color truecolor"));
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

  // Regression: write() ignored short writes on the non-blocking PTY, so
  // a paste larger than what the program had read so far lost its tail.
  void largeWriteReachesTheProgramWhole() {
    TerminalSession session;
    // Raw mode first: the canonical line discipline itself drops what
    // passes 4095 bytes in one line. The program then sleeps without
    // reading, so the PTY fills up and write() has to wait for it.
    session.startProgram("/bin/sh",
                         {"-c", "stty raw -echo; printf R; sleep 1; "
                                "head -c 200000 | wc -c"},
                         80, 24);
    QTRY_VERIFY_WITH_TIMEOUT(cellText(session, 0, 0) == QStringLiteral("R"),
                             5000);
    session.write(QByteArray(200000, 'x'));
    QTRY_VERIFY_WITH_TIMEOUT(
        session.text(0, 1, 0, 79).trimmed() == QStringLiteral("200000"), 10000);
  }
};

QTEST_GUILESS_MAIN(TerminalTest)
#include "terminal_test.moc"
