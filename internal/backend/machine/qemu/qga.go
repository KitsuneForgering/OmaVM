package qemu

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
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

// qgaDial connects to the guest agent and flushes whatever an earlier
// client left unread, with guest-sync: the agent answers it with the
// number it was given, after any stale reply.
func qgaDial(ctx context.Context, socketPath string) (net.Conn, *json.Decoder, error) {
	dialer := net.Dialer{Timeout: 300 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, nil, fmt.Errorf("connect guest agent: %w", err)
	}
	dec, err := qgaSync(conn)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, dec, nil
}

// qgaSync sends guest-sync and reads replies until the one carrying its
// number, discarding whatever came before, whatever its shape.
func qgaSync(conn net.Conn) (*json.Decoder, error) {
	dec := json.NewDecoder(conn)
	id := rand.Int64N(1 << 50)
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		return nil, err
	}
	if err := json.NewEncoder(conn).Encode(map[string]any{"execute": "guest-sync", "arguments": map[string]any{"id": id}}); err != nil {
		return nil, fmt.Errorf("sync guest agent: %w", err)
	}
	want := strconv.FormatInt(id, 10)
	for {
		var reply struct {
			Return json.RawMessage `json:"return"`
		}
		if err := dec.Decode(&reply); err != nil {
			return nil, fmt.Errorf("sync guest agent: %w", err)
		}
		if string(reply.Return) == want {
			return dec, nil
		}
	}
}

func qgaExecute(conn net.Conn, dec *json.Decoder, command string, timeout time.Duration) error {
	_, err := qgaQuery(conn, dec, command, timeout)
	return err
}

// qgaQuery runs an agent command and returns its result.
func qgaQuery(conn net.Conn, dec *json.Decoder, command string, timeout time.Duration) (json.RawMessage, error) {
	return qgaCall(conn, dec, command, nil, timeout)
}

// qgaCall runs an agent command with arguments (none when nil).
func qgaCall(conn net.Conn, dec *json.Decoder, command string, arguments any, timeout time.Duration) (json.RawMessage, error) {
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	request := map[string]any{"execute": command}
	if arguments != nil {
		request["arguments"] = arguments
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return nil, fmt.Errorf("%s: %w", command, err)
	}
	var reply struct {
		Return json.RawMessage `json:"return"`
		Error  any             `json:"error"`
	}
	if err := dec.Decode(&reply); err != nil {
		return nil, fmt.Errorf("%s: %w", command, err)
	}
	if reply.Error != nil {
		return nil, fmt.Errorf("%s: %v", command, reply.Error)
	}
	return reply.Return, nil
}

// virtiofsMountpoint asks the guest agent where a virtiofs filesystem is
// mounted in the guest, "" when none is. guest-get-fsinfo only lists
// filesystems backed by a disk, so it never shows virtiofs (verified with
// qemu-ga on Arch: the share mounted and readable, fsinfo listing only
// /efi and /): the guest's /proc/mounts says. fsinfo stays as the answer
// for an agent that won't run commands.
func virtiofsMountpoint(ctx context.Context, socketPath string) (string, error) {
	code, out, err := guestExec(ctx, socketPath, `awk '$3 == "virtiofs" { print $2; exit }' /proc/mounts`, checkTimeout)
	if err == nil && code == 0 {
		return strings.TrimSpace(out), nil
	}
	conn, dec, err := qgaDial(ctx, socketPath)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	raw, err := qgaQuery(conn, dec, "guest-get-fsinfo", 2*time.Second)
	if err != nil {
		return "", err
	}
	var filesystems []struct {
		Mountpoint string `json:"mountpoint"`
		Type       string `json:"type"`
	}
	if err := json.Unmarshal(raw, &filesystems); err != nil {
		return "", fmt.Errorf("guest-get-fsinfo: %w", err)
	}
	for _, fs := range filesystems {
		if fs.Type == "virtiofs" {
			return fs.Mountpoint, nil
		}
	}
	return "", nil
}

// fsFreezeTimeout bounds freezing (the guest flushes every filesystem
// first) and thawing.
const fsFreezeTimeout = 10 * time.Second

// freezeGuest asks the guest agent to flush and freeze the guest's
// filesystems, so a disk snapshot taken meanwhile is one the guest could
// have shut down to, not one taken as if the power was cut. It returns the
// function that thaws them again, or nil when the guest can't be frozen
// (no agent, or one that refuses), in which case the snapshot goes on as
// before.
func freezeGuest(ctx context.Context, socketPath, machine string) func() {
	conn, dec, err := qgaDial(ctx, socketPath)
	if err != nil {
		return nil
	}
	thaw := func() {
		err := qgaExecute(conn, dec, "guest-fsfreeze-thaw", fsFreezeTimeout)
		conn.Close()
		if err == nil {
			return
		}
		// A frozen guest can't write anything: try again on a new
		// connection before giving up.
		if retry, retryDec, dialErr := qgaDial(context.Background(), socketPath); dialErr == nil {
			err = qgaExecute(retry, retryDec, "guest-fsfreeze-thaw", fsFreezeTimeout)
			retry.Close()
		}
		if err != nil {
			slog.Error("guest filesystems may still be frozen; run fsfreeze -u inside it or restart it", "machine", machine, "error", err)
		}
	}
	if err := qgaExecute(conn, dec, "guest-fsfreeze-freeze", fsFreezeTimeout); err != nil {
		slog.Info("guest filesystems not frozen for the snapshot", "machine", machine, "error", err)
		// It may have frozen some before failing.
		thaw()
		return nil
	}
	return thaw
}
