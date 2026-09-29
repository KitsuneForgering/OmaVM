#include "../displayclient.h"

#include <QDir>
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QProcess>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QTemporaryDir>
#include <QTest>

#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

namespace {
// Reads QMP replies until one that answers a command (skipping events).
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

// What qemu.Backend.attachDisplay does in Go: hand QEMU one end of a
// socket pair over QMP and return the other.
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

bool renderNodeAvailable() {
  for (const QString &node :
       QDir(QStringLiteral("/dev/dri"))
           .entryList({QStringLiteral("renderD*")}, QDir::System)) {
    QFile file(QStringLiteral("/dev/dri/") + node);
    if (file.open(QIODevice::ReadWrite))
      return true;
  }
  return false;
}
} // namespace

// Exercises DisplayClient against a real QEMU (no guest disk: the
// firmware's text screen is enough to produce frames).
class DisplayClientTest : public QObject {
  Q_OBJECT

private slots:
  void sharedMemoryFrames();
  void dmabufFramesArePacedByFrameRendered();

private:
  bool startQemu(const QStringList &display);
  QTemporaryDir m_dir{QStringLiteral("/tmp/omavm-display-XXXXXX")};
  QProcess m_qemu;
  QString m_qmp;

  void cleanup() {
    m_qemu.kill();
    m_qemu.waitForFinished(3000);
  }
};

bool DisplayClientTest::startQemu(const QStringList &display) {
  if (QStandardPaths::findExecutable(QStringLiteral("qemu-system-x86_64"))
          .isEmpty() ||
      !QFile::exists(QStringLiteral("/dev/kvm")))
    return false;
  m_qmp = m_dir.filePath(QStringLiteral("qmp.sock"));
  QFile::remove(m_qmp);
  m_qemu.start(QStringLiteral("qemu-system-x86_64"),
               QStringList{QStringLiteral("-m"), QStringLiteral("128"),
                           QStringLiteral("-enable-kvm"),
                           QStringLiteral("-monitor"), QStringLiteral("none"),
                           QStringLiteral("-serial"), QStringLiteral("none"),
                           QStringLiteral("-qmp"),
                           QStringLiteral("unix:%1,server,nowait").arg(m_qmp)} +
                   display);
  if (!m_qemu.waitForStarted(3000))
    return false;
  for (int i = 0; i < 50 && !QFile::exists(m_qmp); ++i)
    QTest::qWait(100);
  return QFile::exists(m_qmp);
}

void DisplayClientTest::sharedMemoryFrames() {
  if (!startQemu({QStringLiteral("-display"), QStringLiteral("dbus,p2p=yes")}))
    QSKIP("needs qemu-system-x86_64 with KVM");
  const int fd = attachDisplay(m_qmp);
  QVERIFY(fd >= 0);

  DisplayClient client;
  QSignalSpy scanouts(&client, &DisplayClient::imageScanout);
  QSignalSpy updates(&client, &DisplayClient::frameUpdated);
  QSignalSpy errors(&client, &DisplayClient::errorOccurred);
  client.connectToFd(fd);

  QTRY_VERIFY_WITH_TIMEOUT(!scanouts.isEmpty(), 10000);
  QVERIFY(!client.frame().isNull());
  QVERIFY(client.frame().width() >= 640);
  QTRY_VERIFY_WITH_TIMEOUT(!updates.isEmpty(), 10000);
  // The firmware screen is not all one color once text is drawn.
  const QImage frame = client.frame();
  bool varied = false;
  for (int y = 0; y < frame.height() && !varied; y += 4)
    for (int x = 0; x < frame.width() && !varied; x += 4)
      varied = frame.pixel(x, y) != frame.pixel(0, 0);
  QVERIFY(varied);

  // Input on a console without an absolute pointer must not fail the
  // connection.
  client.pressKey(0x1e);
  client.releaseKey(0x1e);
  client.movePointerBy(5, 5);
  QTest::qWait(300);
  QVERIFY2(errors.isEmpty(), qPrintable(errors.value(0).value(0).toString()));
  cleanup();
  QTRY_VERIFY_WITH_TIMEOUT(!errors.isEmpty(), 5000);
}

void DisplayClientTest::dmabufFramesArePacedByFrameRendered() {
  if (!renderNodeAvailable())
    QSKIP("needs a GPU render node");
  if (!startQemu({QStringLiteral("-device"), QStringLiteral("virtio-vga-gl"),
                  QStringLiteral("-display"),
                  QStringLiteral("dbus,p2p=yes,gl=on")}))
    QSKIP("needs qemu-system-x86_64 with KVM and virtio-vga-gl");
  const int fd = attachDisplay(m_qmp);
  QVERIFY(fd >= 0);

  DisplayClient client;
  QList<DisplayClient::Dmabuf> buffers;
  connect(&client, &DisplayClient::dmabufScanout, this,
          [&](DisplayClient::Dmabuf buffer) { buffers.append(buffer); });
  QSignalSpy updates(&client, &DisplayClient::frameUpdated);
  client.connectToFd(fd);

  QTRY_VERIFY_WITH_TIMEOUT(!buffers.isEmpty(), 10000);
  const DisplayClient::Dmabuf buffer = buffers.last();
  QVERIFY(buffer.fd >= 0);
  QVERIFY(buffer.width >= 640 && buffer.stride >= buffer.width * 4);
  QVERIFY(client.frame().isNull());

  // QEMU holds the guest's GPU until each UpdateDMABUF is answered; the
  // client answers on frameRendered() (or its timeout), so updates keep
  // flowing either way.
  QTRY_VERIFY_WITH_TIMEOUT(updates.size() >= 1, 10000);
  const qsizetype before = updates.size();
  client.frameRendered();
  QTRY_VERIFY_WITH_TIMEOUT(updates.size() > before, 10000);

  client.resizeDisplay(1024, 768, 270, 203);
  QTest::qWait(200);
  for (const DisplayClient::Dmabuf &b : std::as_const(buffers))
    close(b.fd);
  cleanup();
}

QTEST_GUILESS_MAIN(DisplayClientTest)
#include "displayclient_test.moc"
