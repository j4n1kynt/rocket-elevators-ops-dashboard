package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// trackingMCPServer is a fake MCP server that records every tools/call name and
// returns a canned JSON-text payload keyed by tool name (or an empty-results
// default for tools not in the map).
type trackingMCPServer struct {
	*httptest.Server
	mu    sync.Mutex
	calls []string
}

func newTrackingMCPServer(t *testing.T, toolPayloads map[string]string) *trackingMCPServer {
	t.Helper()
	srv := &trackingMCPServer{}
	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var rpc struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.Unmarshal(body, &rpc)

		w.Header().Set("Content-Type", "application/json")
		switch rpc.Method {
		case "initialize":
			w.Header().Set("mcp-session-id", "test-session")
			io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`) //nolint:errcheck
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			srv.mu.Lock()
			srv.calls = append(srv.calls, rpc.Params.Name)
			srv.mu.Unlock()

			payload, ok := toolPayloads[rpc.Params.Name]
			if !ok {
				// Default: no confident results returned
				payload = `{"total_returned":0,"results":[]}`
			}
			b, _ := json.Marshal(payload)
			io.WriteString(w, `{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":`+string(b)+`}],"isError":false}}`) //nolint:errcheck
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	return srv
}

func (s *trackingMCPServer) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.calls...)
}

// fakeLLMServer returns a server that echoes a fixed assistant reply.
func fakeLLMServer(t *testing.T, reply string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		b, _ := json.Marshal(reply)
		io.WriteString(w, `{"message":{"role":"assistant","content":`+string(b)+`},"done":true}`) //nolint:errcheck
	}))
}

// ── Unit tests ────────────────────────────────────────────────────────────────

func TestKnowledgeSourceLabel(t *testing.T) {
	cases := []struct {
		tool    string
		wantTag string
	}{
		{"search_maintenance_docs", "[DATA SOURCE: maintenance documentation]\n"},
		{"search_incident_narratives", "[DATA SOURCE: incident narratives]\n"},
		// Any unrecognised name falls back to the maintenance label.
		{"get_fleet_stats", "[DATA SOURCE: maintenance documentation]\n"},
	}
	for _, c := range cases {
		if got := knowledgeSourceLabel(c.tool); got != c.wantTag {
			t.Errorf("knowledgeSourceLabel(%q) = %q, want %q", c.tool, got, c.wantTag)
		}
	}
}

// ── Integration tests ─────────────────────────────────────────────────────────

func TestKnowledgeAgentProcedureUsesMaintenanceDocs(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{
		"search_maintenance_docs": `{"total_returned":1,"results":[{"text":"Check hydraulic fluid level","source_name":"Maintenance Document 10078","similarity_score":0.91}]}`,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "Per Maintenance Document 10078, check hydraulic fluid level.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := knowledgeAgent(context.Background(), AgentRequest{
		Message: "how do I handle hydraulic pressure loss?",
	})

	if resp.AgentName != "knowledge" {
		t.Errorf("agent name: got %q, want %q", resp.AgentName, "knowledge")
	}
	calls := mcp.Calls()
	if len(calls) == 0 || calls[0] != "search_maintenance_docs" {
		t.Errorf("primary tool: got %v, want search_maintenance_docs first", calls)
	}
}

func TestKnowledgeAgentIncidentQueryUsesNarratives(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{
		"search_incident_narratives": `{"total_returned":2,"results":[{"incident_id":1163652,"date_of_occurrence":"2013-06-06","incident_summary":"Governor tripped on uprun"}]}`,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "Yes, we have seen similar incidents: Incident #1163652 (2013-06-06).")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := knowledgeAgent(context.Background(), AgentRequest{
		Message: "have we seen governor trips in the past?",
	})

	if resp.AgentName != "knowledge" {
		t.Errorf("agent name: got %q, want %q", resp.AgentName, "knowledge")
	}
	calls := mcp.Calls()
	if len(calls) == 0 || calls[0] != "search_incident_narratives" {
		t.Errorf("primary tool: got %v, want search_incident_narratives first", calls)
	}
}

// TestKnowledgeAgentFallsBackToSecondCorpus verifies that when the primary
// corpus returns nothing confident, the agent searches the other corpus.
func TestKnowledgeAgentFallsBackToSecondCorpus(t *testing.T) {
	// Primary (maintenance docs) is absent → default empty; fallback (narratives) hits.
	mcp := newTrackingMCPServer(t, map[string]string{
		"search_incident_narratives": `{"total_returned":1,"results":[{"incident_id":999,"date_of_occurrence":"2020-01-01","incident_summary":"Hydraulic seal failed"}]}`,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "Based on incident records, hydraulic seal failures have occurred.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	knowledgeAgent(context.Background(), AgentRequest{
		Message: "how do I handle hydraulic pressure loss?",
	})

	calls := mcp.Calls()
	if len(calls) < 2 {
		t.Fatalf("expected 2 tool calls (primary + fallback), got %d: %v", len(calls), calls)
	}
	if calls[0] != "search_maintenance_docs" {
		t.Errorf("first call must be search_maintenance_docs, got %q", calls[0])
	}
	if calls[1] != "search_incident_narratives" {
		t.Errorf("second call must be search_incident_narratives, got %q", calls[1])
	}
}

// TestKnowledgeAgentNoResultsAdvisoryOnly verifies that when both corpora return
// nothing confident, the agent still returns a non-empty reply (the prompt
// instructs the model to say so clearly rather than fabricate).
func TestKnowledgeAgentNoResultsAdvisoryOnly(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{}) // all tools → empty results
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "The documentation does not cover this. Please contact the TSSA.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := knowledgeAgent(context.Background(), AgentRequest{
		Message: "how do I handle hydraulic pressure loss?",
	})

	if resp.Reply == "" {
		t.Error("reply must not be empty even when no docs are found")
	}
	// Both corpora must have been tried before giving up.
	calls := mcp.Calls()
	if len(calls) != 2 {
		t.Errorf("expected exactly 2 tool calls, got %d: %v", len(calls), calls)
	}
}

// TestKnowledgeAgentNeverCallsDataTools asserts that the knowledge agent is
// scoped to the two knowledge search tools and never invokes data or scheduling
// tools regardless of the message content (acceptance criteria §2).
func TestKnowledgeAgentNeverCallsDataTools(t *testing.T) {
	forbiddenTools := []string{
		"get_fleet_stats", "get_elevator_risk", "get_tssa_shutdown_elevators",
		"get_elevators_needing_followup", "get_incident_count_last_year",
		"get_inspection_history", "get_elevator_incidents", "schedule_inspection",
	}

	mcp := newTrackingMCPServer(t, map[string]string{
		"search_maintenance_docs": `{"total_returned":1,"results":[{"text":"Safety gear procedure","source_name":"Maintenance Document 999","similarity_score":0.88}]}`,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "Check safety gear (Maintenance Document 999).")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	// Include a plausible elevator ID to provoke data-tool routing if the agent
	// were misbehaving (e.g. calling buildMCPArgs on the message).
	knowledgeAgent(context.Background(), AgentRequest{
		Message: "what is the safety gear procedure for elevator 12345?",
	})

	for _, call := range mcp.Calls() {
		for _, forbidden := range forbiddenTools {
			if call == forbidden {
				t.Errorf("knowledge agent must not call data tool %q", call)
			}
		}
	}
}
