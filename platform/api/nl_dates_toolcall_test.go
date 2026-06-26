package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

// nlNow is a fixed Thursday (2026-06-25) so relative-date assertions are stable.
// Weekday(2026-06-25) == Thursday.
var nlNow = time.Date(2026, time.June, 25, 0, 0, 0, 0, time.UTC)

// TestExtractDatesRelative covers the natural-language date forms added so a user
// can say "next tuesday" / "tomorrow" instead of a YYYY-MM-DD string.
func TestExtractDatesRelative(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want []string
	}{
		{"today", "schedule it today", []string{"2026-06-25"}},
		{"tomorrow", "schedule an inspection tomorrow", []string{"2026-06-26"}},
		// 2026-06-25 is Thursday → next Tuesday is 2026-06-30.
		{"next_tuesday", "schedule a periodic inspection for elevator 10054 next tuesday", []string{"2026-06-30"}},
		// "on friday" → 2026-06-26 (the coming Friday).
		{"on_friday", "book an inspection on friday", []string{"2026-06-26"}},
		// today is Thursday → "thursday" means next week's occurrence (strictly future).
		{"same_weekday_rolls_forward", "schedule for thursday", []string{"2026-07-02"}},
		{"in_two_weeks", "schedule the inspection in 2 weeks", []string{"2026-07-09"}},
		{"in_three_days", "in 3 days please", []string{"2026-06-28"}},
		// Explicit ISO still wins position 0; no relative word present.
		{"iso_unchanged", "on 2026-08-15", []string{"2026-08-15"}},
		{"no_date", "schedule an inspection", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractDates(c.msg, nlNow)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("extractDates(%q) = %v; want %v", c.msg, got, c.want)
			}
		})
	}
}

// TestExtractDatesNumeric covers slash/dash numeric date formats (e.g. a user
// answering "15-06-2026") so they reach the scheduling agent instead of falling
// through to the general agent.
func TestExtractDatesNumeric(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want []string
	}{
		{"dd-mm-yyyy", "15-06-2026", []string{"2026-06-15"}},
		{"dd/mm/yyyy", "15/06/2026", []string{"2026-06-15"}},
		{"yyyy/mm/dd", "2026/07/15", []string{"2026-07-15"}},
		{"mm-dd-yyyy_disambiguated", "06-15-2026", []string{"2026-06-15"}}, // 15>12 ⇒ day
		{"ambiguous_skipped", "06-07-2026", nil},                          // both ≤12 ⇒ don't guess
		{"invalid_skipped", "31-31-2026", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractDates(c.msg, nlNow)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("extractDates(%q) = %v; want %v", c.msg, got, c.want)
			}
		})
	}
}

// TestLooksLikeFakeScheduling guards the backstop that stops the general agent
// from passing off a fabricated scheduling confirmation as real.
func TestLooksLikeFakeScheduling(t *testing.T) {
	yes := []string{
		"Inspection scheduled successfully. Inspection ID 9000040 for elevator 22906 on 2026-07-15.",
		"Your inspection has been scheduled: | Field | Value |",
		"You are about to schedule an inspection: Elevator ID 22906",
	}
	no := []string{
		"Inspections are scheduled by TSSA on a periodic basis; check the dashboard for dates.",
		"A HIGH risk level means the model predicts an elevated likelihood of a failed inspection.",
		"I can't schedule inspections myself. Tell me the elevator ID and a date.",
	}
	for _, s := range yes {
		if !looksLikeFakeScheduling(s) {
			t.Errorf("expected fabricated-scheduling detection for: %q", s)
		}
	}
	for _, s := range no {
		if looksLikeFakeScheduling(s) {
			t.Errorf("false positive on legitimate reply: %q", s)
		}
	}
}

// TestGeneralAgentBlocksFakeScheduling verifies the backstop: if the model returns
// a fabricated scheduling confirmation, the general agent replaces it with a safe
// redirect rather than misleading the user that a write happened.
func TestGeneralAgentBlocksFakeScheduling(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)
	llm := fakeLLMServer(t, "Inspection scheduled successfully. Inspection ID 9000040 for elevator 22906 on 2026-07-15.")
	defer llm.Close()
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := generalAgent(context.Background(), AgentRequest{Message: "yes"})

	low := strings.ToLower(resp.Reply)
	if strings.Contains(low, "scheduled successfully") || strings.Contains(resp.Reply, "9000040") {
		t.Errorf("general agent leaked a fabricated scheduling success:\n%s", resp.Reply)
	}
	if !strings.Contains(low, "can't schedule") {
		t.Errorf("expected the safe redirect; got:\n%s", resp.Reply)
	}
}

// TestCleanValidationErrorStripsPydanticSuffix verifies the raw Pydantic tail is
// removed so the user sees only the human-readable message.
func TestCleanValidationErrorStripsPydanticSuffix(t *testing.T) {
	raw := "tool error: 1 validation error for ScheduleInspectionInput\ninspection_date\n  Value error, inspection_date must not be in the past [type=value_error, input_value='2026-06-15', input_type=str]"
	got := cleanValidationError(raw)
	want := "inspection_date must not be in the past"
	if got != want {
		t.Fatalf("cleanValidationError = %q; want %q", got, want)
	}
}

// TestRelativeDateRoutesToSchedulingPhase1 verifies that a relative date now
// carries enough entity signal for the scheduling agent to reach Phase 1 (an
// elevator ID + a resolved date) rather than falling into "needs more info".
func TestRelativeDateRoutesToSchedulingPhase1(t *testing.T) {
	c := ClassifyIntent("Schedule a periodic inspection for elevator 10054 next tuesday", nlNow)
	if c.Intent != IntentAction {
		t.Fatalf("intent = %q; want action (reason: %s)", c.Intent, c.Reason)
	}
	if len(c.Entities.Dates) == 0 || c.Entities.Dates[0] != "2026-06-30" {
		t.Fatalf("dates = %v; want [2026-06-30]", c.Entities.Dates)
	}
}

// TestCollectSchedulingEntities verifies the scheduling agent assembles slots
// across turns: the ID given in one message and the date/type in another.
func TestCollectSchedulingEntities(t *testing.T) {
	t.Run("id_from_history_date_type_from_current", func(t *testing.T) {
		history := []ChatMessage{
			{Role: "user", Content: "I want to schedule an inspection for elevator 10054"},
			{Role: "assistant", Content: "Which date and type?"},
		}
		got := collectSchedulingEntities("I want the inspection for next tuesday, it's a periodic inspection", history, nlNow)
		if len(got.ElevatorIDs) == 0 || got.ElevatorIDs[0] != "10054" {
			t.Errorf("elevator id: got %v, want [10054]", got.ElevatorIDs)
		}
		if len(got.Dates) == 0 || got.Dates[0] != "2026-06-30" {
			t.Errorf("date: got %v, want [2026-06-30]", got.Dates)
		}
		if got.InspectionType != "Periodic" {
			t.Errorf("type: got %q, want Periodic", got.InspectionType)
		}
	})
	t.Run("new_request_does_not_inherit_completed_transaction", func(t *testing.T) {
		// A finished schedule sits in history. A fresh request naming only a new
		// type must NOT borrow the old elevator/date — backfill stops at the
		// completion boundary, so the agent will ask for the missing ID and date.
		history := []ChatMessage{
			{Role: "user", Content: "schedule a periodic inspection for elevator 10054 next friday"},
			{Role: "assistant", Content: "Inspection scheduled successfully. Inspection ID 9000099 for elevator 10054 on 2026-06-26 (status: Pending)."},
		}
		got := collectSchedulingEntities("I need an alteration inspection too", history, nlNow)
		if len(got.ElevatorIDs) != 0 {
			t.Errorf("elevator id must NOT carry across a completed schedule; got %v", got.ElevatorIDs)
		}
		if len(got.Dates) != 0 {
			t.Errorf("date must NOT carry across a completed schedule; got %v", got.Dates)
		}
		if got.InspectionType != "Alteration" {
			t.Errorf("type must come from the new message; got %q want Alteration", got.InspectionType)
		}
	})
	t.Run("id_from_current_date_type_from_history", func(t *testing.T) {
		history := []ChatMessage{
			{Role: "user", Content: "book a periodic inspection for next friday"},
			{Role: "assistant", Content: "Which elevator?"},
		}
		got := collectSchedulingEntities("elevator 10054", history, nlNow)
		if len(got.ElevatorIDs) == 0 || got.ElevatorIDs[0] != "10054" {
			t.Errorf("elevator id: got %v, want [10054]", got.ElevatorIDs)
		}
		if len(got.Dates) == 0 || got.Dates[0] != "2026-06-26" {
			t.Errorf("date: got %v, want [2026-06-26] (next friday)", got.Dates)
		}
		if got.InspectionType != "Periodic" {
			t.Errorf("type: got %q, want Periodic", got.InspectionType)
		}
	})
}

// TestSchedulingMultiTurnRoutesAndFills is the regression for the reported bug:
// a scheduling reply that supplies the date/type in a later turn (no ID, no verb)
// was routing to the general agent ("I don't have access to a scheduling system").
// It must now route to the scheduling agent and assemble a complete pending action
// using the elevator ID from the earlier turn. Date is checked only for presence
// (Route uses the real clock, so the exact value depends on the run date).
func TestSchedulingMultiTurnRoutesAndFills(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{
		"schedule_inspection": `{"success":false,"confirmed":false,"pending_confirmation":true,"summary":"You are about to schedule an inspection:\n  Elevator ID : 10054\n  Type        : Periodic\n\nPlease confirm to proceed."}`,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)
	llm := fakeLLMServer(t, "Here is the inspection summary for your review.")
	defer llm.Close()
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := Route(context.Background(), AgentRequest{
		Message: "I want the inspection for next tuesday, it's a periodic inspection",
		History: []ChatMessage{
			{Role: "user", Content: "I want to schedule an inspection for elevator 10054"},
			{Role: "assistant", Content: "I need the date and inspection type."},
		},
	})

	if resp.AgentName != "scheduling" {
		t.Fatalf("AgentName = %q, want scheduling (the continuation must not fall to the general agent)", resp.AgentName)
	}
	pa := resp.PendingAction
	if pa == nil {
		t.Fatalf("pending_action must be set — the ID from the prior turn should complete the request")
	}
	if pa.ElevatorID != 10054 {
		t.Errorf("pending elevator_id = %d, want 10054 (carried from the earlier turn)", pa.ElevatorID)
	}
	if pa.InspectionType != "Periodic" {
		t.Errorf("pending inspection_type = %q, want Periodic", pa.InspectionType)
	}
	if pa.InspectionDate == "" {
		t.Errorf("pending inspection_date must be populated from 'next tuesday'")
	}
}

// TestLooksLikeToolCall covers the hallucinated tool-call detector that keeps
// fabricated tool-call text (which never implies a real DB write) out of replies.
func TestLooksLikeToolCall(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  bool
	}{
		{"minimax_block", "[TOOL_CALL]\n{tool => \"schedule_inspection\", args => {...}}\n[/TOOL_CALL]", true},
		{"openai_style", `Sure. {"tool_calls":[{"name":"schedule_inspection"}]}`, true},
		{"arrow_syntax", "I'll call tool => schedule_inspection now", true},
		{"plain_answer", "Elevator 10054 is scheduled for a Periodic inspection on 2026-06-30.", false},
		{"mentions_tool_word", "Use the dashboard's scheduling tool for this.", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := looksLikeToolCall(c.reply); got != c.want {
				t.Fatalf("looksLikeToolCall(%q) = %v; want %v", c.reply, got, c.want)
			}
		})
	}
}

// TestSchedulingFallbackOnToolCallHallucination is the regression for the reported
// bug: the model claims it scheduled (by emitting a tool-call block) but the user
// finds nothing. The MCP tool returns a valid Phase 1 summary; the LLM returns a
// fabricated tool-call. The agent must show the deterministic confirmation prompt
// (so the user can actually confirm) — never the hallucinated tool-call text, and
// the pending_action must still be set.
func TestSchedulingFallbackOnToolCallHallucination(t *testing.T) {
	mcp := newTrackingMCPServer(t, map[string]string{
		"schedule_inspection": `{"success":false,"confirmed":false,"pending_confirmation":true,"summary":"You are about to schedule an inspection:\n  Elevator ID : 10054\n  Date        : 2026-06-30\n  Type        : Periodic\n\nPlease confirm to proceed."}`,
	})
	defer mcp.Close()
	t.Setenv("MCP_SERVER_URL", mcp.URL)

	// LLM returns a fabricated tool-call block instead of a natural-language reply.
	llm := fakeLLMServer(t, "[TOOL_CALL]\n{tool => \"schedule_inspection\", args => {--confirmed true --elevator_id 10054}}\n[/TOOL_CALL]")
	defer llm.Close()
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OLLAMA_BASE_URL", llm.URL)
	t.Setenv("OLLAMA_API_KEY", "test-key")

	resp := Route(context.Background(), AgentRequest{
		Message: "Schedule a periodic inspection for elevator 10054 next tuesday",
	})

	if resp.AgentName != "scheduling" {
		t.Fatalf("AgentName = %q, want scheduling", resp.AgentName)
	}
	if looksLikeToolCall(resp.Reply) {
		t.Errorf("reply leaked a tool-call hallucination:\n%s", resp.Reply)
	}
	if !strings.Contains(resp.Reply, "Elevator ID : 10054") {
		t.Errorf("reply must show the deterministic confirmation summary; got:\n%s", resp.Reply)
	}
	if resp.PendingAction == nil {
		t.Errorf("pending_action must still be set so the user can confirm")
	}
}
