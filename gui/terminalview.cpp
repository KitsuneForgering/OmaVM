#include "terminalview.h"
#include "colorstoml.h"

#include <QDir>
#include <QFontMetricsF>
#include <QKeyEvent>
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

} // namespace

TerminalView::TerminalView(QQuickItem *parent) : QQuickPaintedItem(parent) {
  m_font = QFont(QStringLiteral("monospace"));
  m_font.setStyleHint(QFont::Monospace);
  m_font.setPixelSize(15);
  const QFontMetricsF metrics(m_font);
  m_cellWidth = metrics.horizontalAdvance(QLatin1Char('M'));
  m_cellHeight = metrics.height();

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

void TerminalView::setEnvName(const QString &name) {
  if (m_envName == name)
    return;
  m_envName = name;
  emit envNameChanged();
  const int cols = qMax(1, int(width() / m_cellWidth));
  const int rows = qMax(1, int(height() / m_cellHeight));
  m_session.start(m_envName, cols > 1 ? cols : 80, rows > 1 ? rows : 24);
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

QVector<TerminalCell> TerminalView::lineAt(int visibleRow) const {
  const auto &scrollback = m_session.scrollback();
  const auto &grid = m_session.grid();
  const int total = scrollback.size() + grid.size();
  const int bottomIndex = total - 1 - m_scrollOffset;
  const int sourceIndex = bottomIndex - (m_session.rows() - 1 - visibleRow);
  if (sourceIndex < 0 || sourceIndex >= total)
    return QVector<TerminalCell>(m_session.cols());
  if (sourceIndex < scrollback.size())
    return scrollback.at(sourceIndex);
  return grid.at(sourceIndex - scrollback.size());
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
    while (c < cols && c < line.size()) {
      const TerminalCell &first = line.at(c);
      int runEnd = c + 1;
      while (runEnd < cols && runEnd < line.size()) {
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

void TerminalView::keyPressEvent(QKeyEvent *event) {
  QByteArray out;
  const int key = event->key();
  const Qt::KeyboardModifiers mods = event->modifiers();
  const bool ctrl = mods & Qt::ControlModifier;
  const bool alt = mods & Qt::AltModifier;

  switch (key) {
  case Qt::Key_Return:
  case Qt::Key_Enter:
    out = "\r";
    break;
  case Qt::Key_Backspace:
    out = "\x7f";
    break;
  case Qt::Key_Tab:
    out = "\t";
    break;
  case Qt::Key_Escape:
    out = "\x1b";
    break;
  case Qt::Key_Up:
    out = "\x1b[A";
    break;
  case Qt::Key_Down:
    out = "\x1b[B";
    break;
  case Qt::Key_Right:
    out = "\x1b[C";
    break;
  case Qt::Key_Left:
    out = "\x1b[D";
    break;
  case Qt::Key_Home:
    out = "\x1b[H";
    break;
  case Qt::Key_End:
    out = "\x1b[F";
    break;
  case Qt::Key_Insert:
    out = "\x1b[2~";
    break;
  case Qt::Key_Delete:
    out = "\x1b[3~";
    break;
  case Qt::Key_PageUp:
    out = "\x1b[5~";
    break;
  case Qt::Key_PageDown:
    out = "\x1b[6~";
    break;
  default:
    if (key >= Qt::Key_F1 && key <= Qt::Key_F4) {
      static const char *codes[] = {"\x1bOP", "\x1bOQ", "\x1bOR", "\x1bOS"};
      out = codes[key - Qt::Key_F1];
    } else if (key >= Qt::Key_F5 && key <= Qt::Key_F12) {
      static const int nums[] = {15, 17, 18, 19, 20, 21, 23, 24};
      out = QByteArray("\x1b[") + QByteArray::number(nums[key - Qt::Key_F5]) +
            "~";
    } else if (ctrl && key >= Qt::Key_A && key <= Qt::Key_Z) {
      out = QByteArray(1, char(key - Qt::Key_A + 1));
    } else {
      const QString text = event->text();
      if (!text.isEmpty())
        out = text.toUtf8();
    }
    break;
  }

  if (out.isEmpty()) {
    event->ignore();
    return;
  }
  if (alt && !out.startsWith('\x1b'))
    out.prepend('\x1b');
  m_session.write(out);
  if (m_scrollOffset != 0) {
    m_scrollOffset = 0;
    update();
  }
  event->accept();
}

void TerminalView::wheelEvent(QWheelEvent *event) {
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
