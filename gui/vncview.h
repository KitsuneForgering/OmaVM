#pragma once

#include <QQuickPaintedItem>
#include <QTimer>

#include "vncclient.h"

// Renders a VncClient's framebuffer and forwards mouse/keyboard input back
// over the connection — the whole point of Open()'ing a Machine being able
// to actually drive it, not just look at it.
class VncView : public QQuickPaintedItem {
  Q_OBJECT
  Q_PROPERTY(QString socketPath READ socketPath WRITE setSocketPath NOTIFY
                 socketPathChanged)
  Q_PROPERTY(bool shareClipboard READ shareClipboard WRITE setShareClipboard
                 NOTIFY shareClipboardChanged)

public:
  explicit VncView(QQuickItem *parent = nullptr);

  QString socketPath() const { return m_socketPath; }
  void setSocketPath(const QString &path);

  void paint(QPainter *painter) override;
  bool shareClipboard() const { return m_shareClipboard; }
  void setShareClipboard(bool enabled);

signals:
  void shareClipboardChanged();
  void clipboardWarning(const QString &message);
  void socketPathChanged();
  void connectionFailed(const QString &message);

protected:
  void geometryChange(const QRectF &newGeometry,
                      const QRectF &oldGeometry) override;
  void mousePressEvent(QMouseEvent *event) override;
  void mouseReleaseEvent(QMouseEvent *event) override;
  void mouseMoveEvent(QMouseEvent *event) override;
  void hoverMoveEvent(QHoverEvent *event) override;
  void wheelEvent(QWheelEvent *event) override;
  void keyPressEvent(QKeyEvent *event) override;
  void keyReleaseEvent(QKeyEvent *event) override;

private:
  void syncClipboard();
  bool m_shareClipboard = false;
  bool m_receivingClipboard = false;
  QRectF displayRect() const;
  QTimer m_resizeTimer;
  void sendPointer(const QPointF &localPos, int buttonMask);
  int currentButtonMask(Qt::MouseButtons buttons) const;

  QString m_socketPath;
  VncClient m_client;
  int m_buttonMask = 0;
};
