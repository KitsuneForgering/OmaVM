#include "vncview.h"

#include <QClipboard>
#include <QCursor>
#include <QGuiApplication>
#include <QHoverEvent>
#include <QKeyEvent>
#include <QMouseEvent>
#include <QPainter>
#include <QQuickWindow>
#include <QScopedValueRollback>
#include <QWheelEvent>

namespace {
// Latin-1 codepoints double as their own X11 keysym, which covers the
// letters/digits/punctuation the vast majority of typing needs. Anything
// else goes through this table for the keys that don't produce text.
quint32 keysymForSpecialKey(int key) {
  switch (key) {
  case Qt::Key_Backspace:
    return 0xff08;
  case Qt::Key_Tab:
    return 0xff09;
  case Qt::Key_Return:
  case Qt::Key_Enter:
    return 0xff0d;
  case Qt::Key_Escape:
    return 0xff1b;
  case Qt::Key_Delete:
    return 0xffff;
  case Qt::Key_Home:
    return 0xff50;
  case Qt::Key_Left:
    return 0xff51;
  case Qt::Key_Up:
    return 0xff52;
  case Qt::Key_Right:
    return 0xff53;
  case Qt::Key_Down:
    return 0xff54;
  case Qt::Key_PageUp:
    return 0xff55;
  case Qt::Key_PageDown:
    return 0xff56;
  case Qt::Key_End:
    return 0xff57;
  case Qt::Key_Insert:
    return 0xff63;
  case Qt::Key_CapsLock:
    return 0xffe5;
  case Qt::Key_Shift:
    return 0xffe1;
  case Qt::Key_Control:
    return 0xffe3;
  case Qt::Key_Alt:
    return 0xffe9;
  case Qt::Key_Meta:
  case Qt::Key_Super_L:
    return 0xffeb;
  case Qt::Key_Space:
    return 0x0020;
  default:
    if (key >= Qt::Key_F1 && key <= Qt::Key_F12)
      return 0xffbe + (key - Qt::Key_F1);
    return 0;
  }
}

quint32 keysymFor(QKeyEvent *event) {
  const QString text = event->text();
  if (text.size() == 1) {
    const ushort c = text.at(0).unicode();
    if (c >= 0x20 && c <= 0xff)
      return c;
  }
  return keysymForSpecialKey(event->key());
}
} // namespace

VncView::VncView(QQuickItem *parent) : QQuickPaintedItem(parent) {
  connect(QGuiApplication::clipboard(), &QClipboard::dataChanged, this,
          &VncView::syncClipboard);
  connect(&m_client, &VncClient::clipboardReady, this, &VncView::syncClipboard);
  connect(&m_client, &VncClient::clipboardError, this,
          &VncView::clipboardWarning);
  connect(&m_client, &VncClient::clipboardReceived, this,
          [this](const QString &text) {
            if (!m_shareClipboard || !window() || !window()->isActive())
              return;
            if (QGuiApplication::clipboard()->text() == text)
              return;
            QScopedValueRollback<bool> receiving(m_receivingClipboard, true);
            QGuiApplication::clipboard()->setText(text);
          });
  connect(this, &QQuickItem::windowChanged, this, [this](QQuickWindow *window) {
    if (window)
      connect(window, &QWindow::activeChanged, this, &VncView::syncClipboard);
  });
  setAcceptedMouseButtons(Qt::AllButtons);
  // Hover (not just drag) moves the remote pointer, and the host's own
  // cursor is hidden over the view: the guest already draws its own
  // cursor into the framebuffer pixels we render (we don't request VNC's
  // cursor pseudo-encoding), so showing the host arrow on top as well
  // just gives two visibly independent cursors.
  setAcceptHoverEvents(true);
  setCursor(QCursor(Qt::BlankCursor));
  setFlag(QQuickItem::ItemAcceptsInputMethod, true);
  connect(&m_client, &VncClient::frameUpdated, this, [this] { update(); });
  connect(&m_client, &VncClient::errorOccurred, this,
          &VncView::connectionFailed);
  m_resizeTimer.setSingleShot(true);
  m_resizeTimer.setInterval(250);
  connect(&m_resizeTimer, &QTimer::timeout, this, [this] {
    m_client.resizeDesktop(qRound(width()), qRound(height()));
  });
  connect(&m_client, &VncClient::resizeSupported, this,
          [this] { m_resizeTimer.start(); });
}

void VncView::setShareClipboard(bool enabled) {
  if (m_shareClipboard == enabled)
    return;
  m_shareClipboard = enabled;
  emit shareClipboardChanged();
  syncClipboard();
}

void VncView::syncClipboard() {
  const bool active = m_shareClipboard && window() && window()->isActive();
  m_client.setClipboardEnabled(active);
  if (active && !m_receivingClipboard)
    m_client.sendClipboard(QGuiApplication::clipboard()->text());
}

void VncView::geometryChange(const QRectF &newGeometry,
                             const QRectF &oldGeometry) {
  QQuickPaintedItem::geometryChange(newGeometry, oldGeometry);
  if (newGeometry.size() != oldGeometry.size())
    m_resizeTimer.start();
}

QRectF VncView::displayRect() const {
  const QImage frame = m_client.frame();
  if (frame.isNull())
    return {};
  QSizeF size = frame.size();
  size.scale(boundingRect().size(), Qt::KeepAspectRatio);
  return QRectF(
      QPointF((width() - size.width()) / 2, (height() - size.height()) / 2),
      size);
}

void VncView::setSocketPath(const QString &path) {
  if (m_socketPath == path)
    return;
  m_socketPath = path;
  emit socketPathChanged();
  m_client.connectToSocket(path);
  forceActiveFocus();
}

void VncView::paint(QPainter *painter) {
  const QImage frame = m_client.frame();
  painter->fillRect(boundingRect(), Qt::black);
  if (frame.isNull()) {
    painter->fillRect(boundingRect(), Qt::black);
    return;
  }
  painter->drawImage(displayRect(), frame);
}

int VncView::currentButtonMask(Qt::MouseButtons buttons) const {
  int mask = 0;
  if (buttons & Qt::LeftButton)
    mask |= 0x1;
  if (buttons & Qt::MiddleButton)
    mask |= 0x2;
  if (buttons & Qt::RightButton)
    mask |= 0x4;
  return mask;
}

void VncView::sendPointer(const QPointF &localPos, int buttonMask) {
  const QImage frame = m_client.frame();
  if (frame.isNull() || width() <= 0 || height() <= 0)
    return;
  const QRectF rect = displayRect();
  if (rect.isEmpty())
    return;
  const int x = int((localPos.x() - rect.x()) * frame.width() / rect.width());
  const int y = int((localPos.y() - rect.y()) * frame.height() / rect.height());
  m_client.sendPointerEvent(buttonMask, x, y);
}

void VncView::mousePressEvent(QMouseEvent *event) {
  forceActiveFocus();
  m_buttonMask = currentButtonMask(event->buttons());
  sendPointer(event->position(), m_buttonMask);
}

void VncView::mouseReleaseEvent(QMouseEvent *event) {
  m_buttonMask = currentButtonMask(event->buttons());
  sendPointer(event->position(), m_buttonMask);
}

void VncView::mouseMoveEvent(QMouseEvent *event) {
  sendPointer(event->position(), m_buttonMask);
}

void VncView::hoverMoveEvent(QHoverEvent *event) {
  sendPointer(event->position(), m_buttonMask);
}

void VncView::wheelEvent(QWheelEvent *event) {
  const int button = event->angleDelta().y() > 0 ? 0x8 : 0x10;
  const QPointF pos = event->position();
  sendPointer(pos, m_buttonMask | button);
  sendPointer(pos, m_buttonMask);
}

void VncView::keyPressEvent(QKeyEvent *event) {
  const quint32 keysym = keysymFor(event);
  if (keysym != 0)
    m_client.sendKeyEvent(true, keysym);
}

void VncView::keyReleaseEvent(QKeyEvent *event) {
  const quint32 keysym = keysymFor(event);
  if (keysym != 0)
    m_client.sendKeyEvent(false, keysym);
}
