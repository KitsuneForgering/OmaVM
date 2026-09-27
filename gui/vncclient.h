#pragma once

#include <QByteArray>
#include <QImage>
#include <QLocalSocket>
#include <QObject>

// Minimal RFB 3.8 (VNC) client for a local, unauthenticated, Raw-encoding
// only connection — the QEMU unix socket set up by the qemu backend. Not a
// general-purpose VNC client: no TLS/auth, no compressed encodings. That
// scope keeps the implementation self-contained (no GPL VNC library
// pulled into an MIT project) since the connection is always a private,
// trusted, local unix socket.
class VncClient final : public QObject {
  Q_OBJECT

public:
  explicit VncClient(QObject *parent = nullptr);

  void connectToSocket(const QString &path);

  // The most recently assembled framebuffer, or a null QImage before the
  // first update arrives.
  QImage frame() const { return m_frame; }

  Q_INVOKABLE void sendPointerEvent(int buttonMask, int x, int y);
  Q_INVOKABLE void sendKeyEvent(bool down, quint32 keysym);
  void resizeDesktop(int width, int height);
  void setClipboardEnabled(bool enabled);
  void sendClipboard(const QString &text);

signals:
  void clipboardReady();
  void clipboardReceived(const QString &text);
  void clipboardError(const QString &message);
  void resizeSupported();
  void frameUpdated();
  void errorOccurred(const QString &message);

private:
  enum class Step {
    ProtocolVersion,
    SecurityTypeCount,
    SecurityTypes,
    SecurityResult,
    ServerInitHeader,
    ServerInitName,
    MessageType,
    FramebufferUpdateHeader,
    RectHeader,
    RectData,
    DesktopScreens,
    BellOrSkip,
    ServerCutTextHeader,
    ServerCutTextData,
  };

  void onReadyRead();
  void process();
  bool have(qint64 n) const;
  QByteArray take(qint64 n);
  void fail(const QString &message);
  void requestUpdate(bool incremental);
  bool resizeFrame(int width, int height);
  void finishRect();
  void clipboardMessage(quint32 flags, const QByteArray &payload = {});
  void receiveClipboard(const QByteArray &data);

  QLocalSocket m_socket;
  QByteArray m_buffer;
  Step m_step = Step::ProtocolVersion;

  quint16 m_width = 0;
  quint16 m_height = 0;
  quint8 m_securityTypeCount = 0;
  quint32 m_pendingSkip = 0;

  quint16 m_rectCount = 0;
  quint16 m_rectX = 0;
  quint16 m_rectY = 0;
  quint16 m_rectW = 0;
  quint16 m_rectH = 0;
  qint32 m_rectEncoding = 0;

  QImage m_frame;
  bool m_ready = false;
  bool m_resizeSupported = false;
  bool m_failed = false;
  bool m_extendedClipboard = false;
  bool m_clipboardEnabled = false;
  bool m_clipboardSupported = false;
  QByteArray m_clipboardText;
};
