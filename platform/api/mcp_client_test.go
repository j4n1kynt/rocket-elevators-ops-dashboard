package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newMockMCPServer speaks just enough MCP Streamable HTTP to drive CallMCPTool.
// toolResp is the raw body returned for the tools/call request.
func newMockMCPServer(t *testing.T, toolStatus int, toolResp string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &req)

		switch req.Method {
		case "initialize":
			w.Header().Set("mcp-session-id", "test-session")
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`) //nolint:errcheck
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(toolStatus)
			io.WriteString(w, toolResp) //nolint:errcheck
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
}

func TestCallMCPTool_Success(t *testing.T) {
	srv := newMockMCPServer(t, http.StatusOK,
		`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"hello fleet"}],"isError":false}}`)
	defer srv.Close()
	t.Setenv("MCP_SERVER_URL", srv.URL)

	got, err := CallMCPTool(context.Background(), "get_fleet_stats", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello fleet" {
		t.Fatalf("got %q; want %q", got, "hello fleet")
	}
}

func TestCallMCPTool_ToolError(t *testing.T) {
	srv := newMockMCPServer(t, http.StatusOK,
		`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"bad id"}],"isError":true}}`)
	defer srv.Close()
	t.Setenv("MCP_SERVER_URL", srv.URL)

	_, err := CallMCPTool(context.Background(), "get_elevator_risk", map[string]any{"id": "x"})
	if err == nil || !strings.Contains(err.Error(), "bad id") {
		t.Fatalf("want tool error containing 'bad id', got %v", err)
	}
}

func TestCallMCPTool_RPCError(t *testing.T) {
	srv := newMockMCPServer(t, http.StatusOK,
		`{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"method not found"}}`)
	defer srv.Close()
	t.Setenv("MCP_SERVER_URL", srv.URL)

	_, err := CallMCPTool(context.Background(), "nope", nil)
	if err == nil || !strings.Contains(err.Error(), "method not found") {
		t.Fatalf("want rpc error, got %v", err)
	}
}

func TestCallMCPTool_MalformedJSON(t *testing.T) {
	srv := newMockMCPServer(t, http.StatusOK, `{not json`)
	defer srv.Close()
	t.Setenv("MCP_SERVER_URL", srv.URL)

	_, err := CallMCPTool(context.Background(), "get_fleet_stats", nil)
	if err == nil || !strings.Contains(err.Error(), "decode mcp response") {
		t.Fatalf("want decode error, got %v", err)
	}
}

func TestCallMCPTool_Non200(t *testing.T) {
	srv := newMockMCPServer(t, http.StatusInternalServerError, `boom`)
	defer srv.Close()
	t.Setenv("MCP_SERVER_URL", srv.URL)

	_, err := CallMCPTool(context.Background(), "get_fleet_stats", nil)
	if err == nil || !strings.Contains(err.Error(), "mcp status 500") {
		t.Fatalf("want status 500 error, got %v", err)
	}
}

func TestCallMCPTool_Unreachable(t *testing.T) {
	srv := newMockMCPServer(t, http.StatusOK, `{}`)
	srv.Close() // close immediately so the address refuses connections
	t.Setenv("MCP_SERVER_URL", srv.URL)

	_, err := CallMCPTool(context.Background(), "get_fleet_stats", nil)
	if err == nil || !strings.Contains(err.Error(), "initialize") {
		t.Fatalf("want initialize/unreachable error, got %v", err)
	}
}
