#include "displayclient.h"

// GLib names a struct member "signals", which Qt defines as a macro.
#pragma push_macro("signals")
#undef signals
#include <gio/gio.h>
#include <gio/gunixfdlist.h>
#pragma pop_macro("signals")

#include <QAbstractEventDispatcher>
#include <QCoreApplication>

#include <memory>
#include <sys/mman.h>
#include <sys/socket.h>
#include <unistd.h>

namespace {
constexpr const char *ListenerPath = "/org/qemu/Display1/Listener";
constexpr const char *ClipboardPath = "/org/qemu/Display1/Clipboard";
constexpr const char *TextMime = "text/plain;charset=utf-8";
// A paste larger than this is almost certainly not something the user
// meant to share (the limit documented in README.md).
constexpr gsize MaxClipboard = 1024 * 1024;

// The subset of ui/dbus-display1.xml this client implements.
constexpr const char *InterfaceXml = R"(
<node>
  <interface name="org.qemu.Display1.Listener">
    <method name="Scanout">
      <arg type="u" name="width" direction="in"/>
      <arg type="u" name="height" direction="in"/>
      <arg type="u" name="stride" direction="in"/>
      <arg type="u" name="pixman_format" direction="in"/>
      <arg type="ay" name="data" direction="in"/>
    </method>
    <method name="Update">
      <arg type="i" name="x" direction="in"/>
      <arg type="i" name="y" direction="in"/>
      <arg type="i" name="width" direction="in"/>
      <arg type="i" name="height" direction="in"/>
      <arg type="u" name="stride" direction="in"/>
      <arg type="u" name="pixman_format" direction="in"/>
      <arg type="ay" name="data" direction="in"/>
    </method>
    <method name="ScanoutDMABUF">
      <arg type="h" name="dmabuf" direction="in"/>
      <arg type="u" name="width" direction="in"/>
      <arg type="u" name="height" direction="in"/>
      <arg type="u" name="stride" direction="in"/>
      <arg type="u" name="fourcc" direction="in"/>
      <arg type="t" name="modifier" direction="in"/>
      <arg type="b" name="y0_top" direction="in"/>
    </method>
    <method name="UpdateDMABUF">
      <arg type="i" name="x" direction="in"/>
      <arg type="i" name="y" direction="in"/>
      <arg type="i" name="width" direction="in"/>
      <arg type="i" name="height" direction="in"/>
    </method>
    <method name="Disable"/>
    <method name="MouseSet">
      <arg type="i" name="x" direction="in"/>
      <arg type="i" name="y" direction="in"/>
      <arg type="i" name="on" direction="in"/>
    </method>
    <method name="CursorDefine">
      <arg type="i" name="width" direction="in"/>
      <arg type="i" name="height" direction="in"/>
      <arg type="i" name="hot_x" direction="in"/>
      <arg type="i" name="hot_y" direction="in"/>
      <arg type="ay" name="data" direction="in"/>
    </method>
    <property name="Interfaces" type="as" access="read"/>
  </interface>
  <interface name="org.qemu.Display1.Listener.Unix.Map">
    <method name="ScanoutMap">
      <arg type="h" name="handle" direction="in"/>
      <arg type="u" name="offset" direction="in"/>
      <arg type="u" name="width" direction="in"/>
      <arg type="u" name="height" direction="in"/>
      <arg type="u" name="stride" direction="in"/>
      <arg type="u" name="pixman_format" direction="in"/>
    </method>
    <method name="UpdateMap">
      <arg type="i" name="x" direction="in"/>
      <arg type="i" name="y" direction="in"/>
      <arg type="i" name="width" direction="in"/>
      <arg type="i" name="height" direction="in"/>
    </method>
  </interface>
  <interface name="org.qemu.Display1.Clipboard">
    <method name="Register"/>
    <method name="Unregister"/>
    <method name="Grab">
      <arg type="u" name="selection"/>
      <arg type="u" name="serial"/>
      <arg type="as" name="mimes"/>
    </method>
    <method name="Release">
      <arg type="u" name="selection"/>
    </method>
    <method name="Request">
      <arg type="u" name="selection"/>
      <arg type="as" name="mimes"/>
      <arg type="s" name="reply_mime" direction="out"/>
      <arg type="ay" name="data" direction="out"/>
    </method>
    <property name="Interfaces" type="as" access="read"/>
  </interface>
</node>)";

GDBusNodeInfo *interfaces() {
  static GDBusNodeInfo *info =
      g_dbus_node_info_new_for_xml(InterfaceXml, nullptr);
  return info;
}

GDBusInterfaceInfo *interfaceInfo(const char *name) {
  return g_dbus_node_info_lookup_interface(interfaces(), name);
}

// pixman format codes QEMU uses for 2D surfaces (little-endian host).
QImage::Format imageFormat(quint32 pixman) {
  switch (pixman) {
  case 0x20020888: // PIXMAN_x8r8g8b8
    return QImage::Format_RGB32;
  case 0x20028888: // PIXMAN_a8r8g8b8
    return QImage::Format_ARGB32;
  case 0x20030888: // PIXMAN_x8b8g8r8
    return QImage::Format_RGBX8888;
  case 0x20038888: // PIXMAN_a8b8g8r8
    return QImage::Format_RGBA8888;
  case 0x10020565: // PIXMAN_r5g6b5
    return QImage::Format_RGB16;
  default:
    return QImage::Format_Invalid;
  }
}

// Returns the fd at index handle of the call's fd list, owned by the caller.
int takeFd(GDBusMethodInvocation *invocation, gint32 handle) {
  GUnixFDList *list =
      g_dbus_message_get_unix_fd_list(g_dbus_method_invocation_get_message(
          invocation));
  if (!list)
    return -1;
  return g_unix_fd_list_get(list, handle, nullptr);
}

// A read-only memory mapping of a guest surface, kept alive by every
// QImage that points into it.
struct Mapping {
  void *address = MAP_FAILED;
  size_t size = 0;
  ~Mapping() {
    if (address != MAP_FAILED)
      munmap(address, size);
  }
};

void releaseMapping(void *mapping) {
  delete static_cast<std::shared_ptr<Mapping> *>(mapping);
}

GDBusConnection *connectionOnFd(int fd, GError **error) {
  GSocket *socket = g_socket_new_from_fd(fd, error);
  if (!socket) {
    close(fd);
    return nullptr;
  }
  GSocketConnection *stream = g_socket_connection_factory_create_connection(socket);
  g_object_unref(socket);
  // Synchronous on purpose: the handshake is a few bytes over a local
  // socket QEMU is already waiting on, and objects must be exported
  // before the first message is processed.
  GDBusConnection *connection = g_dbus_connection_new_sync(
      G_IO_STREAM(stream), nullptr,
      GDBusConnectionFlags(G_DBUS_CONNECTION_FLAGS_AUTHENTICATION_CLIENT |
                           G_DBUS_CONNECTION_FLAGS_DELAY_MESSAGE_PROCESSING),
      nullptr, nullptr, error);
  g_object_unref(stream);
  return connection;
}

void onListenerCall(GDBusConnection *, const gchar *, const gchar *,
                    const gchar *, const gchar *method, GVariant *parameters,
                    GDBusMethodInvocation *invocation, gpointer self) {
  static_cast<DisplayClient *>(self)->handleListenerCall(method, parameters,
                                                         invocation);
}

void onClipboardCall(GDBusConnection *, const gchar *, const gchar *,
                     const gchar *, const gchar *method, GVariant *parameters,
                     GDBusMethodInvocation *invocation, gpointer self) {
  static_cast<DisplayClient *>(self)->handleClipboardCall(method, parameters,
                                                          invocation);
}

GVariant *onGetInterfaces(GDBusConnection *, const gchar *, const gchar *,
                          const gchar *interface, const gchar *property,
                          GError **, gpointer) {
  if (g_strcmp0(property, "Interfaces") != 0)
    return nullptr;
  if (g_strcmp0(interface, "org.qemu.Display1.Listener") == 0) {
    const gchar *extra[] = {"org.qemu.Display1.Listener.Unix.Map", nullptr};
    return g_variant_new_strv(extra, -1);
  }
  return g_variant_new_strv(nullptr, 0);
}

const GDBusInterfaceVTable ListenerVTable = {onListenerCall, onGetInterfaces,
                                             nullptr, {}};
const GDBusInterfaceVTable ClipboardVTable = {onClipboardCall, onGetInterfaces,
                                              nullptr, {}};

void onClosed(GDBusConnection *, gboolean, GError *, gpointer self) {
  static_cast<DisplayClient *>(self)->fail(
      QStringLiteral("The Machine's display was closed."));
}

// Replies are ignored for input events: QEMU answers each one, and a
// failure (e.g. a key while the guest is paused) has nothing to show.
void ignoreReply(GObject *source, GAsyncResult *result, gpointer) {
  GVariant *reply =
      g_dbus_connection_call_finish(G_DBUS_CONNECTION(source), result, nullptr);
  if (reply)
    g_variant_unref(reply);
}
} // namespace

DisplayClient::DisplayClient(QObject *parent) : QObject(parent) {
  qRegisterMetaType<DisplayClient::Dmabuf>();
  m_cancellable = g_cancellable_new();
  m_updateTimeout.setSingleShot(true);
  m_updateTimeout.setInterval(50);
  connect(&m_updateTimeout, &QTimer::timeout, this,
          &DisplayClient::completePendingUpdates);
  // GDBus dispatches into the GLib main context. Qt drives it on Linux
  // unless built or run without GLib (QT_NO_GLIB); pump it ourselves then.
  auto *dispatcher = QAbstractEventDispatcher::instance();
  if (!dispatcher || !dispatcher->inherits("QEventDispatcherGlib")) {
    auto *pump = new QTimer(this);
    connect(pump, &QTimer::timeout, this,
            [] { while (g_main_context_iteration(nullptr, FALSE)) {} });
    pump->start(4);
  }
}

DisplayClient::~DisplayClient() {
  g_cancellable_cancel(m_cancellable);
  completePendingUpdates();
  if (m_propertiesSubscription)
    g_dbus_connection_signal_unsubscribe(m_connection, m_propertiesSubscription);
  for (GDBusConnection *connection : {m_connection, m_listener}) {
    if (!connection)
      continue;
    g_signal_handlers_disconnect_by_data(connection, this);
    g_dbus_connection_close_sync(connection, nullptr, nullptr);
  }
  if (m_listener) {
    for (unsigned id : m_listenerObjects)
      if (id)
        g_dbus_connection_unregister_object(m_listener, id);
    g_object_unref(m_listener);
  }
  if (m_connection) {
    if (m_clipboardObject)
      g_dbus_connection_unregister_object(m_connection, m_clipboardObject);
    g_object_unref(m_connection);
  }
  g_object_unref(m_cancellable);
}

void DisplayClient::connectToFd(int fd) {
  GError *error = nullptr;
  GDBusConnection *connection = connectionOnFd(fd, &error);
  if (!connection) {
    fail(QStringLiteral("Could not connect to the Machine's display: ") +
         QString::fromUtf8(error ? error->message : "unknown error"));
    g_clear_error(&error);
    return;
  }
  handleConnection(connection, false);
}

void DisplayClient::handleConnection(GDBusConnection *connection,
                                     bool listener) {
  g_signal_connect(connection, "closed", G_CALLBACK(onClosed), this);
  if (listener) {
    m_listener = connection;
    m_listenerObjects[0] = g_dbus_connection_register_object(
        connection, ListenerPath, interfaceInfo("org.qemu.Display1.Listener"),
        &ListenerVTable, this, nullptr, nullptr);
    m_listenerObjects[1] = g_dbus_connection_register_object(
        connection, ListenerPath,
        interfaceInfo("org.qemu.Display1.Listener.Unix.Map"), &ListenerVTable,
        this, nullptr, nullptr);
    g_dbus_connection_start_message_processing(connection);
    return;
  }

  m_connection = connection;
  m_clipboardObject = g_dbus_connection_register_object(
      connection, ClipboardPath, interfaceInfo("org.qemu.Display1.Clipboard"),
      &ClipboardVTable, this, nullptr, nullptr);
  g_dbus_connection_start_message_processing(connection);
  g_dbus_connection_call(
      connection, nullptr, "/org/qemu/Display1/VM",
      "org.freedesktop.DBus.Properties", "Get",
      g_variant_new("(ss)", "org.qemu.Display1.VM", "ConsoleIDs"), nullptr,
      G_DBUS_CALL_FLAGS_NONE, -1, m_cancellable,
      [](GObject *source, GAsyncResult *result, gpointer self) {
        GError *error = nullptr;
        GVariant *reply = g_dbus_connection_call_finish(
            G_DBUS_CONNECTION(source), result, &error);
        if (!reply) {
          if (!g_error_matches(error, G_IO_ERROR, G_IO_ERROR_CANCELLED))
            static_cast<DisplayClient *>(self)->fail(
                QStringLiteral("The Machine's display did not answer: ") +
                QString::fromUtf8(error->message));
          g_error_free(error);
          return;
        }
        GVariant *value = nullptr;
        g_variant_get(reply, "(v)", &value);
        static_cast<DisplayClient *>(self)->handleConsoleIds(value);
        g_variant_unref(value);
        g_variant_unref(reply);
      },
      this);
}

void DisplayClient::handleConsoleIds(GVariant *ids) {
  gsize count = 0;
  const guint32 *values =
      static_cast<const guint32 *>(g_variant_get_fixed_array(ids, &count, 4));
  if (count == 0) {
    fail(QStringLiteral("This Machine has no display."));
    return;
  }
  m_consolePath = "/org/qemu/Display1/Console_" + QByteArray::number(values[0]);
  registerListener();

  m_propertiesSubscription = g_dbus_connection_signal_subscribe(
      m_connection, nullptr, "org.freedesktop.DBus.Properties",
      "PropertiesChanged", m_consolePath.constData(), nullptr,
      G_DBUS_SIGNAL_FLAGS_NONE,
      [](GDBusConnection *, const gchar *, const gchar *, const gchar *,
         const gchar *, GVariant *parameters, gpointer self) {
        const gchar *interface = nullptr;
        GVariant *changed = nullptr;
        g_variant_get(parameters, "(&s@a{sv}@as)", &interface, &changed,
                      nullptr);
        gboolean absolute = FALSE;
        if (g_strcmp0(interface, "org.qemu.Display1.Mouse") == 0 &&
            g_variant_lookup(changed, "IsAbsolute", "b", &absolute))
          static_cast<DisplayClient *>(self)->handleMouseAbsolute(absolute);
        g_variant_unref(changed);
      },
      this, nullptr);
  g_dbus_connection_call(
      m_connection, nullptr, m_consolePath.constData(),
      "org.freedesktop.DBus.Properties", "Get",
      g_variant_new("(ss)", "org.qemu.Display1.Mouse", "IsAbsolute"), nullptr,
      G_DBUS_CALL_FLAGS_NONE, -1, m_cancellable,
      [](GObject *source, GAsyncResult *result, gpointer self) {
        GVariant *reply = g_dbus_connection_call_finish(
            G_DBUS_CONNECTION(source), result, nullptr);
        if (!reply)
          return;
        GVariant *value = nullptr;
        g_variant_get(reply, "(v)", &value);
        static_cast<DisplayClient *>(self)->handleMouseAbsolute(
            g_variant_get_boolean(value));
        g_variant_unref(value);
        g_variant_unref(reply);
      },
      this);
  // Claim the clipboard session. A second viewer of the same Machine is
  // refused (QEMU accepts one clipboard peer) and simply goes without.
  g_dbus_connection_call(m_connection, nullptr, ClipboardPath,
                         "org.qemu.Display1.Clipboard", "Register", nullptr,
                         nullptr, G_DBUS_CALL_FLAGS_NONE, -1, m_cancellable,
                         ignoreReply, nullptr);
}

void DisplayClient::registerListener() {
  int pair[2];
  if (socketpair(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0, pair) != 0) {
    fail(QStringLiteral("Could not create the display listener socket."));
    return;
  }
  GUnixFDList *list = g_unix_fd_list_new();
  const gint index = g_unix_fd_list_append(list, pair[1], nullptr);
  close(pair[1]);
  // QEMU answers RegisterListener only after authenticating on the other
  // end of this socket, so the call must be in flight before our side
  // of the handshake blocks.
  g_dbus_connection_call_with_unix_fd_list(
      m_connection, nullptr, m_consolePath.constData(),
      "org.qemu.Display1.Console", "RegisterListener",
      g_variant_new("(h)", index), nullptr, G_DBUS_CALL_FLAGS_NONE, -1, list,
      m_cancellable,
      [](GObject *source, GAsyncResult *result, gpointer self) {
        GError *error = nullptr;
        GVariant *reply = g_dbus_connection_call_with_unix_fd_list_finish(
            G_DBUS_CONNECTION(source), nullptr, result, &error);
        if (reply) {
          g_variant_unref(reply);
          return;
        }
        if (!g_error_matches(error, G_IO_ERROR, G_IO_ERROR_CANCELLED))
          static_cast<DisplayClient *>(self)->fail(
              QStringLiteral("The Machine refused the display connection: ") +
              QString::fromUtf8(error->message));
        g_error_free(error);
      },
      this);
  g_object_unref(list);
  g_dbus_connection_flush_sync(m_connection, nullptr, nullptr);

  GError *error = nullptr;
  GDBusConnection *listener = connectionOnFd(pair[0], &error);
  if (!listener) {
    fail(QStringLiteral("Could not open the display listener: ") +
         QString::fromUtf8(error ? error->message : "unknown error"));
    g_clear_error(&error);
    return;
  }
  handleConnection(listener, true);
}

void DisplayClient::handleListenerCall(const char *method,
                                       GVariant *parameters,
                                       GDBusMethodInvocation *invocation) {
  const QByteArray name(method);
  if (name == "UpdateDMABUF") {
    gint32 x, y, w, h;
    g_variant_get(parameters, "(iiii)", &x, &y, &w, &h);
    // QEMU blocks the guest's GPU until this is answered: hold the reply
    // until the frame is drawn so the guest renders at the viewer's pace.
    m_pendingUpdates.append(invocation);
    if (!m_updateTimeout.isActive())
      m_updateTimeout.start();
    emit frameUpdated(QRect(x, y, w, h));
    return;
  }

  if (name == "ScanoutDMABUF") {
    gint32 handle;
    Dmabuf buffer;
    gboolean y0Top;
    g_variant_get(parameters, "(huuuutb)", &handle, &buffer.width,
                  &buffer.height, &buffer.stride, &buffer.fourcc,
                  &buffer.modifier, &y0Top);
    buffer.y0Top = y0Top;
    buffer.fd = takeFd(invocation, handle);
    m_frame = QImage();
    g_dbus_method_invocation_return_value(invocation, nullptr);
    if (buffer.fd >= 0)
      emit dmabufScanout(buffer);
    return;
  }

  if (name == "Scanout" || name == "Update") {
    const bool full = name == "Scanout";
    gint32 x = 0, y = 0, w = 0, h = 0;
    guint32 stride, format;
    GVariant *data = nullptr;
    if (full) {
      guint32 uw, uh;
      g_variant_get(parameters, "(uuuu@ay)", &uw, &uh, &stride, &format,
                    &data);
      w = gint32(uw);
      h = gint32(uh);
    } else {
      g_variant_get(parameters, "(iiiiuu@ay)", &x, &y, &w, &h, &stride,
                    &format, &data);
    }
    gsize size = 0;
    const uchar *bytes =
        static_cast<const uchar *>(g_variant_get_fixed_array(data, &size, 1));
    const QImage::Format qformat = imageFormat(format);
    g_dbus_method_invocation_return_value(invocation, nullptr);
    const int bytesPerPixel = qformat == QImage::Format_RGB16 ? 2 : 4;
    if (qformat == QImage::Format_Invalid || w <= 0 || h <= 0 ||
        stride < guint32(w * bytesPerPixel) ||
        size < gsize(stride) * gsize(h - 1) + gsize(w) * bytesPerPixel) {
      g_variant_unref(data);
      return;
    }
    const QImage update =
        QImage(bytes, w, h, qsizetype(stride), qformat).copy();
    g_variant_unref(data);
    if (full) {
      m_frame = update;
      emit imageScanout();
      return;
    }
    if (m_frame.isNull() || m_frame.format() != qformat || x < 0 || y < 0 ||
        x >= m_frame.width())
      return;
    for (int row = 0; row < h && y + row < m_frame.height(); ++row) {
      const int count = qMin(w, m_frame.width() - x) * bytesPerPixel;
      memcpy(m_frame.scanLine(y + row) + x * bytesPerPixel,
             update.constScanLine(row), size_t(count));
    }
    emit frameUpdated(QRect(x, y, w, h));
    return;
  }

  if (name == "ScanoutMap") {
    gint32 handle;
    guint32 offset, w, h, stride, format;
    g_variant_get(parameters, "(huuuuu)", &handle, &offset, &w, &h, &stride,
                  &format);
    const int fd = takeFd(invocation, handle);
    g_dbus_method_invocation_return_value(invocation, nullptr);
    const QImage::Format qformat = imageFormat(format);
    if (fd < 0 || qformat == QImage::Format_Invalid || w == 0 || h == 0) {
      if (fd >= 0)
        close(fd);
      return;
    }
    auto mapping = std::make_shared<Mapping>();
    mapping->size = size_t(offset) + size_t(stride) * h;
    mapping->address =
        mmap(nullptr, mapping->size, PROT_READ, MAP_SHARED, fd, 0);
    close(fd);
    if (mapping->address == MAP_FAILED)
      return;
    m_frame = QImage(static_cast<const uchar *>(mapping->address) + offset,
                     int(w), int(h), qsizetype(stride), qformat,
                     releaseMapping, new std::shared_ptr<Mapping>(mapping));
    emit imageScanout();
    return;
  }

  if (name == "UpdateMap") {
    gint32 x, y, w, h;
    g_variant_get(parameters, "(iiii)", &x, &y, &w, &h);
    g_dbus_method_invocation_return_value(invocation, nullptr);
    emit frameUpdated(QRect(x, y, w, h));
    return;
  }

  if (name == "Disable") {
    g_dbus_method_invocation_return_value(invocation, nullptr);
    m_frame = QImage();
    emit displayDisabled();
    return;
  }

  if (name == "MouseSet") {
    gint32 x, y, on;
    g_variant_get(parameters, "(iii)", &x, &y, &on);
    g_dbus_method_invocation_return_value(invocation, nullptr);
    emit cursorVisibilityChanged(on != 0);
    return;
  }

  if (name == "CursorDefine") {
    gint32 w, h, hotX, hotY;
    GVariant *data = nullptr;
    g_variant_get(parameters, "(iiii@ay)", &w, &h, &hotX, &hotY, &data);
    gsize size = 0;
    const uchar *bytes =
        static_cast<const uchar *>(g_variant_get_fixed_array(data, &size, 1));
    g_dbus_method_invocation_return_value(invocation, nullptr);
    if (w > 0 && h > 0 && w <= 512 && h <= 512 && size >= gsize(w) * h * 4)
      emit cursorChanged(QImage(bytes, w, h, QImage::Format_ARGB32).copy(),
                         QPoint(hotX, hotY));
    g_variant_unref(data);
    return;
  }

  g_dbus_method_invocation_return_dbus_error(
      invocation, "org.qemu.Display1.Error.Unsupported",
      "Not implemented by this viewer");
}

void DisplayClient::handleClipboardCall(const char *method,
                                        GVariant *parameters,
                                        GDBusMethodInvocation *invocation) {
  const QByteArray name(method);
  if (name == "Register") {
    // QEMU resets the grab serials (e.g. after a guest reboot).
    m_clipboardSerial = 0;
    g_dbus_method_invocation_return_value(invocation, nullptr);
    return;
  }
  if (name == "Grab") {
    guint32 selection, serial;
    const gchar **mimes = nullptr;
    g_variant_get(parameters, "(uu^a&s)", &selection, &serial, &mimes);
    g_dbus_method_invocation_return_value(invocation, nullptr);
    const bool text = mimes && g_strv_contains(mimes, TextMime);
    g_free(mimes);
    // The guest copied something: fetch it while it is still current.
    if (selection != 0 || serial < m_clipboardSerial || !text)
      return;
    m_clipboardSerial = serial;
    if (!m_clipboardEnabled)
      return;
    const gchar *wanted[] = {TextMime, nullptr};
    g_dbus_connection_call(
        m_connection, nullptr, ClipboardPath, "org.qemu.Display1.Clipboard",
        "Request", g_variant_new("(u^as)", 0u, wanted),
        G_VARIANT_TYPE("(say)"), G_DBUS_CALL_FLAGS_NONE, -1, m_cancellable,
        [](GObject *source, GAsyncResult *result, gpointer self) {
          GVariant *reply = g_dbus_connection_call_finish(
              G_DBUS_CONNECTION(source), result, nullptr);
          if (!reply)
            return;
          const gchar *mime = nullptr;
          GVariant *data = nullptr;
          g_variant_get(reply, "(&s@ay)", &mime, &data);
          gsize size = 0;
          const char *bytes = static_cast<const char *>(
              g_variant_get_fixed_array(data, &size, 1));
          if (g_strcmp0(mime, TextMime) == 0 && size <= MaxClipboard)
            static_cast<DisplayClient *>(self)->handleGuestClipboard(
                QByteArray(bytes, qsizetype(size)));
          g_variant_unref(data);
          g_variant_unref(reply);
        },
        this);
    return;
  }
  if (name == "Request") {
    // QEMU waits for this reply synchronously: answer immediately.
    guint32 selection;
    const gchar **mimes = nullptr;
    g_variant_get(parameters, "(u^a&s)", &selection, &mimes);
    const bool text = mimes && g_strv_contains(mimes, TextMime);
    g_free(mimes);
    if (selection != 0 || !text || !m_clipboardEnabled) {
      g_dbus_method_invocation_return_dbus_error(
          invocation, "org.qemu.Display1.Error.Failed",
          "Clipboard not shared");
      return;
    }
    g_dbus_method_invocation_return_value(
        invocation,
        g_variant_new("(s@ay)", TextMime,
                      g_variant_new_fixed_array(
                          G_VARIANT_TYPE_BYTE, m_hostClipboard.constData(),
                          gsize(m_hostClipboard.size()), 1)));
    return;
  }
  // Unregister, Release: nothing to clean up on this side.
  g_dbus_method_invocation_return_value(invocation, nullptr);
}

void DisplayClient::handleMouseAbsolute(bool absolute) {
  if (m_mouseAbsolute == absolute)
    return;
  m_mouseAbsolute = absolute;
  emit mouseAbsoluteChanged();
}

void DisplayClient::handleGuestClipboard(const QByteArray &text) {
  if (m_clipboardEnabled)
    emit clipboardReceived(QString::fromUtf8(text));
}

void DisplayClient::fail(const QString &message) {
  if (m_failed)
    return;
  m_failed = true;
  completePendingUpdates();
  emit errorOccurred(message);
}

void DisplayClient::call(const char *interface, const char *method,
                         GVariant *parameters) {
  if (!m_connection || m_consolePath.isEmpty() || m_failed) {
    if (parameters)
      g_variant_unref(g_variant_ref_sink(parameters));
    return;
  }
  g_dbus_connection_call(m_connection, nullptr, m_consolePath.constData(),
                         interface, method, parameters, nullptr,
                         G_DBUS_CALL_FLAGS_NONE, -1, m_cancellable,
                         ignoreReply, nullptr);
}

void DisplayClient::pressKey(quint32 qnum) {
  call("org.qemu.Display1.Keyboard", "Press", g_variant_new("(u)", qnum));
}

void DisplayClient::releaseKey(quint32 qnum) {
  call("org.qemu.Display1.Keyboard", "Release", g_variant_new("(u)", qnum));
}

void DisplayClient::movePointer(int x, int y) {
  call("org.qemu.Display1.Mouse", "SetAbsPosition",
       g_variant_new("(uu)", guint32(qMax(0, x)), guint32(qMax(0, y))));
}

void DisplayClient::movePointerBy(int dx, int dy) {
  if (dx != 0 || dy != 0)
    call("org.qemu.Display1.Mouse", "RelMotion", g_variant_new("(ii)", dx, dy));
}

void DisplayClient::pressButton(int button) {
  call("org.qemu.Display1.Mouse", "Press", g_variant_new("(u)", guint32(button)));
}

void DisplayClient::releaseButton(int button) {
  call("org.qemu.Display1.Mouse", "Release",
       g_variant_new("(u)", guint32(button)));
}

void DisplayClient::resizeDisplay(int width, int height, int widthMM,
                                  int heightMM) {
  if (width < 1 || height < 1)
    return;
  call("org.qemu.Display1.Console", "SetUIInfo",
       g_variant_new("(qqiiuu)", guint16(qBound(0, widthMM, 65535)),
                     guint16(qBound(0, heightMM, 65535)), 0, 0,
                     guint32(width), guint32(height)));
}

void DisplayClient::setClipboardEnabled(bool enabled) {
  m_clipboardEnabled = enabled;
}

void DisplayClient::sendClipboard(const QString &text) {
  const QByteArray utf8 = text.toUtf8();
  if (!m_clipboardEnabled || !m_connection || text.isEmpty() ||
      utf8.size() > qsizetype(MaxClipboard) || utf8 == m_hostClipboard)
    return;
  m_hostClipboard = utf8;
  const gchar *mimes[] = {TextMime, nullptr};
  g_dbus_connection_call(m_connection, nullptr, ClipboardPath,
                         "org.qemu.Display1.Clipboard", "Grab",
                         g_variant_new("(uu^as)", 0u, ++m_clipboardSerial,
                                       mimes),
                         nullptr, G_DBUS_CALL_FLAGS_NONE, -1, m_cancellable,
                         ignoreReply, nullptr);
}

void DisplayClient::frameRendered() { completePendingUpdates(); }

void DisplayClient::completePendingUpdates() {
  m_updateTimeout.stop();
  for (GDBusMethodInvocation *invocation : std::as_const(m_pendingUpdates))
    g_dbus_method_invocation_return_value(invocation, nullptr);
  m_pendingUpdates.clear();
}
