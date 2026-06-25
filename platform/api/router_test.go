package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestRouteSelectsCorrectAgent verifies that Route dispatches each query class
// to the expected agent. Tests go through the real Route() entry point — not by
// calling agents directly — and assert only on AgentResponse.AgentName.
//
// Pre-emption rows (pendingAction != nil) verify that a request carrying a
// PendingAction goes straight to the scheduling agent regardless of what the
// message text would otherwise classify as.
func TestRouteSelectsCorrectAgent(t *testing.T) {
	// Shared PendingAction for the pre-emption rows. Both rows use the same
	// signed PA so we only call signPendingAction once.
	pa := &PendingAction{
		ElevatorID:     12345,
		InspectionDate: "2026-07-15",
		InspectionType: "Periodic",
		Reason:         "schedule an inspection for elevator 12345 on 2026-07-15",
		ExpiresAt:      time.Now().Add(10 * time.Minute).Unix(),
	}
	pa.Signature = signPendingAction(pa)

	cases := []struct {
		name          string
		message       string
		pendingAction *PendingAction
		wantAgent     string
		mcpPayloads   map[string]string
	}{
		// ── Data agent ────────────────────────────────────────────────────────
		// IntentDataQuery → mcp_data_tool → dataAgent
		{
			// "which" (0.5) + "shut down" (1.0) + "tssa" (1.0) = 2.5 → 0.71
			name:      "data: TSSA shutdown query",
			message:   "Which elevators are shut down by TSSA?",
			wantAgent: "data",
			mcpPayloads: map[string]string{
				"get_tssa_shutdown_elevators": `{"count":0,"source":"inspections table","note":"","elevators":[]}`,
			},
		},
		{
			// "how many" (0.5) + "incident" (1.0) = 1.5 → 0.60
			name:      "data: incident count query",
			message:   "How many incidents were reported last year?",
			wantAgent: "data",
			mcpPayloads: map[string]string{
				"get_incident_count_last_year": `{"source":"incidents table","total_incidents":5,"fatal_incidents":0,"injury_incidents":2,"year_queried":2025}`,
			},
		},
		{
			// "risk level" (1.0) + elevator-ID entity bonus (0.5) = 1.5 → 0.60
			name:      "data: per-elevator risk query",
			message:   "What is the risk level for elevator 12345?",
			wantAgent: "data",
			mcpPayloads: map[string]string{
				"get_elevator_risk": `{"elevator_found":true,"prediction_found":true,"risk_score":0.82,"risk_level":"HIGH","model_version":"v2","prediction_date":"2026-05-01"}`,
			},
		},
		{
			// "which" (0.5) + "follow-up" (1.0) = 1.5 → 0.60
			name:      "data: follow-up elevators query",
			message:   "Which elevators need follow-up inspections?",
			wantAgent: "data",
			mcpPayloads: map[string]string{
				"get_elevators_needing_followup": `{"count":0,"source":"inspections table","elevators":[]}`,
			},
		},

		// ── Knowledge agent ───────────────────────────────────────────────────
		// IntentRAG → rag_search → knowledgeAgent
		{
			// "steps to" (1.5) = 1.5 → 0.60 — maintenance corpus
			name:        "knowledge: procedural how-to query",
			message:     "What are the steps to replace a governor?",
			wantAgent:   "knowledge",
			mcpPayloads: map[string]string{},
		},
		{
			// "how do i" (1.5) + "troubleshoot" (1.5) = 3.0 → 0.75 — maintenance corpus
			name:        "knowledge: troubleshooting query",
			message:     "How do I troubleshoot a stuck door?",
			wantAgent:   "knowledge",
			mcpPayloads: map[string]string{},
		},
		{
			// "have we seen" (1.5) + "incident" DATA_QUERY cue (1.0) — RAG wins 1.5 vs 1.0
			// experiential cue outscores the plain incident keyword; must not fall to data agent.
			name:        "knowledge: incident recurrence query (experiential cue outscores data keyword)",
			message:     "Have we seen flooding incidents before?",
			wantAgent:   "knowledge",
			mcpPayloads: map[string]string{},
		},
		{
			// "have we had" (1.5) + "similar incident" (1.5) = 3.0 → 0.75 — incident corpus
			name:        "knowledge: similar-incident query",
			message:     "Have we had similar incidents with the doors?",
			wantAgent:   "knowledge",
			mcpPayloads: map[string]string{},
		},

		// ── Scheduling agent — normal path ────────────────────────────────────
		// IntentAction → action_executor → schedulingAgent (Phase 1 preview)
		{
			// "schedule" (1.0) + ID bonus (0.5) + date bonus (0.5) + action+entity bonus (1.0) = 3.0 → 0.75
			name:      "scheduling: schedule inspection request",
			message:   "Schedule an inspection for elevator 12345 on July 15",
			wantAgent: "scheduling",
			mcpPayloads: map[string]string{
				"schedule_inspection": `{"pending_confirmation":true,"summary":"Elevator 12345 — ED-Periodic Inspection — 2026-07-15"}`,
			},
		},
		{
			// "book" (1.0) + ID bonus (0.5) + date bonus (0.5) + action+entity bonus (1.0) = 3.0 → 0.75
			name:      "scheduling: book inspection request",
			message:   "Book a periodic inspection for elevator 60503 on 2026-08-01",
			wantAgent: "scheduling",
			mcpPayloads: map[string]string{
				"schedule_inspection": `{"pending_confirmation":true,"summary":"Elevator 60503 — ED-Periodic Inspection — 2026-08-01"}`,
			},
		},

		// ── Scheduling agent — pre-emption path ──────────────────────────────
		// A request carrying a PendingAction must go straight to the scheduling
		// agent without reclassification, regardless of the message content.
		// Neither "cancel" nor "yes" contains any action keyword — both would
		// classify as advisory (general agent) if evaluated by ClassifyIntent.
		// Route must bypass classification entirely when PendingAction != nil.
		{
			// User cancels the pending confirmation — no MCP call, no write.
			name:          "scheduling: pre-emption — cancel bypasses classification",
			message:       "cancel",
			pendingAction: pa,
			wantAgent:     "scheduling",
			mcpPayloads:   map[string]string{},
		},
		{
			// User confirms the pending action — Phase 2 write via schedule_inspection.
			name:          "scheduling: pre-emption — yes bypasses classification",
			message:       "yes",
			pendingAction: pa,
			wantAgent:     "scheduling",
			mcpPayloads: map[string]string{
				"schedule_inspection": `{"success":true,"inspection_id":42,"message":"Inspection scheduled successfully."}`,
			},
		},

		// ── General agent ─────────────────────────────────────────────────────
		// IntentAdvisory → advisory → generalAgent (no MCP calls).
		//
		// The design relies on the confidence floor (0.60) as the sole gate for
		// the fallback path: any message whose best intent score produces
		// confidence < 0.60 lands here, regardless of whether keywords matched.
		// The rows below cover three distinct ways a message can fall through.
		{
			// Pure advisory content — no keyword matches at all → score 0 → confidence 0.
			name:        "general: domain terminology query",
			message:     "What is a hydraulic elevator?",
			wantAgent:   "general",
			mcpPayloads: nil,
		},
		{
			// Pure advisory content — no keyword matches → score 0 → confidence 0.
			name:        "general: open-ended advisory query",
			message:     "Tell me about elevator safety regulations",
			wantAgent:   "general",
			mcpPayloads: nil,
		},
		{
			// Single medium keyword hit: "tssa" (DataQuery 1.0) → confidence 1.0/2.0 = 0.50 < floor.
			// A keyword match alone is not enough — the score must clear 1.5 to reach 0.60.
			name:        "general: medium keyword hit below floor",
			message:     "What does TSSA stand for?",
			wantAgent:   "general",
			mcpPayloads: nil,
		},
		{
			// Single weak keyword hit: "list" (DataQuery 0.5) → confidence 0.5/1.5 = 0.33 < floor.
			// Even a clearly relevant keyword falls through when its weight is too low.
			name:        "general: single weak keyword below floor",
			message:     "List all elevators please",
			wantAgent:   "general",
			mcpPayloads: nil,
		},
		{
			// Cross-intent ambiguity: "list" (DataQuery 0.5) + "replace" (RAG 0.5).
			// Both intents score equally at 0.5; DataQuery wins the tie by precedence,
			// but confidence = 0.5/1.5 = 0.33 < floor — competing signals cancel out.
			name:        "general: ambiguous cross-intent message below floor",
			message:     "List what I need to replace",
			wantAgent:   "general",
			mcpPayloads: nil,
		},
		{
			// Vague follow-on with no context: "list" (DataQuery 0.5) → 0.33 < floor.
			// Mirrors TestConfidenceFallbackToAdvisory in intent_test.go but exercises
			// the same path through Route().
			name:        "general: vague follow-on message",
			message:     "please list those",
			wantAgent:   "general",
			mcpPayloads: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mcp := newTrackingMCPServer(t, tc.mcpPayloads)
			defer mcp.Close()
			t.Setenv("MCP_SERVER_URL", mcp.URL)

			llm := fakeLLMServer(t, "test reply")
			defer llm.Close()
			t.Setenv("OLLAMA_BASE_URL", llm.URL)
			t.Setenv("OLLAMA_API_KEY", "test-key")

			resp := Route(context.Background(), AgentRequest{
				Message:       tc.message,
				PendingAction: tc.pendingAction,
			})

			if resp.AgentName != tc.wantAgent {
				t.Errorf("Route(%q) agentName = %q, want %q", tc.message, resp.AgentName, tc.wantAgent)
			}
		})
	}
}

// TestMultiDomainQueriesRoutedDeterministically verifies that a message whose
// text contains keywords from more than one intent class is always routed to
// the single correct agent (the highest-scoring intent wins), returns a
// non-empty reply, and — crucially — never calls a tool that belongs to a
// different domain. The third assertion is the unique value here: classification
// and tool-selection are already proven at the unit level; this test confirms
// the winning agent's scope guard holds in the full Route() pipeline.
//
// Score workings are in the row comments. Floor = 0.60; confidence = score/(score+1).
func TestMultiDomainQueriesRoutedDeterministically(t *testing.T) {
	cases := []struct {
		name           string
		message        string
		wantAgent      string
		wantToolCalled string   // must appear in mcp.Calls() (first call)
		forbiddenTools []string // must NOT appear in mcp.Calls() under any circumstance
		mcpPayloads    map[string]string
	}{
		{
			// RAG wins: "procedure" (RAG 1.5) > "incident" (DataQuery 1.0).
			// Confidence = 1.5/2.5 = 0.60. The DataQuery "incident" signal must not
			// cause a data tool (e.g. get_incident_count_last_year) to fire.
			name:           "rag beats data: procedure + incident keyword",
			message:        "What's the procedure for reporting an incident?",
			wantAgent:      "knowledge",
			wantToolCalled: "search_maintenance_docs",
			forbiddenTools: []string{
				"get_incident_count_last_year", "get_elevator_incidents",
				"get_fleet_stats", "get_tssa_shutdown_elevators",
			},
			mcpPayloads: map[string]string{
				// Return a confident result so the fallback corpus is not tried.
				"search_maintenance_docs": `{"total_returned":1,"results":[{"text":"File a report within 24 hours.","source_name":"Maintenance Document 42","similarity_score":0.88}]}`,
			},
		},
		{
			// DataQuery wins: "tssa" (1.0) + "shut down" (1.0) + "which" (0.5) = 2.5
			// vs RAG "maintenance" (0.5). Confidence = 2.5/3.5 = 0.71.
			// The RAG "maintenance" signal must not cause search_maintenance_docs to fire.
			name:           "data beats rag: shutdown keywords + maintenance keyword",
			message:        "Which TSSA shutdown elevators need maintenance?",
			wantAgent:      "data",
			wantToolCalled: "get_tssa_shutdown_elevators",
			forbiddenTools: []string{"search_maintenance_docs", "search_incident_narratives"},
			mcpPayloads: map[string]string{
				"get_tssa_shutdown_elevators": `{"count":0,"source":"inspections table","note":"","elevators":[]}`,
			},
		},
		{
			// TRUE TIE broken by precedence:
			//   "how do i"        → RAG       1.5
			//   "inspection history" → DataQuery 1.0
			//   elevator-ID bonus → DataQuery +0.5
			// Both intents score exactly 1.5. tieBreakOrder=[Action, DataQuery, RAG]
			// and the loop is strict (>), so DataQuery wins the tie over RAG.
			// Confidence = 1.5/2.5 = 0.60. This is the only case where the fixed
			// precedence rule alone decides the winner — the result must be the data
			// agent, not the knowledge agent, and search_* must never be called.
			name:           "data ties rag at 1.5, precedence decides: how-do-i + inspection history + elevator ID",
			message:        "How do I check the inspection history for elevator 12345?",
			wantAgent:      "data",
			wantToolCalled: "get_inspection_history",
			forbiddenTools: []string{"search_maintenance_docs", "search_incident_narratives"},
			mcpPayloads: map[string]string{
				"get_inspection_history": `{"found":true,"elevator_id":12345,"total_returned":1,"source":"inspections table","inspections":[{"inspection_type":"ED-Periodic Inspection","latest_inspection_date":"2025-03-20","outcome":"Passed"}]}`,
			},
		},
		{
			// Action wins: "schedule" (1.0) + ID bonus (0.5) + date bonus (0.5)
			// + verb-and-entity bonus (1.0) = 3.0 vs RAG "procedure" (1.5).
			// Confidence = 3.0/4.0 = 0.75. The RAG "procedure" signal must not
			// cause search_maintenance_docs to fire.
			name:           "action beats rag: schedule verb + procedure keyword",
			message:        "What's the procedure to schedule an inspection for elevator 12345 on 2026-07-15?",
			wantAgent:      "scheduling",
			wantToolCalled: "schedule_inspection",
			forbiddenTools: []string{"search_maintenance_docs", "search_incident_narratives"},
			mcpPayloads: map[string]string{
				"schedule_inspection": `{"pending_confirmation":true,"summary":"Elevator 12345 — ED-Periodic Inspection — 2026-07-15"}`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mcp := newTrackingMCPServer(t, tc.mcpPayloads)
			defer mcp.Close()
			t.Setenv("MCP_SERVER_URL", mcp.URL)

			llm := fakeLLMServer(t, "Here is what I found.")
			defer llm.Close()
			t.Setenv("OLLAMA_BASE_URL", llm.URL)
			t.Setenv("OLLAMA_API_KEY", "test-key")

			resp := Route(context.Background(), AgentRequest{Message: tc.message})

			if resp.AgentName != tc.wantAgent {
				t.Errorf("AgentName = %q, want %q", resp.AgentName, tc.wantAgent)
			}
			// Non-empty and free of leaked raw errors or JSON.
			assertSafeReply(t, resp.Reply)
			calls := mcp.Calls()
			if len(calls) == 0 || calls[0] != tc.wantToolCalled {
				t.Errorf("first MCP call = %v, want [%s]", calls, tc.wantToolCalled)
			}
			for _, call := range calls {
				for _, forbidden := range tc.forbiddenTools {
					if call == forbidden {
						t.Errorf("cross-domain tool %q must not be called for a %s-winning message", forbidden, tc.wantAgent)
					}
				}
			}
			// Determinism: same message, same result.
			resp2 := Route(context.Background(), AgentRequest{Message: tc.message})
			if resp2.AgentName != resp.AgentName {
				t.Errorf("Route is not deterministic: first=%q second=%q", resp.AgentName, resp2.AgentName)
			}
		})
	}
}

// TestMultiDomainSignalTraceRecordsAllKeywords verifies that ClassifyIntent's
// Signals trace captures every keyword that fired — including the losing-domain
// keywords — so callers can audit why a multi-domain message was resolved the
// way it was. This is the classification-layer complement to the Route()-level
// test above.
func TestMultiDomainSignalTraceRecordsAllKeywords(t *testing.T) {
	cases := []struct {
		name         string
		message      string
		wantIntent   Intent
		mustContain  []string // keyword strings that must appear in Signals
	}{
		{
			name:        "procedure + incident: RAG wins, incident signal still recorded",
			message:     "What's the procedure for reporting an incident?",
			wantIntent:  IntentRAG,
			mustContain: []string{"procedure", "incident"},
		},
		{
			// "shutdown" (one word) matches the "shutdown" keyword, not "shut down".
			name:        "shutdown + maintenance: DataQuery wins, maintenance signal still recorded",
			message:     "Which TSSA shutdown elevators need maintenance?",
			wantIntent:  IntentDataQuery,
			mustContain: []string{"shutdown", "tssa", "which", "maintenance"},
		},
		{
			// True tie: "how do i" (RAG 1.5) and "inspection history" (DataQuery 1.0)
			// + ID bonus (DataQuery +0.5) both reach 1.5. The trace must record both
			// keywords so the precedence decision — DataQuery beats RAG in the
			// tieBreakOrder — is auditable after the fact.
			name:        "how-do-i + inspection history: true 1.5 tie, both signals recorded",
			message:     "How do I check the inspection history for elevator 12345?",
			wantIntent:  IntentDataQuery,
			mustContain: []string{"how do i", "inspection history"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyIntent(tc.message, fixedNow)
			if got.Intent != tc.wantIntent {
				t.Fatalf("intent = %q, want %q (reason: %s)", got.Intent, tc.wantIntent, got.Reason)
			}
			signalKeywords := make([]string, len(got.Signals))
			for i, s := range got.Signals {
				signalKeywords[i] = s.Keyword
			}
			joined := strings.Join(signalKeywords, " | ")
			for _, kw := range tc.mustContain {
				found := false
				for _, s := range got.Signals {
					if s.Keyword == kw {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Signals missing keyword %q — signals: %s", kw, joined)
				}
			}
		})
	}
}

// TestAmbiguousQueriesHandledGracefully verifies the full Route() → generalAgent()
// pipeline for low-signal messages: correct agent, non-empty reply, zero MCP calls.
//
// Three distinct mechanisms drive messages below the confidence floor (0.60):
//
//  1. Zero signal — no keyword matches any intent; score=0, confidence=0.
//  2. Single keyword below floor — one match, but not enough weight to clear 0.60.
//  3. Competing signals tie — two intents score equally; DataQuery wins by
//     precedence but the score is still too low (confidence < 0.60).
//
// TestRouteSelectsCorrectAgent already verifies AgentName for these messages.
// This test adds the graceful-handling dimension: non-empty reply and no
// MCP tool calls, which the routing table does not assert.
func TestAmbiguousQueriesHandledGracefully(t *testing.T) {
	cases := []struct {
		name    string
		message string
	}{
		{
			// score=0, confidence=0 — pure advisory, no keyword match
			name:    "zero signal: no keyword match",
			message: "What is a hydraulic elevator?",
		},
		{
			// "tssa" (DataQuery 1.0) → confidence=1.0/2.0=0.50 < floor (0.60)
			// A single named-entity keyword is not enough on its own.
			name:    "single keyword below floor",
			message: "What does TSSA stand for?",
		},
		{
			// "list" (DataQuery 0.5) + "replace" (RAG 0.5) → tie at 0.5
			// DataQuery wins by precedence; confidence=0.5/1.5=0.33 < floor.
			name:    "competing signals cancel below floor",
			message: "List what I need to replace",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mcp := newTrackingMCPServer(t, nil)
			defer mcp.Close()
			t.Setenv("MCP_SERVER_URL", mcp.URL)

			llm := fakeLLMServer(t, "I can help you with that question.")
			defer llm.Close()
			t.Setenv("OLLAMA_BASE_URL", llm.URL)
			t.Setenv("OLLAMA_API_KEY", "test-key")

			resp := Route(context.Background(), AgentRequest{Message: tc.message})

			if resp.AgentName != "general" {
				t.Errorf("AgentName = %q, want %q", resp.AgentName, "general")
			}
			// Ambiguous queries must get a graceful, safe response — non-empty and
			// free of leaked raw errors or JSON.
			assertSafeReply(t, resp.Reply)
			if calls := mcp.Calls(); len(calls) != 0 {
				t.Errorf("no MCP tools must be called for an ambiguous query, got: %v", calls)
			}
		})
	}
}

// assertErrorContract asserts the design's §4.3 error contract on a response
// produced through Route() under a failure: the Reply is populated and
// plain-language (never a raw infrastructure error or an unparsed JSON blob) and
// no pending write is left dangling. It is the single place that encodes what
// "degrades gracefully" means for the chatbot's public surface.
func assertErrorContract(t *testing.T, resp AgentResponse) {
	t.Helper()
	if strings.TrimSpace(resp.Reply) == "" {
		t.Error("contract §4.3 violated: Reply must always be populated")
	}
	if looksLikeRawError(resp.Reply) || looksLikeJSON(resp.Reply) {
		t.Errorf("contract §4.3 violated: Reply must be plain-language (no raw error / JSON)\n--- reply ---\n%s", resp.Reply)
	}
	// Defence in depth against leaked infrastructure strings anywhere in the reply.
	assertSafeReply(t, resp.Reply)
	if resp.PendingAction != nil {
		t.Error("contract §4.3 violated: PendingAction must be nil on an error path")
	}
}

// TestRouteErrorContract verifies the design's error contract (multi-agent-design.md
// §4.3 + router-refactor.md) on the chatbot's public surface — the real Route()
// entry point — across every failure mode and every agent.
//
// The distinguishing stress here: in each row the upstream service is down AND
// the LLM itself misbehaves (parrots a raw error string, returns a JSON blob, or
// is unreachable). The contract — populated, plain-language reply; nil
// PendingAction; never a Go error to the caller — must hold without relying on a
// cooperative model. The per-agent matrix in agents_test.go uses a clean canned
// LLM; this proves the guarantee survives an uncooperative one.
func TestRouteErrorContract(t *testing.T) {
	cases := []struct {
		name      string
		message   string
		mcpDown   bool              // close MCP before Route → connection refused
		payloads  map[string]string // tool payloads when MCP is up
		llmReply  string            // canned LLM reply (ignored when llmDown)
		llmDown   bool              // close LLM before Route → connection refused
		wantAgent string
	}{
		{
			name:      "data: MCP unreachable + LLM parrots a raw error",
			message:   "what is the risk level of elevator 12345?",
			mcpDown:   true,
			llmReply:  "mcp server unreachable: dial tcp 127.0.0.1:8765: connection refused",
			wantAgent: "data",
		},
		{
			name:      "knowledge: MCP unreachable + LLM returns a JSON blob",
			message:   "how do I troubleshoot a stuck door?",
			mcpDown:   true,
			llmReply:  `{"error":"boom","trace":"goroutine 1 [running]"}`,
			wantAgent: "knowledge",
		},
		{
			name:      "scheduling: MCP unreachable + LLM parrots a raw error",
			message:   "schedule an inspection for elevator 12345 on 2026-07-01",
			mcpDown:   true,
			llmReply:  "Post \"http://mcp/\": dial tcp: connection refused",
			wantAgent: "scheduling",
		},
		{
			// Scheduling under a clean LLM outage: the tool reports a validation
			// failure (so no pending write is created) and the LLM that would phrase
			// it is unreachable. The contract must still hold end-to-end.
			name:      "scheduling: tool validation error + LLM unreachable",
			message:   "schedule an inspection for elevator 99999 on 2026-07-01",
			payloads:  map[string]string{"schedule_inspection": `{"success":false,"error":"Elevator 99999 does not exist in the fleet."}`},
			llmDown:   true,
			wantAgent: "scheduling",
		},
		{
			name:      "data: tool error envelope + LLM unreachable",
			message:   "what is the risk level of elevator 12345?",
			payloads:  map[string]string{"get_elevator_risk": `{"error":true,"message":"internal database failure"}`},
			llmDown:   true,
			wantAgent: "data",
		},
		{
			name:      "general: LLM unreachable",
			message:   "what does TSSA stand for?",
			llmDown:   true,
			wantAgent: "general",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mcp := newTrackingMCPServer(t, tc.payloads)
			if tc.mcpDown {
				mcp.Close()
			} else {
				defer mcp.Close()
			}
			t.Setenv("MCP_SERVER_URL", mcp.URL)

			llm := fakeLLMServer(t, tc.llmReply)
			if tc.llmDown {
				llm.Close()
			} else {
				defer llm.Close()
			}
			t.Setenv("OLLAMA_BASE_URL", llm.URL)
			t.Setenv("OLLAMA_API_KEY", "test-key")

			resp := Route(context.Background(), AgentRequest{Message: tc.message})

			assertErrorContract(t, resp)
			if resp.AgentName != tc.wantAgent {
				t.Errorf("AgentName = %q, want %q", resp.AgentName, tc.wantAgent)
			}
		})
	}
}

// TestRouteRecoversFromAgentPanic verifies Route's last-resort guarantee
// (router-refactor.md): if a dispatched agent panics, the defer/recover block
// still returns a populated, plain-language AgentResponse tagged "router" — the
// caller never sees a crash, a Go error, or an empty reply. A panicking stub is
// swapped into the package-level agents map for the duration of the test (Go runs
// package tests sequentially, and the original is restored via t.Cleanup).
func TestRouteRecoversFromAgentPanic(t *testing.T) {
	orig := agents["mcp_data_tool"]
	t.Cleanup(func() { agents["mcp_data_tool"] = orig })
	agents["mcp_data_tool"] = func(ctx context.Context, req AgentRequest) AgentResponse {
		panic("simulated agent crash")
	}

	// Routes to mcp_data_tool (the panicking stub).
	resp := Route(context.Background(), AgentRequest{
		Message: "what is the risk level of elevator 12345?",
	})

	assertErrorContract(t, resp)
	if resp.AgentName != "router" {
		t.Errorf("AgentName after recover = %q, want %q", resp.AgentName, "router")
	}
}
