package main

import (
	"reflect"
	"testing"
	"time"
)

// fixedNow gives the date extractor a stable "current year" so tests are
// deterministic regardless of when they run.
var fixedNow = time.Date(2026, time.June, 16, 0, 0, 0, 0, time.UTC)

const eps = 0.001

// ── Phase A: classification + advisory default ──────────────────────────────

func TestClassifyIntent(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want Intent
	}{
		// 1. empty message
		{"empty", "", IntentAdvisory},
		// 2. general knowledge
		{"general_knowledge", "What is a hydraulic elevator?", IntentAdvisory},
		// 3. data query
		{"data_query_shutdown", "Which elevators are shut down by TSSA?", IntentDataQuery},
		// 4. rag (steps to replace -> rag, not action)
		{"rag_steps", "What are the steps to replace a governor?", IntentRAG},
		// 5. action
		{"action_schedule", "Schedule an inspection for elevator 12345 on July 15", IntentAction},
		// 6. case-insensitive data query
		{"data_query_uppercase", "WHICH ELEVATORS ARE SHUT DOWN?", IntentDataQuery},
		// extra coverage
		{"data_query_followup", "Which elevators need follow up inspections?", IntentDataQuery},
		{"rag_howto", "How do I troubleshoot a stuck door?", IntentRAG},
		// Risk phrases — FEATURE-2 (elevator ID present supplies the entity bonus)
		{"risk_high_risk_hyphen", "Why is elevator 12345 high-risk?", IntentDataQuery},
		{"risk_explain_risk", "Explain the risk for elevator 12345", IntentDataQuery},
		{"risk_level_with_id", "What is the risk level for elevator 12345?", IntentDataQuery},
		{"risk_dangerous", "Is elevator 12345 dangerous?", IntentDataQuery},
		{"risk_assessment", "Show me the risk assessment for elevator 12345", IntentDataQuery},
		{"risk_rated_high", "Why is elevator 12345 rated HIGH?", IntentDataQuery},
		{"risk_score", "What is the risk score for elevator 12345?", IntentDataQuery},
		// FOUNDATION-1B — experiential / recurrence questions route to RAG
		// (incident narrative corpus). "incident" alone is a DATA_QUERY keyword,
		// so the experiential phrase must out-score it.
		{"rag_have_we_seen", "have we seen flooding incidents?", IntentRAG},
		{"rag_has_this_happened", "has this happened before?", IntentRAG},
		{"rag_similar_incidents", "have we had similar incidents with the doors?", IntentRAG},
		{"rag_procedure", "what's the procedure for hydraulic pressure loss?", IntentRAG},
		{"rag_how_replace_governor", "how do I replace a governor?", IntentRAG},
		// FOUNDATION-1B — structured incident queries must NOT regress to RAG.
		{"data_incident_count", "how many incidents were reported last year?", IntentDataQuery},
		{"data_incident_by_elevator", "what incidents have been reported for elevator 12345?", IntentDataQuery},
		// 26. ID present but advisory phrasing -> advisory (below floor)
		{"id_advisory_phrasing", "Is elevator 12345 a hydraulic type?", IntentAdvisory},
		// 28. punctuation-only
		{"punctuation_only", "???", IntentAdvisory},
		{"whitespace_only", "   ", IntentAdvisory},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyIntent(c.msg, fixedNow)
			if got.Intent != c.want {
				t.Fatalf("ClassifyIntent(%q) intent = %q; want %q (reason: %s)",
					c.msg, got.Intent, c.want, got.Reason)
			}
		})
	}
}

// ── Phase B: entity extraction ──────────────────────────────────────────────

func TestExtractElevatorIDs(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want []string
	}{
		{"context_cue", "show me elevator 12345", []string{"12345"}},
		{"no_id", "which elevators are shut down?", nil},
		{"multiple_standalone", "compare 10054 and 10078", []string{"10054", "10078"}},
		{"short_id_with_cue", "device 10 status", []string{"10"}},
		{"count_not_id", "replace 3 governors", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractElevatorIDs(c.msg)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("extractElevatorIDs(%q) = %v; want %v", c.msg, got, c.want)
			}
		})
	}
}

func TestExtractDates(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want []string
	}{
		{"natural_no_year", "on July 15", []string{"2026-07-15"}},
		{"iso", "by 2026-07-15 please", []string{"2026-07-15"}},
		{"natural_with_year", "July 15, 2027", []string{"2027-07-15"}},
		{"invalid_date", "February 30", nil},
		{"no_date", "schedule an inspection", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractDates(c.msg, fixedNow)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("extractDates(%q) = %v; want %v", c.msg, got, c.want)
			}
		})
	}
}

func TestExtractActionType(t *testing.T) {
	cases := []struct {
		msg  string
		want string
	}{
		{"schedule an inspection", "schedule_inspection"},
		{"replace a governor", "replace"},
		{"how many incidents?", ""},
	}
	for _, c := range cases {
		t.Run(c.msg, func(t *testing.T) {
			if got := extractActionType(c.msg); got != c.want {
				t.Fatalf("extractActionType(%q) = %q; want %q", c.msg, got, c.want)
			}
		})
	}
}

// ── Phase C: confidence + fallback ──────────────────────────────────────────

func TestConfidenceAboveFloor(t *testing.T) {
	// 17. strong keyword + entities -> confidence >= floor, intent stands
	got := ClassifyIntent("Schedule an inspection for elevator 12345 on July 15", fixedNow)
	if got.Intent != IntentAction {
		t.Fatalf("intent = %q; want action", got.Intent)
	}
	if got.Confidence < ConfidenceFloor {
		t.Fatalf("confidence = %.3f; want >= %.2f", got.Confidence, ConfidenceFloor)
	}
}

func TestConfidenceFallbackToAdvisory(t *testing.T) {
	// 18. a single weak keyword only -> below floor -> advisory
	got := ClassifyIntent("please list those", fixedNow)
	if got.Intent != IntentAdvisory {
		t.Fatalf("intent = %q; want advisory (reason: %s)", got.Intent, got.Reason)
	}
	if got.Confidence >= ConfidenceFloor {
		t.Fatalf("confidence = %.3f; want < %.2f", got.Confidence, ConfidenceFloor)
	}
	if got.Reason == "" {
		t.Fatalf("fallback must record a reason")
	}
}

func TestRiskPhraseConfidenceFloor(t *testing.T) {
	phrases := []string{
		"Why is elevator 12345 high-risk?",
		"Explain the risk for elevator 12345",
		"What is the risk level for elevator 12345?",
		"Is elevator 12345 dangerous?",
		"Show me the risk assessment for elevator 12345",
		"Why is elevator 12345 rated HIGH?",
		"What is the risk score for elevator 12345?",
	}
	for _, p := range phrases {
		t.Run(p, func(t *testing.T) {
			got := ClassifyIntent(p, fixedNow)
			if got.Intent != IntentDataQuery {
				t.Fatalf("intent = %q; want data_query (reason: %s)", got.Intent, got.Reason)
			}
			if got.Confidence < ConfidenceFloor {
				t.Fatalf("confidence = %.3f; want >= %.2f", got.Confidence, ConfidenceFloor)
			}
		})
	}
}

func TestConfidenceFormula(t *testing.T) {
	// 19. data query "which" (0.5) + "shut down" (1.0) = 1.5 -> 1.5/2.5 = 0.6
	got := ClassifyIntent("Which elevators are shut down?", fixedNow)
	want := 1.5 / 2.5
	if diff := got.Confidence - want; diff > eps || diff < -eps {
		t.Fatalf("confidence = %.4f; want ~%.4f", got.Confidence, want)
	}
	// 21. exactly at the floor does NOT fall back
	if got.Intent != IntentDataQuery {
		t.Fatalf("intent = %q; want data_query (confidence at floor should stand)", got.Intent)
	}
}

func TestAdvisoryByDefaultConfidence(t *testing.T) {
	// 22. no signals at all -> advisory with confidence 0
	got := ClassifyIntent("Tell me about elevators in general", fixedNow)
	if got.Intent != IntentAdvisory {
		t.Fatalf("intent = %q; want advisory", got.Intent)
	}
	if got.Confidence != 0 {
		t.Fatalf("confidence = %.3f; want 0 for pure advisory", got.Confidence)
	}
}

// ── Phase D: full result, trace, determinism ────────────────────────────────

func TestActionEntitiesPopulated(t *testing.T) {
	// 23. full action scenario fills all entities; no type keyword → InspectionType ""
	got := ClassifyIntent("Schedule an inspection for elevator 12345 on July 15", fixedNow)
	want := Entities{
		ElevatorIDs:    []string{"12345"},
		Dates:          []string{"2026-07-15"},
		ActionType:     "schedule_inspection",
		InspectionType: "",
	}
	if !reflect.DeepEqual(got.Entities, want) {
		t.Fatalf("entities = %+v; want %+v", got.Entities, want)
	}
}

func TestExtractInspectionType(t *testing.T) {
	cases := []struct {
		msg  string
		want string
	}{
		{"Schedule a periodic inspection for elevator 12345", "Periodic"},
		{"Book an annual inspection on July 15", "Periodic"},
		{"Schedule a routine check for elevator 12345", "Periodic"},
		{"Schedule a followup inspection for elevator 12345", "Followup"},
		{"Book a follow up for elevator 12345 on July 15", "Followup"},
		{"Schedule a follow-up inspection", "Followup"},
		{"Schedule an initial inspection for elevator 12345", "Initial"},
		{"Book an incident inspection for elevator 12345", "Incident"},
		{"Schedule a near-miss inspection", "Incident"},
		{"Schedule an alteration inspection for elevator 12345", "Alteration"},
		{"Book a modification inspection on July 15", "Alteration"},
		{"Schedule an inspection for elevator 12345 on July 15", ""},
	}
	for _, c := range cases {
		t.Run(c.msg, func(t *testing.T) {
			if got := extractInspectionType(c.msg); got != c.want {
				t.Fatalf("extractInspectionType(%q) = %q; want %q", c.msg, got, c.want)
			}
		})
	}
}

func TestSignalsAndReason(t *testing.T) {
	// 24 + 25. winner has non-empty signals and a stable reason
	got := ClassifyIntent("Which elevators are shut down by TSSA?", fixedNow)
	if len(got.Signals) == 0 {
		t.Fatalf("expected non-empty signals for a classified intent")
	}
	if got.Reason == "" {
		t.Fatalf("expected a non-empty reason")
	}
}

func TestDeterminism(t *testing.T) {
	// 27. same input -> identical result
	msg := "Schedule an inspection for elevator 12345 on July 15"
	a := ClassifyIntent(msg, fixedNow)
	b := ClassifyIntent(msg, fixedNow)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("classification not deterministic:\n a=%+v\n b=%+v", a, b)
	}
}

// ── Routing ─────────────────────────────────────────────────────────────────

// ── FOUNDATION-1B: RAG corpus split + incident routing in buildMCPArgs ───────

func TestBuildMCPArgsRouting(t *testing.T) {
	cases := []struct {
		name     string
		msg      string
		wantTool string
		// wantArgs is checked key-by-key; only the listed keys are asserted.
		wantArgs map[string]any
	}{
		// Experiential / recurrence questions → incident narrative corpus.
		{
			"rag_have_we_seen_incidents",
			"have we seen flooding incidents?",
			"search_incident_narratives",
			map[string]any{"query": "have we seen flooding incidents?", "limit": 5},
		},
		{
			"rag_has_this_happened",
			"has this happened before?",
			"search_incident_narratives",
			map[string]any{"query": "has this happened before?", "limit": 5},
		},
		{
			"rag_similar_incidents",
			"have we had similar incidents with the doors?",
			"search_incident_narratives",
			map[string]any{"query": "have we had similar incidents with the doors?", "limit": 5},
		},
		// Procedural / how-to questions → maintenance manual corpus (unchanged).
		{
			"rag_procedure",
			"what's the procedure for hydraulic pressure loss?",
			"search_maintenance_docs",
			map[string]any{"query": "what's the procedure for hydraulic pressure loss?", "n_results": 5},
		},
		{
			"rag_replace_governor",
			"how do I replace a governor?",
			"search_maintenance_docs",
			map[string]any{"query": "how do I replace a governor?", "n_results": 5},
		},
		// A procedural question that happens to mention "incident" must route to
		// the maintenance manuals — "procedure" (1.5) wins IntentRAG, and there is
		// no bare "incident" cue to drag it onto the narrative corpus.
		{
			"rag_procedure_reporting_incident",
			"What's the procedure for reporting an incident?",
			"search_maintenance_docs",
			map[string]any{"query": "What's the procedure for reporting an incident?", "n_results": 5},
		},
		// Structured incident queries must keep their existing data tools.
		{
			"data_incident_count",
			"how many incidents were reported last year?",
			"get_incident_count_last_year",
			map[string]any{},
		},
		{
			"data_incident_by_elevator",
			"what incidents have been reported for elevator 12345?",
			"get_elevator_incidents",
			map[string]any{"elevator_id": 12345, "limit": 10},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cls := ClassifyIntent(c.msg, fixedNow)
			tool, args := buildMCPArgs(cls, c.msg)
			if tool != c.wantTool {
				t.Fatalf("buildMCPArgs(%q) tool = %q; want %q (intent=%s reason=%s)",
					c.msg, tool, c.wantTool, cls.Intent, cls.Reason)
			}
			for k, want := range c.wantArgs {
				got, ok := args[k]
				if !ok {
					t.Fatalf("buildMCPArgs(%q) missing arg %q; got %+v", c.msg, k, args)
				}
				if got != want {
					t.Fatalf("buildMCPArgs(%q) arg %q = %v (%T); want %v (%T)",
						c.msg, k, got, got, want, want)
				}
			}
		})
	}
}

func TestContextCarry(t *testing.T) {
	cases := []struct {
		name        string
		msg         string
		history     []ChatMessage
		wantIntent  Intent
		wantEntity  string // first elevator ID expected, "" if none required
	}{
		{
			// data follow-up: elevator ID triggers carry
			name: "data_elevator_id",
			msg:  "And for 20718?",
			history: []ChatMessage{
				{Role: "user", Content: "Show me the inspection history for elevator 14575"},
				{Role: "assistant", Content: "Here are the inspections for elevator 14575."},
			},
			wantIntent: IntentDataQuery,
			wantEntity: "20718",
		},
		{
			// RAG follow-up: connector phrase triggers carry, no elevator ID needed
			name: "rag_connector",
			msg:  "What about the hydraulic system?",
			history: []ChatMessage{
				{Role: "user", Content: "What are the steps to replace a governor?"},
				{Role: "assistant", Content: "Here are the steps..."},
			},
			wantIntent: IntentRAG,
			wantEntity: "",
		},
		{
			// No history → no change
			name:       "no_history",
			msg:        "And for 20718?",
			history:    nil,
			wantIntent: IntentAdvisory,
			wantEntity: "20718",
		},
		{
			// Genuine advisory question must not be carried
			name: "genuine_advisory_not_carried",
			msg:  "What is a hydraulic elevator?",
			history: []ChatMessage{
				{Role: "user", Content: "Show me the inspection history for elevator 14575"},
			},
			wantIntent: IntentAdvisory,
			wantEntity: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := ClassifyIntent(tc.msg, fixedNow)
			carried := contextCarry(c, tc.msg, tc.history, fixedNow)

			if carried.Intent != tc.wantIntent {
				t.Fatalf("intent: got %s, want %s", carried.Intent, tc.wantIntent)
			}
			if tc.wantEntity != "" {
				if len(carried.Entities.ElevatorIDs) == 0 || carried.Entities.ElevatorIDs[0] != tc.wantEntity {
					t.Fatalf("entity: got %v, want [%s]", carried.Entities.ElevatorIDs, tc.wantEntity)
				}
			}
		})
	}
}

func TestContextCarryEndToEnd(t *testing.T) {
	cases := []struct {
		name         string
		msg          string
		history      []ChatMessage
		wantTool     string
		wantElevator int
	}{
		{
			// "And for 20718?" has no sub-topic keywords of its own, so the carry
			// inherits the previous turn's specific tool (inspection history) via
			// KeywordSource — not the elevator-risk default. The new elevator ID is
			// still the one looked up.
			name: "short_followup_inherits_inspection_history",
			msg:  "And for 20718?",
			history: []ChatMessage{
				{Role: "user", Content: "Show me the inspection history for elevator 14575"},
				{Role: "assistant", Content: "Here are the inspections for elevator 14575."},
			},
			wantTool:     "get_inspection_history",
			wantElevator: 20718,
		},
		{
			// Regression for the reported bug: an ID-only "What about NNNNN" after
			// an inspection-history question must return inspections, not risk.
			name: "what_about_id_inherits_inspection_history",
			msg:  "What about 20718",
			history: []ChatMessage{
				{Role: "user", Content: "Show me the inspection history for elevator 20657"},
				{Role: "assistant", Content: "Here are the inspections for elevator 20657."},
			},
			wantTool:     "get_inspection_history",
			wantElevator: 20718,
		},
		{
			name: "generic_followup_with_id",
			msg:  "And for 20718?",
			history: []ChatMessage{
				{Role: "user", Content: "What is the risk for elevator 14575?"},
				{Role: "assistant", Content: "Elevator 14575 is high risk."},
			},
			wantTool:     "get_elevator_risk",
			wantElevator: 20718,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := ClassifyIntent(tc.msg, fixedNow)
			c := contextCarry(base, tc.msg, tc.history, fixedNow)

			toolName, args := buildMCPArgs(c, tc.msg)

			if toolName != tc.wantTool {
				t.Errorf("tool: got %s, want %s", toolName, tc.wantTool)
			}
			id, ok := args["elevator_id"]
			if !ok {
				t.Fatalf("elevator_id missing from args: %v", args)
			}
			if id != tc.wantElevator {
				t.Errorf("elevator_id: got %v, want %d", id, tc.wantElevator)
			}
		})
	}
}

// TestContextCarryRAGCorpus locks in a behavior that the data-path fix also
// changed: on a RAG follow-up, buildMCPArgs picks the corpus (incident
// narratives vs. maintenance manuals) from the carried turn via KeywordSource,
// while the search query text still comes from the current message. This keeps
// the corpus consistent within one thread.
func TestContextCarryRAGCorpus(t *testing.T) {
	cases := []struct {
		name      string
		msg       string
		history   []ChatMessage
		wantTool  string
		wantQuery string
	}{
		{
			// Previous turn is an experiential "have we seen" question (narrative
			// corpus). The connector follow-up inherits that corpus, but searches
			// it with the current words.
			name: "followup_inherits_narrative_corpus",
			msg:  "What about the brakes?",
			history: []ChatMessage{
				{Role: "user", Content: "Have we seen similar incidents with the door sensors?"},
				{Role: "assistant", Content: "Yes, there were a few similar incidents."},
			},
			wantTool:  "search_incident_narratives",
			wantQuery: "What about the brakes?",
		},
		{
			// Previous turn is a how-to question (manuals corpus). The follow-up
			// stays on the manuals, again with the current words.
			name: "followup_inherits_manuals_corpus",
			msg:  "And how about the brakes?",
			history: []ChatMessage{
				{Role: "user", Content: "How do I replace the governor?"},
				{Role: "assistant", Content: "Here are the steps to replace the governor."},
			},
			wantTool:  "search_maintenance_docs",
			wantQuery: "And how about the brakes?",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := ClassifyIntent(tc.msg, fixedNow)
			c := contextCarry(base, tc.msg, tc.history, fixedNow)

			toolName, args := buildMCPArgs(c, tc.msg)

			if toolName != tc.wantTool {
				t.Errorf("tool: got %s, want %s", toolName, tc.wantTool)
			}
			if q := args["query"]; q != tc.wantQuery {
				t.Errorf("query: got %v, want %q (must be the current message, not the carried turn)", q, tc.wantQuery)
			}
		})
	}
}

func TestRouteIntent(t *testing.T) {
	cases := []struct {
		intent     Intent
		wantTarget string
		wantStub   bool
	}{
		{IntentDataQuery, "mcp_data_tool", false},
		{IntentRAG, "rag_search", false},
		{IntentAction, "action_executor", false},
		{IntentAdvisory, "advisory", false},
	}
	for _, c := range cases {
		t.Run(string(c.intent), func(t *testing.T) {
			r := routeIntent(Classification{Intent: c.intent})
			if r.Target != c.wantTarget || r.Stub != c.wantStub {
				t.Fatalf("routeIntent(%q) = {%q,%t}; want {%q,%t}",
					c.intent, r.Target, r.Stub, c.wantTarget, c.wantStub)
			}
		})
	}
}
