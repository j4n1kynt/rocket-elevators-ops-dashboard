package main

// FOUNDATION-4: Intent Classification & Routing.
//
// A deterministic, keyword/rule-based classifier. It reads a user message and
// returns one of 4 intents (advisory, data_query, rag, action), the entities it
// found (elevator IDs, dates, action type), a confidence score, and a trace of
// every keyword that matched. Same input always produces the same output.
//
// Keyword seed lists come from docs/chat-design/chatbot_design_doc.md §3.4.
//
// This file is PURE: no DB, no HTTP, no Ollama. That keeps the tests fast and
// deterministic. Routing (routeIntent) decides the target; PostChat in chat.go
// calls the appropriate MCP tool and injects the result into the system prompt.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Intent is the routing category. String-typed enum, matching the project's
// existing string-enum style (e.g. risk levels "HIGH"/"LOW").
type Intent string

const (
	IntentAdvisory  Intent = "advisory"
	IntentDataQuery Intent = "data_query"
	IntentRAG       Intent = "rag"
	IntentAction    Intent = "action"
)

// ConfidenceFloor is the threshold. A winning operational intent below this
// falls back to advisory.
const ConfidenceFloor = 0.6

// Entities holds everything pulled out of the message. Zero/nil when absent.
type Entities struct {
	ElevatorIDs []string // digit strings, e.g. ["12345"]; nil if none
	Dates       []string // normalized "2006-01-02"; nil if none
	ActionType  string   // "schedule_inspection", "replace", or "" if none
}

// Signal is one matched keyword and the weight it added. Drives the trace.
type Signal struct {
	Intent  Intent
	Keyword string
	Weight  float64
}

// Classification is the full deterministic result.
type Classification struct {
	Intent     Intent
	Confidence float64 // 0.0–1.0
	Entities   Entities
	Signals    []Signal // every keyword that matched, in scan order — the TRACE
	Reason     string   // one-line explanation, for logs
}

// ── Keyword tables ──────────────────────────────────────────────────────────
//
// Ordered slices (never maps) so the Signals trace order is stable. Matching is
// substring-based on the lowercased message, so we list base words and avoid
// listing a word and its own substring twice.

type keyword struct {
	text   string
	weight float64
}

type keywordGroup struct {
	intent   Intent
	keywords []keyword
}

// keywordGroups is scanned in this fixed order; advisory is the default and has
// no list.
var keywordGroups = []keywordGroup{
	{IntentDataQuery, []keyword{
		{"shut down", 1.0},
		{"shutdown", 1.0},
		{"out of service", 1.0},
		{"tssa", 1.0},
		{"inspection history", 1.0},
		{"incident", 1.0},
		{"follow up", 1.0},
		{"follow-up", 1.0},
		{"overdue", 1.0},
		{"status of", 1.0},
		{"last inspected", 1.0},
		{"risk level",  1.0},
		{"dangerous",   1.0},
		{"risk",        1.0},
		{"rated high",  1.0},
		{"flagged",     0.5},
		{"how many", 0.5},
		{"list", 0.5},
		{"which", 0.5},
	}},
	{IntentRAG, []keyword{
		// Procedural anchors (manual corpus). Weight 1.5 so a single procedural
		// phrase clears the confidence floor (1.5/2.5 = 0.6) on its own.
		{"steps to", 1.5},
		{"how to", 1.5},
		{"how do i", 1.5},
		{"how do you", 1.5},
		{"procedure", 1.5},
		{"instructions", 1.5},
		{"troubleshoot", 1.5},
		// Experiential / recurrence anchors (incident-narrative corpus). Weight
		// 1.5 so they out-score the DATA_QUERY "incident" keyword (1.0): for
		// "have we seen flooding incidents?" RAG=1.5 beats data_query=1.0, so the
		// experiential signal wins WITHOUT touching tieBreakOrder (no 1.0–1.0 tie).
		{"have we seen", 1.5},
		{"have we had", 1.5},
		{"has this happened", 1.5},
		{"happened before", 1.5},
		{"seen before", 1.5},
		{"ever had", 1.5},
		{"in the past", 1.5},
		{"similar incident", 1.5},
		{"guide", 0.5},
		{"manual", 0.5},
		{"replace", 0.5},
		{"repair", 0.5},
		{"install", 0.5},
		{"lubricate", 0.5},
		{"maintenance", 0.5},
	}},
	{IntentAction, []keyword{
		{"schedule", 1.0},
		{"book", 1.0},
		{"set up", 1.0},
	}},
}

// tieBreakOrder is the fixed precedence used to pick a winner and to break
// score ties — most specific first.
var tieBreakOrder = []Intent{IntentAction, IntentDataQuery, IntentRAG}

// ── Entity extraction ───────────────────────────────────────────────────────

var (
	// Digits after a context cue (elevator/device/etc.) — catches short IDs.
	idContextRe = regexp.MustCompile(`(?i)(?:\b(?:elevator|device|unit|elev|car|id)\b|#)\s*#?\s*(\d{1,8})`)
	// Standalone long digit runs — 5+ digits avoids matching a 4-digit year.
	idStandaloneRe = regexp.MustCompile(`\b(\d{5,8})\b`)

	isoDateRe   = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`)
	monthDateRe = regexp.MustCompile(`(?i)\b(january|february|march|april|may|june|july|august|september|october|november|december)\s+(\d{1,2})(?:,?\s+(\d{4}))?\b`)
)

// extractElevatorIDs returns all elevator IDs found, in order, de-duplicated.
func extractElevatorIDs(msg string) []string {
	var ids []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			ids = append(ids, s)
		}
	}
	for _, m := range idContextRe.FindAllStringSubmatch(msg, -1) {
		add(m[1])
	}
	for _, m := range idStandaloneRe.FindAllStringSubmatch(msg, -1) {
		add(m[1])
	}
	return ids
}

// extractDates returns dates normalized to "2006-01-02". A missing year defaults
// to now.Year(); passing now in keeps this function pure and tests deterministic.
func extractDates(msg string, now time.Time) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, m := range isoDateRe.FindAllStringSubmatch(msg, -1) {
		if t, err := time.Parse("2006-01-02", m[1]); err == nil {
			add(t.Format("2006-01-02"))
		}
	}
	for _, m := range monthDateRe.FindAllStringSubmatch(msg, -1) {
		month, day, year := m[1], m[2], m[3]
		if year == "" {
			year = strconv.Itoa(now.Year())
		}
		monthTitle := strings.ToUpper(month[:1]) + strings.ToLower(month[1:])
		cand := fmt.Sprintf("%s %s %s", monthTitle, day, year)
		if t, err := time.Parse("January 2 2006", cand); err == nil {
			add(t.Format("2006-01-02"))
		}
	}
	return out
}

// extractActionType maps action verbs to a canonical action name.
func extractActionType(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "schedule"), strings.Contains(lower, "book"), strings.Contains(lower, "set up"):
		return "schedule_inspection"
	case strings.Contains(lower, "replace"), strings.Contains(lower, "swap"):
		return "replace"
	}
	return ""
}

func extractEntities(msg string, now time.Time) Entities {
	return Entities{
		ElevatorIDs: extractElevatorIDs(msg),
		Dates:       extractDates(msg, now),
		ActionType:  extractActionType(msg),
	}
}

// ── Classifier ──────────────────────────────────────────────────────────────

// ClassifyIntent is the entry point. It is deterministic: same (msg, now) in,
// same Classification out.
func ClassifyIntent(msg string, now time.Time) Classification {
	trimmed := strings.TrimSpace(msg)
	ents := extractEntities(trimmed, now)

	if trimmed == "" {
		return Classification{
			Intent:   IntentAdvisory,
			Entities: ents,
			Reason:   "empty message; defaulting to advisory",
		}
	}

	lower := strings.ToLower(trimmed)
	scores := map[Intent]float64{}
	var signals []Signal
	actionVerb := false

	for _, grp := range keywordGroups {
		for _, kw := range grp.keywords {
			if strings.Contains(lower, kw.text) {
				scores[grp.intent] += kw.weight
				signals = append(signals, Signal{Intent: grp.intent, Keyword: kw.text, Weight: kw.weight})
				if grp.intent == IntentAction {
					actionVerb = true
				}
			}
		}
	}

	// Entity bonuses. An elevator ID suggests a specific-elevator lookup or an
	// action target; a date suggests scheduling.
	if len(ents.ElevatorIDs) > 0 {
		scores[IntentDataQuery] += 0.5
		scores[IntentAction] += 0.5
	}
	if len(ents.Dates) > 0 {
		scores[IntentAction] += 0.5
	}
	// A real action only fires strongly when a scheduling verb co-occurs with a
	// concrete target (ID or date) — keeps "steps to replace ..." in RAG.
	if actionVerb && (len(ents.ElevatorIDs) > 0 || len(ents.Dates) > 0) {
		scores[IntentAction] += 1.0
	}

	// Pick the winner using the fixed precedence (strict > keeps the first,
	// highest-priority intent on a tie).
	best := IntentAdvisory
	bestScore := 0.0
	for _, it := range tieBreakOrder {
		if scores[it] > bestScore {
			bestScore = scores[it]
			best = it
		}
	}

	if bestScore == 0 {
		return Classification{
			Intent:   IntentAdvisory,
			Entities: ents,
			Signals:  signals,
			Reason:   "no operational signals matched; defaulting to advisory",
		}
	}

	confidence := bestScore / (bestScore + 1.0)

	ties := 0
	for _, it := range tieBreakOrder {
		if scores[it] == bestScore {
			ties++
		}
	}

	if confidence < ConfidenceFloor {
		return Classification{
			Intent:     IntentAdvisory,
			Confidence: confidence,
			Entities:   ents,
			Signals:    signals,
			Reason: fmt.Sprintf("top intent %s confidence %.2f below floor %.2f; falling back to advisory",
				best, confidence, ConfidenceFloor),
		}
	}

	reason := fmt.Sprintf("classified as %s (confidence %.2f) from %d signal(s)", best, confidence, len(signals))
	if ties > 1 {
		reason += fmt.Sprintf("; tie at score %.1f resolved by precedence action>data_query>rag", bestScore)
	}

	return Classification{
		Intent:     best,
		Confidence: confidence,
		Entities:   ents,
		Signals:    signals,
		Reason:     reason,
	}
}

// ── Routing ─────────────────────────────────────────────────────────────────

// RouteResult names where a classified message should go.
type RouteResult struct {
	Target string
	Stub   bool
}

func routeIntent(c Classification) RouteResult {
	switch c.Intent {
	case IntentDataQuery:
		return RouteResult{Target: "mcp_data_tool"}
	case IntentRAG:
		return RouteResult{Target: "rag_search"}
	case IntentAction:
		return RouteResult{Target: "action_executor"}
	default:
		return RouteResult{Target: "advisory"}
	}
}
