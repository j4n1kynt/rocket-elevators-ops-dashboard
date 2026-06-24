package main

import (
	"context"
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
