package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newMCPMockServer builds a test server that handles the 3-step MCP protocol.
// toolBody and toolStatus configure the third request (tools/call).
func newMCPMockServer(t *testing.T, toolBody string, toolStatus int) *httptest.Server {
	t.Helper()
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		switch call {
		case 1: // initialize
			w.Header().Set("mcp-session-id", "test-session-001")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"jsonrpc": "2.0",
				"id":      1,
				"result":  map[string]any{"protocolVersion": "2024-11-05"},
			})
		case 2: // notifications/initialized
			w.WriteHeader(http.StatusAccepted)
		default: // tools/call
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(toolStatus)
			w.Write([]byte(toolBody)) //nolint:errcheck
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCallMCPTool_Success(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"{\"count\":3}"}],"isError":false}}`
	srv := newMCPMockServer(t, body, http.StatusOK)
	t.Setenv("MCP_SERVER_URL", srv.URL)

	got, err := CallMCPTool(context.Background(), "get_fleet_stats", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{"count":3}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCallMCPTool_ServerUnreachable(t *testing.T) {
	t.Setenv("MCP_SERVER_URL", "http://127.0.0.1:19999")

	_, err := CallMCPTool(context.Background(), "get_fleet_stats", map[string]any{})
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}

func TestCallMCPTool_NonOKStatus(t *testing.T) {
	srv := newMCPMockServer(t, `{"error":"internal server error"}`, http.StatusInternalServerError)
	t.Setenv("MCP_SERVER_URL", srv.URL)

	_, err := CallMCPTool(context.Background(), "get_fleet_stats", map[string]any{})
	if err == nil {
		t.Fatal("expected error for non-200 tools/call status, got nil")
	}
}

func TestCallMCPTool_MalformedJSON(t *testing.T) {
	srv := newMCPMockServer(t, `not valid json at all`, http.StatusOK)
	t.Setenv("MCP_SERVER_URL", srv.URL)

	_, err := CallMCPTool(context.Background(), "get_fleet_stats", map[string]any{})
	if err == nil {
		t.Fatal("expected error for malformed JSON response, got nil")
	}
}

func TestCallMCPTool_IsError(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"elevator not found"}],"isError":true}}`
	srv := newMCPMockServer(t, body, http.StatusOK)
	t.Setenv("MCP_SERVER_URL", srv.URL)

	_, err := CallMCPTool(context.Background(), "get_elevator_risk", map[string]any{"elevator_id": 99999})
	if err == nil {
		t.Fatal("expected error for isError:true tool result, got nil")
	}
}
