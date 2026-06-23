package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// trackingMCPServer is a fake MCP server that records every tools/call name and
// returns a canned JSON-text payload keyed by tool name (or an empty-results
// default for tools not in the map).
type trackingMCPServer struct {
	*httptest.Server
	mu       sync.Mutex
	calls    []string
	callArgs []map[string]any
}

func newTrackingMCPServer(t *testing.T, toolPayloads map[string]string) *trackingMCPServer {
	t.Helper()
	srv := &trackingMCPServer{}
	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var rpc struct {
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
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
			srv.callArgs = append(srv.callArgs, rpc.Params.Arguments)
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

func (s *trackingMCPServer) CallArgs() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]map[string]any, len(s.callArgs))
	copy(result, s.callArgs)
	return result
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

// ── Scheduling agent tests ────────────────────────────────────────────────────

// TestSchedulingAgentPhase1Preview verifies that given a valid elevator ID and
// date, the scheduling agent calls schedule_inspection with confirmed=false,
// returns a non-nil PendingAction, and does not write to the database.
func TestSchedulingAgentPhase1Preview(t *testing.T) {
	phase1Payload := `{"pending_confirmation":true,"summary":"Elevator 12345 — ED-Periodic Inspection — 2026-07-01"}`

	mcp := newTrackingMCPServer(t, map[string]string{
		"schedule_inspection": phase1Payload,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "Here is a preview of the inspection. Would you like to proceed? Reply yes to confirm or no to cancel.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := schedulingAgent(context.Background(), AgentRequest{
		Message: "schedule an inspection for elevator 12345 on 2026-07-01",
	})

	if resp.AgentName != "scheduling" {
		t.Errorf("agent name: got %q, want %q", resp.AgentName, "scheduling")
	}

	// schedule_inspection must have been called exactly once.
	calls := mcp.Calls()
	if len(calls) != 1 || calls[0] != "schedule_inspection" {
		t.Errorf("tool calls: got %v, want [schedule_inspection]", calls)
	}

	// The call must carry confirmed=false — Phase 1 previews, never writes.
	args := mcp.CallArgs()
	if len(args) > 0 {
		if confirmed, ok := args[0]["confirmed"].(bool); !ok || confirmed {
			t.Errorf("schedule_inspection must be called with confirmed=false, got confirmed=%v (ok=%v)", args[0]["confirmed"], ok)
		}
	}

	// A non-nil PendingAction means the preview was returned to the caller
	// and nothing has been written to the database yet.
	if resp.PendingAction == nil {
		t.Fatal("PendingAction must be non-nil after Phase 1 preview")
	}
	if resp.PendingAction.Signature == "" {
		t.Error("PendingAction.Signature must be set so Phase 2 can verify the request")
	}
}

// TestSchedulingAgentPhase2WritesAfterConfirmation verifies that when the user
// replies "yes" to a valid pending action, the agent calls schedule_inspection
// with confirmed=true and returns a non-empty reply with no further PendingAction.
func TestSchedulingAgentPhase2WritesAfterConfirmation(t *testing.T) {
	phase2Payload := `{"success":true,"inspection_id":42,"message":"Inspection scheduled successfully."}`

	mcp := newTrackingMCPServer(t, map[string]string{
		"schedule_inspection": phase2Payload,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "The inspection has been successfully scheduled.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	// Build a valid PendingAction. signPendingAction computes the HMAC the agent
	// will re-verify in Phase 2 before allowing the write.
	pa := &PendingAction{
		ElevatorID:     12345,
		InspectionDate: "2026-07-01",
		InspectionType: "Periodic",
		Reason:         "schedule an inspection for elevator 12345 on 2026-07-01",
		ExpiresAt:      time.Now().Add(10 * time.Minute).Unix(),
	}
	pa.Signature = signPendingAction(pa)

	resp := schedulingAgent(context.Background(), AgentRequest{
		Message:       "yes",
		PendingAction: pa,
	})

	if resp.AgentName != "scheduling" {
		t.Errorf("agent name: got %q, want %q", resp.AgentName, "scheduling")
	}

	// schedule_inspection must have been called exactly once with confirmed=true.
	calls := mcp.Calls()
	if len(calls) != 1 || calls[0] != "schedule_inspection" {
		t.Errorf("tool calls: got %v, want [schedule_inspection]", calls)
	}
	args := mcp.CallArgs()
	if len(args) > 0 {
		if confirmed, ok := args[0]["confirmed"].(bool); !ok || !confirmed {
			t.Errorf("schedule_inspection must be called with confirmed=true in Phase 2, got confirmed=%v (ok=%v)", args[0]["confirmed"], ok)
		}
	}

	// After a successful write the pending state is cleared — no further confirmation needed.
	if resp.PendingAction != nil {
		t.Error("PendingAction must be nil after a successful Phase 2 write")
	}

	// The agent must report the outcome to the user.
	if resp.Reply == "" {
		t.Error("reply must not be empty after Phase 2 write")
	}
}

// TestSchedulingAgentCancelWritesNothing verifies that when the user replies
// "cancel" to a pending action, no MCP tool is called and the reply confirms
// nothing was written to the database.
func TestSchedulingAgentCancelWritesNothing(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "The inspection scheduling has been cancelled. No inspection was booked.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	pa := &PendingAction{
		ElevatorID:     12345,
		InspectionDate: "2026-07-01",
		InspectionType: "Periodic",
		Reason:         "schedule an inspection for elevator 12345 on 2026-07-01",
		ExpiresAt:      time.Now().Add(10 * time.Minute).Unix(),
	}
	pa.Signature = signPendingAction(pa)

	resp := schedulingAgent(context.Background(), AgentRequest{
		Message:       "cancel",
		PendingAction: pa,
	})

	if resp.AgentName != "scheduling" {
		t.Errorf("agent name: got %q, want %q", resp.AgentName, "scheduling")
	}

	// No MCP tool must be called — the cancel path sets context directly and skips MCP.
	calls := mcp.Calls()
	if len(calls) != 0 {
		t.Errorf("no MCP tools must be called on cancel, got: %v", calls)
	}

	// Belt-and-suspenders: confirm specifically that schedule_inspection with
	// confirmed=true was never called, which is the write that must not happen.
	for i, call := range calls {
		if call == "schedule_inspection" {
			if confirmed, ok := mcp.CallArgs()[i]["confirmed"].(bool); ok && confirmed {
				t.Error("schedule_inspection must not be called with confirmed=true when user cancels")
			}
		}
	}

	// Pending state is cleared — no further confirmation prompt.
	if resp.PendingAction != nil {
		t.Error("PendingAction must be nil after cancellation")
	}

	// The reply must confirm the cancellation to the user.
	if resp.Reply == "" {
		t.Error("reply must not be empty after cancellation")
	}
}

// TestSchedulingAgentNeverCallsForbiddenTools asserts that the scheduling agent
// never calls data or knowledge tools regardless of message content. The message
// used here contains a numeric elevator ID and a date but leads with the data
// keyword "risk", so ClassifyIntent returns IntentDataQuery and buildMCPArgs
// would produce get_elevator_risk — the guard in Phase 1 must block it.
func TestSchedulingAgentNeverCallsForbiddenTools(t *testing.T) {
	forbiddenTools := []string{
		"get_fleet_stats", "get_inspection_history", "get_elevator_risk",
		"get_elevator_incidents", "get_elevators_needing_followup",
		"get_tssa_shutdown_elevators", "get_incident_count_last_year",
		"search_maintenance_docs", "search_incident_narratives",
	}

	mcp := newTrackingMCPServer(t, map[string]string{})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "I can only help with scheduling. For risk data, please use the dashboard.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	schedulingAgent(context.Background(), AgentRequest{
		Message: "what is the risk for elevator 12345 on 2026-07-01",
	})

	for _, call := range mcp.Calls() {
		for _, forbidden := range forbiddenTools {
			if call == forbidden {
				t.Errorf("scheduling agent must not call forbidden tool %q", call)
			}
		}
	}
}
