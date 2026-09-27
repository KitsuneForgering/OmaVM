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
func qmpExecute(socketPath, command string, arguments map[string]any) (json.RawMessage, error) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial qmp socket: %w", err)
	}
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var greeting map[string]any
	if err := dec.Decode(&greeting); err != nil {
		return nil, fmt.Errorf("read qmp greeting: %w", err)
	}

	if err := enc.Encode(map[string]any{"execute": "qmp_capabilities"}); err != nil {
		return nil, fmt.Errorf("negotiate qmp capabilities: %w", err)
	}
	var capsReply map[string]any
	if err := dec.Decode(&capsReply); err != nil {
		return nil, fmt.Errorf("read qmp capabilities reply: %w", err)
	}

	req := map[string]any{"execute": command}
	if arguments != nil {
		req["arguments"] = arguments
	}
	if err := enc.Encode(req); err != nil {
		return nil, fmt.Errorf("send qmp command %s: %w", command, err)
	}
	for {
		var reply struct {
			Return json.RawMessage `json:"return"`
			Error  any             `json:"error"`
			Event  string          `json:"event"`
		}
		if err := dec.Decode(&reply); err != nil {
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

func qmpCommand(socketPath string, command string) error {
	_, err := qmpExecute(socketPath, command, nil)
	return err
}

// qmpScreendump asks QEMU to write the current framebuffer to dst as a
// PPM image — no VNC/RFB client implementation needed, since QEMU does
// the capture itself and writes straight to the local filesystem.
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
