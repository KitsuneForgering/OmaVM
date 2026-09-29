#pragma once

#include <QCursor>
#include <QQuickItem>
#include <QSet>
#include <QTimer>

#include "displayclient.h"

class QSGTexture;

// Shows a Machine's display and drives it: frames from a DisplayClient
// are drawn as GPU textures (dma-bufs imported through EGL, zero-copy) or,
// without a host GPU, uploaded from shared memory; keyboard, pointer,
// clipboard and window size go back to the guest.
class DisplayView : public QQuickItem {
  Q_OBJECT
  Q_PROPERTY(int connectionFd READ connectionFd WRITE setConnectionFd NOTIFY
                 connectionFdChanged)
  Q_PROPERTY(bool shareClipboard READ shareClipboard WRITE setShareClipboard
                 NOTIFY shareClipboardChanged)

public:
  explicit DisplayView(QQuickItem *parent = nullptr);
  ~DisplayView() override;

  int connectionFd() const { return m_connectionFd; }
  void setConnectionFd(int fd);
  bool shareClipboard() const { return m_shareClipboard; }
  void setShareClipboard(bool enabled);

signals:
  void connectionFdChanged();
  void shareClipboardChanged();
  void connectionFailed(const QString &message);

protected:
  QSGNode *updatePaintNode(QSGNode *oldNode, UpdatePaintNodeData *) override;
  void releaseResources() override;
  void geometryChange(const QRectF &newGeometry,
                      const QRectF &oldGeometry) override;
  void mousePressEvent(QMouseEvent *event) override;
  void mouseReleaseEvent(QMouseEvent *event) override;
  void mouseMoveEvent(QMouseEvent *event) override;
  void hoverMoveEvent(QHoverEvent *event) override;
  void wheelEvent(QWheelEvent *event) override;
  void keyPressEvent(QKeyEvent *event) override;
  void keyReleaseEvent(QKeyEvent *event) override;
  void focusOutEvent(QFocusEvent *event) override;

private:
  // GPU resources, touched only on the render thread.
  struct GpuFrame {
    void *eglImage = nullptr;
    unsigned texture = 0;
    QSGTexture *sgTexture = nullptr;
  };

  QSize frameSize() const;
  QRectF displayRect() const;
  void sendPointer(const QPointF &position);
  void sendResize();
  void syncClipboard();
  void releaseKeys();
  bool importDmabuf();
  static void destroyGpuFrame(GpuFrame frame);

  DisplayClient m_client;
  int m_connectionFd = -1;
  bool m_shareClipboard = false;
  bool m_receivingClipboard = false;

  // Latest dma-buf scanout, waiting to be imported on the render thread.
  DisplayClient::Dmabuf m_pendingDmabuf;
  DisplayClient::Dmabuf m_dmabuf;
  bool m_dmabufMode = false;
  bool m_imageDirty = false;
  GpuFrame m_gpu;
  QSGTexture *m_imageTexture = nullptr;

  QTimer m_resizeTimer;
  QPointF m_lastPointer;
  bool m_havePointer = false;
  int m_wheelDelta = 0;
  QSet<quint32> m_pressedKeys;
  QCursor m_guestCursor = QCursor(Qt::BlankCursor);
  bool m_guestCursorVisible = true;
};
