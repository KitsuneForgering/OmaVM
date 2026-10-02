#pragma once

#include <QLocalServer>
#include <QObject>
#include <QString>

// One Experience Center per user session: a second launch (the launcher
// clicked twice, "Open OmaVM" from a viewer) asks the running one to come
// forward instead of opening another window over the same environments.
// Viewers and terminals are not single-instance: one per environment.
class SingleInstance : public QObject {
  Q_OBJECT

public:
  explicit SingleInstance(const QString &socketPath, QObject *parent = nullptr);

  // True when another instance answered and was asked to come forward;
  // this process should then exit.
  bool notifyRunning() const;
  // Starts answering later launches. A socket left by a crashed instance
  // (nobody answers on it) is replaced.
  bool listen();

  // $XDG_RUNTIME_DIR/omavm-gui.sock: per user, gone at logout.
  static QString defaultSocketPath();

signals:
  void activated();

private:
  QString m_path;
  QLocalServer m_server;
};
