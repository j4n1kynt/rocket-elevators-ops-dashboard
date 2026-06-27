package main

// TestEvalMatrix runs every query from docs/agent-evaluation.md Part 2 through
// ClassifyIntent + routeIntent and asserts the expected agent.
//
// Pre-emption scenarios (SA1_cancel, SA4_confirm) are excluded here: the
// pre-emption check runs in the router before ClassifyIntent is called and
// cannot be exercised without an HTTP request carrying a pending_action field.
// Those paths are covered by TestRouteSelectsCorrectAgent (pre-emption cases).
//
// SA1_cancel / SA4_confirm without a pending_action fall through to advisory,
// so their classifier-only result is "general" — included as a note, not a
// blocking assertion, since the real test is the pre-emption path.
import (
	"testing"
	"time"
)

func agentFromRoute(target string) string {
	switch target {
	case "mcp_data_tool":
		return "data"
	case "rag_search":
		return "knowledge"
	case "action_executor":
		return "scheduling"
	default:
		return "general"
	}
}

func TestEvalMatrix(t *testing.T) {
	now := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		id       string
		message  string
		expected string // expected agent name
	}{
		// ── Data agent ───────────────────────────────────────────────────────────
		{"D1", "What is the risk level of elevator 48210?", "data"},
		{"D2", "Show me the inspection history for elevator 92341.", "data"},
		{"D3", "How many elevators are currently flagged for TSSA shutdown?", "data"},
		{"D4", "Which elevators need a followup inspection?", "data"},
		{"D5", "How many incidents were reported in the last year?", "data"},

		// ── Scheduling agent (keyword path) ──────────────────────────────────────
		{"SA1", "Schedule a periodic inspection for elevator 55123 on 2026-07-20.", "scheduling"},
		{"SA2", "Book a followup inspection for elevator 78432 next Monday.", "scheduling"},
		{"SA3", "I need to arrange a followup — elevator 44210 failed last week.", "scheduling"},
		{"SA4", "Schedule a periodic inspection for elevator 37180 on 2026-08-01.", "scheduling"},
		// SA1_cancel / SA4_confirm: pre-emption path — not classifiable without pending_action.
		// Without pending_action "Cancel that." and "Yes, confirm." score 0 → advisory → general.
		// Pre-emption coverage lives in TestRouteSelectsCorrectAgent.

		// ── Knowledge agent ───────────────────────────────────────────────────────
		{"K1", "What is the procedure for hydraulic pressure loss?", "knowledge"},
		{"K2", "What maintenance is required after a cable replacement?", "knowledge"},
		{"K3", "When is an inspection considered overdue under TSSA regulations?", "knowledge"},
		{"K4", "What patterns appear in past incident narratives for traction elevators?", "knowledge"},

		// ── General agent ─────────────────────────────────────────────────────────
		{"G1", "What does TSSA stand for?", "general"},
		{"G2", "What is a Customer Shutdown?", "general"},
		{"G3", "Hello, what can you help me with?", "general"},

		// ── Borderline / ambiguous ────────────────────────────────────────────────
		{"B1", "What causes elevator 48210 to be high risk?", "data"},
		{"B2", "How often does elevator 92341 get inspected?", "data"},
		{"B3", "What are the TSSA requirements for getting a shutdown elevator back in service?", "knowledge"},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			c := ClassifyIntent(tc.message, now)
			r := routeIntent(c)
			got := agentFromRoute(r.Target)
			if got != tc.expected {
				t.Errorf(
					"[%s] %q\n  routing signal: intent=%s confidence=%.2f signals=%v\n  got agent=%q, want=%q",
					tc.id, tc.message, c.Intent, c.Confidence,
					func() []string {
						var ss []string
						for _, s := range c.Signals {
							ss = append(ss, s.Keyword)
						}
						return ss
					}(),
					got, tc.expected,
				)
			}
		})
	}
}
