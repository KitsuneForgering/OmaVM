#include "displayview.h"
#include "keymap.h"

#include <QClipboard>
#include <QGuiApplication>
#include <QHoverEvent>
#include <QKeyEvent>
#include <QMouseEvent>
#include <QOpenGLContext>
#include <QOpenGLFunctions>
#include <QPixmap>
#include <QQuickOpenGLUtils>
#include <QQuickWindow>
#include <QRunnable>
#include <functional>
#include <QSGSimpleTextureNode>
#include <QScopedValueRollback>
#include <QScreen>
#include <QWheelEvent>
#include <QtQuick/qsgtexture_platform.h>

#define EGL_NO_X11
#include <EGL/egl.h>
#include <EGL/eglext.h>

#include <unistd.h>

namespace {
// DRM_FORMAT_MOD_INVALID: the buffer's layout is implied, not declared.
constexpr quint64 ModifierInvalid = 0x00ffffffffffffffULL;

using ImageTargetTexture = void (*)(GLenum target, void *image);

struct Egl {
  PFNEGLCREATEIMAGEKHRPROC createImage = nullptr;
  PFNEGLDESTROYIMAGEKHRPROC destroyImage = nullptr;
  ImageTargetTexture imageTargetTexture = nullptr;
};

const Egl &egl() {
  static const Egl functions = [] {
    Egl f;
    f.createImage = reinterpret_cast<PFNEGLCREATEIMAGEKHRPROC>(
        eglGetProcAddress("eglCreateImageKHR"));
    f.destroyImage = reinterpret_cast<PFNEGLDESTROYIMAGEKHRPROC>(
        eglGetProcAddress("eglDestroyImageKHR"));
    f.imageTargetTexture = reinterpret_cast<ImageTargetTexture>(
        eglGetProcAddress("glEGLImageTargetTexture2DOES"));
    return f;
  }();
  return functions;
}

int qemuButton(Qt::MouseButton button) {
  switch (button) {
  case Qt::LeftButton:
    return 0;
  case Qt::MiddleButton:
    return 1;
  case Qt::RightButton:
    return 2;
  case Qt::BackButton:
    return 5;
  case Qt::ForwardButton:
    return 6;
  default:
    return -1;
  }
}

quint32 qnumFor(const QKeyEvent *event) {
  // X11/Wayland keycodes are evdev codes offset by 8.
  const quint32 native = event->nativeScanCode();
  return native > 8 ? evdevToQnum(native - 8) : 0;
}
} // namespace

// Frees a frame's GPU resources on the render thread, where its GL
// context is current.
class GpuFrameCleanup : public QRunnable {
public:
  explicit GpuFrameCleanup(std::function<void()> cleanup)
      : m_cleanup(std::move(cleanup)) {}
  void run() override { m_cleanup(); }

private:
  std::function<void()> m_cleanup;
};

DisplayView::DisplayView(QQuickItem *parent) : QQuickItem(parent) {
  setFlag(ItemHasContents, true);
  setFlag(ItemAcceptsInputMethod, true);
  setAcceptedMouseButtons(Qt::AllButtons);
  setAcceptHoverEvents(true);
  // Until the guest defines a cursor it may be drawing its own into the
  // framebuffer; a host arrow on top would be a second, lagging pointer.
  setCursor(m_guestCursor);

  connect(&m_client, &DisplayClient::dmabufScanout, this,
          [this](DisplayClient::Dmabuf buffer) {
            if (m_pendingDmabuf.fd >= 0)
              close(m_pendingDmabuf.fd);
            m_pendingDmabuf = buffer;
            m_dmabuf = buffer;
            m_dmabufMode = true;
            sendResize();
            update();
          });
  connect(&m_client, &DisplayClient::imageScanout, this, [this] {
    m_dmabufMode = false;
    m_imageDirty = true;
    sendResize();
    update();
  });
  connect(&m_client, &DisplayClient::frameUpdated, this, [this] {
    if (!m_dmabufMode)
      m_imageDirty = true;
    update();
  });
  connect(&m_client, &DisplayClient::displayDisabled, this, [this] {
    m_dmabufMode = false;
    m_dmabuf = {};
    m_imageDirty = true;
    update();
  });
  connect(&m_client, &DisplayClient::cursorChanged, this,
          [this](const QImage &image, const QPoint &hotSpot) {
            m_guestCursor =
                QCursor(QPixmap::fromImage(image), hotSpot.x(), hotSpot.y());
            if (m_guestCursorVisible)
              setCursor(m_guestCursor);
          });
  connect(&m_client, &DisplayClient::cursorVisibilityChanged, this,
          [this](bool visible) {
            m_guestCursorVisible = visible;
            setCursor(visible ? m_guestCursor : QCursor(Qt::BlankCursor));
          });
  connect(&m_client, &DisplayClient::errorOccurred, this,
          &DisplayView::connectionFailed);
  connect(&m_client, &DisplayClient::clipboardReceived, this,
          [this](const QString &text) {
            if (!m_shareClipboard || !window() || !window()->isActive() ||
                m_clipboardDirection == QStringLiteral("to-guest"))
              return;
            QScopedValueRollback<bool> receiving(m_receivingClipboard, true);
            QGuiApplication::clipboard()->setText(text);
          });
  connect(QGuiApplication::clipboard(), &QClipboard::dataChanged, this,
          &DisplayView::syncClipboard);
  connect(this, &QQuickItem::windowChanged, this, [this](QQuickWindow *win) {
    if (!win)
      return;
    connect(win, &QWindow::activeChanged, this, &DisplayView::syncClipboard);
    // Emitted on the render thread once a frame is on screen.
    connect(win, &QQuickWindow::frameSwapped, this,
            [this] { m_client.frameRendered(); }, Qt::QueuedConnection);
  });

  m_resizeTimer.setSingleShot(true);
  m_resizeTimer.setInterval(250);
  connect(&m_resizeTimer, &QTimer::timeout, this, &DisplayView::sendResize);
}

DisplayView::~DisplayView() {
  if (m_pendingDmabuf.fd >= 0)
    close(m_pendingDmabuf.fd);
}

void DisplayView::setConnectionFd(int fd) {
  if (m_connectionFd >= 0 || fd < 0)
    return;
  m_connectionFd = fd;
  emit connectionFdChanged();
  m_client.connectToFd(fd);
  forceActiveFocus();
}

void DisplayView::setShareClipboard(bool enabled) {
  if (m_shareClipboard == enabled)
    return;
  m_shareClipboard = enabled;
  emit shareClipboardChanged();
  syncClipboard();
}

void DisplayView::syncClipboard() {
  const bool active = m_shareClipboard && window() && window()->isActive();
  m_client.setClipboardEnabled(active);
  if (active && !m_receivingClipboard &&
      m_clipboardDirection != QStringLiteral("to-host"))
    m_client.sendClipboard(QGuiApplication::clipboard()->text());
}

QSize DisplayView::frameSize() const {
  if (m_dmabufMode)
    return QSize(int(m_dmabuf.width), int(m_dmabuf.height));
  return m_client.frame().size();
}

QRectF DisplayView::displayRect() const {
  const QSize frame = frameSize();
  if (frame.isEmpty())
    return {};
  QSizeF size = frame;
  size.scale(boundingRect().size(), Qt::KeepAspectRatio);
  return QRectF(
      QPointF((width() - size.width()) / 2, (height() - size.height()) / 2),
      size);
}

bool DisplayView::importDmabuf() {
  const int fd = m_pendingDmabuf.fd;
  const DisplayClient::Dmabuf buffer = m_pendingDmabuf;
  m_pendingDmabuf = {};
  QOpenGLContext *context = QOpenGLContext::currentContext();
  const EGLDisplay display = eglGetCurrentDisplay();
  const Egl &f = egl();
  if (!context || display == EGL_NO_DISPLAY || !f.createImage ||
      !f.imageTargetTexture) {
    close(fd);
    return false;
  }
  QList<EGLAttrib> attributes = {
      EGL_WIDTH,
      EGLAttrib(buffer.width),
      EGL_HEIGHT,
      EGLAttrib(buffer.height),
      EGL_LINUX_DRM_FOURCC_EXT,
      EGLAttrib(buffer.fourcc),
      EGL_DMA_BUF_PLANE0_FD_EXT,
      EGLAttrib(fd),
      EGL_DMA_BUF_PLANE0_OFFSET_EXT,
      0,
      EGL_DMA_BUF_PLANE0_PITCH_EXT,
      EGLAttrib(buffer.stride),
  };
  if (buffer.modifier != ModifierInvalid) {
    attributes << EGL_DMA_BUF_PLANE0_MODIFIER_LO_EXT
               << EGLAttrib(buffer.modifier & 0xffffffff)
               << EGL_DMA_BUF_PLANE0_MODIFIER_HI_EXT
               << EGLAttrib(buffer.modifier >> 32);
  }
  QList<EGLint> intAttributes;
  for (EGLAttrib a : std::as_const(attributes))
    intAttributes << EGLint(a);
  intAttributes << EGL_NONE;
  const EGLImageKHR image =
      f.createImage(display, EGL_NO_CONTEXT, EGL_LINUX_DMA_BUF_EXT, nullptr,
                    intAttributes.constData());
  // EGL holds its own reference to the buffer.
  close(fd);
  if (image == EGL_NO_IMAGE_KHR)
    return false;

  QOpenGLFunctions *gl = context->functions();
  GLuint texture = 0;
  gl->glGenTextures(1, &texture);
  gl->glBindTexture(GL_TEXTURE_2D, texture);
  gl->glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
  gl->glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
  gl->glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
  gl->glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
  f.imageTargetTexture(GL_TEXTURE_2D, image);
  gl->glBindTexture(GL_TEXTURE_2D, 0);
  // Raw GL calls above: tell the scene graph its cached state is stale.
  QQuickOpenGLUtils::resetOpenGLState();

  m_gpu.eglImage = image;
  m_gpu.texture = texture;
  m_gpu.sgTexture = QNativeInterface::QSGOpenGLTexture::fromNative(
      texture, window(), QSize(int(buffer.width), int(buffer.height)),
      QQuickWindow::TextureIsOpaque);
  return true;
}

void DisplayView::destroyGpuFrame(GpuFrame frame) {
  delete frame.sgTexture;
  if (QOpenGLContext *context = QOpenGLContext::currentContext()) {
    if (frame.texture)
      context->functions()->glDeleteTextures(1, &frame.texture);
  }
  if (frame.eglImage && egl().destroyImage)
    egl().destroyImage(eglGetCurrentDisplay(), frame.eglImage);
}

QSGNode *DisplayView::updatePaintNode(QSGNode *oldNode,
                                      UpdatePaintNodeData *) {
  // Runs on the render thread while the GUI thread is blocked, so the
  // client's state is safe to read here.
  if (m_pendingDmabuf.fd >= 0) {
    const GpuFrame previous = m_gpu;
    m_gpu = {};
    if (!importDmabuf()) {
      QMetaObject::invokeMethod(
          this,
          [this] {
            emit connectionFailed(
                QStringLiteral("This display needs a GPU with EGL dma-buf "
                               "import (Wayland session with Mesa)."));
          },
          Qt::QueuedConnection);
    }
    if (oldNode)
      static_cast<QSGSimpleTextureNode *>(oldNode)->setTexture(nullptr);
    destroyGpuFrame(previous);
  }

  QSGTexture *texture = nullptr;
  if (m_dmabufMode) {
    texture = m_gpu.sgTexture;
  } else {
    if (m_imageDirty) {
      m_imageDirty = false;
      if (oldNode)
        static_cast<QSGSimpleTextureNode *>(oldNode)->setTexture(nullptr);
      delete m_imageTexture;
      m_imageTexture = nullptr;
      // A shallow copy: it keeps a shared mapping alive until uploaded.
      const QImage frame = m_client.frame();
      if (!frame.isNull())
        m_imageTexture = window()->createTextureFromImage(
            frame, QQuickWindow::TextureIsOpaque);
    }
    texture = m_imageTexture;
  }
  if (!texture) {
    delete oldNode;
    return nullptr;
  }

  auto *node = static_cast<QSGSimpleTextureNode *>(oldNode);
  if (!node) {
    node = new QSGSimpleTextureNode;
    node->setFiltering(QSGTexture::Linear);
  }
  node->setTexture(texture);
  node->setRect(displayRect());
  // QEMU's y0_top is relative to GL's bottom-up window framebuffer (its
  // own GTK UI flips when it is false); the scene graph samples top-down,
  // so the flip is needed in the opposite case.
  node->setTextureCoordinatesTransform(
      m_dmabufMode && m_dmabuf.y0Top ? QSGSimpleTextureNode::MirrorVertically
                                     : QSGSimpleTextureNode::NoTransform);
  // The dma-buf's contents change in place: redraw even when the texture
  // object is the same.
  node->markDirty(QSGNode::DirtyMaterial);
  return node;
}

void DisplayView::releaseResources() {
  const GpuFrame gpu = m_gpu;
  QSGTexture *image = m_imageTexture;
  m_gpu = {};
  m_imageTexture = nullptr;
  if (window())
    window()->scheduleRenderJob(new GpuFrameCleanup([gpu, image] {
                                  destroyGpuFrame(gpu);
                                  delete image;
                                }),
                                QQuickWindow::BeforeSynchronizingStage);
}

void DisplayView::geometryChange(const QRectF &newGeometry,
                                 const QRectF &oldGeometry) {
  QQuickItem::geometryChange(newGeometry, oldGeometry);
  if (newGeometry.size() != oldGeometry.size()) {
    m_resizeTimer.start();
    update();
  }
}

void DisplayView::sendResize() {
  const int w = qRound(width());
  const int h = qRound(height());
  if (w < 1 || h < 1 || frameSize() == QSize(w, h))
    return;
  // Physical size lets the guest pick a sensible DPI for this monitor.
  int widthMM = 0, heightMM = 0;
  if (window() && window()->screen()) {
    const QScreen *screen = window()->screen();
    const qreal dpi = screen->physicalDotsPerInch();
    if (dpi > 0) {
      const qreal pixels = screen->devicePixelRatio() * 25.4 / dpi;
      widthMM = qRound(w * pixels);
      heightMM = qRound(h * pixels);
    }
  }
  m_client.resizeDisplay(w, h, widthMM, heightMM);
}

void DisplayView::sendPointer(const QPointF &position) {
  const QSize frame = frameSize();
  const QRectF rect = displayRect();
  if (frame.isEmpty() || rect.isEmpty())
    return;
  const int x = qBound(0,
                       int((position.x() - rect.x()) * frame.width() /
                           rect.width()),
                       frame.width() - 1);
  const int y = qBound(0,
                       int((position.y() - rect.y()) * frame.height() /
                           rect.height()),
                       frame.height() - 1);
  if (m_client.mouseAbsolute()) {
    m_client.movePointer(x, y);
  } else if (m_havePointer) {
    m_client.movePointerBy(x - int(m_lastPointer.x()),
                           y - int(m_lastPointer.y()));
  }
  m_lastPointer = QPointF(x, y);
  m_havePointer = true;
}

void DisplayView::mousePressEvent(QMouseEvent *event) {
  forceActiveFocus();
  sendPointer(event->position());
  const int button = qemuButton(event->button());
  if (button >= 0)
    m_client.pressButton(button);
}

void DisplayView::mouseReleaseEvent(QMouseEvent *event) {
  sendPointer(event->position());
  const int button = qemuButton(event->button());
  if (button >= 0)
    m_client.releaseButton(button);
}

void DisplayView::mouseMoveEvent(QMouseEvent *event) {
  sendPointer(event->position());
}

void DisplayView::hoverMoveEvent(QHoverEvent *event) {
  sendPointer(event->position());
}

void DisplayView::wheelEvent(QWheelEvent *event) {
  sendPointer(event->position());
  // Touchpads report small high-resolution steps; the guest wants notches.
  m_wheelDelta += event->angleDelta().y();
  while (qAbs(m_wheelDelta) >= 120) {
    const int button = m_wheelDelta > 0 ? 3 : 4;
    m_client.pressButton(button);
    m_client.releaseButton(button);
    m_wheelDelta += m_wheelDelta > 0 ? -120 : 120;
  }
}

void DisplayView::keyPressEvent(QKeyEvent *event) {
  const quint32 qnum = qnumFor(event);
  if (qnum == 0) {
    event->ignore();
    return;
  }
  // Auto-repeated presses are forwarded (the emulated keyboard has no
  // typematic repeat of its own); their paired releases are not.
  m_pressedKeys.insert(qnum);
  m_client.pressKey(qnum);
}

void DisplayView::keyReleaseEvent(QKeyEvent *event) {
  const quint32 qnum = qnumFor(event);
  if (qnum == 0) {
    event->ignore();
    return;
  }
  if (event->isAutoRepeat())
    return;
  m_pressedKeys.remove(qnum);
  m_client.releaseKey(qnum);
}

void DisplayView::focusOutEvent(QFocusEvent *event) {
  // A key held while switching away (Super+number) never gets its release
  // here; left pressed, the guest would see it stuck.
  releaseKeys();
  QQuickItem::focusOutEvent(event);
}

void DisplayView::releaseKeys() {
  for (quint32 qnum : std::as_const(m_pressedKeys))
    m_client.releaseKey(qnum);
  m_pressedKeys.clear();
}
