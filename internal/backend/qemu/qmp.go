package qemu

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// qmpCommand sends a single QMP command over the machine's monitor
// socket and returns after the reply, performing the capabilities
// handshake QMP requires on every new connection. It's intentionally
// minimal: OmaVM only needs "quit" today, not the full QMP protocol.
func qmpCommand(socketPath string, command string) error {
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

	if err := enc.Encode(map[string]any{"execute": command}); err != nil {
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
