#include "terminal.h"

#include <QCoreApplication>
#include <QDir>
#include <QFileInfo>
#include <QSocketNotifier>
#include <QStringList>
#include <QStandardPaths>

#include <cerrno>
#include <cstdlib>
#include <cstring>
#include <fcntl.h>
#include <pty.h>
#include <sys/ioctl.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <termios.h>
#include <unistd.h>

namespace {

// Same resolution order as Backend::cliPath() (gui/backend.cpp): the
// omavm binary next to this process first, PATH otherwise. Resolved
// before forkpty() — not in the child — since Qt API calls after fork()
// in a multi-threaded process aren't guaranteed safe.
QByteArray resolveCliPath() {
  const QString sibling = QDir(QCoreApplication::applicationDirPath())
                              .filePath(QStringLiteral("omavm"));
  if (QFileInfo::exists(sibling))
    return sibling.toLocal8Bit();
  const QString installed =
      QStandardPaths::findExecutable(QStringLiteral("omavm"));
  if (!installed.isEmpty())
    return installed.toLocal8Bit();
  return sibling.toLocal8Bit();
}

QVector<TerminalCell> blankRow(int cols) { return QVector<TerminalCell>(cols); }

QVector<QVector<TerminalCell>>
resizedGrid(const QVector<QVector<TerminalCell>> &old, int oldCols, int oldRows,
            int newCols, int newRows) {
  QVector<QVector<TerminalCell>> next(newRows, blankRow(newCols));
  const int rowsToCopy = qMin(oldRows, newRows);
  const int colsToCopy = qMin(oldCols, newCols);
  for (int r = 0; r < rowsToCopy; ++r)
    for (int c = 0; c < colsToCopy; ++c)
      next[r][c] = old[r][c];
  return next;
}

} // namespace

TerminalSession::TerminalSession(QObject *parent) : QObject(parent) {
  resizeGrid(m_cols, m_rows);
}

TerminalSession::~TerminalSession() {
  if (m_notifier)
    m_notifier->setEnabled(false);
  if (m_writeNotifier)
    m_writeNotifier->setEnabled(false);
  if (m_masterFd >= 0)
    ::close(m_masterFd);
  if (m_childPid > 0) {
    int status = 0;
    ::waitpid(static_cast<pid_t>(m_childPid), &status, WNOHANG);
  }
}

void TerminalSession::start(const QString &envName, int cols, int rows) {
  startProgram(resolveCliPath(), {QByteArrayLiteral("open"), envName.toUtf8()},
               cols, rows);
}

void TerminalSession::startProgram(const QByteArray &program,
                                   const QList<QByteArray> &args, int cols,
                                   int rows) {
  m_cols = cols > 0 ? cols : 80;
  m_rows = rows > 0 ? rows : 24;
  m_scrollBottom = m_rows - 1;
  resizeGrid(m_cols, m_rows);

  struct winsize ws{};
  ws.ws_col = static_cast<unsigned short>(m_cols);
  ws.ws_row = static_cast<unsigned short>(m_rows);

  int master = -1;
  const pid_t pid = forkpty(&master, nullptr, nullptr, &ws);
  if (pid < 0) {
    emit errorOccurred(QStringLiteral("forkpty failed: ") +
                       QString::fromLocal8Bit(strerror(errno)));
    return;
  }
  if (pid == 0) {
    setenv("TERM", "xterm-256color", 1);
    QList<char *> argv;
    argv.append(const_cast<char *>(program.constData()));
    for (const QByteArray &arg : args)
      argv.append(const_cast<char *>(arg.constData()));
    argv.append(nullptr);
    execvp(program.constData(), argv.data());
    _exit(127);
  }

  m_masterFd = master;
  m_childPid = pid;
  fcntl(m_masterFd, F_SETFL, O_NONBLOCK);
  m_notifier = new QSocketNotifier(m_masterFd, QSocketNotifier::Read, this);
  connect(m_notifier, &QSocketNotifier::activated, this,
          &TerminalSession::onReadyRead);
}

void TerminalSession::onReadyRead() {
  char buf[4096];
  for (;;) {
    const ssize_t n = ::read(m_masterFd, buf, sizeof(buf));
    if (n > 0) {
      feed(QByteArray(buf, static_cast<int>(n)));
      if (n < static_cast<ssize_t>(sizeof(buf)))
        return;
      continue;
    }
    if (n < 0) {
      if (errno == EAGAIN || errno == EWOULDBLOCK)
        return;
      if (errno == EINTR)
        continue;
      reap();
      return;
    }
    reap();
    return;
  }
}

void TerminalSession::reap() {
  m_pendingInput.clear();
  if (m_writeNotifier) {
    m_writeNotifier->setEnabled(false);
    m_writeNotifier->deleteLater();
    m_writeNotifier = nullptr;
  }
  if (m_notifier) {
    m_notifier->setEnabled(false);
    m_notifier->deleteLater();
    m_notifier = nullptr;
  }
  if (m_masterFd >= 0) {
    ::close(m_masterFd);
    m_masterFd = -1;
  }
  int code = 0;
  if (m_childPid > 0) {
    int status = 0;
    if (::waitpid(static_cast<pid_t>(m_childPid), &status, 0) ==
        static_cast<pid_t>(m_childPid)) {
      // Death by a signal reads as 128 + signal, as in a shell: it must
      // not look like the clean exit (0) the viewer closes itself on.
      if (WIFEXITED(status))
        code = WEXITSTATUS(status);
      else if (WIFSIGNALED(status))
        code = 128 + WTERMSIG(status);
    }
    m_childPid = -1;
  }
  emit finished(code);
}

void TerminalSession::write(const QByteArray &bytes) {
  if (m_masterFd < 0 || bytes.isEmpty())
    return;
  m_pendingInput.append(bytes);
  flushInput();
}

void TerminalSession::flushInput() {
  while (m_masterFd >= 0 && !m_pendingInput.isEmpty()) {
    const ssize_t written =
        ::write(m_masterFd, m_pendingInput.constData(),
                static_cast<size_t>(m_pendingInput.size()));
    if (written > 0) {
      m_pendingInput.remove(0, static_cast<qsizetype>(written));
      continue;
    }
    if (written < 0 && errno == EINTR)
      continue;
    if (written < 0 && (errno == EAGAIN || errno == EWOULDBLOCK)) {
      // Full: wait until the program reads some.
      if (!m_writeNotifier) {
        m_writeNotifier =
            new QSocketNotifier(m_masterFd, QSocketNotifier::Write, this);
        connect(m_writeNotifier, &QSocketNotifier::activated, this,
                &TerminalSession::flushInput);
      }
      m_writeNotifier->setEnabled(true);
      return;
    }
    // The program is gone; reap() reports it.
    m_pendingInput.clear();
    break;
  }
  if (m_writeNotifier)
    m_writeNotifier->setEnabled(false);
}

void TerminalSession::resize(int cols, int rows) {
  if (cols <= 0 || rows <= 0 || (cols == m_cols && rows == m_rows))
    return;
  resizeGrid(cols, rows);
  m_cols = cols;
  m_rows = rows;
  m_scrollTop = 0;
  m_scrollBottom = m_rows - 1;
  ensureCursorInBounds();
  if (m_masterFd >= 0) {
    struct winsize ws{};
    ws.ws_col = static_cast<unsigned short>(cols);
    ws.ws_row = static_cast<unsigned short>(rows);
    ioctl(m_masterFd, TIOCSWINSZ, &ws);
  }
  emit updated();
}

void TerminalSession::resizeGrid(int newCols, int newRows) {
  if (m_grid.isEmpty()) {
    m_grid = QVector<QVector<TerminalCell>>(newRows, blankRow(newCols));
    m_altGrid = QVector<QVector<TerminalCell>>(newRows, blankRow(newCols));
    return;
  }
  m_grid = resizedGrid(m_grid, m_cols, m_rows, newCols, newRows);
  m_altGrid = resizedGrid(m_altGrid, m_cols, m_rows, newCols, newRows);
}

void TerminalSession::ensureCursorInBounds() {
  m_cursorRow = qBound(0, m_cursorRow, m_rows - 1);
  m_cursorCol = qBound(0, m_cursorCol, m_cols - 1);
}

QVector<QVector<TerminalCell>> &TerminalSession::activeGrid() {
  return m_altScreenActive ? m_altGrid : m_grid;
}

void TerminalSession::clearGrid(QVector<QVector<TerminalCell>> &grid) {
  for (auto &row : grid)
    row = blankRow(row.size());
}

// ---- Parser -----------------------------------------------------------

void TerminalSession::feed(const QByteArray &bytes) {
  for (int i = 0; i < bytes.size(); ++i)
    handleByte(static_cast<unsigned char>(bytes[i]));
  emit updated();
}

void TerminalSession::handleByte(unsigned char byte) {
  switch (m_state) {
  case ParseState::Ground:
    if (byte == 0x1b) {
      m_state = ParseState::Escape;
      return;
    }
    if (byte == '\n') {
      newline();
      return;
    }
    if (byte == '\r') {
      carriageReturn();
      return;
    }
    if (byte == '\b') {
      if (m_cursorCol > 0)
        --m_cursorCol;
      m_wrapPending = false;
      return;
    }
    if (byte == '\t') {
      m_cursorCol = qMin(((m_cursorCol / 8) + 1) * 8, m_cols - 1);
      return;
    }
    if (byte == 0x07) {
      emit bell();
      return;
    }
    if (byte < 0x20)
      return;
    handleUtf8Byte(byte);
    return;

  case ParseState::Escape:
    m_state = ParseState::Ground;
    if (byte == '[') {
      m_state = ParseState::CsiParam;
      m_csiParams.clear();
      m_csiPrivate = false;
      return;
    }
    if (byte == ']') {
      m_state = ParseState::OscString;
      m_oscBuffer.clear();
      return;
    }
    if (byte == '7') {
      saveCursor();
      return;
    }
    if (byte == '8') {
      restoreCursor();
      return;
    }
    if (byte == 'D') {
      newline();
      return;
    }
    if (byte == 'E') {
      newline();
      carriageReturn();
      return;
    }
    if (byte == 'M') {
      reverseIndex();
      return;
    }
    if (byte == 'c') {
      clearGrid(m_grid);
      clearGrid(m_altGrid);
      m_cursorRow = m_cursorCol = 0;
      m_scrollTop = 0;
      m_scrollBottom = m_rows - 1;
      m_curFg = TerminalColor();
      m_curBg = TerminalColor();
      m_curBold = m_curUnderline = m_curReverse = m_curFaint = false;
      m_altScreenActive = false;
      m_cursorVisible = true;
      m_wrapPending = false;
      return;
    }
    return; // unknown escape: ignore

  case ParseState::CsiParam:
    if (byte == '?' && m_csiParams.isEmpty()) {
      m_csiPrivate = true;
      return;
    }
    if ((byte >= '0' && byte <= '9') || byte == ';' || byte == ':') {
      m_csiParams.append(char(byte));
      return;
    }
    if (byte >= 0x40 && byte <= 0x7e) {
      dispatchCsi(char(byte));
      m_state = ParseState::Ground;
      return;
    }
    return; // intermediate byte (0x20-0x2f): ignored, keep collecting

  case ParseState::OscString:
    if (byte == 0x07) {
      dispatchOsc();
      m_state = ParseState::Ground;
      return;
    }
    if (byte == 0x1b) {
      m_state = ParseState::OscEscape;
      return;
    }
    if (m_oscBuffer.size() < kMaxOscLength)
      m_oscBuffer.append(char(byte));
    else
      m_oscOverflow = true;
    return;

  case ParseState::OscEscape:
    if (byte == '\\') {
      dispatchOsc();
      m_state = ParseState::Ground;
      return;
    }
    dispatchOsc();
    m_state = ParseState::Ground;
    handleByte(0x1b);
    handleByte(byte);
    return;
  }
}

void TerminalSession::handleUtf8Byte(unsigned char byte) {
  if (m_utf8Remaining == 0) {
    if (byte < 0x80) {
      putChar(QString(QChar(byte)));
      return;
    }
    int len = 0;
    if ((byte & 0xe0) == 0xc0)
      len = 1;
    else if ((byte & 0xf0) == 0xe0)
      len = 2;
    else if ((byte & 0xf8) == 0xf0)
      len = 3;
    else
      return; // invalid leading byte
    m_utf8Pending.clear();
    m_utf8Pending.append(char(byte));
    m_utf8Remaining = len;
    return;
  }
  m_utf8Pending.append(char(byte));
  if (--m_utf8Remaining == 0) {
    const QString decoded = QString::fromUtf8(m_utf8Pending);
    m_utf8Pending.clear();
    if (!decoded.isEmpty())
      putChar(decoded);
  }
}

namespace {
// Columns a character occupies, like wcwidth() but from Qt's Unicode
// tables instead of the C library's, whose answer depends on the process
// locale (-1 for everything non-ASCII under LANG=C).
int cellWidth(const QString &ch) {
  if (ch.isEmpty())
    return 1;
  const char32_t cp = ch.toUcs4().value(0);
  const QChar::Category category = QChar::category(cp);
  if (category == QChar::Mark_NonSpacing ||
      category == QChar::Mark_Enclosing || (cp >= 0x200b && cp <= 0x200d) ||
      (cp >= 0xfe00 && cp <= 0xfe0f))
    return 0;
  // East Asian Wide/Fullwidth ranges and emoji.
  if ((cp >= 0x1100 && cp <= 0x115f) ||
      (cp >= 0x2e80 && cp <= 0xa4cf && cp != 0x303f) ||
      (cp >= 0xac00 && cp <= 0xd7a3) || (cp >= 0xf900 && cp <= 0xfaff) ||
      (cp >= 0xfe30 && cp <= 0xfe4f) || (cp >= 0xff00 && cp <= 0xff60) ||
      (cp >= 0xffe0 && cp <= 0xffe6) || (cp >= 0x1f300 && cp <= 0x1f64f) ||
      (cp >= 0x1f680 && cp <= 0x1f6ff) || (cp >= 0x1f900 && cp <= 0x1f9ff) ||
      (cp >= 0x1fa70 && cp <= 0x1faff) || (cp >= 0x20000 && cp <= 0x3fffd))
    return 2;
  return 1;
}

// A wide character is a head cell followed by a continuation cell with an
// empty ch. Writing over either half leaves the other half blank.
void breakWideCharacterAt(QVector<TerminalCell> &row, int col) {
  if (col < 0 || col >= row.size())
    return;
  if (row[col].ch.isEmpty()) {
    if (col > 0)
      row[col - 1].ch = QStringLiteral(" ");
  } else if (col + 1 < row.size() && row[col + 1].ch.isEmpty()) {
    row[col + 1].ch = QStringLiteral(" ");
  }
}
} // namespace

void TerminalSession::putChar(const QString &ch) {
  auto &grid = activeGrid();
  if (grid.isEmpty())
    return;
  const int width = cellWidth(ch);
  if (width == 0) {
    // Combining mark: part of the character just written.
    if (m_cursorRow < 0 || m_cursorRow >= grid.size())
      return;
    auto &row = grid[m_cursorRow];
    int col = m_wrapPending ? m_cursorCol : m_cursorCol - 1;
    if (col > 0 && col < row.size() && row[col].ch.isEmpty())
      --col;
    if (col >= 0 && col < row.size())
      row[col].ch += ch;
    return;
  }
  if (m_wrapPending) {
    m_wrapPending = false;
    newline();
    carriageReturn();
  }
  if (width == 2 && m_cursorCol + 1 >= m_cols) {
    // No room for both halves on this line.
    if (m_autoWrap) {
      newline();
      carriageReturn();
    } else {
      m_cursorCol = qMax(0, m_cols - 2);
    }
  }
  if (m_cursorRow < 0 || m_cursorRow >= grid.size())
    return;
  auto &row = grid[m_cursorRow];
  if (m_cursorCol < 0 || m_cursorCol + width - 1 >= row.size())
    return;
  breakWideCharacterAt(row, m_cursorCol);
  if (width == 2)
    breakWideCharacterAt(row, m_cursorCol + 1);
  TerminalCell &cell = row[m_cursorCol];
  cell.ch = ch;
  cell.fg = m_curFg;
  cell.bg = m_curBg;
  cell.bold = m_curBold;
  cell.underline = m_curUnderline;
  cell.reverse = m_curReverse;
  cell.faint = m_curFaint;
  if (width == 2) {
    row[m_cursorCol + 1] = cell;
    row[m_cursorCol + 1].ch = QString();
  }
  const int last = m_cursorCol + width - 1;
  if (last + 1 >= m_cols) {
    m_cursorCol = last;
    if (m_autoWrap)
      m_wrapPending = true;
  } else {
    m_cursorCol = last + 1;
  }
}

void TerminalSession::newline() {
  if (m_cursorRow == m_scrollBottom)
    scrollRegionUp(1);
  else if (m_cursorRow < m_rows - 1)
    ++m_cursorRow;
  m_wrapPending = false;
}

void TerminalSession::reverseIndex() {
  if (m_cursorRow == m_scrollTop)
    scrollRegionDown(1);
  else if (m_cursorRow > 0)
    --m_cursorRow;
  m_wrapPending = false;
}

void TerminalSession::cursorUp(int n) {
  m_cursorRow = qMax(0, m_cursorRow - n);
  m_wrapPending = false;
}
void TerminalSession::cursorDown(int n) {
  m_cursorRow = qMin(m_rows - 1, m_cursorRow + n);
  m_wrapPending = false;
}
void TerminalSession::cursorForward(int n) {
  m_cursorCol = qMin(m_cols - 1, m_cursorCol + n);
  m_wrapPending = false;
}
void TerminalSession::cursorBack(int n) {
  m_cursorCol = qMax(0, m_cursorCol - n);
  m_wrapPending = false;
}
void TerminalSession::cursorPosition(int row1, int col1) {
  m_cursorRow = qBound(0, row1 - 1, m_rows - 1);
  m_cursorCol = qBound(0, col1 - 1, m_cols - 1);
  m_wrapPending = false;
}

void TerminalSession::eraseInDisplay(int mode) {
  auto &grid = activeGrid();
  if (grid.isEmpty())
    return;
  if (mode == 0) {
    for (int c = m_cursorCol; c < grid[m_cursorRow].size(); ++c)
      grid[m_cursorRow][c] = TerminalCell{};
    for (int r = m_cursorRow + 1; r < grid.size(); ++r)
      grid[r] = blankRow(m_cols);
  } else if (mode == 1) {
    for (int c = 0; c <= m_cursorCol && c < grid[m_cursorRow].size(); ++c)
      grid[m_cursorRow][c] = TerminalCell{};
    for (int r = 0; r < m_cursorRow; ++r)
      grid[r] = blankRow(m_cols);
  } else {
    clearGrid(grid);
  }
}

void TerminalSession::eraseInLine(int mode) {
  auto &grid = activeGrid();
  if (grid.isEmpty() || m_cursorRow >= grid.size())
    return;
  auto &row = grid[m_cursorRow];
  if (mode == 0) {
    for (int c = m_cursorCol; c < row.size(); ++c)
      row[c] = TerminalCell{};
  } else if (mode == 1) {
    for (int c = 0; c <= m_cursorCol && c < row.size(); ++c)
      row[c] = TerminalCell{};
  } else {
    row = blankRow(row.size());
  }
}

void TerminalSession::eraseChars(int n) {
  auto &grid = activeGrid();
  if (grid.isEmpty() || m_cursorRow >= grid.size())
    return;
  auto &row = grid[m_cursorRow];
  for (int c = m_cursorCol; c < qMin(m_cursorCol + n, int(row.size())); ++c)
    row[c] = TerminalCell{};
}

void TerminalSession::insertChars(int n) {
  auto &grid = activeGrid();
  if (grid.isEmpty() || m_cursorRow >= grid.size())
    return;
  auto &row = grid[m_cursorRow];
  n = qMin(n, int(row.size()) - m_cursorCol);
  if (n <= 0)
    return;
  for (int c = row.size() - 1; c >= m_cursorCol + n; --c)
    row[c] = row[c - n];
  for (int c = m_cursorCol; c < m_cursorCol + n; ++c)
    row[c] = TerminalCell{};
}

void TerminalSession::deleteChars(int n) {
  auto &grid = activeGrid();
  if (grid.isEmpty() || m_cursorRow >= grid.size())
    return;
  auto &row = grid[m_cursorRow];
  n = qMin(n, int(row.size()) - m_cursorCol);
  if (n <= 0)
    return;
  for (int c = m_cursorCol; c < row.size() - n; ++c)
    row[c] = row[c + n];
  for (int c = row.size() - n; c < row.size(); ++c)
    row[c] = TerminalCell{};
}

void TerminalSession::insertLines(int n) {
  auto &grid = activeGrid();
  if (grid.isEmpty())
    return;
  const int top = m_cursorRow;
  const int bottom = m_scrollBottom;
  n = qMin(n, bottom - top + 1);
  if (n <= 0)
    return;
  for (int r = bottom; r - n >= top; --r)
    grid[r] = grid[r - n];
  for (int r = top; r < top + n && r <= bottom; ++r)
    grid[r] = blankRow(m_cols);
}

void TerminalSession::deleteLines(int n) {
  auto &grid = activeGrid();
  if (grid.isEmpty())
    return;
  const int top = m_cursorRow;
  const int bottom = m_scrollBottom;
  n = qMin(n, bottom - top + 1);
  if (n <= 0)
    return;
  for (int r = top; r + n <= bottom; ++r)
    grid[r] = grid[r + n];
  for (int r = bottom - n + 1; r <= bottom; ++r)
    if (r >= top)
      grid[r] = blankRow(m_cols);
}

void TerminalSession::scrollRegionUp(int n) {
  auto &grid = activeGrid();
  if (grid.isEmpty())
    return;
  n = qMin(n, m_scrollBottom - m_scrollTop + 1);
  if (n <= 0)
    return;
  if (!m_altScreenActive && m_scrollTop == 0) {
    for (int i = 0; i < n; ++i) {
      m_scrollback.append(grid[i]);
      if (m_scrollback.size() > kMaxScrollback) {
        m_scrollback.removeFirst();
        ++m_droppedLines;
      }
    }
  }
  for (int r = m_scrollTop; r + n <= m_scrollBottom; ++r)
    grid[r] = grid[r + n];
  for (int r = m_scrollBottom - n + 1; r <= m_scrollBottom; ++r)
    if (r >= 0 && r < grid.size())
      grid[r] = blankRow(m_cols);
}

void TerminalSession::scrollRegionDown(int n) {
  auto &grid = activeGrid();
  if (grid.isEmpty())
    return;
  n = qMin(n, m_scrollBottom - m_scrollTop + 1);
  if (n <= 0)
    return;
  for (int r = m_scrollBottom; r - n >= m_scrollTop; --r)
    grid[r] = grid[r - n];
  for (int r = m_scrollTop; r < m_scrollTop + n && r <= m_scrollBottom; ++r)
    grid[r] = blankRow(m_cols);
}

void TerminalSession::setScrollRegion(int top1, int bottom1) {
  int top = top1 - 1;
  int bottom = bottom1 - 1;
  if (top < 0)
    top = 0;
  if (bottom >= m_rows || bottom < 0)
    bottom = m_rows - 1;
  if (top >= bottom) {
    top = 0;
    bottom = m_rows - 1;
  }
  m_scrollTop = top;
  m_scrollBottom = bottom;
  m_cursorRow = m_scrollTop;
  m_cursorCol = 0;
  m_wrapPending = false;
}

void TerminalSession::saveCursor() {
  m_savedCursor.row = m_cursorRow;
  m_savedCursor.col = m_cursorCol;
  m_savedCursor.fg = m_curFg;
  m_savedCursor.bg = m_curBg;
  m_savedCursor.bold = m_curBold;
  m_savedCursor.underline = m_curUnderline;
  m_savedCursor.reverse = m_curReverse;
  m_savedCursor.faint = m_curFaint;
}

void TerminalSession::restoreCursor() {
  m_cursorRow = qBound(0, m_savedCursor.row, m_rows - 1);
  m_cursorCol = qBound(0, m_savedCursor.col, m_cols - 1);
  m_curFg = m_savedCursor.fg;
  m_curBg = m_savedCursor.bg;
  m_curBold = m_savedCursor.bold;
  m_curUnderline = m_savedCursor.underline;
  m_curReverse = m_savedCursor.reverse;
  m_curFaint = m_savedCursor.faint;
  m_wrapPending = false;
}

void TerminalSession::switchAlternateScreen(bool enable, bool saveCursorToo) {
  if (enable == m_altScreenActive)
    return;
  if (saveCursorToo && enable)
    saveCursor();
  if (enable)
    clearGrid(m_altGrid);
  m_altScreenActive = enable;
  if (saveCursorToo && !enable)
    restoreCursor();
  m_wrapPending = false;
}

QVector<int> TerminalSession::splitParams(const QByteArray &raw) {
  QVector<int> result;
  if (raw.isEmpty())
    return result;
  int value = -1;
  bool any = false;
  for (char c : raw) {
    if (c == ';' || c == ':') {
      result.append(any ? value : -1);
      value = -1;
      any = false;
    } else if (c >= '0' && c <= '9') {
      if (!any) {
        value = 0;
        any = true;
      }
      // Capped like xterm: ESC[2147483647C made cursor + n overflow to a
      // negative column, and the next erase wrote outside the row.
      value = qMin(value * 10 + (c - '0'), 65535);
    }
  }
  result.append(any ? value : -1);
  return result;
}

int TerminalSession::paramAt(const QVector<int> &params, int index,
                             int fallback) {
  if (index < 0 || index >= params.size())
    return fallback;
  const int v = params[index];
  return v < 0 ? fallback : v;
}

void TerminalSession::dispatchCsi(char final) {
  const QVector<int> params = splitParams(m_csiParams);
  auto count = [&](int idx) { return qMax(1, paramAt(params, idx, 1)); };
  switch (final) {
  case 'A':
    cursorUp(count(0));
    break;
  case 'B':
    cursorDown(count(0));
    break;
  case 'C':
    cursorForward(count(0));
    break;
  case 'D':
    cursorBack(count(0));
    break;
  case 'H':
  case 'f':
    cursorPosition(count(0), count(1));
    break;
  case 'G':
    m_cursorCol = qBound(0, count(0) - 1, m_cols - 1);
    m_wrapPending = false;
    break;
  case 'd':
    m_cursorRow = qBound(0, count(0) - 1, m_rows - 1);
    m_wrapPending = false;
    break;
  case 'J':
    eraseInDisplay(paramAt(params, 0, 0));
    break;
  case 'K':
    eraseInLine(paramAt(params, 0, 0));
    break;
  case 'r':
    setScrollRegion(count(0), paramAt(params, 1, m_rows));
    break;
  case 'm':
    handleSgr(params);
    break;
  case 's':
    saveCursor();
    break;
  case 'u':
    restoreCursor();
    break;
  case 'h':
    setPrivateMode(params, true);
    break;
  case 'l':
    setPrivateMode(params, false);
    break;
  case 'S':
    scrollRegionUp(count(0));
    break;
  case 'T':
    scrollRegionDown(count(0));
    break;
  case 'L':
    insertLines(count(0));
    break;
  case 'M':
    deleteLines(count(0));
    break;
  case 'P':
    deleteChars(count(0));
    break;
  case '@':
    insertChars(count(0));
    break;
  case 'X':
    eraseChars(count(0));
    break;
  default:
    break;
  }
}

void TerminalSession::setPrivateMode(const QVector<int> &params, bool enable) {
  if (!m_csiPrivate)
    return;
  for (int mode : params) {
    switch (mode) {
    case 1:
      m_applicationCursorKeys = enable;
      break;
    case 7:
      m_autoWrap = enable;
      break;
    case 2004:
      m_bracketedPaste = enable;
      break;
    case 25:
      m_cursorVisible = enable;
      break;
    case 47:
      switchAlternateScreen(enable, false);
      break;
    case 1049:
      switchAlternateScreen(enable, true);
      break;
    default:
      // Mouse reporting (1000/1002/1003/1006) is not implemented — safe
      // to ignore rather than misbehave.
      break;
    }
  }
}

void TerminalSession::handleSgr(const QVector<int> &paramsIn) {
  QVector<int> params = paramsIn;
  if (params.isEmpty())
    params.append(-1);
  for (int i = 0; i < params.size(); ++i) {
    int p = params[i];
    if (p < 0)
      p = 0;
    if (p == 0) {
      m_curFg = TerminalColor();
      m_curBg = TerminalColor();
      m_curBold = m_curUnderline = m_curReverse = m_curFaint = false;
    } else if (p == 1) {
      m_curBold = true;
    } else if (p == 2) {
      m_curFaint = true;
    } else if (p == 4) {
      m_curUnderline = true;
    } else if (p == 7) {
      m_curReverse = true;
    } else if (p == 22) {
      m_curBold = false;
      m_curFaint = false;
    } else if (p == 24) {
      m_curUnderline = false;
    } else if (p == 27) {
      m_curReverse = false;
    } else if (p >= 30 && p <= 37) {
      m_curFg = {TerminalColorKind::Palette, quint8(p - 30), 0};
    } else if (p == 38) {
      if (i + 2 < params.size() && params[i + 1] == 5) {
        m_curFg = {TerminalColorKind::Palette,
                   quint8(qBound(0, params[i + 2], 255)), 0};
        i += 2;
      } else if (i + 4 < params.size() && params[i + 1] == 2) {
        m_curFg = {TerminalColorKind::True, 0,
                   qRgb(qBound(0, params[i + 2], 255),
                        qBound(0, params[i + 3], 255),
                        qBound(0, params[i + 4], 255))};
        i += 4;
      }
    } else if (p == 39) {
      m_curFg = TerminalColor();
    } else if (p >= 40 && p <= 47) {
      m_curBg = {TerminalColorKind::Palette, quint8(p - 40), 0};
    } else if (p == 48) {
      if (i + 2 < params.size() && params[i + 1] == 5) {
        m_curBg = {TerminalColorKind::Palette,
                   quint8(qBound(0, params[i + 2], 255)), 0};
        i += 2;
      } else if (i + 4 < params.size() && params[i + 1] == 2) {
        m_curBg = {TerminalColorKind::True, 0,
                   qRgb(qBound(0, params[i + 2], 255),
                        qBound(0, params[i + 3], 255),
                        qBound(0, params[i + 4], 255))};
        i += 4;
      }
    } else if (p == 49) {
      m_curBg = TerminalColor();
    } else if (p >= 90 && p <= 97) {
      m_curFg = {TerminalColorKind::Palette, quint8(p - 90 + 8), 0};
    } else if (p >= 100 && p <= 107) {
      m_curBg = {TerminalColorKind::Palette, quint8(p - 100 + 8), 0};
    }
  }
}

void TerminalSession::dispatchOsc() {
  const QByteArray buffer = m_oscBuffer;
  const bool overflow = m_oscOverflow;
  m_oscBuffer.clear();
  m_oscOverflow = false;
  const int semi = buffer.indexOf(';');
  if (semi < 0 || overflow)
    return;
  const QByteArray code = buffer.left(semi);
  const QByteArray payload = buffer.mid(semi + 1);
  if (code == "0" || code == "2")
    emit titleChanged(QString::fromUtf8(payload));
  else if (code == "52")
    handleOsc52(payload);
}

// OSC 52 ; <targets> ; <base64>. Only writes: "?" asks for the clipboard's
// contents, which would let anything running in the Box read what the user
// copied on the host, so it is ignored (xterm's own default).
void TerminalSession::handleOsc52(const QByteArray &payload) {
  const int semi = payload.indexOf(';');
  if (semi < 0)
    return;
  const QByteArray targets = payload.left(semi);
  const QByteArray data = payload.mid(semi + 1);
  // Empty targets mean the default "s 0"; c is the clipboard, p/s the
  // primary selection. Anything else (cut buffers) isn't a clipboard here.
  if (!targets.isEmpty() && !targets.contains('c') && !targets.contains('p') &&
      !targets.contains('s'))
    return;
  if (data == "?")
    return;
  const auto decoded = QByteArray::fromBase64Encoding(
      data, QByteArray::AbortOnBase64DecodingErrors);
  if (!decoded)
    return;
  emit clipboardWriteRequested(QString::fromUtf8(*decoded));
}

int TerminalSession::bufferLineCount() const {
  return m_scrollback.size() + grid().size();
}

QVector<TerminalCell> TerminalSession::bufferLine(int index) const {
  if (index < 0 || index >= bufferLineCount())
    return {};
  if (index < m_scrollback.size())
    return m_scrollback.at(index);
  return grid().at(index - m_scrollback.size());
}

QString TerminalSession::text(int startLine, int startCol, int endLine,
                              int endCol) const {
  if (startLine > endLine || (startLine == endLine && startCol > endCol)) {
    qSwap(startLine, endLine);
    qSwap(startCol, endCol);
  }
  QStringList lines;
  for (int l = qMax(0, startLine); l <= endLine && l < bufferLineCount();
       ++l) {
    const QVector<TerminalCell> line = bufferLine(l);
    int first = l == startLine ? qMax(0, startCol) : 0;
    const int last = l == endLine ? qMin(endCol, int(line.size()) - 1)
                                  : int(line.size()) - 1;
    // Starting on the right half of a wide character copies all of it.
    while (first > 0 && first < line.size() && line.at(first).ch.isEmpty())
      --first;
    QString out;
    for (int c = first; c <= last; ++c)
      out += line.at(c).ch;
    while (out.endsWith(QLatin1Char(' ')))
      out.chop(1);
    lines << out;
  }
  return lines.join(QLatin1Char('\n'));
}

QPair<int, int> TerminalSession::wordBounds(int line, int col) const {
  const QVector<TerminalCell> cells = bufferLine(line);
  if (col < 0 || col >= cells.size())
    return {col, col};
  // Paths, URLs and e-mail addresses select as one word, like in most
  // terminals.
  const auto isWord = [&cells](int c) {
    const QString &ch = cells.at(c).ch;
    if (ch.isEmpty()) // right half of a wide character
      return true;
    const QChar first = ch.at(0);
    return first.isLetterOrNumber() || first.isMark() ||
           QStringLiteral("-_./~:@%+#?=&").contains(first) ||
           first.unicode() > 0x2e7f; // CJK and other wide scripts
  };
  while (col > 0 && cells.at(col).ch.isEmpty())
    --col;
  if (!isWord(col))
    return {col, col};
  int first = col;
  int last = col;
  while (first > 0 && isWord(first - 1))
    --first;
  while (first < cells.size() && cells.at(first).ch.isEmpty())
    ++first;
  while (last + 1 < cells.size() && isWord(last + 1))
    ++last;
  return {first, last};
}

bool TerminalSession::isCopyShortcut(int key,
                                     Qt::KeyboardModifiers modifiers) {
  const Qt::KeyboardModifiers relevant =
      modifiers & (Qt::ShiftModifier | Qt::ControlModifier | Qt::AltModifier);
  return (key == Qt::Key_Insert && relevant == Qt::ControlModifier) ||
         (key == Qt::Key_C &&
          relevant == (Qt::ControlModifier | Qt::ShiftModifier));
}

QByteArray TerminalSession::keySequence(int key,
                                        Qt::KeyboardModifiers modifiers,
                                        const QString &text) const {
  const bool shift = modifiers & Qt::ShiftModifier;
  const bool alt = modifiers & Qt::AltModifier;
  const bool ctrl = modifiers & Qt::ControlModifier;
  // xterm's modifier parameter: 1 + Shift(1) + Alt(2) + Ctrl(4).
  const int modifier = 1 + (shift ? 1 : 0) + (alt ? 2 : 0) + (ctrl ? 4 : 0);

  // Cursor keys, Home/End and F1-F4: a final letter after CSI or SS3.
  char final = 0;
  bool cursorKey = false;
  switch (key) {
  case Qt::Key_Up: final = 'A'; cursorKey = true; break;
  case Qt::Key_Down: final = 'B'; cursorKey = true; break;
  case Qt::Key_Right: final = 'C'; cursorKey = true; break;
  case Qt::Key_Left: final = 'D'; cursorKey = true; break;
  case Qt::Key_Home: final = 'H'; cursorKey = true; break;
  case Qt::Key_End: final = 'F'; cursorKey = true; break;
  case Qt::Key_F1: final = 'P'; break;
  case Qt::Key_F2: final = 'Q'; break;
  case Qt::Key_F3: final = 'R'; break;
  case Qt::Key_F4: final = 'S'; break;
  default: break;
  }
  if (final) {
    if (modifier > 1)
      return QByteArray("\x1b[1;") + QByteArray::number(modifier) + final;
    const bool ss3 = !cursorKey || m_applicationCursorKeys;
    return QByteArray(ss3 ? "\x1bO" : "\x1b[") + final;
  }

  // Editing keys and F5-F12: CSI number ~.
  int number = 0;
  switch (key) {
  case Qt::Key_Insert: number = 2; break;
  case Qt::Key_Delete: number = 3; break;
  case Qt::Key_PageUp: number = 5; break;
  case Qt::Key_PageDown: number = 6; break;
  default:
    if (key >= Qt::Key_F5 && key <= Qt::Key_F12) {
      static const int numbers[] = {15, 17, 18, 19, 20, 21, 23, 24};
      number = numbers[key - Qt::Key_F5];
    }
    break;
  }
  if (number) {
    QByteArray out = "\x1b[" + QByteArray::number(number);
    if (modifier > 1)
      out += ';' + QByteArray::number(modifier);
    return out + '~';
  }

  QByteArray out;
  switch (key) {
  case Qt::Key_Return:
  case Qt::Key_Enter:
    out = "\r";
    break;
  case Qt::Key_Backspace:
    out = ctrl ? "\x08" : "\x7f";
    break;
  case Qt::Key_Tab:
    out = "\t";
    break;
  case Qt::Key_Backtab:
    return QByteArrayLiteral("\x1b[Z");
  case Qt::Key_Escape:
    out = "\x1b";
    break;
  default:
    if (ctrl && key >= Qt::Key_A && key <= Qt::Key_Z) {
      out = QByteArray(1, char(key - Qt::Key_A + 1));
    } else if (ctrl && (key == Qt::Key_Space || key == Qt::Key_At)) {
      out = QByteArray(1, '\0');
    } else if (ctrl && key >= Qt::Key_BracketLeft && key <= Qt::Key_Underscore) {
      // Ctrl+[, Ctrl+\, Ctrl+], Ctrl+^ and Ctrl+_ are ESC, FS, GS, RS and US.
      out = QByteArray(1, char(key - Qt::Key_BracketLeft + 0x1b));
    } else if (!text.isEmpty()) {
      out = text.toUtf8();
    }
    break;
  }
  if (!out.isEmpty() && alt && !out.startsWith('\x1b'))
    out.prepend('\x1b');
  return out;
}

bool TerminalSession::isPasteShortcut(int key,
                                      Qt::KeyboardModifiers modifiers) {
  const Qt::KeyboardModifiers relevant =
      modifiers & (Qt::ShiftModifier | Qt::ControlModifier | Qt::AltModifier);
  return (key == Qt::Key_Insert && relevant == Qt::ShiftModifier) ||
         (key == Qt::Key_V &&
          relevant == (Qt::ControlModifier | Qt::ShiftModifier));
}

QByteArray TerminalSession::pasteSequence(const QString &text) const {
  QString body = text;
  body.replace(QStringLiteral("\r\n"), QStringLiteral("\r"));
  body.replace(QLatin1Char('\n'), QLatin1Char('\r'));
  if (!m_bracketedPaste)
    return body.toUtf8();
  // An end marker inside the text would close the bracket early and turn
  // the rest of the paste into typed input.
  body.remove(QStringLiteral("\x1b[201~"));
  return "\x1b[200~" + body.toUtf8() + "\x1b[201~";
}
