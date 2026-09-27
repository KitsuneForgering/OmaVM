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

signals:
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
};
