// Fuzzes DisplayClient from QEMU's side of the protocol: a fake QEMU
// (GDBus, in its own thread, the authentication server on both
// connections like the real one) registers the viewer's listener and then
// calls it with random scanouts, updates, shared-memory maps and cursors —
// sizes pulled toward the edges (0, -1, INT_MAX, strides shorter than a
// row, memfds smaller than declared). After each frame the test reads its
// pixels, as DisplayView does when uploading a texture. Build with
// sanitizers to catch what a read past a mapping doesn't crash on.
#include "../displayclient.h"

#pragma push_macro("signals")
#undef signals
#include <gio/gio.h>
#include <gio/gunixfdlist.h>
#pragma pop_macro("signals")

#include <QCoreApplication>
#include <QEventLoop>
#include <QTest>
#include <QTimer>

#include <atomic>
#include <climits>
#include <random>
#include <sys/mman.h>
#include <sys/socket.h>
#include <thread>
#include <unistd.h>

namespace {
constexpr const char *ServerXml = R"(
<node>
  <interface name="org.qemu.Display1.VM">
    <property name="ConsoleIDs" type="au" access="read"/>
  </interface>
  <interface name="org.qemu.Display1.Console">
    <method name="RegisterListener"><arg type="h" direction="in"/></method>
  </interface>
  <interface name="org.qemu.Display1.Mouse">
    <property name="IsAbsolute" type="b" access="read"/>
  </interface>
</node>)";

const quint32 Formats[] = {0x20020888, 0x20028888, 0x20030888, 0x20038888,
                           0x10020565, 0, 0xdeadbeef};

// The fake QEMU. Runs entirely in its own thread and main context.
class FakeQemu {
public:
  explicit FakeQemu(int fd, unsigned seed, int calls)
      : m_fd(fd), m_random(seed), m_calls(calls) {}
  void run();
  std::atomic<bool> done{false};
  std::atomic<int> sent{0};
  QString failure;

private:
  static void onMethod(GDBusConnection *, const gchar *, const gchar *,
                       const gchar *, const gchar *method, GVariant *,
                       GDBusMethodInvocation *invocation, gpointer self);
  static GVariant *onProperty(GDBusConnection *, const gchar *, const gchar *,
                              const gchar *, const gchar *property, GError **,
                              gpointer);
  void fuzz(GDBusConnection *listener);
  quint32 edgeU();
  gint32 edgeI();
  GVariant *bytes(gsize size);

  int m_fd;
  std::mt19937 m_random;
  int m_calls;
  GMainContext *m_context = nullptr;
  GDBusConnection *m_listener = nullptr;
};

quint32 FakeQemu::edgeU() {
  static const quint32 edges[] = {0, 1, 2, 3, 7, 64, 640, 1920, 4096,
                                  65535, 0x7fffffff, 0x80000000, UINT_MAX};
  if (m_random() % 2)
    return edges[m_random() % std::size(edges)];
  return m_random() % 300;
}

gint32 FakeQemu::edgeI() {
  static const gint32 edges[] = {0, 1, -1, 2, 64, 639, 640, INT_MAX, INT_MIN,
                                 -100000};
  if (m_random() % 2)
    return edges[m_random() % std::size(edges)];
  return gint32(m_random() % 300) - 20;
}

GVariant *FakeQemu::bytes(gsize size) {
  // Sizes are what is being fuzzed; pixel values don't matter.
  const QByteArray data(qsizetype(size), char(m_random() & 0xff));
  return g_variant_new_fixed_array(G_VARIANT_TYPE_BYTE, data.constData(),
                                   size, 1);
}

void FakeQemu::onMethod(GDBusConnection *, const gchar *, const gchar *,
                        const gchar *, const gchar *method, GVariant *,
                        GDBusMethodInvocation *invocation, gpointer self) {
  auto *qemu = static_cast<FakeQemu *>(self);
  if (g_strcmp0(method, "RegisterListener") != 0) {
    g_dbus_method_invocation_return_value(invocation, nullptr);
    return;
  }
  GUnixFDList *list = g_dbus_message_get_unix_fd_list(
      g_dbus_method_invocation_get_message(invocation));
  const int fd = list ? g_unix_fd_list_get(list, 0, nullptr) : -1;
  g_dbus_method_invocation_return_value(invocation, nullptr);
  GSocket *socket = g_socket_new_from_fd(fd, nullptr);
  GSocketConnection *stream =
      g_socket_connection_factory_create_connection(socket);
  g_object_unref(socket);
  gchar *guid = g_dbus_generate_guid();
  GError *error = nullptr;
  qemu->m_listener = g_dbus_connection_new_sync(
      G_IO_STREAM(stream), guid,
      G_DBUS_CONNECTION_FLAGS_AUTHENTICATION_SERVER, nullptr, nullptr, &error);
  g_free(guid);
  g_object_unref(stream);
  if (!qemu->m_listener) {
    qemu->failure = QString::fromUtf8(error->message);
    g_error_free(error);
  }
}

GVariant *FakeQemu::onProperty(GDBusConnection *, const gchar *,
                               const gchar *, const gchar *,
                               const gchar *property, GError **, gpointer) {
  if (g_strcmp0(property, "ConsoleIDs") == 0) {
    const guint32 ids[] = {0};
    return g_variant_new_fixed_array(G_VARIANT_TYPE_UINT32, ids, 1, 4);
  }
  return g_variant_new_boolean(TRUE);
}

void FakeQemu::run() {
  m_context = g_main_context_new();
  g_main_context_push_thread_default(m_context);
  GDBusNodeInfo *node = g_dbus_node_info_new_for_xml(ServerXml, nullptr);
  const GDBusInterfaceVTable vtable = {onMethod, onProperty, nullptr, {}};
  GSocket *socket = g_socket_new_from_fd(m_fd, nullptr);
  GSocketConnection *stream =
      g_socket_connection_factory_create_connection(socket);
  g_object_unref(socket);
  gchar *guid = g_dbus_generate_guid();
  GError *error = nullptr;
  GDBusConnection *main = g_dbus_connection_new_sync(
      G_IO_STREAM(stream), guid,
      GDBusConnectionFlags(G_DBUS_CONNECTION_FLAGS_AUTHENTICATION_SERVER |
                           G_DBUS_CONNECTION_FLAGS_DELAY_MESSAGE_PROCESSING),
      nullptr, nullptr, &error);
  g_free(guid);
  g_object_unref(stream);
  if (!main) {
    failure = QString::fromUtf8(error->message);
    g_error_free(error);
  } else {
    g_dbus_connection_register_object(main, "/org/qemu/Display1/VM",
                                      node->interfaces[0], &vtable, this,
                                      nullptr, nullptr);
    for (int i : {1, 2})
      g_dbus_connection_register_object(main, "/org/qemu/Display1/Console_0",
                                        node->interfaces[i], &vtable, this,
                                        nullptr, nullptr);
    g_dbus_connection_start_message_processing(main);
    // Wait for the viewer to register its listener.
    for (int i = 0; i < 500 && !m_listener && failure.isEmpty(); ++i) {
      while (g_main_context_iteration(m_context, FALSE)) {
      }
      g_usleep(10000);
    }
    if (m_listener)
      fuzz(m_listener);
    else if (failure.isEmpty())
      failure = QStringLiteral("the viewer never registered a listener");
    if (m_listener) {
      g_dbus_connection_close_sync(m_listener, nullptr, nullptr);
      g_object_unref(m_listener);
    }
    g_dbus_connection_close_sync(main, nullptr, nullptr);
    g_object_unref(main);
  }
  g_dbus_node_info_unref(node);
  g_main_context_pop_thread_default(m_context);
  g_main_context_unref(m_context);
  done = true;
}

void FakeQemu::fuzz(GDBusConnection *listener) {
  const char *path = "/org/qemu/Display1/Listener";
  const char *iface = "org.qemu.Display1.Listener";
  for (int i = 0; i < m_calls; ++i) {
    GVariant *args = nullptr;
    const char *method = nullptr;
    const char *interface = iface;
    GUnixFDList *fds = nullptr;
    switch (m_random() % 8) {
    case 0: {
      method = "Scanout";
      const quint32 w = edgeU() % 2048, h = edgeU() % 2048;
      const quint32 stride = m_random() % 3 ? w * 4 : edgeU();
      const gsize size = m_random() % 3 ? gsize(stride) * h : m_random() % 4096;
      args = g_variant_new("(uuuu@ay)", w, h, stride,
                           Formats[m_random() % std::size(Formats)],
                           bytes(qMin<gsize>(size, 16 << 20)));
      break;
    }
    case 1: {
      method = "Update";
      const gint32 w = edgeI(), h = edgeI();
      const quint32 stride = edgeU();
      const gsize size = m_random() % 8192;
      args = g_variant_new("(iiiiuu@ay)", edgeI(), edgeI(), w, h, stride,
                           Formats[m_random() % std::size(Formats)],
                           bytes(size));
      break;
    }
    case 2: {
      method = "ScanoutMap";
      interface = "org.qemu.Display1.Listener.Unix.Map";
      // A memfd whose real size may be smaller than what is declared.
      const int memfd = memfd_create("fuzz-surface", MFD_CLOEXEC);
      const off_t real = off_t(m_random() % 3 ? 1 << 20 : m_random() % 8192);
      if (ftruncate(memfd, real) != 0) {
        close(memfd);
        continue;
      }
      fds = g_unix_fd_list_new();
      const gint handle = g_unix_fd_list_append(fds, memfd, nullptr);
      close(memfd);
      args = g_variant_new("(huuuuu)", handle, edgeU() % 8192, edgeU(),
                           edgeU(), edgeU(),
                           Formats[m_random() % std::size(Formats)]);
      break;
    }
    case 3:
      method = "UpdateMap";
      interface = "org.qemu.Display1.Listener.Unix.Map";
      args = g_variant_new("(iiii)", edgeI(), edgeI(), edgeI(), edgeI());
      break;
    case 4: {
      method = "CursorDefine";
      const gint32 w = edgeI(), h = edgeI();
      args = g_variant_new("(iiii@ay)", w, h, edgeI(), edgeI(),
                           bytes(m_random() % 2 && w > 0 && h > 0 && w < 600 && h < 600
                                     ? gsize(w) * gsize(h) * 4
                                     : m_random() % 4096));
      break;
    }
    case 5:
      method = "MouseSet";
      args = g_variant_new("(iii)", edgeI(), edgeI(), edgeI());
      break;
    case 6:
      method = "Disable";
      break;
    case 7: {
      method = "ScanoutDMABUF";
      const int memfd = memfd_create("fuzz-dmabuf", MFD_CLOEXEC);
      fds = g_unix_fd_list_new();
      const gint handle = g_unix_fd_list_append(fds, memfd, nullptr);
      close(memfd);
      // Sometimes a handle that isn't in the fd list.
      args = g_variant_new("(huuuutb)", m_random() % 4 ? handle : 7, edgeU(),
                           edgeU(), edgeU(), edgeU(), quint64(m_random()),
                           gboolean(m_random() % 2));
      break;
    }
    }
    GError *error = nullptr;
    GVariant *reply = g_dbus_connection_call_with_unix_fd_list_sync(
        listener, nullptr, path, interface, method, args, nullptr,
        G_DBUS_CALL_FLAGS_NONE, 5000, fds, nullptr, nullptr, &error);
    if (fds)
      g_object_unref(fds);
    if (reply)
      g_variant_unref(reply);
    if (error) {
      // The viewer may refuse; it must never go away.
      if (g_dbus_connection_is_closed(listener)) {
        failure = QStringLiteral("the viewer dropped the connection at %1: %2")
                      .arg(QString::fromUtf8(method),
                           QString::fromUtf8(error->message));
        g_error_free(error);
        return;
      }
      g_error_free(error);
    }
    ++sent;
  }
  // Last, a valid frame: the client must still work.
  const quint32 w = 4, h = 2;
  QByteArray red(int(w * h * 4), Qt::Uninitialized);
  for (int i = 0; i < red.size(); i += 4) {
    red[i] = 0;          // B
    red[i + 1] = 0;      // G
    red[i + 2] = char(0xff); // R
    red[i + 3] = char(0xff);
  }
  GVariant *reply = g_dbus_connection_call_sync(
      listener, nullptr, path, iface, "Scanout",
      g_variant_new("(uuuu@ay)", w, h, w * 4, 0x20020888u,
                    g_variant_new_fixed_array(G_VARIANT_TYPE_BYTE,
                                              red.constData(), red.size(), 1)),
      nullptr, G_DBUS_CALL_FLAGS_NONE, 5000, nullptr, nullptr);
  if (reply)
    g_variant_unref(reply);
}
} // namespace

class DisplayClientFuzzTest : public QObject {
  Q_OBJECT

private slots:
  void randomCallsFromQemu_data();
  void randomCallsFromQemu();
};

void DisplayClientFuzzTest::randomCallsFromQemu_data() {
  QTest::addColumn<unsigned>("seed");
  const int seeds = qEnvironmentVariableIntValue("OMAVM_FUZZ_SEEDS");
  for (unsigned seed = 1; seed <= unsigned(seeds > 0 ? seeds : 8); ++seed)
    QTest::newRow(qPrintable(QStringLiteral("seed %1").arg(seed))) << seed;
}

void DisplayClientFuzzTest::randomCallsFromQemu() {
  QFETCH(unsigned, seed);
  int pair[2];
  QVERIFY(socketpair(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0, pair) == 0);
  FakeQemu qemu(pair[1], seed, 300);
  std::thread server([&qemu] { qemu.run(); });

  DisplayClient client;
  QString failed;
  connect(&client, &DisplayClient::errorOccurred, this,
          [&](const QString &message) { failed = message; });
  // What DisplayView does with each frame: read every pixel of it.
  quint64 checksum = 0;
  QImage last;
  const auto readFrame = [&] {
    last = client.frame();
    if (last.isNull())
      return;
    for (int y = 0; y < last.height(); ++y) {
      const uchar *line = last.constScanLine(y);
      for (qsizetype x = 0; x < last.bytesPerLine(); ++x)
        checksum += line[x];
    }
  };
  connect(&client, &DisplayClient::imageScanout, this, readFrame);
  connect(&client, &DisplayClient::frameUpdated, this, readFrame);
  connect(&client, &DisplayClient::dmabufScanout, this,
          [](DisplayClient::Dmabuf buffer) {
            if (buffer.fd >= 0)
              close(buffer.fd);
          });
  client.connectToFd(pair[0]);
  // A real event loop: QTest::qWait sleeps 10 ms between rounds, and each
  // of the fake QEMU's calls waits for the viewer's reply.
  QEventLoop loop;
  QTimer poll;
  connect(&poll, &QTimer::timeout, &loop, [&] {
    if (qemu.done)
      loop.quit();
  });
  poll.start(1);
  QTimer::singleShot(60000, &loop, &QEventLoop::quit);
  loop.exec();
  QVERIFY2(qemu.done, "the fake QEMU didn't finish in 60 s");
  server.join();
  QVERIFY2(qemu.failure.isEmpty(), qPrintable(qemu.failure));
  QVERIFY2(qemu.sent > 0, "no call reached the viewer");
  // The closing valid Scanout came through intact.
  QTRY_COMPARE(client.frame().size(), QSize(4, 2));
  QCOMPARE(client.frame().pixel(3, 1), qRgb(0xff, 0, 0));
  Q_UNUSED(checksum);
  Q_UNUSED(failed);
}

int main(int argc, char **argv) {
  QCoreApplication app(argc, argv);
  DisplayClientFuzzTest test;
  return QTest::qExec(&test, argc, argv);
}
#include "displayclient_fuzz_test.moc"
