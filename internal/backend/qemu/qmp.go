package qemu

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// qmpExecute sends a single QMP command (with optional arguments) over
// the machine's monitor socket and returns after the reply, performing
// the capabilities handshake QMP requires on every new connection. It's
// intentionally minimal: OmaVM only needs "quit" and "screendump" today,
// not the full QMP protocol.
func qmpExecute(socketPath, command string, arguments map[string]any) error {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return fmt.Errorf("dial qmp socket: %w", err)
	}
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var greeting map[string]any
	if err := dec.Decode(&greeting); err != nil {
		return fmt.Errorf("read qmp greeting: %w", err)
	}

	if err := enc.Encode(map[string]any{"execute": "qmp_capabilities"}); err != nil {
		return fmt.Errorf("negotiate qmp capabilities: %w", err)
	}
	var capsReply map[string]any
	if err := dec.Decode(&capsReply); err != nil {
		return fmt.Errorf("read qmp capabilities reply: %w", err)
	}

	req := map[string]any{"execute": command}
	if arguments != nil {
		req["arguments"] = arguments
	}
	if err := enc.Encode(req); err != nil {
		return fmt.Errorf("send qmp command %s: %w", command, err)
	}
	var reply map[string]any
	if err := dec.Decode(&reply); err != nil {
		return fmt.Errorf("read qmp reply for %s: %w", command, err)
	}
	if errObj, ok := reply["error"]; ok {
		return fmt.Errorf("qmp command %s failed: %v", command, errObj)
	}
	return nil
}

func qmpCommand(socketPath string, command string) error {
	return qmpExecute(socketPath, command, nil)
}

// qmpScreendump asks QEMU to write the current framebuffer to dst as a
// PPM image — no VNC/RFB client implementation needed, since QEMU does
// the capture itself and writes straight to the local filesystem.
func qmpScreendump(socketPath, dst string) error {
	return qmpExecute(socketPath, "screendump", map[string]any{"filename": dst})
}
