package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// newFailingLLMServer returns an LLM endpoint that always responds with the given
// HTTP status and an error body, so callChatLLM returns an error. Agents must
// then fall back to a graceful plain-language reply (buildReply) or, on the data
// hybrid path, show the deterministic block alone — never crash or leak the error.
func newFailingLLMServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		io.WriteString(w, `{"error":{"message":"the model service is down"}}`) //nolint:errcheck
	}))
}

// assertSafeReply asserts that a user-facing reply is safe to show: non-empty
// and free of leaked raw infrastructure errors or unparsed JSON blobs. Edge-case
// tests use it to prove that however a query degrades (ambiguous, multi-domain,
// no-results, or a misbehaving LLM), the user never sees internal error text or
// structured payloads.
func assertSafeReply(t *testing.T, reply string) {
	t.Helper()
	if strings.TrimSpace(reply) == "" {
		t.Error("reply must not be empty")
		return
	}
	// No unparsed JSON blob. looksLikeJSON catches a leading "{" or an array that
	// parses as JSON; the `{"` substring catches an object embedded mid-reply.
	if looksLikeJSON(reply) || strings.Contains(reply, `{"`) {
		t.Errorf("reply must not leak raw JSON\n--- reply ---\n%s", reply)
	}
	// No leaked infrastructure error text anywhere in the reply.
	lower := strings.ToLower(reply)
	for _, marker := range []string{
		"connection refused", "dial tcp", "connectex:",
		"mcp server unreachable", "mcp status", "llm returned status",
		"panic:", "goroutine ",
	} {
		if strings.Contains(lower, marker) {
			t.Errorf("reply must not leak raw error text %q\n--- reply ---\n%s", marker, reply)
		}
	}
}

// ── Unit tests ────────────────────────────────────────────────────────────────

func TestToolInScope(t *testing.T) {
	allowed := []string{"get_fleet_stats", "get_elevator_risk"}
	if !toolInScope("get_elevator_risk", allowed) {
		t.Error("get_elevator_risk should be in scope")
	}
	if toolInScope("search_maintenance_docs", allowed) {
		t.Error("search_maintenance_docs should not be in scope")
	}
	if toolInScope("get_fleet_stats", nil) {
		t.Error("nothing is in scope for an empty allowed set")
	}
}

func TestFormatRiskBlock(t *testing.T) {
	t.Run("full prediction with explanation", func(t *testing.T) {
		block, ok := formatRiskBlock(`{"elevator_found":true,"prediction_found":true,"source":"predictions table","elevator_id":12345,"risk_score":0.82,"risk_level":"HIGH","model_version":"v2","prediction_date":"2026-05-01","risk_explanation":"Two overdue orders."}`)
		if !ok {
			t.Fatal("ok should be true for a valid payload")
		}
		for _, want := range []string{
			"Source: live fleet database — predictions (model v2, 2026-05-01)",
			"Elevator: 12345",
			"Risk level: HIGH",
			"Score: 0.82",
			"Explanation: Two overdue orders.",
		} {
			if !strings.Contains(block, want) {
				t.Errorf("block missing %q\n--- block ---\n%s", want, block)
			}
		}
	})

	t.Run("prediction without explanation omits the line", func(t *testing.T) {
		block, ok := formatRiskBlock(`{"elevator_found":true,"prediction_found":true,"elevator_id":7,"risk_score":0.3,"risk_level":"LOW","risk_explanation":null}`)
		if !ok {
			t.Fatal("ok should be true")
		}
		if strings.Contains(block, "Explanation:") {
			t.Errorf("block should not contain an Explanation line when none is present\n%s", block)
		}
		if !strings.Contains(block, "Score: 0.30") {
			t.Errorf("score should be formatted to two decimals\n%s", block)
		}
	})

	t.Run("elevator not found", func(t *testing.T) {
		block, ok := formatRiskBlock(`{"elevator_found":false,"prediction_found":false,"elevator_id":99999}`)
		if !ok {
			t.Fatal("ok should be true")
		}
		if !strings.Contains(block, "was not found") {
			t.Errorf("block should state the elevator was not found\n%s", block)
		}
	})

	t.Run("found but no prediction", func(t *testing.T) {
		block, ok := formatRiskBlock(`{"elevator_found":true,"prediction_found":false,"elevator_id":42}`)
		if !ok {
			t.Fatal("ok should be true")
		}
		if !strings.Contains(block, "no risk prediction") {
			t.Errorf("block should state no prediction is available\n%s", block)
		}
	})

	t.Run("invalid JSON falls back", func(t *testing.T) {
		if _, ok := formatRiskBlock("not json"); ok {
			t.Error("ok should be false for invalid JSON")
		}
	})
}

func TestFormatToolResult(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		payload string
		want    []string // substrings that must appear in the block
	}{
		{
			name:    "fleet stats",
			tool:    "get_fleet_stats",
			payload: `{"total_elevators":1000,"source":"fleet database (aggregate)","risk_distribution":{"low":600,"medium":300,"high":80,"unknown":20},"inspection_pass_rate_pct":87.5,"equipment_type_distribution":{"Passenger Elevator":900,"Freight Elevator":100}}`,
			want: []string{
				"Source: live fleet database — fleet-wide aggregate",
				"Total elevators: 1000",
				"Risk — low: 600, medium: 300, high: 80, unknown: 20",
				"Inspection pass rate: 87.5%",
				"Equipment types: Passenger Elevator: 900, Freight Elevator: 100",
			},
		},
		{
			name:    "inspection history",
			tool:    "get_inspection_history",
			payload: `{"found":true,"elevator_id":12345,"total_returned":2,"source":"inspections table","inspections":[{"inspection_type":"ED-Periodic Inspection","latest_inspection_date":"2025-03-20","outcome":"Passed"},{"inspection_type":"ED-Followup Inspection","latest_inspection_date":"2024-03-15","outcome":"Follow up"}]}`,
			want: []string{
				"Source: live fleet database — inspections",
				"Elevator: 12345",
				"Inspections found: 2",
				"- 2025-03-20 — ED-Periodic Inspection — Passed",
				"- 2024-03-15 — ED-Followup Inspection — Follow up",
			},
		},
		{
			name:    "inspection history not found",
			tool:    "get_inspection_history",
			payload: `{"found":false,"elevator_id":99999,"inspections":[]}`,
			want:    []string{"Elevator 99999 was not found"},
		},
		{
			name:    "elevator incidents",
			tool:    "get_elevator_incidents",
			payload: `{"found":true,"elevator_id":102,"total_returned":1,"source":"incidents table","incidents":[{"incident_id":4821,"date_of_occurrence":"2015-06-06","category":"Entrapment","incident_summary":"Doors failed to open.","injury_severity":"minor","fatal_injury":false}]}`,
			want: []string{
				"Source: live fleet database — incidents",
				"Incidents found: 1",
				"- Incident #4821 (2015-06-06) — category: Entrapment, injury: minor",
				"  Summary: Doors failed to open.",
			},
		},
		{
			name:    "needing followup",
			tool:    "get_elevators_needing_followup",
			payload: `{"count":1,"source":"inspections table (most-recent inspection per elevator)","elevators":[{"elevator_id":555,"location":"Toronto","status":"Active","latest_inspection_date":"2025-01-10","outcome":"Follow up","inspection_type":"ED-Periodic Inspection"}]}`,
			want: []string{
				"Elevators needing follow-up: 1",
				"- Elevator 555 — Toronto — Follow up (last inspection 2025-01-10)",
			},
		},
		{
			name:    "tssa shutdown keeps note",
			tool:    "get_tssa_shutdown_elevators",
			payload: `{"count":1,"source":"inspections table (most-recent inspection per elevator)","note":"No explicit shutdown flag exists in the database.","elevators":[{"elevator_id":777,"location":"Ottawa","status":"Active","latest_inspection_date":"2024-12-01","outcome":"Fail","inspection_type":"ED-Periodic Inspection"}]}`,
			want: []string{
				"Elevators flagged for TSSA shutdown: 1",
				"Note: No explicit shutdown flag exists in the database.",
				"- Elevator 777 — Ottawa — Fail (last inspection 2024-12-01)",
			},
		},
		{
			name:    "incident count last year",
			tool:    "get_incident_count_last_year",
			payload: `{"source":"incidents table (aggregate)","total_incidents":42,"fatal_incidents":1,"injury_incidents":10,"year_queried":2015}`,
			want: []string{
				"Year: 2015",
				"Total incidents: 42",
				"With injury: 10",
				"Fatal: 1",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			block, ok := formatToolResult(c.tool, c.payload)
			if !ok {
				t.Fatalf("formatToolResult(%q) returned ok=false", c.tool)
			}
			for _, want := range c.want {
				if !strings.Contains(block, want) {
					t.Errorf("block missing %q\n--- block ---\n%s", want, block)
				}
			}
		})
	}
}

func TestFormatToolResultUnknownTool(t *testing.T) {
	if _, ok := formatToolResult("search_maintenance_docs", `{}`); ok {
		t.Error("a tool without a formatter must return ok=false")
	}
}

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
	// The reply must carry the source document name so the user can trace the answer.
	if !strings.Contains(resp.Reply, "Maintenance Document 10078") {
		t.Errorf("reply must cite source document name\n--- reply ---\n%s", resp.Reply)
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
	// The reply must reference the incident ID so the user can verify the source.
	if !strings.Contains(resp.Reply, "1163652") {
		t.Errorf("reply must reference incident ID from the tool result\n--- reply ---\n%s", resp.Reply)
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

// TestKnowledgeAgentNoResultsSystemPromptClean verifies that when both corpora
// return no results, the LLM receives a system message that:
//   - contains the "no fabrication" instruction from the knowledge prompt, and
//   - does NOT contain a [DATA SOURCE: ...] tag (no phantom source was injected).
//
// This guards against the agent silently claiming to have documentation it did
// not retrieve.
func TestKnowledgeAgentNoResultsSystemPromptClean(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{}) // all tools → empty results
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := newCapturingLLMServer(t, "The documentation does not cover this topic.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	knowledgeAgent(context.Background(), AgentRequest{
		Message: "how do I calibrate the load-weighing device?",
	})

	systems := llm.SystemMessages()
	if len(systems) == 0 {
		t.Fatal("no system message captured — LLM was never called")
	}
	sys := systems[0]

	// The base prompt's no-fabrication instruction must be present.
	if !strings.Contains(sys, "fabricat") {
		t.Errorf("system message must carry the no-fabrication instruction\ngot: %s", sys)
	}
	// No phantom data source must be injected — that would imply the model was
	// told it had documentation it never retrieved.
	if strings.Contains(sys, "[DATA SOURCE:") {
		t.Errorf("system message must not inject a [DATA SOURCE:] tag when no results were found\ngot: %s", sys)
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

// ── Data agent ──────────────────────────────────────────────────────────────

// TestDataAgentRiskLookupUsesRiskTool verifies a risk question reaches the data
// agent's risk tool and the answer carries the agent name (acceptance criteria
// §1 — grounded risk lookup).
func TestDataAgentRiskLookupUsesRiskTool(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{
		"get_elevator_risk": `{"elevator_found":true,"prediction_found":true,"risk_score":0.82,"risk_level":"HIGH","model_version":"v2","prediction_date":"2026-05-01"}`,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "According to the live fleet database, elevator 12345 is HIGH risk (score 0.82).")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := dataAgent(context.Background(), AgentRequest{
		Message: "what is the risk level of elevator 12345?",
	})

	if resp.AgentName != "data" {
		t.Errorf("agent name: got %q, want %q", resp.AgentName, "data")
	}
	calls := mcp.Calls()
	if len(calls) == 0 || calls[0] != "get_elevator_risk" {
		t.Errorf("tool: got %v, want get_elevator_risk", calls)
	}
	// Hybrid: the deterministic Go block must appear in the reply, exactly,
	// regardless of what the model wrote for the intro line.
	for _, want := range []string{"Source: live fleet database", "Risk level: HIGH", "Score: 0.82"} {
		if !strings.Contains(resp.Reply, want) {
			t.Errorf("reply missing %q\n--- reply ---\n%s", want, resp.Reply)
		}
	}
}

// TestDataAgentFleetStatsBlockInReply verifies that for a fleet-wide query the
// data agent calls get_fleet_stats and the reply contains the exact deterministic
// block produced by Go — not a model-generated paraphrase of the numbers.
//
// Message scoring: "which" (0.5) + "dangerous" (1.0) + "flagged" (0.5) = 2.0
// → confidence 0.67 → IntentDataQuery; no elevator ID, no specific sub-keyword
// → buildMCPArgs falls through to the default → get_fleet_stats.
func TestDataAgentFleetStatsBlockInReply(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{
		"get_fleet_stats": `{"total_elevators":1000,"source":"fleet database (aggregate)","risk_distribution":{"low":600,"medium":300,"high":80,"unknown":20},"inspection_pass_rate_pct":87.5,"equipment_type_distribution":{"Passenger Elevator":900,"Freight Elevator":100}}`,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "Here is the current fleet overview.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := dataAgent(context.Background(), AgentRequest{
		Message: "Which elevators are flagged as dangerous?",
	})

	if calls := mcp.Calls(); len(calls) == 0 || calls[0] != "get_fleet_stats" {
		t.Errorf("tool: got %v, want get_fleet_stats", calls)
	}
	// The deterministic Go block must appear verbatim in the reply regardless of
	// whatever intro sentence the model wrote — it never touches the numbers.
	for _, want := range []string{
		"Source: live fleet database — fleet-wide aggregate",
		"Total elevators: 1000",
		"Inspection pass rate: 87.5%",
		"Risk — low: 600, medium: 300, high: 80, unknown: 20",
	} {
		if !strings.Contains(resp.Reply, want) {
			t.Errorf("reply missing %q\n--- reply ---\n%s", want, resp.Reply)
		}
	}
}

// TestDataAgentNoResultsReportsNoRecordsNotInvented verifies the "unanswerable"
// edge case for the data agent: when a data tool returns no records (elevator
// not found, found-but-no-prediction, or an empty list), the user-facing reply
// must carry the deterministic "no records / not found" block — never an
// invented answer.
//
// The guarantee is structural: formatToolResult returns ok=true for these
// payloads too, so the hybrid path sets dataBlock and injects it verbatim into
// resp.Reply. The LLM only writes the one-line intro and never touches the
// block. The fake LLM here returns a neutral intro that contains none of the
// asserted wording, so the "no records" text can only have originated from the
// Go-built block — proving the answer is grounded in what the tool returned.
//
// Formatter-level coverage of these branches lives in TestFormatRiskBlock and
// TestFormatToolResult (unit); this test proves the block survives all the way
// into the reply the user sees.
func TestDataAgentNoResultsReportsNoRecordsNotInvented(t *testing.T) {
	cases := []struct {
		name        string
		message     string
		wantTool    string
		toolPayload string
		wantInReply string
	}{
		{
			// elevator_found:false → notFoundBlock
			name:        "elevator not found",
			message:     "what is the risk level of elevator 99999?",
			wantTool:    "get_elevator_risk",
			toolPayload: `{"elevator_found":false,"prediction_found":false,"elevator_id":99999}`,
			wantInReply: "Elevator 99999 was not found in the fleet database.",
		},
		{
			// elevator exists but the model scored no prediction for it
			name:        "found but no prediction",
			message:     "what is the risk level of elevator 42?",
			wantTool:    "get_elevator_risk",
			toolPayload: `{"elevator_found":true,"prediction_found":false,"elevator_id":42}`,
			wantInReply: "Elevator 42 has no risk prediction.",
		},
		{
			// list query that legitimately matches zero elevators
			name:        "empty list result",
			message:     "Which elevators are shut down by TSSA?",
			wantTool:    "get_tssa_shutdown_elevators",
			toolPayload: `{"count":0,"source":"inspections table","note":"","elevators":[]}`,
			wantInReply: "Elevators flagged for TSSA shutdown: 0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mcp := newTrackingMCPServer(t, map[string]string{tc.wantTool: tc.toolPayload})
			defer mcp.Close()
			t.Setenv("MCP_SERVER_URL", mcp.URL)

			// Neutral intro — deliberately contains none of the wantInReply text,
			// so the "no records" wording can only come from the Go-built block.
			llm := fakeLLMServer(t, "Here is what the fleet database shows.")
			defer llm.Close()
			t.Setenv("OLLAMA_BASE_URL", llm.URL)
			t.Setenv("OLLAMA_API_KEY", "test-key")

			resp := dataAgent(context.Background(), AgentRequest{Message: tc.message})

			if resp.AgentName != "data" {
				t.Errorf("agent name: got %q, want %q", resp.AgentName, "data")
			}
			if calls := mcp.Calls(); len(calls) == 0 || calls[0] != tc.wantTool {
				t.Errorf("tool: got %v, want %s", calls, tc.wantTool)
			}
			// The reply must be non-empty and never leak raw errors or JSON.
			assertSafeReply(t, resp.Reply)
			// The grounded "no records" text must reach the reply verbatim.
			if !strings.Contains(resp.Reply, tc.wantInReply) {
				t.Errorf("reply must carry the deterministic no-records block %q\n--- reply ---\n%s", tc.wantInReply, resp.Reply)
			}
		})
	}
}

// TestDataAgentHybridIntroDropsMalformedLLMOutput verifies that the data agent's
// hybrid path stays safe even when the summary model misbehaves. The intro line
// is the one piece of unguarded LLM output in that path (the block below it is
// Go-built), so if the model returns a raw error string or a JSON blob it must be
// dropped — the user then sees the deterministic block alone, never the leak.
//
// buildReply already guards the fallback/general/knowledge paths
// (TestBuildReplyMalformedLLMOutput); this covers the hybrid data path it does
// not run through.
func TestDataAgentHybridIntroDropsMalformedLLMOutput(t *testing.T) {
	cases := []struct {
		name     string
		llmReply string
	}{
		{name: "raw error intro", llmReply: "mcp server unreachable: dial tcp 127.0.0.1:8765: connection refused"},
		{name: "json blob intro", llmReply: `{"risk_level":"LOW","risk_score":0.1}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mcp := newTrackingMCPServer(t, map[string]string{
				"get_elevator_risk": `{"elevator_found":true,"prediction_found":true,"risk_score":0.82,"risk_level":"HIGH","model_version":"v2","prediction_date":"2026-05-01"}`,
			})
			defer mcp.Close()
			t.Setenv("MCP_SERVER_URL", mcp.URL)

			llm := fakeLLMServer(t, tc.llmReply)
			defer llm.Close()
			t.Setenv("OLLAMA_BASE_URL", llm.URL)
			t.Setenv("OLLAMA_API_KEY", "test-key")

			resp := dataAgent(context.Background(), AgentRequest{
				Message: "what is the risk level of elevator 12345?",
			})

			// The malformed intro must be dropped, never leaked.
			assertSafeReply(t, resp.Reply)
			// The grounded Go block must still be delivered — the answer is never lost.
			for _, want := range []string{"Source: live fleet database", "Risk level: HIGH", "Score: 0.82"} {
				if !strings.Contains(resp.Reply, want) {
					t.Errorf("reply missing grounded block %q\n--- reply ---\n%s", want, resp.Reply)
				}
			}
		})
	}
}

// TestDataAgentNeverCallsForbiddenTools asserts the data agent is scoped to the
// data tools and never invokes the knowledge or scheduling tools, even when the
// message would otherwise classify as RAG or action (acceptance criteria §2).
func TestDataAgentNeverCallsForbiddenTools(t *testing.T) {
	forbiddenTools := []string{
		"search_maintenance_docs", "search_incident_narratives", "schedule_inspection",
	}

	// These messages classify as RAG / action inside the agent; the scope guard
	// must stop the resulting tool from ever reaching MCP.
	messages := []string{
		"what is the procedure for hydraulic pressure loss?",
		"schedule an inspection for elevator 12345 on 2026-07-01",
	}

	for _, msg := range messages {
		mcp := newTrackingMCPServer(t, map[string]string{})
		t.Setenv("MCP_SERVER_URL", mcp.URL)

		llm := fakeLLMServer(t, "I can only answer fleet data questions.")
		t.Setenv("OLLAMA_BASE_URL", llm.URL)
		t.Setenv("OLLAMA_API_KEY", "test-key")

		dataAgent(context.Background(), AgentRequest{Message: msg})

		for _, call := range mcp.Calls() {
			for _, forbidden := range forbiddenTools {
				if call == forbidden {
					t.Errorf("data agent must not call %q (message: %q)", call, msg)
				}
			}
		}
		mcp.Close()
		llm.Close()
	}
}

// TestDataAgentRespectsAllowedTools verifies that the router-supplied scope is
// honored: a data tool absent from AllowedTools is not called.
func TestDataAgentRespectsAllowedTools(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{
		"get_elevator_risk": `{"elevator_found":true,"prediction_found":true,"risk_score":0.4,"risk_level":"MEDIUM"}`,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "No risk data available right now.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	// AllowedTools omits get_elevator_risk → the agent must skip the MCP call.
	dataAgent(context.Background(), AgentRequest{
		Message:      "what is the risk level of elevator 12345?",
		AllowedTools: []string{"get_fleet_stats"},
	})

	if calls := mcp.Calls(); len(calls) != 0 {
		t.Errorf("expected no MCP calls when tool is out of allowed scope, got %v", calls)
	}
}

// capturingLLMServer records the system-message content of every request so
// tests can assert on what the agent actually sent to the model.
type capturingLLMServer struct {
	*httptest.Server
	mu      sync.Mutex
	systems []string
}

func newCapturingLLMServer(t *testing.T, reply string) *capturingLLMServer {
	t.Helper()
	srv := &capturingLLMServer{}
	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err == nil {
			srv.mu.Lock()
			for _, m := range req.Messages {
				if m.Role == "system" {
					srv.systems = append(srv.systems, m.Content)
				}
			}
			srv.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := json.Marshal(reply)
		io.WriteString(w, `{"message":{"role":"assistant","content":`+string(b)+`},"done":true}`) //nolint:errcheck
	}))
	return srv
}

func (s *capturingLLMServer) SystemMessages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.systems...)
}

// TestDataAgentMCPTransportFailureNoRawError verifies that when the MCP server
// is unreachable, dataAgent injects the plain-language unavailability notice
// rather than leaking raw Go error strings (e.g. "connection refused", "dial
// tcp") into the system message sent to the LLM.
func TestDataAgentMCPTransportFailureNoRawError(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{})
	mcp.Close() // shut down before the agent runs — produces "connection refused"
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := newCapturingLLMServer(t, "The data service is currently unavailable.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	dataAgent(context.Background(), AgentRequest{
		Message: "how many elevators are offline?",
	})

	systems := llm.SystemMessages()
	if len(systems) == 0 {
		t.Fatal("no system message captured — LLM was never called")
	}
	sys := systems[0]

	if !strings.Contains(sys, "DATA SERVICE UNAVAILABLE") {
		t.Errorf("system message must contain DATA SERVICE UNAVAILABLE\ngot: %s", sys)
	}
	for _, bad := range []string{"connection refused", "dial tcp", "mcp server unreachable", "mcp status"} {
		if strings.Contains(sys, bad) {
			t.Errorf("system message must not contain raw error text %q\ngot: %s", bad, sys)
		}
	}
}

// TestKnowledgeAgentBothCorporaToolErrorNoRawError verifies that when both
// corpus tools return error payloads (isToolError path), knowledgeAgent injects
// the plain-language unavailability notice and does not forward raw error
// strings to the LLM.
func TestKnowledgeAgentBothCorporaToolErrorNoRawError(t *testing.T) {
	errorPayload := `{"error":true,"message":"internal tool error"}`
	mcp := newTrackingMCPServer(t, map[string]string{
		"search_maintenance_docs":    errorPayload,
		"search_incident_narratives": errorPayload,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := newCapturingLLMServer(t, "The documentation search service is currently unavailable.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	knowledgeAgent(context.Background(), AgentRequest{
		Message: "how do I maintain hydraulic systems?",
	})

	systems := llm.SystemMessages()
	if len(systems) == 0 {
		t.Fatal("no system message captured — LLM was never called")
	}
	sys := systems[0]

	if !strings.Contains(sys, "DATA SERVICE UNAVAILABLE") {
		t.Errorf("system message must contain DATA SERVICE UNAVAILABLE\ngot: %s", sys)
	}
	for _, bad := range []string{"tool error", "connection refused", "dial tcp", "internal tool error"} {
		if strings.Contains(sys, bad) {
			t.Errorf("system message must not contain raw error text %q\ngot: %s", bad, sys)
		}
	}
}

// ── Error recovery: downstream service unavailable (AC §4) ────────────────────
//
// These three table-driven tests complete the failure-mode × agent matrix for
// "a tool or downstream service is down". The two pre-existing cells are
// TestDataAgentMCPTransportFailureNoRawError (data / MCP unreachable) and
// TestKnowledgeAgentBothCorporaToolErrorNoRawError (knowledge / tool error
// payload); the cells below are the ones those did not cover.
//
// rawErrorMarkers are infrastructure error fragments that must never reach the
// model's system message — leaking them would mean the user could see a Go
// transport error instead of a plain-language notice.
var rawErrorMarkers = []string{
	"connection refused", "dial tcp", "connectex:",
	"mcp server unreachable", "mcp status", "llm returned status",
	"panic:", "goroutine ",
}

func assertNoRawErrorLeak(t *testing.T, sys string) {
	t.Helper()
	lower := strings.ToLower(sys)
	for _, bad := range rawErrorMarkers {
		if strings.Contains(lower, bad) {
			t.Errorf("system message must not contain raw error text %q\n--- system ---\n%s", bad, sys)
		}
	}
}

// TestAgentsMCPUnreachableRecoverGracefully simulates the MCP server being down
// (closed before the agent runs → "connection refused") for the knowledge and
// scheduling agents. Each must inject a plain-language notice into the system
// message, never leak the raw transport error, and still return a safe reply.
func TestAgentsMCPUnreachableRecoverGracefully(t *testing.T) {
	cases := []struct {
		name       string
		req        AgentRequest
		agent      func(context.Context, AgentRequest) AgentResponse
		wantName   string
		wantNotice string // must appear in the system message sent to the LLM
	}{
		{
			name:       "knowledge: both corpora unreachable",
			req:        AgentRequest{Message: "how do I maintain hydraulic systems?"},
			agent:      knowledgeAgent,
			wantName:   "knowledge",
			wantNotice: "DATA SERVICE UNAVAILABLE",
		},
		{
			// Phase 1 with a valid ID + date so the agent reaches the MCP call;
			// the closed server yields a transport error that cleanValidationError
			// must scrub down to a generic message.
			name:       "scheduling: schedule_inspection unreachable",
			req:        AgentRequest{Message: "schedule an inspection for elevator 12345 on 2026-07-01", AllowedTools: []string{"schedule_inspection"}},
			agent:      schedulingAgent,
			wantName:   "scheduling",
			wantNotice: "ACTION VALIDATION ERROR",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mcp := newTrackingMCPServer(t, map[string]string{})
			mcp.Close() // down before the agent runs → connection refused
			t.Setenv("MCP_SERVER_URL", mcp.URL)

			llm := newCapturingLLMServer(t, "Live fleet data is currently unavailable; please try again shortly.")
			defer llm.Close()
			t.Setenv("OLLAMA_BASE_URL", llm.URL)
			t.Setenv("OLLAMA_API_KEY", "test-key")

			resp := tc.agent(context.Background(), tc.req)

			if resp.AgentName != tc.wantName {
				t.Errorf("agent name: got %q, want %q", resp.AgentName, tc.wantName)
			}
			systems := llm.SystemMessages()
			if len(systems) == 0 {
				t.Fatal("no system message captured — LLM was never called")
			}
			sys := systems[0]
			if !strings.Contains(sys, tc.wantNotice) {
				t.Errorf("system message must contain %q\n--- system ---\n%s", tc.wantNotice, sys)
			}
			assertNoRawErrorLeak(t, sys)
			assertSafeReply(t, resp.Reply)
			// A scheduling failure must never leave a pending write outstanding.
			if resp.PendingAction != nil {
				t.Error("PendingAction must be nil when the MCP call failed")
			}
		})
	}
}

// TestAgentsToolErrorPayloadRecoverGracefully simulates a reachable tool that
// returns an application-level error envelope (data agent) or a validation
// failure (scheduling agent). The agent must surface a plain-language notice and
// never forward the raw error payload to the model.
func TestAgentsToolErrorPayloadRecoverGracefully(t *testing.T) {
	cases := []struct {
		name       string
		req        AgentRequest
		agent      func(context.Context, AgentRequest) AgentResponse
		payloads   map[string]string
		wantName   string
		wantNotice string
		mustHide   string // raw payload text that must NOT reach the system message
	}{
		{
			name:       "data: tool returns error envelope",
			req:        AgentRequest{Message: "what is the risk level of elevator 12345?"},
			agent:      dataAgent,
			payloads:   map[string]string{"get_elevator_risk": `{"error":true,"message":"internal database failure"}`},
			wantName:   "data",
			wantNotice: "DATA SERVICE ERROR",
			mustHide:   "internal database failure",
		},
		{
			// success=false + error string → extractScheduleError → ACTION VALIDATION
			// ERROR. The clean validation message is allowed in the system message;
			// only raw infrastructure text is forbidden.
			name:       "scheduling: schedule_inspection validation failure",
			req:        AgentRequest{Message: "schedule an inspection for elevator 99999 on 2026-07-01", AllowedTools: []string{"schedule_inspection"}},
			agent:      schedulingAgent,
			payloads:   map[string]string{"schedule_inspection": `{"success":false,"error":"Elevator 99999 does not exist in the fleet."}`},
			wantName:   "scheduling",
			wantNotice: "ACTION VALIDATION ERROR",
			mustHide:   "", // the validation message itself is user-appropriate
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mcp := newTrackingMCPServer(t, tc.payloads)
			defer mcp.Close()
			t.Setenv("MCP_SERVER_URL", mcp.URL)

			llm := newCapturingLLMServer(t, "I can't reach live fleet data right now, so I can't answer that with real records.")
			defer llm.Close()
			t.Setenv("OLLAMA_BASE_URL", llm.URL)
			t.Setenv("OLLAMA_API_KEY", "test-key")

			resp := tc.agent(context.Background(), tc.req)

			if resp.AgentName != tc.wantName {
				t.Errorf("agent name: got %q, want %q", resp.AgentName, tc.wantName)
			}
			systems := llm.SystemMessages()
			if len(systems) == 0 {
				t.Fatal("no system message captured — LLM was never called")
			}
			sys := systems[0]
			if !strings.Contains(sys, tc.wantNotice) {
				t.Errorf("system message must contain %q\n--- system ---\n%s", tc.wantNotice, sys)
			}
			if tc.mustHide != "" && strings.Contains(sys, tc.mustHide) {
				t.Errorf("system message must not forward raw payload text %q\n--- system ---\n%s", tc.mustHide, sys)
			}
			assertNoRawErrorLeak(t, sys)
			assertSafeReply(t, resp.Reply)
			if resp.PendingAction != nil {
				t.Error("PendingAction must be nil when the tool returned an error")
			}
		})
	}
}

// TestAgentsLLMFailureRecoverGracefully simulates the LLM endpoint being down
// (HTTP 503) for all four agents. Agents that answer through buildReply must
// return the graceful fallback message; the data agent's hybrid path must drop
// the failed intro and show the deterministic Go block alone — the grounded
// answer is never lost. No path may crash or leak the transport error.
func TestAgentsLLMFailureRecoverGracefully(t *testing.T) {
	cases := []struct {
		name        string
		req         AgentRequest
		agent       func(context.Context, AgentRequest) AgentResponse
		payloads    map[string]string
		wantName    string
		wantInReply string // substring proving graceful recovery
	}{
		{
			// Hybrid path: MCP is healthy, only the summary LLM is down → reply is
			// the deterministic block alone, so the grounded data still reaches the user.
			name:        "data: summary LLM down, block survives",
			req:         AgentRequest{Message: "what is the risk level of elevator 12345?"},
			agent:       dataAgent,
			payloads:    map[string]string{"get_elevator_risk": `{"elevator_found":true,"prediction_found":true,"risk_score":0.82,"risk_level":"HIGH","model_version":"v2","prediction_date":"2026-05-01"}`},
			wantName:    "data",
			wantInReply: "Risk level: HIGH",
		},
		{
			name:        "knowledge: LLM down → graceful fallback",
			req:         AgentRequest{Message: "how do I troubleshoot a stuck door?"},
			agent:       knowledgeAgent,
			payloads:    map[string]string{},
			wantName:    "knowledge",
			wantInReply: "trouble",
		},
		{
			// Missing-info path needs no MCP — isolates the LLM failure cleanly.
			name:        "scheduling: LLM down → graceful fallback",
			req:         AgentRequest{Message: "Schedule an inspection for elevator 12345", AllowedTools: []string{"schedule_inspection"}},
			agent:       schedulingAgent,
			payloads:    map[string]string{},
			wantName:    "scheduling",
			wantInReply: "trouble",
		},
		{
			name:        "general: LLM down → graceful fallback",
			req:         AgentRequest{Message: "what does TSSA stand for?"},
			agent:       generalAgent,
			payloads:    map[string]string{},
			wantName:    "general",
			wantInReply: "trouble",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mcp := newTrackingMCPServer(t, tc.payloads)
			defer mcp.Close()
			t.Setenv("MCP_SERVER_URL", mcp.URL)

			llm := newFailingLLMServer(t, http.StatusServiceUnavailable)
			defer llm.Close()
			t.Setenv("OLLAMA_BASE_URL", llm.URL)
			t.Setenv("OLLAMA_API_KEY", "test-key")

			resp := tc.agent(context.Background(), tc.req)

			if resp.AgentName != tc.wantName {
				t.Errorf("agent name: got %q, want %q", resp.AgentName, tc.wantName)
			}
			// Non-empty, safe, and no leaked transport error.
			assertSafeReply(t, resp.Reply)
			if !strings.Contains(resp.Reply, tc.wantInReply) {
				t.Errorf("reply must contain %q (graceful recovery)\n--- reply ---\n%s", tc.wantInReply, resp.Reply)
			}
		})
	}
}

// TestBuildReplyMalformedLLMOutput verifies that buildReply returns a safe
// fallback when the LLM produces a raw error string or a JSON blob, passes
// short replies through unchanged (logged only, not discarded), trims leading
// whitespace before the prefix-based checks, and does not discard markdown
// links or citations that legitimately begin with "[".
func TestBuildReplyMalformedLLMOutput(t *testing.T) {
	const safeFallback = "I'm having trouble generating a response right now. Please try again."

	cases := []struct {
		name      string
		llmReply  string
		wantReply string
	}{
		{
			name:      "raw_error",
			llmReply:  "mcp server unreachable: dial tcp 127.0.0.1:8765: connection refused",
			wantReply: safeFallback,
		},
		{
			name:      "json_object_blob",
			llmReply:  `{"elevators":[{"id":1,"status":"active"}]}`,
			wantReply: safeFallback,
		},
		{
			name:      "json_array_blob",
			llmReply:  `[{"id":1,"status":"active"},{"id":2,"status":"offline"}]`,
			wantReply: safeFallback,
		},
		{
			// Improvement: leading whitespace/newlines must not let a JSON blob
			// slip past the prefix-based checks.
			name:      "json_blob_with_leading_whitespace",
			llmReply:  "\n\n  {\"elevators\":[{\"id\":1}]}",
			wantReply: safeFallback,
		},
		{
			// Improvement: a reply that begins with "[" but is not valid JSON
			// (a markdown link) is a real answer and must pass through.
			name:      "markdown_link_not_json",
			llmReply:  "[TSSA guidance](https://example.com) covers the annual inspection requirement.",
			wantReply: "[TSSA guidance](https://example.com) covers the annual inspection requirement.",
		},
		{
			// Improvement: a citation-style reply starting with "[1]" is not
			// valid JSON and must pass through.
			name:      "citation_not_json",
			llmReply:  "[1] According to the maintenance log, the unit was serviced in March.",
			wantReply: "[1] According to the maintenance log, the unit was serviced in March.",
		},
		{
			name:      "short_reply",
			llmReply:  "OK",
			wantReply: "OK",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			llm := fakeLLMServer(t, tc.llmReply)
			defer llm.Close()
			t.Setenv("OLLAMA_BASE_URL", llm.URL)
			t.Setenv("OLLAMA_API_KEY", "test-key")

			got := buildReply(context.Background(), "You are a helpful assistant.", "", nil, "test message")
			if got != tc.wantReply {
				t.Errorf("got %q, want %q", got, tc.wantReply)
			}
		})
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
		Message:      "schedule an inspection for elevator 12345 on 2026-07-01",
		AllowedTools: []string{"schedule_inspection"},
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
	if len(args) != 1 {
		t.Fatalf("expected 1 call args entry, got %d", len(args))
	}
	if confirmed, ok := args[0]["confirmed"].(bool); !ok || confirmed {
		t.Errorf("schedule_inspection must be called with confirmed=false, got confirmed=%v (ok=%v)", args[0]["confirmed"], ok)
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
	phase2Payload := `{"success":true,"confirmed":true,"inspection_id":42,"elevator_id":12345,"location":"123 MAIN ST TORONTO ON CA","inspection_date":"2026-07-01","reason":"test","outcome":"Pending"}`

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
	if len(args) != 1 {
		t.Fatalf("expected 1 call args entry, got %d", len(args))
	}
	if confirmed, ok := args[0]["confirmed"].(bool); !ok || !confirmed {
		t.Errorf("schedule_inspection must be called with confirmed=true in Phase 2, got confirmed=%v (ok=%v)", args[0]["confirmed"], ok)
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
		Message:      "what is the risk for elevator 12345 on 2026-07-01",
		AllowedTools: []string{"schedule_inspection"},
	})

	for _, call := range mcp.Calls() {
		for _, forbidden := range forbiddenTools {
			if call == forbidden {
				t.Errorf("scheduling agent must not call forbidden tool %q", call)
			}
		}
	}
}

// TestSchedulingAgentMissingInfoAsksNotFabricates verifies that when the user's
// request has an elevator ID but no date (one required field missing), the
// scheduling agent asks for the missing information rather than calling MCP with
// fabricated values.
func TestSchedulingAgentMissingInfoAsksNotFabricates(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "Please provide the inspection date so I can complete the scheduling request.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := schedulingAgent(context.Background(), AgentRequest{
		Message:      "Schedule an inspection for elevator 12345",
		AllowedTools: []string{"schedule_inspection"},
	})

	// No MCP call must be made — fabricating a date and writing it would be wrong.
	if calls := mcp.Calls(); len(calls) != 0 {
		t.Errorf("no MCP tools must be called when date is missing, got: %v", calls)
	}
	if resp.AgentName != "scheduling" {
		t.Errorf("agent name: got %q, want %q", resp.AgentName, "scheduling")
	}
	// The agent must reply (asking for the missing info), not silently fail.
	if resp.Reply == "" {
		t.Error("reply must not be empty — agent should ask for the missing date")
	}
	// No pending action must be issued — nothing to confirm yet.
	if resp.PendingAction != nil {
		t.Error("PendingAction must be nil when required fields are missing")
	}
}

// TestSchedulingAgentLLMDownAcrossPhases closes the scheduling-specific gap in
// the LLM-down matrix: TestAgentsLLMFailureRecoverGracefully exercises only the
// missing-info branch. Here the two real scheduling branches run with the LLM
// down (HTTP 503), proving the confirmation token and the database write both
// survive an LLM outage while the user still gets a safe, populated reply.
func TestSchedulingAgentLLMDownAcrossPhases(t *testing.T) {
	t.Run("phase 1 preview — confirmation token survives the LLM outage", func(t *testing.T) {
		mcp := newTrackingMCPServer(t, map[string]string{
			"schedule_inspection": `{"pending_confirmation":true,"summary":"Elevator 12345 — ED-Periodic Inspection — 2026-07-01"}`,
		})
		defer mcp.Close()
		t.Setenv("MCP_SERVER_URL", mcp.URL)

		llm := newFailingLLMServer(t, http.StatusServiceUnavailable)
		defer llm.Close()
		t.Setenv("OLLAMA_BASE_URL", llm.URL)
		t.Setenv("OLLAMA_API_KEY", "test-key")

		resp := schedulingAgent(context.Background(), AgentRequest{
			Message:      "schedule an inspection for elevator 12345 on 2026-07-01",
			AllowedTools: []string{"schedule_inspection"},
		})

		if resp.AgentName != "scheduling" {
			t.Errorf("agent name: got %q, want %q", resp.AgentName, "scheduling")
		}
		// The preview is computed before the (failed) intro LLM call, so the signed
		// confirmation token must survive — the user can still confirm on the next
		// turn rather than being forced to restart over a transient LLM blip.
		if resp.PendingAction == nil {
			t.Error("PendingAction must survive an LLM outage during the Phase 1 preview")
		}
		// Preview only — confirmed=false, nothing written.
		args := mcp.CallArgs()
		if len(args) != 1 {
			t.Fatalf("expected exactly 1 schedule_inspection call, got %d", len(args))
		}
		if confirmed, _ := args[0]["confirmed"].(bool); confirmed {
			t.Error("Phase 1 must call schedule_inspection with confirmed=false")
		}
		// Reply is the graceful fallback — populated and free of raw errors.
		assertSafeReply(t, resp.Reply)
	})

	t.Run("phase 2 write — committed inspection is not lost when the LLM is down", func(t *testing.T) {
		mcp := newTrackingMCPServer(t, map[string]string{
			"schedule_inspection": `{"success":true,"confirmed":true,"inspection_id":42,"elevator_id":12345,"location":"123 MAIN ST TORONTO ON CA","inspection_date":"2026-07-01","reason":"test","outcome":"Pending"}`,
		})
		defer mcp.Close()
		t.Setenv("MCP_SERVER_URL", mcp.URL)

		llm := newFailingLLMServer(t, http.StatusServiceUnavailable)
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
			Message:       "yes",
			PendingAction: pa,
		})

		// The write must go through with confirmed=true even though the LLM that
		// phrases the success message is down — the outage must not silently drop a
		// committed inspection.
		calls := mcp.Calls()
		if len(calls) != 1 || calls[0] != "schedule_inspection" {
			t.Fatalf("tool calls: got %v, want [schedule_inspection]", calls)
		}
		if confirmed, _ := mcp.CallArgs()[0]["confirmed"].(bool); !confirmed {
			t.Error("Phase 2 must call schedule_inspection with confirmed=true even when the LLM is down")
		}
		// Write done → pending state cleared.
		if resp.PendingAction != nil {
			t.Error("PendingAction must be nil after the Phase 2 write")
		}
		assertSafeReply(t, resp.Reply)
	})
}

// assertNoConfirmedWrite is the write-gate invariant: schedule_inspection must
// never have been called with confirmed=true. confirmed=true is the only call
// shape that writes to the database (Phase 2), so this is the single check that
// proves no unintended write occurred — regardless of how many confirmed=false
// previews ran.
func assertNoConfirmedWrite(t *testing.T, mcp *trackingMCPServer) {
	t.Helper()
	args := mcp.CallArgs()
	for i, call := range mcp.Calls() {
		if call != "schedule_inspection" {
			continue
		}
		if confirmed, _ := args[i]["confirmed"].(bool); confirmed {
			t.Errorf("write-gate violated: schedule_inspection called with confirmed=true on a failure path (call #%d)", i)
		}
	}
}

// TestSchedulingFailuresNeverWriteToDatabase confirms the core safety property:
// no failure or invalid input during scheduling ever produces a database write.
// A write happens only when schedule_inspection is called with confirmed=true,
// so every case asserts that never happens. The unit tests in chat_test.go prove
// verifyPendingAction / detectConfirmation return the right verdicts; this test
// proves the agent *acts* on those verdicts by refusing the write.
//
// Each case uses a LIVE tracking MCP server (not a closed one) so that any
// erroneous write attempt would be recorded and caught — a silent failure cannot
// hide the bug.
func TestSchedulingFailuresNeverWriteToDatabase(t *testing.T) {
	// A schedule_inspection payload that, if the agent wrongly reached the write,
	// would look like a successful write — making an accidental confirmed=true call
	// observable rather than masked by an empty default.
	writeLikePayload := map[string]string{
		"schedule_inspection": `{"success":true,"confirmed":true,"inspection_id":99,"elevator_id":12345,"location":"123 MAIN ST TORONTO ON CA","inspection_date":"2026-07-01","reason":"test","outcome":"Pending"}`,
	}

	t.Run("tampered signature on confirmation is rejected without a write", func(t *testing.T) {
		mcp := newTrackingMCPServer(t, writeLikePayload)
		defer mcp.Close()
		t.Setenv("MCP_SERVER_URL", mcp.URL)
		llm := fakeLLMServer(t, "That request could not be verified. Please start the scheduling request again.")
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
		pa.ElevatorID = 99999 // tamper AFTER signing — signature no longer matches

		resp := schedulingAgent(context.Background(), AgentRequest{Message: "yes", PendingAction: pa})

		assertNoConfirmedWrite(t, mcp)
		// The reject branch sets the notice directly and makes no MCP call at all.
		if calls := mcp.Calls(); len(calls) != 0 {
			t.Errorf("a tampered confirmation must make no MCP call, got %v", calls)
		}
		if resp.PendingAction != nil {
			t.Error("PendingAction must be nil after rejecting a tampered confirmation")
		}
		assertSafeReply(t, resp.Reply)
	})

	t.Run("expired confirmation is rejected without a write", func(t *testing.T) {
		mcp := newTrackingMCPServer(t, writeLikePayload)
		defer mcp.Close()
		t.Setenv("MCP_SERVER_URL", mcp.URL)
		llm := fakeLLMServer(t, "This confirmation has expired. Please start the scheduling request again.")
		defer llm.Close()
		t.Setenv("OLLAMA_BASE_URL", llm.URL)
		t.Setenv("OLLAMA_API_KEY", "test-key")

		// Signed correctly (so verifyPendingAction passes) but already expired, so
		// the agent's expiry check must reject before reaching the write.
		pa := &PendingAction{
			ElevatorID:     12345,
			InspectionDate: "2026-07-01",
			InspectionType: "Periodic",
			Reason:         "schedule an inspection for elevator 12345 on 2026-07-01",
			ExpiresAt:      time.Now().Add(-1 * time.Minute).Unix(),
		}
		pa.Signature = signPendingAction(pa)

		resp := schedulingAgent(context.Background(), AgentRequest{Message: "yes", PendingAction: pa})

		assertNoConfirmedWrite(t, mcp)
		if calls := mcp.Calls(); len(calls) != 0 {
			t.Errorf("an expired confirmation must make no MCP call, got %v", calls)
		}
		if resp.PendingAction != nil {
			t.Error("PendingAction must be nil after rejecting an expired confirmation")
		}
		assertSafeReply(t, resp.Reply)
	})

	t.Run("ambiguous yes+cancel defaults to no write", func(t *testing.T) {
		mcp := newTrackingMCPServer(t, writeLikePayload)
		defer mcp.Close()
		t.Setenv("MCP_SERVER_URL", mcp.URL)
		llm := fakeLLMServer(t, "Cancelled. No inspection was booked.")
		defer llm.Close()
		t.Setenv("OLLAMA_BASE_URL", llm.URL)
		t.Setenv("OLLAMA_API_KEY", "test-key")

		// A perfectly valid, signed, unexpired action — so the ONLY thing stopping a
		// write is the ambiguity guard (isConfirm && isCancel → isConfirm=false).
		pa := &PendingAction{
			ElevatorID:     12345,
			InspectionDate: "2026-07-01",
			InspectionType: "Periodic",
			Reason:         "schedule an inspection for elevator 12345 on 2026-07-01",
			ExpiresAt:      time.Now().Add(10 * time.Minute).Unix(),
		}
		pa.Signature = signPendingAction(pa)

		resp := schedulingAgent(context.Background(), AgentRequest{Message: "yes cancel", PendingAction: pa})

		assertNoConfirmedWrite(t, mcp)
		if calls := mcp.Calls(); len(calls) != 0 {
			t.Errorf("an ambiguous confirmation must make no MCP call, got %v", calls)
		}
		if resp.PendingAction != nil {
			t.Error("PendingAction must be nil after an ambiguous confirmation defaults to cancel")
		}
		assertSafeReply(t, resp.Reply)
	})

	t.Run("MCP unreachable during preview never escalates to a write", func(t *testing.T) {
		mcp := newTrackingMCPServer(t, writeLikePayload)
		mcp.Close() // down before the agent runs
		t.Setenv("MCP_SERVER_URL", mcp.URL)
		llm := fakeLLMServer(t, "Live fleet data is unavailable right now; please try again shortly.")
		defer llm.Close()
		t.Setenv("OLLAMA_BASE_URL", llm.URL)
		t.Setenv("OLLAMA_API_KEY", "test-key")

		// Valid Phase 1 request (ID + date). The preview call fails because MCP is
		// down — it must not fall through to a write, and no pending action is issued.
		resp := schedulingAgent(context.Background(), AgentRequest{
			Message:      "schedule an inspection for elevator 12345 on 2026-07-01",
			AllowedTools: []string{"schedule_inspection"},
		})

		assertNoConfirmedWrite(t, mcp)
		if resp.PendingAction != nil {
			t.Error("PendingAction must be nil when the preview call failed")
		}
		assertSafeReply(t, resp.Reply)
	})

	t.Run("tool validation error during preview never escalates to a write", func(t *testing.T) {
		mcp := newTrackingMCPServer(t, map[string]string{
			"schedule_inspection": `{"success":false,"error":"Elevator 99999 does not exist in the fleet."}`,
		})
		defer mcp.Close()
		t.Setenv("MCP_SERVER_URL", mcp.URL)
		llm := fakeLLMServer(t, "That elevator does not exist; please check the number and try again.")
		defer llm.Close()
		t.Setenv("OLLAMA_BASE_URL", llm.URL)
		t.Setenv("OLLAMA_API_KEY", "test-key")

		resp := schedulingAgent(context.Background(), AgentRequest{
			Message:      "schedule an inspection for elevator 99999 on 2026-07-01",
			AllowedTools: []string{"schedule_inspection"},
		})

		assertNoConfirmedWrite(t, mcp)
		// The preview ran (confirmed=false) but returned a validation error → no write,
		// no pending action.
		if resp.PendingAction != nil {
			t.Error("PendingAction must be nil after a validation error")
		}
		assertSafeReply(t, resp.Reply)
	})
}

// TestSchedulingAgentPhase2MCPFailurePreservesPendingAction verifies that when
// the Phase 2 MCP call fails (transport error or application-level error), the
// original PendingAction is returned so the user can retry "yes" without
// restarting the whole scheduling flow. Before this fix, pendingAction was nil
// on any Phase 2 failure, leaving the user stuck with no way to retry.
func TestSchedulingAgentPhase2MCPFailurePreservesPendingAction(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]string
	}{
		{
			name:    "transport failure (MCP unreachable)",
			payload: nil, // mcp will be closed before the call
		},
		{
			name:    "application error (elevator does not exist)",
			payload: map[string]string{"schedule_inspection": `{"success":false,"error":"Elevator 99999 does not exist."}`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mcp *trackingMCPServer
			if tc.payload == nil {
				mcp = newTrackingMCPServer(t, map[string]string{})
				mcp.Close() // down before the call → transport error
			} else {
				mcp = newTrackingMCPServer(t, tc.payload)
				defer mcp.Close()
			}
			t.Setenv("MCP_SERVER_URL", mcp.URL)

			llm := fakeLLMServer(t, "The scheduling service is unavailable. Please try again.")
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
				Message:       "yes",
				PendingAction: pa,
			})

			if resp.AgentName != "scheduling" {
				t.Errorf("agent name: got %q, want %q", resp.AgentName, "scheduling")
			}
			// Write failed — the original pending action must be returned so the
			// user can retry without restarting the scheduling flow from scratch.
			if resp.PendingAction == nil {
				t.Fatal("PendingAction must be preserved when Phase 2 MCP call fails")
			}
			if resp.PendingAction.ElevatorID != pa.ElevatorID {
				t.Errorf("preserved PendingAction.ElevatorID: got %d, want %d", resp.PendingAction.ElevatorID, pa.ElevatorID)
			}
			assertSafeReply(t, resp.Reply)
		})
	}
}

// TestSchedulingAgentPhase2SuccessLLMDownUsesGroundedConfirmation verifies that
// when the Phase 2 MCP write succeeds (inspection committed to the database) but
// the LLM is down, the agent returns the grounded confirmation built from the MCP
// result rather than the generic "I'm having trouble" fallback. Without this, the
// user does not know whether their inspection was actually booked and may try to
// schedule again — risking a duplicate.
func TestSchedulingAgentPhase2SuccessLLMDownUsesGroundedConfirmation(t *testing.T) {
	// Payload matches the real write_tools.py Phase 2 response — no "message" field.
	mcp := newTrackingMCPServer(t, map[string]string{
		"schedule_inspection": `{"success":true,"confirmed":true,"inspection_id":42,"elevator_id":12345,"location":"123 MAIN ST TORONTO ON CA","inspection_date":"2026-07-01","reason":"test","outcome":"Pending"}`,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	// LLM is down — buildReply will return the "I'm having trouble" fallback.
	llm := newFailingLLMServer(t, http.StatusServiceUnavailable)
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
		Message:       "yes",
		PendingAction: pa,
	})

	if resp.AgentName != "scheduling" {
		t.Errorf("agent name: got %q, want %q", resp.AgentName, "scheduling")
	}
	// Write is done — no pending action needed.
	if resp.PendingAction != nil {
		t.Error("PendingAction must be nil after a successful Phase 2 write")
	}
	// The reply must NOT be the generic LLM fallback — the user must know
	// the inspection was booked.
	if strings.HasPrefix(resp.Reply, "I'm having trouble") {
		t.Errorf("reply must not be the generic LLM fallback when Phase 2 write succeeded\n--- reply ---\n%s", resp.Reply)
	}
	// The grounded confirmation must include the inspection ID from the MCP result.
	if !strings.Contains(resp.Reply, "42") {
		t.Errorf("reply must include the inspection ID from the MCP result\n--- reply ---\n%s", resp.Reply)
	}
	assertSafeReply(t, resp.Reply)
}

// TestSchedulingAgentInspectionTypeResponseReroutesThroughPhase1 covers the bug
// where a user supplies the inspection type in reply to the agent's question
// (e.g. "Periodic") while a pending action with InspectionType="" is active.
// Before the fix, detectConfirmation("Periodic") returned false/false, which
// triggered the default re-dispatch branch and sent the message through Route(),
// where ClassifyIntent("Periodic") scored it as advisory → the general agent
// refused scheduling entirely. The fix re-runs Phase 1 with the supplied type
// and rebuilds the pending action so the user sees a fresh confirmation summary
// that includes the inspection type.
func TestSchedulingAgentInspectionTypeResponseReroutesThroughPhase1(t *testing.T) {
	phase1WithType := `{"pending_confirmation":true,"summary":"Elevator 12345 — ED-Periodic Inspection — 2026-07-01"}`

	mcp := newTrackingMCPServer(t, map[string]string{
		"schedule_inspection": phase1WithType,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "I'll schedule a Periodic inspection for elevator 12345 on 2026-07-01. Would you like to confirm?")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	// Pending action with InspectionType="" — the state produced by Phase 1 when
	// the user did not include the inspection type in the original request.
	pa := &PendingAction{
		ElevatorID:     12345,
		InspectionDate: "2026-07-01",
		InspectionType: "", // missing — this is the bug trigger
		Reason:         "schedule an inspection for elevator 12345 on 2026-07-01",
		ExpiresAt:      time.Now().Add(10 * time.Minute).Unix(),
	}
	pa.Signature = signPendingAction(pa)

	resp := schedulingAgent(context.Background(), AgentRequest{
		Message:       "Periodic",
		PendingAction: pa,
	})

	if resp.AgentName != "scheduling" {
		t.Errorf("agent name: got %q, want %q", resp.AgentName, "scheduling")
	}

	// Phase 1 must have been re-run exactly once with the supplied type.
	calls := mcp.Calls()
	if len(calls) != 1 || calls[0] != "schedule_inspection" {
		t.Errorf("tool calls: got %v, want [schedule_inspection]", calls)
	}

	args := mcp.CallArgs()
	if len(args) != 1 {
		t.Fatalf("expected 1 call args entry, got %d", len(args))
	}
	// Must be a Phase 1 preview (confirmed=false) — not a write.
	if confirmed, ok := args[0]["confirmed"].(bool); !ok || confirmed {
		t.Errorf("re-run must use confirmed=false, got confirmed=%v (ok=%v)", args[0]["confirmed"], ok)
	}
	// All three carry-over fields from the pending action must be forwarded so
	// the MCP tool can re-validate the full inspection record (not just the type).
	if got, _ := args[0]["inspection_type"].(string); got != "Periodic" {
		t.Errorf("re-run must forward inspection_type=%q, got %q", "Periodic", got)
	}
	if got, _ := args[0]["elevator_id"].(float64); int(got) != 12345 {
		t.Errorf("re-run must forward elevator_id=12345, got %v", args[0]["elevator_id"])
	}
	if got, _ := args[0]["inspection_date"].(string); got != "2026-07-01" {
		t.Errorf("re-run must forward inspection_date=%q, got %q", "2026-07-01", got)
	}
	if got, _ := args[0]["reason"].(string); got == "" {
		t.Error("re-run must forward a non-empty reason")
	}

	// A fresh pending action must be returned so the user can confirm on the next turn.
	if resp.PendingAction == nil {
		t.Fatal("PendingAction must be non-nil after re-running Phase 1 with the supplied type")
	}
	if resp.PendingAction.InspectionType != "Periodic" {
		t.Errorf("PendingAction.InspectionType: got %q, want %q", resp.PendingAction.InspectionType, "Periodic")
	}
	if resp.PendingAction.Signature == "" {
		t.Error("fresh PendingAction must carry a new HMAC signature")
	}

	// The reply must be safe — no raw errors or JSON blobs.
	assertSafeReply(t, resp.Reply)
}

// ── General agent ─────────────────────────────────────────────────────────────

// TestGeneralAgentNeverCallsMCPTools verifies that the general agent makes zero
// MCP tool calls — it is purely advisory and must not hit the MCP server.
func TestGeneralAgentNeverCallsMCPTools(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "TSSA stands for Technical Standards and Safety Authority.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	generalAgent(context.Background(), AgentRequest{
		Message: "what does TSSA stand for?",
	})

	if calls := mcp.Calls(); len(calls) != 0 {
		t.Errorf("general agent must make zero MCP calls, got %d: %v", len(calls), calls)
	}
}

// TestGeneralAgentTerminologyQuestion verifies a terminology question returns a
// non-empty reply with no MCP tool calls.
func TestGeneralAgentTerminologyQuestion(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "A periodic inspection is the standard annual inspection required by TSSA.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := generalAgent(context.Background(), AgentRequest{
		Message: "what is a periodic inspection?",
	})

	if resp.AgentName != "general" {
		t.Errorf("agent name: got %q, want %q", resp.AgentName, "general")
	}
	if resp.Reply == "" {
		t.Error("reply must not be empty")
	}
	if calls := mcp.Calls(); len(calls) != 0 {
		t.Errorf("general agent must make zero MCP calls, got %d: %v", len(calls), calls)
	}
}

// TestGeneralAgentGeneralChat verifies conversational messages are answered
// without any MCP tool calls.
func TestGeneralAgentGeneralChat(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "Hello! I can help you with elevator fleet operations questions.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := generalAgent(context.Background(), AgentRequest{
		Message: "hello",
	})

	if resp.Reply == "" {
		t.Error("reply must not be empty")
	}
	if calls := mcp.Calls(); len(calls) != 0 {
		t.Errorf("general agent must make zero MCP calls, got %d: %v", len(calls), calls)
	}
}

// TestGeneralAgentNoFallbackOnSubstantiveMessage verifies that even a message
// long enough to have triggered the old shouldTryRagFallback gate (>= 4 words,
// procedural content) still results in zero MCP calls after S3-6.
func TestGeneralAgentNoFallbackOnSubstantiveMessage(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	llm := fakeLLMServer(t, "Hydraulic pressure loss can indicate a seal failure.")
	defer llm.Close()
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	// This message would have triggered shouldTryRagFallback (>= 4 words, no ?).
	generalAgent(context.Background(), AgentRequest{
		Message: "the hydraulic car will not level",
	})

	if calls := mcp.Calls(); len(calls) != 0 {
		t.Errorf("general agent must not fall back to MCP after S3-6, got %d calls: %v", len(calls), calls)
	}
}
