#include "../displayview.h"

#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QOffscreenSurface>
#include <QOpenGLContext>
#include <QOpenGLFramebufferObject>
#include <QProcess>
#include <QQuickGraphicsDevice>
#include <QQuickRenderControl>
#include <QQuickRenderTarget>
#include <QQuickWindow>
#include <QStandardPaths>
#include <QTemporaryDir>
#include <QTest>

#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

namespace {
QJsonObject qmpReply(int fd) {
  QByteArray line;
  char c;
  while (read(fd, &c, 1) == 1) {
    if (c != '\n') {
      line.append(c);
      continue;
    }
    const QJsonObject reply = QJsonDocument::fromJson(line).object();
    line.clear();
    if (reply.contains(QStringLiteral("return")) ||
        reply.contains(QStringLiteral("error")) ||
        reply.contains(QStringLiteral("QMP")))
      return reply;
  }
  return {};
}

bool qmpSend(int fd, const QByteArray &command, int passFd = -1) {
  QByteArray data = command + '\n';
  iovec io{data.data(), size_t(data.size())};
  msghdr msg{};
  msg.msg_iov = &io;
  msg.msg_iovlen = 1;
  char control[CMSG_SPACE(sizeof(int))] = {};
  if (passFd >= 0) {
    msg.msg_control = control;
    msg.msg_controllen = sizeof(control);
    cmsghdr *header = CMSG_FIRSTHDR(&msg);
    header->cmsg_level = SOL_SOCKET;
    header->cmsg_type = SCM_RIGHTS;
    header->cmsg_len = CMSG_LEN(sizeof(int));
    memcpy(CMSG_DATA(header), &passFd, sizeof(int));
  }
  return sendmsg(fd, &msg, 0) == qsizetype(data.size()) &&
         !qmpReply(fd).contains(QStringLiteral("error"));
}

int attachDisplay(const QString &qmpPath) {
  const int qmp = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
  sockaddr_un address{};
  address.sun_family = AF_UNIX;
  const QByteArray path = QFile::encodeName(qmpPath);
  memcpy(address.sun_path, path.constData(), size_t(path.size()));
  if (::connect(qmp, reinterpret_cast<sockaddr *>(&address),
                sizeof(address)) != 0) {
    close(qmp);
    return -1;
  }
  int pair[2];
  socketpair(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0, pair);
  const bool ok =
      !qmpReply(qmp).isEmpty() &&
      qmpSend(qmp, R"({"execute":"qmp_capabilities"})") &&
      qmpSend(qmp, R"({"execute":"getfd","arguments":{"fdname":"t"}})",
              pair[1]) &&
      qmpSend(qmp, R"({"execute":"add_client","arguments":)"
                   R"({"protocol":"@dbus-display","fdname":"t"}})");
  close(qmp);
  close(pair[1]);
  if (!ok) {
    close(pair[0]);
    return -1;
  }
  return pair[0];
}

// Fraction of rows in [from, to) that contain anything but the most
// common (background) color.
double inkedRows(const QImage &image, int from, int to) {
  const QRgb background = image.pixel(image.width() - 1, image.height() / 2);
  int inked = 0;
  for (int y = from; y < to; ++y) {
    for (int x = 0; x < image.width(); x += 2) {
      if (image.pixel(x, y) != background) {
        ++inked;
        break;
      }
    }
  }
  return to > from ? double(inked) / (to - from) : 0;
}
} // namespace

// Renders DisplayView offscreen (no window is shown) against a real QEMU
// checking both frame paths end to end: the firmware's text must land at
// the top of the view, not mirrored.
class DisplayViewTest : public QObject {
  Q_OBJECT

private slots:
  void rendersFramesUpright_data();
  void rendersFramesUpright();
};

void DisplayViewTest::rendersFramesUpright_data() {
  QTest::addColumn<QStringList>("display");
  QTest::newRow("dma-buf (host GPU)")
      << QStringList{QStringLiteral("-device"), QStringLiteral("virtio-vga-gl"),
                     QStringLiteral("-display"),
                     QStringLiteral("dbus,p2p=yes,gl=on")};
  QTest::newRow("shared memory (no host GPU)")
      << QStringList{QStringLiteral("-display"), QStringLiteral("dbus,p2p=yes")};
}

void DisplayViewTest::rendersFramesUpright() {
  QFETCH(QStringList, display);
  if (QGuiApplication::platformName() != QStringLiteral("wayland"))
    QSKIP("needs a Wayland session (EGL dma-buf import)");
  if (QStandardPaths::findExecutable(QStringLiteral("qemu-system-x86_64"))
          .isEmpty() ||
      !QFile::exists(QStringLiteral("/dev/kvm")))
    QSKIP("needs qemu-system-x86_64 with KVM");

  QTemporaryDir dir(QStringLiteral("/tmp/omavm-view-XXXXXX"));
  const QString qmp = dir.filePath(QStringLiteral("qmp.sock"));
  QProcess qemu;
  qemu.start(QStringLiteral("qemu-system-x86_64"),
             QStringList{QStringLiteral("-m"), QStringLiteral("128"),
              QStringLiteral("-enable-kvm"), QStringLiteral("-monitor"),
              QStringLiteral("none"), QStringLiteral("-serial"),
              QStringLiteral("none"), QStringLiteral("-qmp"),
              QStringLiteral("unix:%1,server,nowait").arg(qmp)} +
                 display);
  QVERIFY(qemu.waitForStarted(3000));
  auto stopQemu = qScopeGuard([&] {
    qemu.kill();
    qemu.waitForFinished(3000);
  });
  QTRY_VERIFY_WITH_TIMEOUT(QFile::exists(qmp), 5000);
  const int fd = attachDisplay(qmp);
  QVERIFY(fd >= 0);

  QOpenGLContext context;
  QVERIFY(context.create());
  QOffscreenSurface surface;
  surface.setFormat(context.format());
  surface.create();
  QVERIFY(context.makeCurrent(&surface));

  const QSize size(720, 400);
  QOpenGLFramebufferObject fbo(size);
  QQuickRenderControl control;
  QQuickWindow window(&control);
  window.setGraphicsDevice(
      QQuickGraphicsDevice::fromOpenGLContext(&context));
  QVERIFY(control.initialize());
  window.setRenderTarget(
      QQuickRenderTarget::fromOpenGLTexture(fbo.texture(), size));
  window.resize(size);
  window.contentItem()->setSize(size);

  DisplayView view(window.contentItem());
  view.setSize(size);
  QString failure;
  connect(&view, &DisplayView::connectionFailed, this,
          [&](const QString &message) { failure = message; });
  view.setConnectionFd(fd);

  QImage frame;
  for (int attempt = 0; attempt < 100 && failure.isEmpty(); ++attempt) {
    QTest::qWait(100);
    control.polishItems();
    control.beginFrame();
    control.sync();
    control.render();
    control.endFrame();
    frame = fbo.toImage();
    // Wait for the firmware to have printed something.
    if (inkedRows(frame, 0, frame.height()) > 0.05)
      break;
  }
  QVERIFY2(failure.isEmpty(), qPrintable(failure));
  QVERIFY2(inkedRows(frame, 0, frame.height()) > 0.05,
           "no guest content was rendered");
  // Firmware text starts at the top of the screen.
  const double top = inkedRows(frame, 0, frame.height() / 4);
  const double bottom = inkedRows(frame, frame.height() * 3 / 4, frame.height());
  QVERIFY2(top > bottom, qPrintable(QStringLiteral("text not at the top: "
                                                   "top %1, bottom %2")
                                        .arg(top)
                                        .arg(bottom)));
}

int main(int argc, char **argv) {
  // Without a Wayland session there is nothing to render with; fall back
  // to the offscreen platform so the test reports a skip, not a crash.
  if (qEnvironmentVariableIsEmpty("WAYLAND_DISPLAY"))
    qputenv("QT_QPA_PLATFORM", "offscreen");
  QGuiApplication app(argc, argv);
  DisplayViewTest test;
  return QTest::qExec(&test, argc, argv);
}
#include "displayview_test.moc"
