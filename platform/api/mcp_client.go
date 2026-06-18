package main

// MCP HTTP client for the FastMCP server (platform/mcp/server.py).
//
// Transport: MCP Streamable HTTP — POST /mcp, JSON-RPC 2.0.
// The server may return either application/json or text/event-stream; both are
// handled. A fresh session (initialize → notifications/initialized → tools/call)
// is opened for every call — stateless from the caller's perspective.
//
// The caller controls the deadline via context. A 10-second timeout is
// recommended for interactive chat use (see PostChat in chat.go).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func getMCPServerURL() string {
	if v := os.Getenv("MCP_SERVER_URL"); v != "" {
		return v
	}
	return "http://localhost:8765"
}

// mcpHTTPClient is shared so TCP connections to the MCP server are pooled.
var mcpHTTPClient = &http.Client{}

// ── JSON-RPC 2.0 wire types ────────────────────────────────────────────────────

type mcpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"` // nil for notifications
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpRPCError    `json:"error,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ── Transport layer ────────────────────────────────────────────────────────────

// mcpPost sends one JSON-RPC message to POST {baseURL}/mcp and returns the
// response body. For HTTP 202 Accepted (notifications), it returns nil, "", nil.
// If the server responds with text/event-stream it extracts the first data: line.
func mcpPost(ctx context.Context, baseURL, sessionID string, msg any) ([]byte, string, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/mcp", bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("mcp-session-id", sessionID)
	}

	resp, err := mcpHTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("mcp server unreachable: %w", err)
	}
	defer resp.Body.Close()

	newSession := resp.Header.Get("mcp-session-id")

	if resp.StatusCode == http.StatusAccepted {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return nil, newSession, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, newSession, fmt.Errorf("mcp status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		data, err := readFirstSSEData(resp.Body)
		return data, newSession, err
	}
	data, err := io.ReadAll(resp.Body)
	return data, newSession, err
}

// readFirstSSEData reads an SSE stream and returns the first non-empty data: line.
func readFirstSSEData(r io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if after, ok := strings.CutPrefix(line, "data:"); ok {
			trimmed := strings.TrimSpace(after)
			if trimmed != "" {
				io.Copy(io.Discard, r) //nolint:errcheck
				return []byte(trimmed), nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read SSE: %w", err)
	}
	return nil, fmt.Errorf("no data event in SSE stream")
}

// ── Public API ─────────────────────────────────────────────────────────────────

// CallMCPTool opens a fresh MCP session, calls toolName with args, and returns
// the text content from the tool result. Effective deadline is min(ctx, 10s).
func CallMCPTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	base := getMCPServerURL()
	id1, id2 := 1, 2

	// Step 1: initialize — establishes the session and gets mcp-session-id
	_, sessionID, err := mcpPost(ctx, base, "", mcpRequest{
		JSONRPC: "2.0",
		ID:      &id1,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "ops-go-api", "version": "1.0.0"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("mcp initialize: %w", err)
	}

	// Step 2: notifications/initialized — required by MCP spec before tool calls
	if _, _, err := mcpPost(ctx, base, sessionID, mcpRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}); err != nil {
		log.Printf("mcp notifications/initialized warning (non-fatal): %v", err)
	}

	// Step 3: tools/call
	respData, _, err := mcpPost(ctx, base, sessionID, mcpRequest{
		JSONRPC: "2.0",
		ID:      &id2,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	})
	if err != nil {
		return "", fmt.Errorf("mcp tools/call: %w", err)
	}

	var rpc mcpRPCResponse
	if err := json.Unmarshal(respData, &rpc); err != nil {
		return "", fmt.Errorf("decode mcp response: %w", err)
	}
	if rpc.Error != nil {
		return "", fmt.Errorf("mcp rpc error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}

	var result mcpToolResult
	if err := json.Unmarshal(rpc.Result, &result); err != nil {
		return "", fmt.Errorf("decode tool result: %w", err)
	}
	if result.IsError {
		if len(result.Content) > 0 {
			return "", fmt.Errorf("tool error: %s", result.Content[0].Text)
		}
		return "", fmt.Errorf("tool returned an error with no message")
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("tool returned no content")
	}

	return result.Content[0].Text, nil
}
