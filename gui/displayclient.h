#pragma once

#include <QImage>
#include <QList>
#include <QMetaType>
#include <QObject>
#include <QPoint>
#include <QRect>
#include <QString>
#include <QTimer>

struct _GCancellable;
struct _GDBusConnection;
struct _GDBusMethodInvocation;
struct _GVariant;

// A peer-to-peer client for QEMU's D-Bus display (-display dbus,p2p=yes),
// the successor of the RFB client this viewer used before: frames arrive
// as dma-bufs the host GPU can sample directly (or, without a usable host
// GPU, as shared memory mappings), instead of pixels copied over a socket.
//
// The protocol is spoken with GDBus rather than QtDBus because every
// connection here is an already-connected socket handed over by someone
// else (QEMU gets the other end through QMP), which QtDBus cannot adopt.
// The interface is documented in QEMU's ui/dbus-display1.xml.
class DisplayClient : public QObject {
  Q_OBJECT

public:
  // A dma-buf scanout. The receiver of dmabufScanout owns fd.
  struct Dmabuf {
    int fd = -1;
    quint32 width = 0;
    quint32 height = 0;
    quint32 stride = 0;
    quint32 fourcc = 0;
    quint64 modifier = 0;
    bool y0Top = true;
  };

  explicit DisplayClient(QObject *parent = nullptr);
  ~DisplayClient() override;

  // Adopts fd, a socket QEMU accepted through QMP add_client.
  void connectToFd(int fd);

  // The current frame when it is shared as memory (Scanout/ScanoutMap);
  // null while frames come as dma-bufs.
  QImage frame() const { return m_frame; }
  bool mouseAbsolute() const { return m_mouseAbsolute; }

  void pressKey(quint32 qnum);
  void releaseKey(quint32 qnum);
  // Guest framebuffer coordinates; only valid while mouseAbsolute().
  void movePointer(int x, int y);
  void movePointerBy(int dx, int dy);
  // QEMU button numbers: 0 left, 1 middle, 2 right, 3 wheel up,
  // 4 wheel down, 5 side, 6 extra.
  void pressButton(int button);
  void releaseButton(int button);
  // Asks the guest to adopt this resolution (virtio-gpu honors it).
  void resizeDisplay(int width, int height, int widthMM, int heightMM);

  // Text clipboard sharing with the guest; off until enabled.
  void setClipboardEnabled(bool enabled);
  void sendClipboard(const QString &text);

  // Acknowledges dma-buf updates once they reached the screen. QEMU holds
  // the guest's GPU until then, which paces rendering to the viewer.
  void frameRendered();

  // Called from GDBus callbacks.
  void handleConnection(_GDBusConnection *connection, bool listener);
  void handleListenerCall(const char *method, _GVariant *parameters,
                          _GDBusMethodInvocation *invocation);
  void handleClipboardCall(const char *method, _GVariant *parameters,
                           _GDBusMethodInvocation *invocation);
  void handleConsoleIds(_GVariant *ids);
  void handleMouseAbsolute(bool absolute);
  void handleGuestClipboard(const QByteArray &text);
  void fail(const QString &message);

signals:
  void dmabufScanout(DisplayClient::Dmabuf buffer);
  // frame() was replaced (new size or format).
  void imageScanout();
  void frameUpdated(const QRect &rect);
  void displayDisabled();
  void cursorChanged(const QImage &image, const QPoint &hotSpot);
  void cursorVisibilityChanged(bool visible);
  void mouseAbsoluteChanged();
  void clipboardReceived(const QString &text);
  void errorOccurred(const QString &message);

private:
  void registerListener();
  void call(const char *interface, const char *method, _GVariant *parameters);
  void completePendingUpdates();

  _GCancellable *m_cancellable = nullptr;
  _GDBusConnection *m_connection = nullptr;
  _GDBusConnection *m_listener = nullptr;
  unsigned m_listenerObjects[2] = {0, 0};
  unsigned m_clipboardObject = 0;
  unsigned m_propertiesSubscription = 0;
  QByteArray m_consolePath;
  QImage m_frame;
  bool m_mouseAbsolute = false;
  bool m_failed = false;
  // UpdateDMABUF calls answered by frameRendered(), or by m_updateTimeout
  // when nothing is being drawn (a hidden window never renders).
  QList<_GDBusMethodInvocation *> m_pendingUpdates;
  QTimer m_updateTimeout;

  bool m_clipboardEnabled = false;
  quint32 m_clipboardSerial = 0;
  QByteArray m_hostClipboard;
};

Q_DECLARE_METATYPE(DisplayClient::Dmabuf)
