package qemu

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

// qmpConn is one QMP session: QMP requires the capabilities handshake on
// every new connection, and fd passing (getfd) only lasts for the session
// that received the fd.
type qmpConn struct {
	conn *net.UnixConn
	dec  *json.Decoder
}

func qmpDial(socketPath string) (*qmpConn, error) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial qmp socket: %w", err)
	}
	q := &qmpConn{conn: conn.(*net.UnixConn), dec: json.NewDecoder(conn)}
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		conn.Close()
		return nil, err
	}
	var greeting map[string]any
	if err := q.dec.Decode(&greeting); err != nil {
		conn.Close()
		return nil, fmt.Errorf("read qmp greeting: %w", err)
	}
	if _, err := q.execute("qmp_capabilities", nil, nil); err != nil {
		conn.Close()
		return nil, fmt.Errorf("negotiate qmp capabilities: %w", err)
	}
	return q, nil
}

func (q *qmpConn) Close() error { return q.conn.Close() }

// execute sends one command, passing file alongside it (SCM_RIGHTS) when
// non-nil, and returns after its reply.
func (q *qmpConn) execute(command string, arguments map[string]any, file *os.File) (json.RawMessage, error) {
	req := map[string]any{"execute": command}
	if arguments != nil {
		req["arguments"] = arguments
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var oob []byte
	if file != nil {
		oob = syscall.UnixRights(int(file.Fd()))
	}
	if _, _, err := q.conn.WriteMsgUnix(append(data, '\n'), oob, nil); err != nil {
		return nil, fmt.Errorf("send qmp command %s: %w", command, err)
	}
	for {
		var reply struct {
			Return json.RawMessage `json:"return"`
			Error  any             `json:"error"`
			Event  string          `json:"event"`
		}
		if err := q.dec.Decode(&reply); err != nil {
			return nil, fmt.Errorf("read qmp reply for %s: %w", command, err)
		}
		if reply.Error != nil {
			return nil, fmt.Errorf("qmp command %s failed: %v", command, reply.Error)
		}
		if reply.Return != nil {
			return reply.Return, nil
		}
		// QMP events may be interleaved with command replies.
	}
}

// qmpExecute runs a single QMP command (with optional arguments) over the
// machine's monitor socket. It's intentionally minimal: OmaVM only needs a
// handful of commands, not the full QMP protocol.
func qmpExecute(socketPath, command string, arguments map[string]any) (json.RawMessage, error) {
	q, err := qmpDial(socketPath)
	if err != nil {
		return nil, err
	}
	defer q.Close()
	return q.execute(command, arguments, nil)
}

// qmpAddDisplayClient hands QEMU one end of a peer-to-peer connection to
// its D-Bus display (-display dbus,p2p=yes).
func qmpAddDisplayClient(socketPath string, file *os.File) error {
	q, err := qmpDial(socketPath)
	if err != nil {
		return err
	}
	defer q.Close()
	const fdname = "omavm-display"
	if _, err := q.execute("getfd", map[string]any{"fdname": fdname}, file); err != nil {
		return err
	}
	_, err = q.execute("add_client", map[string]any{"protocol": "@dbus-display", "fdname": fdname}, nil)
	return err
}

func qmpCommand(socketPath string, command string) error {
	_, err := qmpExecute(socketPath, command, nil)
	return err
}

// qmpScreendump asks QEMU to write the current framebuffer to dst as a
// PPM image — no display client needed, since QEMU does the capture
// itself and writes straight to the local filesystem.
func qmpScreendump(socketPath, dst string) error {
	_, err := qmpExecute(socketPath, "screendump", map[string]any{"filename": dst})
	return err
}

func qmpStatus(socketPath string) (string, error) {
	raw, err := qmpExecute(socketPath, "query-status", nil)
	if err != nil {
		return "", err
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return "", fmt.Errorf("decode QMP status: %w", err)
	}
	return status.Status, nil
}
