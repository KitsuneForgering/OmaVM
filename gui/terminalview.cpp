#include "terminalview.h"
#include "colorstoml.h"

#include <QClipboard>
#include <QDropEvent>
#include <QMimeData>
#include <QDir>
#include <QFontMetricsF>
#include <QSettings>
#include <QGuiApplication>
#include <QKeyEvent>
#include <QMouseEvent>
#include <QPainter>
#include <QStandardPaths>
#include <QWheelEvent>

namespace {

QRgb parseHexColor(const QString &value, QRgb fallback) {
  QString hex = value;
  if (hex.startsWith(QLatin1Char('#')))
    hex.remove(0, 1);
  bool ok = false;
  const uint rgb = hex.toUInt(&ok, 16);
  if (!ok || hex.size() != 6)
    return fallback;
  return qRgb((rgb >> 16) & 0xff, (rgb >> 8) & 0xff, rgb & 0xff);
}

// Standard xterm 256-color cube (indices 16-231) and grayscale ramp
// (232-255) — not themed, only the base 16 come from colors.toml.
QRgb xterm256(int index) {
  static const int steps[] = {0, 95, 135, 175, 215, 255};
  if (index >= 232) {
    const int gray = 8 + (index - 232) * 10;
    return qRgb(gray, gray, gray);
  }
  const int i = index - 16;
  return qRgb(steps[(i / 36) % 6], steps[(i / 6) % 6], steps[i % 6]);
}

// Font sizes in pixels; the default is what the terminal always used.
constexpr int kDefaultFontSize = 15;
constexpr int kMinFontSize = 8;
constexpr int kMaxFontSize = 48;

} // namespace

TerminalView::TerminalView(QQuickItem *parent) : QQuickPaintedItem(parent) {
  m_font = QFont(QStringLiteral("monospace"));
  m_font.setStyleHint(QFont::Monospace);
  setFontPixelSize(
      QSettings().value(QStringLiteral("terminal/fontPixelSize"), kDefaultFontSize)
          .toInt());

  loadPalette();

  m_resizeTimer.setSingleShot(true);
  m_resizeTimer.setInterval(150);
  connect(&m_resizeTimer, &QTimer::timeout, this,
          &TerminalView::updateGridSize);

  connect(&m_session, &TerminalSession::updated, this, [this] { update(); });
  connect(&m_session, &TerminalSession::bell, this,
          [this] { /* visual bell TODO */ });
  connect(&m_session, &TerminalSession::titleChanged, this,
          &TerminalView::titleChanged);
  connect(&m_session, &TerminalSession::clipboardWriteRequested, this,
          [this](const QString &text) {
            if (m_shareClipboard)
              QGuiApplication::clipboard()->setText(text);
          });
  setAcceptedMouseButtons(Qt::LeftButton | Qt::MiddleButton);
  setFlag(ItemAcceptsDrops);
  connect(&m_session, &TerminalSession::finished, this,
          &TerminalView::finished);
  connect(&m_session, &TerminalSession::errorOccurred, this,
          &TerminalView::errorOccurred);
}

void TerminalView::loadPalette() {
  const QString path =
      QDir::homePath() +
      QStringLiteral("/.local/state/omarchy/current/theme/colors.toml");
  const QHash<QString, QString> values = loadColorsToml(path);
  auto get = [&values](const QString &key, QRgb fallback) {
    const auto it = values.constFind(key);
    return it != values.constEnd() ? parseHexColor(it.value(), fallback)
                                   : fallback;
  };

  m_defaultBg = get(QStringLiteral("background"), qRgb(0x10, 0x10, 0x10));
  m_defaultFg = get(QStringLiteral("foreground"), qRgb(0xee, 0xee, 0xee));
  m_selection = get(QStringLiteral("selection_background"),
                    get(QStringLiteral("selection"), qRgb(0x44, 0x44, 0x44)));

  m_palette[0] = m_defaultBg;
  m_palette[1] = get(QStringLiteral("red"), qRgb(0xcd, 0x00, 0x00));
  m_palette[2] = get(QStringLiteral("green"), qRgb(0x00, 0xcd, 0x00));
  m_palette[3] = get(QStringLiteral("yellow"), qRgb(0xcd, 0xcd, 0x00));
  m_palette[4] = get(QStringLiteral("blue"), qRgb(0x00, 0x00, 0xee));
  m_palette[5] = get(QStringLiteral("magenta"), qRgb(0xcd, 0x00, 0xcd));
  m_palette[6] = get(QStringLiteral("cyan"), qRgb(0x00, 0xcd, 0xcd));
  m_palette[7] = m_defaultFg;
  m_palette[8] = get(QStringLiteral("muted"), qRgb(0x7f, 0x7f, 0x7f));
  m_palette[9] = get(QStringLiteral("bright_red"), qRgb(0xff, 0x00, 0x00));
  m_palette[10] = get(QStringLiteral("bright_green"), qRgb(0x00, 0xff, 0x00));
  m_palette[11] = get(QStringLiteral("bright_yellow"), qRgb(0xff, 0xff, 0x00));
  m_palette[12] = get(QStringLiteral("bright_blue"), qRgb(0x5c, 0x5c, 0xff));
  m_palette[13] = get(QStringLiteral("bright_magenta"), qRgb(0xff, 0x00, 0xff));
  m_palette[14] = get(QStringLiteral("bright_cyan"), qRgb(0x00, 0xff, 0xff));
  m_palette[15] =
      get(QStringLiteral("bright_foreground"), qRgb(0xff, 0xff, 0xff));
}

void TerminalView::reloadPalette() {
  loadPalette();
  update();
}

void TerminalView::setEnvName(const QString &name) {
  if (m_envName == name)
    return;
  m_envName = name;
  emit envNameChanged();
  const int cols = qMax(1, int(width() / m_cellWidth));
  const int rows = qMax(1, int(height() / m_cellHeight));
  m_session.start(m_envName, cols > 1 ? cols : 80, rows > 1 ? rows : 24);
}

void TerminalView::restart() {
  if (m_session.running() || m_envName.isEmpty())
    return;
  m_session.feed(QByteArrayLiteral("\r\n"));
  m_session.start(m_envName, m_session.cols(), m_session.rows());
  forceActiveFocus();
}

void TerminalView::geometryChange(const QRectF &newGeometry,
                                  const QRectF &oldGeometry) {
  QQuickPaintedItem::geometryChange(newGeometry, oldGeometry);
  if (newGeometry.size() != oldGeometry.size())
    m_resizeTimer.start();
}

void TerminalView::updateGridSize() {
  const int cols = qMax(1, int(width() / m_cellWidth));
  const int rows = qMax(1, int(height() / m_cellHeight));
  m_session.resize(cols, rows);
}

QRgb TerminalView::resolveColor(const TerminalColor &color,
                                bool foreground) const {
  switch (color.kind) {
  case TerminalColorKind::Default:
    return foreground ? m_defaultFg : m_defaultBg;
  case TerminalColorKind::True:
    return color.rgb;
  case TerminalColorKind::Palette:
    return color.index < 16 ? m_palette[color.index] : xterm256(color.index);
  }
  return foreground ? m_defaultFg : m_defaultBg;
}

int TerminalView::visibleToBuffer(int visibleRow) const {
  const int bottomIndex = m_session.bufferLineCount() - 1 - m_scrollOffset;
  return bottomIndex - (m_session.rows() - 1 - visibleRow);
}

QVector<TerminalCell> TerminalView::lineAt(int visibleRow) const {
  const int sourceIndex = visibleToBuffer(visibleRow);
  if (sourceIndex < 0 || sourceIndex >= m_session.bufferLineCount())
    return QVector<TerminalCell>(m_session.cols());
  return m_session.bufferLine(sourceIndex);
}

TerminalView::CellPos TerminalView::cellAt(const QPointF &pos) const {
  const int row = qBound(0, int(pos.y() / m_cellHeight), m_session.rows() - 1);
  const int col = qBound(0, int(pos.x() / m_cellWidth), m_session.cols() - 1);
  return {visibleToBuffer(row) + m_session.droppedLines(), col};
}

bool TerminalView::isSelected(int bufferLine, int col) const {
  CellPos start = m_selAnchor;
  CellPos end = m_selEnd;
  if (start.line > end.line || (start.line == end.line && start.col > end.col))
    qSwap(start, end);
  const qint64 line = bufferLine + m_session.droppedLines();
  if (line < start.line || line > end.line)
    return false;
  if (line == start.line && col < start.col)
    return false;
  if (line == end.line && col > end.col)
    return false;
  return true;
}

QString TerminalView::selectedText() const {
  const qint64 dropped = m_session.droppedLines();
  return m_session.text(int(m_selAnchor.line - dropped), m_selAnchor.col,
                        int(m_selEnd.line - dropped), m_selEnd.col);
}

void TerminalView::clearSelection() {
  if (!hasSelection())
    return;
  m_selecting = false;
  m_selected = false;
  update();
}

void TerminalView::copySelection(QClipboard::Mode mode) {
  if (!m_selected)
    return;
  const QString text = selectedText();
  if (!text.isEmpty())
    QGuiApplication::clipboard()->setText(text, mode);
}

void TerminalView::mousePressEvent(QMouseEvent *event) {
  forceActiveFocus();
  if (event->button() == Qt::MiddleButton) {
    // X11/Wayland convention: middle click pastes the primary selection.
    const QString text =
        QGuiApplication::clipboard()->text(QClipboard::Selection);
    if (!text.isEmpty())
      m_session.write(m_session.pasteSequence(text));
    event->accept();
    return;
  }
  m_selAnchor = cellAt(event->position());
  m_selEnd = m_selAnchor;
  m_selecting = true;
  m_selected = false;
  update();
  event->accept();
}

void TerminalView::mouseMoveEvent(QMouseEvent *event) {
  if (!m_selecting)
    return;
  m_selEnd = cellAt(event->position());
  update();
  event->accept();
}

void TerminalView::mouseReleaseEvent(QMouseEvent *event) {
  if (!m_selecting)
    return;
  m_selecting = false;
  // A click without a drag clears the selection instead of selecting a
  // single cell.
  m_selected = m_selAnchor.line != m_selEnd.line ||
               m_selAnchor.col != m_selEnd.col;
  if (m_selected)
    copySelection(QClipboard::Selection);
  update();
  event->accept();
}

void TerminalView::mouseDoubleClickEvent(QMouseEvent *event) {
  if (event->button() != Qt::LeftButton)
    return;
  const CellPos pos = cellAt(event->position());
  const auto [first, last] = m_session.wordBounds(
      int(pos.line - m_session.droppedLines()), pos.col);
  m_selAnchor = {pos.line, first};
  m_selEnd = {pos.line, last};
  m_selecting = false;
  m_selected = true;
  copySelection(QClipboard::Selection);
  update();
  event->accept();
}

void TerminalView::paint(QPainter *painter) {
  painter->setFont(m_font);
  painter->fillRect(boundingRect(), QColor(m_defaultBg));

  const int rows = m_session.rows();
  const int cols = m_session.cols();
  const bool showCursor = m_scrollOffset == 0 && m_session.cursorVisible();

  for (int r = 0; r < rows; ++r) {
    const QVector<TerminalCell> line = lineAt(r);
    const qreal y = r * m_cellHeight;
    int c = 0;
    // A wide character (head cell + empty continuation cell) is drawn on
    // its own across both cells: inside a run, its glyph's advance rarely
    // matches two cells exactly and would shift the rest of the line.
    const auto isWide = [&line](int col) {
      return col + 1 < line.size() && !line.at(col).ch.isEmpty() &&
             line.at(col + 1).ch.isEmpty();
    };
    while (c < cols && c < line.size()) {
      const TerminalCell &first = line.at(c);
      const bool wide = isWide(c);
      int runEnd = c + (wide ? 2 : 1);
      while (!wide && runEnd < cols && runEnd < line.size() &&
             !isWide(runEnd) && !line.at(runEnd).ch.isEmpty()) {
        const TerminalCell &next = line.at(runEnd);
        if (next.fg == first.fg && next.bg == first.bg &&
            next.bold == first.bold && next.underline == first.underline &&
            next.reverse == first.reverse && next.faint == first.faint)
          ++runEnd;
        else
          break;
      }

      QRgb fg = resolveColor(first.fg, true);
      QRgb bg = resolveColor(first.bg, false);
      if (first.reverse)
        qSwap(fg, bg);
      if (first.faint)
        fg = qRgb(qRed(fg) * 2 / 3, qGreen(fg) * 2 / 3, qBlue(fg) * 2 / 3);

      const QRectF runRect(c * m_cellWidth, y, (runEnd - c) * m_cellWidth,
                           m_cellHeight);
      if (bg != m_defaultBg)
        painter->fillRect(runRect, QColor(bg));

      QString text;
      text.reserve(runEnd - c);
      for (int i = c; i < runEnd; ++i)
        text += line.at(i).ch;

      QFont runFont = m_font;
      runFont.setBold(first.bold);
      runFont.setUnderline(first.underline);
      painter->setFont(runFont);
      painter->setPen(QColor(fg));
      painter->drawText(
          QRectF(c * m_cellWidth, y, (runEnd - c) * m_cellWidth, m_cellHeight),
          Qt::AlignLeft | Qt::AlignTop, text);

      c = runEnd;
    }

    if (hasSelection()) {
      const int bufferLine = visibleToBuffer(r);
      QColor overlay(m_selection);
      overlay.setAlpha(150);
      for (int col = 0; col < cols; ++col) {
        if (isSelected(bufferLine, col))
          painter->fillRect(QRectF(col * m_cellWidth, y, m_cellWidth,
                                   m_cellHeight),
                            overlay);
      }
    }
  }

  // With m_scrollOffset == 0 (the only time the cursor is drawn), a
  // visible row index equals the grid row index directly — lineAt()'s
  // window is anchored to the live bottom.
  if (showCursor) {
    const QRectF cursorRect(m_session.cursorCol() * m_cellWidth,
                            m_session.cursorRow() * m_cellHeight, m_cellWidth,
                            m_cellHeight);
    painter->fillRect(cursorRect, QColor(m_defaultFg).lighter(150));
  }
}

void TerminalView::setFontPixelSize(int size) {
  size = qBound(kMinFontSize, size, kMaxFontSize);
  m_font.setPixelSize(size);
  const QFontMetricsF metrics(m_font);
  m_cellWidth = metrics.horizontalAdvance(QLatin1Char('M'));
  m_cellHeight = metrics.height();
  QSettings settings;
  if (settings.value(QStringLiteral("terminal/fontPixelSize"), kDefaultFontSize)
          .toInt() != size)
    settings.setValue(QStringLiteral("terminal/fontPixelSize"), size);
  if (width() > 0 && height() > 0)
    updateGridSize();
  update();
}

void TerminalView::scrollTo(int offset) {
  m_scrollOffset = qBound(0, offset, int(m_session.scrollback().size()));
  update();
}

void TerminalView::keyPressEvent(QKeyEvent *event) {
  const int page = qMax(1, int(height() / m_cellHeight) - 1);
  switch (TerminalSession::viewShortcut(event->key(), event->modifiers())) {
  case TerminalSession::ViewShortcut::None:
    break;
  case TerminalSession::ViewShortcut::PageUp:
  case TerminalSession::ViewShortcut::PageDown:
  case TerminalSession::ViewShortcut::Top:
  case TerminalSession::ViewShortcut::Bottom:
    // Full-screen programs (vim, less) have no history here: the keys
    // are theirs.
    if (m_session.alternateScreen())
      break;
    switch (TerminalSession::viewShortcut(event->key(), event->modifiers())) {
    case TerminalSession::ViewShortcut::PageUp:
      scrollTo(m_scrollOffset + page);
      break;
    case TerminalSession::ViewShortcut::PageDown:
      scrollTo(m_scrollOffset - page);
      break;
    case TerminalSession::ViewShortcut::Top:
      scrollTo(int(m_session.scrollback().size()));
      break;
    default:
      scrollTo(0);
    }
    event->accept();
    return;
  case TerminalSession::ViewShortcut::ZoomIn:
    setFontPixelSize(m_font.pixelSize() + 1);
    event->accept();
    return;
  case TerminalSession::ViewShortcut::ZoomOut:
    setFontPixelSize(m_font.pixelSize() - 1);
    event->accept();
    return;
  case TerminalSession::ViewShortcut::ZoomReset:
    setFontPixelSize(kDefaultFontSize);
    event->accept();
    return;
  }
  if (TerminalSession::isCopyShortcut(event->key(), event->modifiers())) {
    copySelection(QClipboard::Clipboard);
    event->accept();
    return;
  }
  if (TerminalSession::isPasteShortcut(event->key(), event->modifiers())) {
    const QString text = QGuiApplication::clipboard()->text();
    if (!text.isEmpty())
      m_session.write(m_session.pasteSequence(text));
    event->accept();
    return;
  }
  const QByteArray out =
      m_session.keySequence(event->key(), event->modifiers(), event->text());
  if (out.isEmpty()) {
    event->ignore();
    return;
  }
  m_session.write(out);
  clearSelection();
  if (m_scrollOffset != 0) {
    m_scrollOffset = 0;
    update();
  }
  event->accept();
}

void TerminalView::dragEnterEvent(QDragEnterEvent *event) {
  if (event->mimeData()->hasUrls())
    event->acceptProposedAction();
}

void TerminalView::dropEvent(QDropEvent *event) {
  QStringList paths;
  for (const QUrl &url : event->mimeData()->urls())
    if (url.isLocalFile())
      paths << url.toLocalFile();
  if (paths.isEmpty())
    return;
  m_session.write(m_session.pasteSequence(TerminalSession::droppedPaths(paths)));
  event->acceptProposedAction();
  forceActiveFocus();
}

void TerminalView::wheelEvent(QWheelEvent *event) {
  if (event->modifiers() & Qt::ControlModifier) {
    const int notches = event->angleDelta().y() / 120;
    if (notches != 0)
      setFontPixelSize(m_font.pixelSize() + notches);
    event->accept();
    return;
  }
  if (m_session.alternateScreen()) {
    event->ignore();
    return;
  }
  const int steps = event->angleDelta().y() / 40; // ~3 lines per notch
  m_scrollOffset =
      qBound(0, m_scrollOffset + steps, m_session.scrollback().size());
  update();
  event->accept();
}
