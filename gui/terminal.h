#pragma once

#include <QByteArray>
#include <QList>
#include <QObject>
#include <QRgb>
#include <QString>
#include <QVector>

class QSocketNotifier;

// One character cell's color: either "whatever the default fg/bg is"
// (Default), a 0-255 ANSI/xterm palette index (Palette — 0-15 are the
// standard 16 colors, 16-231 the 6x6x6 cube, 232-255 grayscale), or an
// explicit 24-bit color from a `38;2;r;g;b` truecolor SGR (True).
enum class TerminalColorKind : quint8 { Default, Palette, True };

struct TerminalColor {
  TerminalColorKind kind = TerminalColorKind::Default;
  quint8 index = 0;
  QRgb rgb = 0;

  bool operator==(const TerminalColor &other) const {
    return kind == other.kind && index == other.index && rgb == other.rgb;
  }
};

struct TerminalCell {
  QString ch = QStringLiteral(" ");
  TerminalColor fg;
  TerminalColor bg;
  bool bold = false;
  bool underline = false;
  bool reverse = false;
  bool faint = false;
};

// Owns a PTY and the child process attached to it (`omavm open <name>`,
// unchanged — see internal/backend/distrobox), and turns the raw byte
// stream coming back from that child into a grid of TerminalCell plus a
// cursor position, via a small Ground/Escape/CSI/OSC state machine. See
// gui/README-worthy comment in terminal.cpp for the exact escape-sequence
// coverage; this is not a general-purpose terminfo-complete emulator.
//
// feed() is exposed publicly (not just wired to the PTY read notifier) so
// tests can push raw byte sequences straight into the parser without a
// live PTY.
class TerminalSession : public QObject {
  Q_OBJECT

public:
  explicit TerminalSession(QObject *parent = nullptr);
  ~TerminalSession() override;

  // Spawns `<cli> open <envName>` (cli resolved the same way
  // Backend::cliPath() does) attached to a fresh PTY sized cols x rows.
  void start(const QString &envName, int cols, int rows);

  // Spawns an arbitrary program attached to a fresh PTY. start() is a
  // thin wrapper around this for the omavm CLI case; exposed directly so
  // tests can exercise the PTY/parser pipeline with a trivial program
  // instead of the real CLI.
  void startProgram(const QByteArray &program, const QList<QByteArray> &args,
                    int cols, int rows);

  void feed(const QByteArray &bytes);
  void write(const QByteArray &bytes);
  void resize(int cols, int rows);

  int cols() const { return m_cols; }
  int rows() const { return m_rows; }
  int cursorRow() const { return m_cursorRow; }
  int cursorCol() const { return m_cursorCol; }
  bool cursorVisible() const { return m_cursorVisible; }
  bool running() const { return m_masterFd >= 0; }
  // Bytes to send to the child for a key press, following the modes the
  // program set (application cursor keys) and xterm's modifier encoding,
  // which is what TERM=xterm-256color promises. Empty for keys that send
  // nothing (a lone modifier).
  QByteArray keySequence(int key, Qt::KeyboardModifiers modifiers,
                         const QString &text) const;
  // Shift+Insert (what Omarchy's universal paste sends to terminals) or
  // Ctrl+Shift+V.
  static bool isPasteShortcut(int key, Qt::KeyboardModifiers modifiers);
  // Bytes to send for pasted text: newlines as Enter, wrapped in bracketed
  // paste markers when the program enabled them.
  QByteArray pasteSequence(const QString &text) const;
  // Ctrl+Shift+C, or Ctrl+Insert (what Omarchy's universal copy sends to
  // terminals). Never forwarded to the program: Ctrl+Shift+C would
  // otherwise reach it as Ctrl+C.
  static bool isCopyShortcut(int key, Qt::KeyboardModifiers modifiers);

  // The scrollback followed by the visible screen, as one list of lines.
  int bufferLineCount() const;
  QVector<TerminalCell> bufferLine(int index) const;
  // Lines dropped from the front of the scrollback so far: an index plus
  // this stays attached to the same line as more output arrives.
  qint64 droppedLines() const { return m_droppedLines; }
  // Text from (startLine, startCol) to (endLine, endCol), both inclusive,
  // in buffer lines. Trailing blanks of each line are dropped, as every
  // terminal does; a wide character is copied whole.
  QString text(int startLine, int startCol, int endLine, int endCol) const;
  // The columns [first, last] of the word under (line, col), for
  // double-click selection. A blank cell selects just itself.
  QPair<int, int> wordBounds(int line, int col) const;
  // Largest OSC 52 payload accepted (base64 text), about 1 MiB of text.
  static constexpr int kMaxOscLength = 1400 * 1024;

  bool alternateScreen() const { return m_altScreenActive; }
  const QVector<QVector<TerminalCell>> &grid() const {
    return m_altScreenActive ? m_altGrid : m_grid;
  }
  const QList<QVector<TerminalCell>> &scrollback() const {
    return m_scrollback;
  }

signals:
  void updated();
  void bell();
  void titleChanged(const QString &title);
  // A program asked to put text on the clipboard (OSC 52). Reading the
  // clipboard that way is never answered.
  void clipboardWriteRequested(const QString &text);
  void finished(int exitCode);
  void errorOccurred(const QString &message);

private:
  enum class ParseState { Ground, Escape, CsiParam, OscString, OscEscape };

  void onReadyRead();
  void reap();

  // Parser
  void handleByte(unsigned char byte);
  void handleUtf8Byte(unsigned char byte);
  void putChar(const QString &ch);
  void dispatchCsi(char final);
  void dispatchOsc();
  void handleOsc52(const QByteArray &payload);
  void handleSgr(const QVector<int> &params);
  void setPrivateMode(const QVector<int> &params, bool enable);
  static QVector<int> splitParams(const QByteArray &raw);
  static int paramAt(const QVector<int> &params, int index, int fallback);

  // Grid/cursor operations
  void resizeGrid(int cols, int rows);
  void ensureCursorInBounds();
  void newline();
  void reverseIndex();
  void carriageReturn() { m_cursorCol = 0; }
  void cursorUp(int n);
  void cursorDown(int n);
  void cursorForward(int n);
  void cursorBack(int n);
  void cursorPosition(int row1, int col1);
  void eraseInDisplay(int mode);
  void eraseInLine(int mode);
  void eraseChars(int n);
  void insertChars(int n);
  void deleteChars(int n);
  void insertLines(int n);
  void deleteLines(int n);
  void scrollRegionUp(int n);
  void scrollRegionDown(int n);
  void setScrollRegion(int top1, int bottom1);
  void saveCursor();
  void restoreCursor();
  void switchAlternateScreen(bool enable, bool saveCursorToo);
  void clearGrid(QVector<QVector<TerminalCell>> &grid);
  QVector<QVector<TerminalCell>> &activeGrid();

  int m_masterFd = -1;
  qint64 m_childPid = -1; // pid_t, kept as qint64 to avoid a <sys/types.h>
                          // dependency in this header — see terminal.cpp
  QSocketNotifier *m_notifier = nullptr;

  int m_cols = 80;
  int m_rows = 24;
  int m_cursorRow = 0;
  int m_cursorCol = 0;
  bool m_cursorVisible = true;
  bool m_wrapPending = false;
  bool m_autoWrap = true;
  // DECCKM (\e[?1h): cursor keys send SS3 (\eOA) instead of CSI (\e[A).
  bool m_applicationCursorKeys = false;
  // \e[?2004h: the program wants pastes wrapped in \e[200~ ... \e[201~.
  bool m_bracketedPaste = false;
  int m_scrollTop = 0;
  int m_scrollBottom = 23;

  QVector<QVector<TerminalCell>> m_grid;
  QVector<QVector<TerminalCell>> m_altGrid;
  bool m_altScreenActive = false;
  QList<QVector<TerminalCell>> m_scrollback;
  static constexpr int kMaxScrollback = 5000;

  // Current SGR "pen"
  TerminalColor m_curFg;
  TerminalColor m_curBg;
  bool m_curBold = false;
  bool m_curUnderline = false;
  bool m_curReverse = false;
  bool m_curFaint = false;

  struct SavedCursor {
    int row = 0;
    int col = 0;
    TerminalColor fg;
    TerminalColor bg;
    bool bold = false;
    bool underline = false;
    bool reverse = false;
    bool faint = false;
  };
  SavedCursor m_savedCursor;

  ParseState m_state = ParseState::Ground;
  QByteArray m_csiParams;
  bool m_csiPrivate = false;
  QByteArray m_oscBuffer;
  // Set when an OSC outgrew kMaxOscLength: the rest is skipped and the
  // sequence ignored, instead of growing without bound.
  bool m_oscOverflow = false;
  qint64 m_droppedLines = 0;

  // Pending UTF-8 continuation bytes (a PTY read can split a multi-byte
  // sequence across two feed() calls).
  QByteArray m_utf8Pending;
  int m_utf8Remaining = 0;
};
