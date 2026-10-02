#include "singleinstance.h"

#include <QLocalSocket>
#include <QStandardPaths>

SingleInstance::SingleInstance(const QString &socketPath, QObject *parent)
    : QObject(parent), m_path(socketPath) {
  connect(&m_server, &QLocalServer::newConnection, this, [this]() {
    while (QLocalSocket *client = m_server.nextPendingConnection()) {
      connect(client, &QLocalSocket::disconnected, client,
              &QObject::deleteLater);
      emit activated();
      client->disconnectFromServer();
    }
  });
}

bool SingleInstance::notifyRunning() const {
  QLocalSocket socket;
  socket.connectToServer(m_path);
  // A local socket answers at once; a stale file refuses at once.
  return socket.waitForConnected(500);
}

bool SingleInstance::listen() {
  m_server.setSocketOptions(QLocalServer::UserAccessOption);
  if (m_server.listen(m_path))
    return true;
  QLocalServer::removeServer(m_path);
  return m_server.listen(m_path);
}

QString SingleInstance::defaultSocketPath() {
  QString dir =
      QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation);
  if (dir.isEmpty())
    dir = QStandardPaths::writableLocation(QStandardPaths::TempLocation);
  return dir + QStringLiteral("/omavm-gui.sock");
}
