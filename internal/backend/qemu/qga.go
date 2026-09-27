package qemu

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

func qgaPing(ctx context.Context, socketPath string) error {
	dialer := net.Dialer{Timeout: 300 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect guest agent: %w", err)
	}
	defer conn.Close()
	return qgaPingConn(conn)
}

func qgaPingConn(conn net.Conn) error {
	if err := conn.SetDeadline(time.Now().Add(700 * time.Millisecond)); err != nil {
		return err
	}
	if err := json.NewEncoder(conn).Encode(map[string]string{"execute": "guest-ping"}); err != nil {
		return fmt.Errorf("ping guest agent: %w", err)
	}
	var reply struct {
		Return json.RawMessage `json:"return"`
		Error  any             `json:"error"`
	}
	if err := json.NewDecoder(conn).Decode(&reply); err != nil {
		return fmt.Errorf("read guest agent: %w", err)
	}
	if reply.Error != nil {
		return fmt.Errorf("guest agent rejected ping: %v", reply.Error)
	}
	if reply.Return == nil {
		return fmt.Errorf("guest agent returned no result")
	}
	return nil
}
