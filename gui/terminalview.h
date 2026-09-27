#pragma once

#include <QFont>
#include <QQuickPaintedItem>
#include <QTimer>

#include "terminal.h"

// Renders a TerminalSession's grid and forwards keyboard/wheel input back
// to it — the terminal-mode counterpart of VncView for a Machine's
// display. See gui/terminal.h for what the parser does and doesn't cover.
class TerminalView : public QQuickPaintedItem {
  Q_OBJECT
  Q_PROPERTY(
      QString envName READ envName WRITE setEnvName NOTIFY envNameChanged)

public:
  explicit TerminalView(QQuickItem *parent = nullptr);

  QString envName() const { return m_envName; }
  void setEnvName(const QString &name);

  void paint(QPainter *painter) override;

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

private:
  void updateGridSize();
  void loadPalette();
  QRgb resolveColor(const TerminalColor &color, bool foreground) const;
  QVector<TerminalCell> lineAt(int visibleRow) const;

  QString m_envName;
  TerminalSession m_session;
  QFont m_font;
  qreal m_cellWidth = 8;
  qreal m_cellHeight = 16;
  int m_scrollOffset = 0;
  QTimer m_resizeTimer;

  QRgb m_palette[16];
  QRgb m_defaultFg;
  QRgb m_defaultBg;
};
