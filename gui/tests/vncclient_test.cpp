#include "../vncclient.h"
#include <QLocalServer>
#include <QProcess>
#include <QSignalSpy>
#include <QStandardPaths>
#include <QTemporaryDir>
#include <QtEndian>
#include <QtTest>

static void u16(QByteArray &b, quint16 n) {
  n = qToBigEndian(n);
  b.append(reinterpret_cast<char *>(&n), 2);
}
static void u32(QByteArray &b, quint32 n) {
  n = qToBigEndian(n);
  b.append(reinterpret_cast<char *>(&n), 4);
}
static QByteArray rectangle(int width, int height, qint32 encoding,
                            int status = 0) {
  QByteArray b = QByteArray::fromHex("00000001");
  u16(b, 0);
  u16(b, status);
  u16(b, width);
  u16(b, height);
  u32(b, quint32(encoding));
  return b;
}

static QByteArray clipboard(quint32 flags, const QByteArray &payload = {}) {
  QByteArray b(4, '\0');
  b[0] = char(3);
  u32(b, quint32(-qint32(4 + payload.size())));
  u32(b, flags);
  b.append(payload);
  return b;
}

class VncTest : public QObject {
  Q_OBJECT
private slots:
  void qemuHandshake() {
    const QString qemu = QStandardPaths::findExecutable("qemu-system-x86_64");
    if (qemu.isEmpty())
      QSKIP("QEMU not installed; protocol fixture still runs");
    QTemporaryDir dir;
    const QString socket = dir.filePath("vnc");
    QProcess process;
    process.start(
        qemu, {"-machine", "q35", "-accel", "tcg", "-m", "64", "-nodefaults",
               "-device", "virtio-vga", "-display", "none", "-vnc",
               "unix:" + socket, "-device", "virtio-serial-pci", "-chardev",
               "qemu-vdagent,id=clipboard,clipboard=on,mouse=off", "-device",
               "virtserialport,chardev=clipboard,name=com.redhat.spice.0"});
    QVERIFY(process.waitForStarted());
    QTRY_VERIFY_WITH_TIMEOUT(QFileInfo::exists(socket), 5000);
    VncClient client;
    QSignalSpy frames(&client, &VncClient::frameUpdated);
    QSignalSpy support(&client, &VncClient::resizeSupported);
    QSignalSpy errors(&client, &VncClient::errorOccurred);
    QSignalSpy clipboardReady(&client, &VncClient::clipboardReady);
    client.connectToSocket(socket);
    QTRY_VERIFY_WITH_TIMEOUT(frames.count() > 0, 5000);
    QTRY_COMPARE_WITH_TIMEOUT(support.count(), 1, 5000);
    const int before = frames.count();
    client.resizeDesktop(1000, 700);
    QTRY_VERIFY_WITH_TIMEOUT(frames.count() > before, 5000);
    QCOMPARE(errors.count(), 0);
    QTRY_COMPARE(clipboardReady.count(), 1);
    VncClient second;
    QSignalSpy secondReady(&second, &VncClient::clipboardReady);
    QSignalSpy received(&second, &VncClient::clipboardReceived);
    QSignalSpy returned(&client, &VncClient::clipboardReceived);
    second.connectToSocket(socket);
    QTRY_COMPARE(secondReady.count(), 1);
    client.setClipboardEnabled(true);
    second.setClipboardEnabled(true);
    const QString text = QString::fromUtf8("Olá, 日本語 🦊\nsegunda linha");
    client.sendClipboard(text);
    QTRY_COMPARE(received.count(), 1);
    QCOMPARE(received.takeFirst().at(0).toString(), text);
    second.sendClipboard("reply");
    QTRY_COMPARE(returned.count(), 1);
    QCOMPARE(returned.takeFirst().at(0).toString(), QString("reply"));
    second.setClipboardEnabled(false);
    client.sendClipboard("must not be received");
    QTest::qWait(100);
    QCOMPARE(received.count(), 0);
    // No guest OS here: this checks QEMU negotiation and forwarding only.
    process.terminate();
    QVERIFY(process.waitForFinished(5000));
  }
  void resizeAndBounds() {
    QTemporaryDir dir;
    QLocalServer server;
    QVERIFY(server.listen(dir.filePath("vnc")));
    VncClient client;
    QSignalSpy frames(&client, &VncClient::frameUpdated);
    QSignalSpy support(&client, &VncClient::resizeSupported);
    QSignalSpy errors(&client, &VncClient::errorOccurred);
    client.connectToSocket(server.fullServerName());
    QTRY_VERIFY(server.hasPendingConnections());
    auto peer = server.nextPendingConnection();
    peer->write("RFB 003.008\n");
    peer->flush();
    QTRY_COMPARE(peer->bytesAvailable(), 12);
    peer->readAll();
    peer->write(QByteArray::fromHex("0101"));
    peer->flush();
    QTRY_COMPARE(peer->bytesAvailable(), 1);
    peer->readAll();
    peer->write(QByteArray(4, '\0'));
    peer->flush();
    QTRY_COMPARE(peer->bytesAvailable(), 1);
    peer->readAll();
    QByteArray init;
    u16(init, 640);
    u16(init, 480);
    init.append(16, '\0');
    u32(init, 0);
    peer->write(init);
    peer->flush();
    QTRY_COMPARE(peer->bytesAvailable(), 50);
    auto setup = peer->readAll();
    QCOMPARE(setup.mid(20, 20).toHex(),
             QByteArray("0200000400000000ffffff21fffffeccc0a1e5ce"));
    QSignalSpy received(&client, &VncClient::clipboardReceived);
    QSignalSpy warnings(&client, &VncClient::clipboardError);
    QByteArray limit;
    u32(limit, 0);
    peer->write(clipboard(0x1b000001, limit));
    peer->flush();
    QTRY_COMPARE(peer->bytesAvailable(), 16);
    peer->readAll();
    client.sendClipboard("disabled");
    QTest::qWait(20);
    QCOMPARE(peer->bytesAvailable(), 0);
    client.setClipboardEnabled(true);
    QByteArray plain;
    u32(plain, 5);
    plain.append("test\0", 5);
    const QByteArray incoming = clipboard(0x10000001, qCompress(plain).mid(4));
    peer->write(incoming.left(10));
    peer->flush();
    QTest::qWait(20);
    QCOMPARE(received.count(), 0);
    peer->write(incoming.mid(10));
    peer->flush();
    QTRY_COMPARE(received.count(), 1);
    QCOMPARE(received.takeFirst().at(0).toString(), QString("test"));
    peer->write(clipboard(0x10000001,
                          qCompress(QByteArray(2 * 1024 * 1024, 'x')).mid(4)));
    peer->flush();
    QTRY_COMPARE(warnings.count(), 1);
    QCOMPARE(received.count(), 0);
    client.resizeDesktop(800, 600); // no extension confirmation yet
    QTest::qWait(20);
    QCOMPARE(peer->bytesAvailable(), 0);
    QByteArray ext = rectangle(640, 480, -308);
    ext.append(char(1));
    ext.append(3, '\0');
    ext.append(16, '\0');
    peer->write(ext.left(18));
    peer->flush();
    QTest::qWait(20);
    QCOMPARE(support.count(), 0);
    peer->write(ext.mid(18));
    peer->flush();
    QTRY_COMPARE(support.count(), 1);
    QTRY_COMPARE(peer->bytesAvailable(), 10);
    peer->readAll();
    client.resizeDesktop(800, 600);
    QTRY_COMPARE(peer->bytesAvailable(), 24);
    QCOMPARE(peer->readAll().toHex(),
             QByteArray("fb0003200258010000000000000000000320025800000000"));
    // QEMU's forwarded response is not a completed resize.
    auto forwarded = rectangle(800, 600, -308, 4);
    forwarded.append(char(1));
    forwarded.append(19, '\0');
    peer->write(forwarded);
    peer->flush();
    QTRY_COMPARE(frames.count(), 2);
    QCOMPARE(client.frame().size(), QSize(640, 480));
    peer->write(rectangle(800, 600, -223));
    peer->flush();
    QTRY_COMPARE(client.frame().size(), QSize(800, 600));
    auto raw = rectangle(1, 1, 0);
    raw.append(QByteArray::fromHex("33221100"));
    peer->write(raw);
    peer->flush();
    QTRY_COMPARE(client.frame().pixelColor(0, 0), QColor(0x11, 0x22, 0x33));
    peer->write(rectangle(801, 1, 0));
    peer->flush();
    QTRY_COMPARE(errors.count(), 1);
  }
};
QTEST_GUILESS_MAIN(VncTest)
#include "vncclient_test.moc"
