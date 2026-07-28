package lsp

import (
	"encoding/json"
	"testing"
)

func TestLSPServerCapabilities(t *testing.T) {
	server := NewServer()

	// 1. Test Document Diagnostics
	server.diagnoseDocument("file:///test.cb", "fn add(a: int, b: int) -> int:\n    return a + b\n")

	// 2. Test JSON-RPC Unmarshal
	rawReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	var req Request
	err := json.Unmarshal([]byte(rawReq), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON-RPC request: %v", err)
	}

	if req.Method != "initialize" {
		t.Errorf("expected method 'initialize', got %s", req.Method)
	}
}
