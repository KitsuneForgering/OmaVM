#pragma once

#include <QClipboard>
#include <QFont>
#include <QQuickPaintedItem>
#include <QTimer>

#include "terminal.h"

// Renders a TerminalSession's grid and forwards keyboard/wheel input back
// to it — the terminal-mode counterpart of DisplayView for a Machine's
// display. See gui/terminal.h for what the parser does and doesn't cover.
class TerminalView : public QQuickPaintedItem {
  Q_OBJECT
  Q_PROPERTY(
      QString envName READ envName WRITE setEnvName NOTIFY envNameChanged)
  // Whether programs in the Box may set the clipboard (OSC 52). The Box's
  // own setting, on by default.
  Q_PROPERTY(bool shareClipboard MEMBER m_shareClipboard)

public:
  explicit TerminalView(QQuickItem *parent = nullptr);

  QString envName() const { return m_envName; }
  void setEnvName(const QString &name);

  void paint(QPainter *painter) override;
  // Runs `omavm open` again after the session ended, below what the last
  // one printed (e.g. after starting the Box's container engine).
  Q_INVOKABLE void restart();
  Q_INVOKABLE void reloadPalette();

signals:
  void envNameChanged();
  void titleChanged(const QString &title);
  void finished(int exitCode);
  void errorOccurred(const QString &message);

protected:
  void geometryChange(const QRectF &newGeometry,
                      const QRectF &oldGeometry) override;
  void keyPressEvent(QKeyEvent *event) override;
  void wheelEvent(QWheelEvent *event) override;
  void mousePressEvent(QMouseEvent *event) override;
  void mouseMoveEvent(QMouseEvent *event) override;
  void mouseReleaseEvent(QMouseEvent *event) override;
  void mouseDoubleClickEvent(QMouseEvent *event) override;

private:
  // A cell in buffer coordinates that survive scrollback trimming: line
  // counts from the first line ever written (see droppedLines()).
  struct CellPos {
    qint64 line = 0;
    int col = 0;
  };
  CellPos cellAt(const QPointF &pos) const;
  bool hasSelection() const {
    return m_selected || (m_selecting && (m_selAnchor.line != m_selEnd.line ||
                                          m_selAnchor.col != m_selEnd.col));
  }
  bool isSelected(int bufferLine, int col) const;
  QString selectedText() const;
  void clearSelection();
  void copySelection(QClipboard::Mode mode);

  void updateGridSize();
  void loadPalette();
  // Applies a font size in pixels (bounded), remembers it for the next
  // terminal, and refits the grid to the window.
  void setFontPixelSize(int size);
  void scrollTo(int offset);
  QRgb resolveColor(const TerminalColor &color, bool foreground) const;
  QVector<TerminalCell> lineAt(int visibleRow) const;
  int visibleToBuffer(int visibleRow) const;

  QString m_envName;
  TerminalSession m_session;
  QFont m_font;
  qreal m_cellWidth = 8;
  qreal m_cellHeight = 16;
  int m_scrollOffset = 0;
  QTimer m_resizeTimer;

  CellPos m_selAnchor;
  CellPos m_selEnd;
  bool m_selecting = false; // a drag is in progress
  bool m_selected = false;  // a finished selection is shown
  bool m_shareClipboard = true;
  QRgb m_selection;

  QRgb m_palette[16];
  QRgb m_defaultFg;
  QRgb m_defaultBg;
};
