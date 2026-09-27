#include "vncclient.h"

#include <QtEndian>
#include <zlib.h>

namespace {
constexpr char kProtocolVersion[] = "RFB 003.008\n";
constexpr quint8 kSecurityNone = 1;
constexpr quint32 Text = 1, Caps = 1u << 24, Request = 1u << 25,
                  Notify = 1u << 27, Provide = 1u << 28;
constexpr int MaxClipboard = 1024 * 1024;

quint16 readU16(const uchar *p) { return qFromBigEndian<quint16>(p); }
quint32 readU32(const uchar *p) { return qFromBigEndian<quint32>(p); }
qint32 readS32(const uchar *p) { return qFromBigEndian<qint32>(p); }

void appendU16(QByteArray &out, quint16 v) {
  quint16 be = qToBigEndian(v);
  out.append(reinterpret_cast<const char *>(&be), 2);
}
void appendU32(QByteArray &out, quint32 v) {
  quint32 be = qToBigEndian(v);
  out.append(reinterpret_cast<const char *>(&be), 4);
}
} // namespace

VncClient::VncClient(QObject *parent) : QObject(parent) {
  connect(&m_socket, &QLocalSocket::readyRead, this, &VncClient::onReadyRead);
  connect(
      &m_socket, &QLocalSocket::errorOccurred, this,
      [this](QLocalSocket::LocalSocketError) { fail(m_socket.errorString()); });
}

void VncClient::connectToSocket(const QString &path) {
  m_socket.connectToServer(path);
}

bool VncClient::have(qint64 n) const { return m_buffer.size() >= n; }

QByteArray VncClient::take(qint64 n) {
  QByteArray out = m_buffer.left(n);
  m_buffer.remove(0, n);
  return out;
}

void VncClient::fail(const QString &message) {
  if (m_failed)
    return;
  m_failed = true;
  m_ready = false;
  m_socket.abort();
  emit errorOccurred(message);
}

bool VncClient::resizeFrame(int width, int height) {
  if (width < 1 || height < 1 || width > 8192 || height > 8192 ||
      qint64(width) * height > 33554432) {
    fail("Unsupported display size (maximum 32 million pixels)");
    return false;
  }
  QImage frame(width, height, QImage::Format_RGB32);
  if (frame.isNull()) {
    fail("Unable to allocate display image");
    return false;
  }
  frame.fill(Qt::black);
  m_frame = frame;
  m_width = width;
  m_height = height;
  return true;
}

void VncClient::finishRect() {
  if (--m_rectCount > 0) {
    m_step = Step::RectHeader;
    return;
  }
  m_step = Step::MessageType;
  emit frameUpdated();
  requestUpdate(true);
}

void VncClient::onReadyRead() {
  m_buffer.append(m_socket.readAll());
  process();
}

void VncClient::process() {
  // Loops because one readyRead can deliver several protocol steps' worth
  // of bytes at once (e.g. the whole handshake arriving in one packet).
  while (!m_failed) {
    switch (m_step) {
    case Step::ProtocolVersion: {
      if (!have(12))
        return;
      take(12); // we only speak 3.8; assume the server does too.
      m_socket.write(kProtocolVersion, 12);
      m_step = Step::SecurityTypeCount;
      break;
    }
    case Step::SecurityTypeCount: {
      if (!have(1))
        return;
      m_securityTypeCount = static_cast<quint8>(take(1).at(0));
      if (m_securityTypeCount == 0) {
        m_step = Step::ServerCutTextHeader; // reuse: read u32 len + skip
        m_pendingSkip = 4;                  // length of the failure reason
        fail("VNC handshake failed before offering a security type");
        return;
      }
      m_step = Step::SecurityTypes;
      break;
    }
    case Step::SecurityTypes: {
      if (!have(m_securityTypeCount))
        return;
      const QByteArray types = take(m_securityTypeCount);
      if (!types.contains(static_cast<char>(kSecurityNone))) {
        fail("VNC server requires authentication; only unauthenticated "
             "local sockets are supported");
        return;
      }
      m_socket.write(reinterpret_cast<const char *>(&kSecurityNone), 1);
      m_step = Step::SecurityResult;
      break;
    }
    case Step::SecurityResult: {
      if (!have(4))
        return;
      const QByteArray result = take(4);
      if (readU32(reinterpret_cast<const uchar *>(result.constData())) != 0) {
        fail("VNC server rejected the connection");
        return;
      }
      const quint8 shared = 1;
      m_socket.write(reinterpret_cast<const char *>(&shared), 1);
      m_step = Step::ServerInitHeader;
      break;
    }
    case Step::ServerInitHeader: {
      // width(2) height(2) + 16-byte PIXEL_FORMAT + name-length(4)
      if (!have(24))
        return;
      const QByteArray header = take(24);
      const auto *p = reinterpret_cast<const uchar *>(header.constData());
      if (!resizeFrame(readU16(p), readU16(p + 2)))
        return;
      m_pendingSkip = readU32(p + 20);
      m_step = Step::ServerInitName;
      break;
    }
    case Step::ServerInitName: {
      if (!have(m_pendingSkip))
        return;
      take(m_pendingSkip); // desktop name: display-only, not needed.

      // SetPixelFormat: 32bpp truecolour, little-endian, matching
      // QImage::Format_RGB32's in-memory byte order exactly, so rects
      // decode with a straight memcpy per scanline.
      QByteArray setPixelFormat;
      setPixelFormat.append(char(0));    // message-type
      setPixelFormat.append(3, char(0)); // padding
      setPixelFormat.append(char(32));   // bits-per-pixel
      setPixelFormat.append(char(24));   // depth
      setPixelFormat.append(char(0));    // big-endian-flag: little-endian
      setPixelFormat.append(char(1));    // true-colour-flag
      appendU16(setPixelFormat, 255);    // red-max
      appendU16(setPixelFormat, 255);    // green-max
      appendU16(setPixelFormat, 255);    // blue-max
      setPixelFormat.append(char(16));   // red-shift
      setPixelFormat.append(char(8));    // green-shift
      setPixelFormat.append(char(0));    // blue-shift
      setPixelFormat.append(3, char(0)); // padding
      m_socket.write(setPixelFormat);

      QByteArray setEncodings;
      setEncodings.append(char(2)); // message-type
      setEncodings.append(char(0)); // padding
      appendU16(setEncodings, 4);
      appendU32(setEncodings, 0);             // Raw
      appendU32(setEncodings, quint32(-223)); // DesktopSize
      appendU32(setEncodings, quint32(-308)); // ExtendedDesktopSize
      appendU32(setEncodings, 0xc0a1e5ce);    // ExtendedClipboard
      m_socket.write(setEncodings);

      m_ready = true;
      requestUpdate(false);
      m_step = Step::MessageType;
      break;
    }
    case Step::MessageType: {
      if (!have(1))
        return;
      const auto type = static_cast<quint8>(take(1).at(0));
      if (type == 0) {
        m_step = Step::FramebufferUpdateHeader;
      } else if (type == 1) {
        fail("VNC server uses a colour map; unsupported (truecolour only)");
        return;
      } else if (type == 2) {
        // Bell: no body.
      } else if (type == 3) {
        m_step = Step::ServerCutTextHeader;
      } else {
        fail("unexpected VNC message type");
        return;
      }
      break;
    }
    case Step::FramebufferUpdateHeader: {
      if (!have(3))
        return;
      const QByteArray header = take(3); // padding(1) + rect-count(2)
      m_rectCount =
          readU16(reinterpret_cast<const uchar *>(header.constData()) + 1);
      m_step = m_rectCount > 0 ? Step::RectHeader : Step::MessageType;
      if (m_rectCount == 0)
        requestUpdate(true);
      break;
    }
    case Step::RectHeader: {
      if (!have(12))
        return;
      const QByteArray header = take(12);
      const auto *p = reinterpret_cast<const uchar *>(header.constData());
      m_rectX = readU16(p);
      m_rectY = readU16(p + 2);
      m_rectW = readU16(p + 4);
      m_rectH = readU16(p + 6);
      m_rectEncoding = readS32(p + 8);
      if (m_rectEncoding == -223) {
        if (!resizeFrame(m_rectW, m_rectH))
          return;
        finishRect();
        break;
      }
      if (m_rectEncoding == -308) {
        m_step = Step::DesktopScreens;
        break;
      }
      if (m_rectEncoding != 0) {
        fail("VNC server sent an encoding other than Raw");
        return;
      }
      if (int(m_rectX) + m_rectW > m_width ||
          int(m_rectY) + m_rectH > m_height) {
        fail("VNC rectangle exceeds display bounds");
        return;
      }
      m_step = Step::RectData;
      break;
    }
    case Step::RectData: {
      const qint64 needed = qint64(m_rectW) * m_rectH * 4;
      if (!have(needed))
        return;
      const QByteArray data = take(needed);
      const uchar *src = reinterpret_cast<const uchar *>(data.constData());
      for (int row = 0; row < m_rectH; ++row) {
        if (m_rectY + row >= m_frame.height())
          break;
        uchar *dst = m_frame.scanLine(m_rectY + row) + qint64(m_rectX) * 4;
        const uchar *srcRow = src + qint64(row) * m_rectW * 4;
        const int rowBytes =
            qMin(qint64(m_rectW) * 4, qint64(m_frame.width() - m_rectX) * 4);
        if (rowBytes > 0)
          memcpy(dst, srcRow, size_t(rowBytes));
      }
      finishRect();
      break;
    }
    case Step::DesktopScreens: {
      if (!have(4))
        return;
      const int screens = quint8(m_buffer.at(0));
      if (!have(4 + screens * 16))
        return;
      take(4 + screens * 16);
      if (m_rectY == 0 && !resizeFrame(m_rectW, m_rectH))
        return;
      const bool first = !m_resizeSupported;
      m_resizeSupported = true;
      finishRect();
      if (first)
        emit resizeSupported();
      break;
    }
    case Step::BellOrSkip: {
      m_step = Step::MessageType;
      break;
    }
    case Step::ServerCutTextHeader: {
      if (!have(7))
        return;
      const QByteArray header = take(7); // padding(3) + length(4)
      const qint32 length =
          readS32(reinterpret_cast<const uchar *>(header.constData()) + 3);
      m_extendedClipboard = length < 0;
      const qint64 size = length < 0 ? -qint64(length) : length;
      if (size > MaxClipboard + 65536 || (m_extendedClipboard && size < 4)) {
        fail("Invalid clipboard message size");
        return;
      }
      m_pendingSkip = quint32(size);
      m_step = Step::ServerCutTextData;
      break;
    }
    case Step::ServerCutTextData: {
      if (!have(m_pendingSkip))
        return;
      const QByteArray data = take(m_pendingSkip);
      if (m_extendedClipboard)
        receiveClipboard(data);
      else if (m_clipboardEnabled)
        emit clipboardReceived(QString::fromLatin1(data));
      m_step = Step::MessageType;
      break;
    }
    }
  }
}

void VncClient::clipboardMessage(quint32 flags, const QByteArray &payload) {
  if (!m_ready)
    return;
  QByteArray message(4, '\0');
  message[0] = char(6);
  appendU32(message, quint32(-qint32(4 + payload.size())));
  appendU32(message, flags);
  message.append(payload);
  m_socket.write(message);
}

void VncClient::setClipboardEnabled(bool enabled) {
  if (m_clipboardEnabled == enabled)
    return;
  m_clipboardEnabled = enabled;
  if (!enabled) {
    m_clipboardText.clear();
    if (m_clipboardSupported)
      clipboardMessage(Notify);
  }
}

void VncClient::sendClipboard(const QString &text) {
  if (!m_clipboardEnabled || !m_clipboardSupported)
    return;
  const QByteArray data = text.toUtf8();
  if (data.size() > MaxClipboard - 5) {
    m_clipboardText.clear();
    clipboardMessage(Notify);
    emit clipboardError("Clipboard text is too large (maximum 1 MiB)");
    return;
  }
  m_clipboardText = data;
  m_clipboardText.append('\0');
  clipboardMessage(Notify | Text);
}

void VncClient::receiveClipboard(const QByteArray &data) {
  const quint32 flags =
      readU32(reinterpret_cast<const uchar *>(data.constData()));
  if (flags & Caps) {
    m_clipboardSupported = (flags & (Text | Request | Notify | Provide)) ==
                           (Text | Request | Notify | Provide);
    QByteArray limit;
    appendU32(limit, 0); // request before transfer
    clipboardMessage(Caps | Text | Request | Notify | Provide, limit);
    if (m_clipboardSupported)
      emit clipboardReady();
    return;
  }
  if (!m_clipboardEnabled || !m_clipboardSupported)
    return;
  if ((flags & Notify) && (flags & Text))
    clipboardMessage(Request | Text);
  if ((flags & Request) && (flags & Text) && !m_clipboardText.isEmpty()) {
    QByteArray plain;
    appendU32(plain, m_clipboardText.size());
    plain.append(m_clipboardText);
    const QByteArray compressed = qCompress(plain).mid(4);
    if (compressed.size() > MaxClipboard - 4) {
      m_clipboardText.clear();
      clipboardMessage(Notify);
      emit clipboardError("Clipboard text is too large to transfer");
      return;
    }
    clipboardMessage(Provide | Text, compressed);
  }
  if ((flags & Provide) && (flags & Text)) {
    QByteArray plain(MaxClipboard, '\0');
    uLongf length = plain.size();
    const int result = uncompress(
        reinterpret_cast<Bytef *>(plain.data()), &length,
        reinterpret_cast<const Bytef *>(data.constData() + 4), data.size() - 4);
    if (result != Z_OK || length < 4) {
      emit clipboardError("Invalid or oversized clipboard data");
      return;
    }
    const quint32 size =
        readU32(reinterpret_cast<const uchar *>(plain.constData()));
    if (size > length - 4) {
      emit clipboardError("Invalid clipboard text length");
      return;
    }
    QByteArray text = plain.mid(4, size);
    if (text.endsWith('\0'))
      text.chop(1);
    emit clipboardReceived(QString::fromUtf8(text));
  }
}

void VncClient::requestUpdate(bool incremental) {
  QByteArray request;
  request.append(char(3)); // message-type: FramebufferUpdateRequest
  request.append(char(incremental ? 1 : 0));
  appendU16(request, 0);
  appendU16(request, 0);
  appendU16(request, m_width);
  appendU16(request, m_height);
  m_socket.write(request);
}

void VncClient::sendPointerEvent(int buttonMask, int x, int y) {
  if (!m_ready)
    return;
  QByteArray event;
  event.append(char(5)); // message-type: PointerEvent
  event.append(char(buttonMask & 0xff));
  appendU16(event, quint16(qBound(0, x, int(m_width) - 1)));
  appendU16(event, quint16(qBound(0, y, int(m_height) - 1)));
  m_socket.write(event);
}

void VncClient::sendKeyEvent(bool down, quint32 keysym) {
  if (!m_ready)
    return;
  QByteArray event;
  event.append(char(4)); // message-type: KeyEvent
  event.append(char(down ? 1 : 0));
  appendU16(event, 0); // padding
  appendU32(event, keysym);
  m_socket.write(event);
}

void VncClient::resizeDesktop(int width, int height) {
  if (!m_ready || !m_resizeSupported || width < 1 || height < 1 ||
      width > 8192 || height > 8192 || qint64(width) * height > 33554432)
    return;
  if (width == m_width && height == m_height)
    return;
  QByteArray request;
  request.append(char(251));
  request.append(char(0));
  appendU16(request, width);
  appendU16(request, height);
  request.append(char(1)); // one screen
  request.append(char(0));
  appendU32(request, 0); // QEMU's single screen ID
  appendU16(request, 0);
  appendU16(request, 0);
  appendU16(request, width);
  appendU16(request, height);
  appendU32(request, 0);
  m_socket.write(request);
}
