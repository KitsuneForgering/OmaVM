package qemu

import (
	"encoding/json"
	"net"
	"testing"
)

func TestQGAPing(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		var request map[string]string
		if json.NewDecoder(server).Decode(&request) == nil && request["execute"] == "guest-ping" {
			_ = json.NewEncoder(server).Encode(map[string]any{"return": map[string]any{}})
		}
	}()
	if err := qgaPingConn(client); err != nil {
		t.Fatal(err)
	}
}
