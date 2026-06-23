package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// systemPromptBase is the original monolithic OpsBot prompt (PROMPT-1 / EVAL-1).
// Retained for reference; agents now use the focused prompts below.
//
//go:embed prompts/system_prompt.md
var systemPromptBase string

//go:embed prompts/general_prompt.md
var generalPrompt string

//go:embed prompts/data_prompt.md
var dataPrompt string

//go:embed prompts/knowledge_prompt.md
var knowledgePrompt string

//go:embed prompts/scheduling_prompt.md
var schedulingPrompt string

func getOllamaBaseURL() string {
	if v := os.Getenv("OLLAMA_BASE_URL"); v != "" {
		return v
	}
	return "https://ollama.com/api"
}

func getOllamaModel() string {
	if v := os.Getenv("OLLAMA_MODEL"); v != "" {
		return v
	}
	return "minimax-m2.5:cloud"
}

func getOllamaKey() string {
	return os.Getenv("OLLAMA_API_KEY")
}

// llmClient is shared across all calls so TCP connections to the provider are
// pooled. No Timeout is set — callers pass a context that governs the deadline.
var llmClient = &http.Client{}

// ── Ollama cloud client ────────────────────────────────────────────────────────

type llmMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatReq struct {
	Model    string   `json:"model"`
	Messages []llmMsg `json:"messages"`
	Stream   bool     `json:"stream"`
}

// ollamaChatResp matches the Ollama native /api/chat response shape: the reply
// lives in message.content and done=true signals a complete non-streamed response.
type ollamaChatResp struct {
	Message llmMsg `json:"message"`
	Done    bool   `json:"done"`
}

func callLLM(ctx context.Context, baseURL, apiKey, model string, messages []llmMsg) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("OLLAMA_API_KEY is not set")
	}

	payload, err := json.Marshal(ollamaChatReq{Model: model, Messages: messages, Stream: false})
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := llmClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", err
		}
		return "", fmt.Errorf("llm provider unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("llm returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result ollamaChatResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	content := strings.TrimSpace(result.Message.Content)
	if content == "" {
		return "", fmt.Errorf("llm returned empty content")
	}
	return content, nil
}

// WarmUpLLM fires a single no-op request to Ollama at server startup so the
// model is loaded before the first real user request arrives. Runs in a
// goroutine — never blocks startup and any error is logged and discarded.
func WarmUpLLM() {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	_, err := callLLM(ctx, getOllamaBaseURL(), getOllamaKey(), getOllamaModel(),
		[]llmMsg{{Role: "user", Content: "ping"}},
	)
	if err != nil {
		log.Printf("[llm warm-up] failed: %v", err)
		return
	}
	log.Printf("[llm warm-up] done")
}

// ── buildMCPArgs / isToolError ────────────────────────────────────────────────

// isToolError reports whether a tool's JSON payload is an application-level
// error envelope ({"error": true, ...}) rather than real data.
func isToolError(jsonText string) bool {
	var probe struct {
		Error bool `json:"error"`
	}
	_ = json.Unmarshal([]byte(jsonText), &probe)
	return probe.Error
}

// hasConfidentResults reports whether a search tool payload returned at least one
// result that cleared the tool's similarity threshold (total_returned > 0). The
// advisory RAG fallback uses it to decide whether to ground the answer in the
// manuals or stay in advisory mode — a 0 count means nothing relevant was found.
func hasConfidentResults(jsonText string) bool {
	var probe struct {
		TotalReturned int `json:"total_returned"`
	}
	if err := json.Unmarshal([]byte(jsonText), &probe); err != nil {
		return false
	}
	return probe.TotalReturned > 0
}

// shouldTryRagFallback reports whether an advisory-classified message is
// substantive enough to justify a semantic maintenance-doc search. The keyword
// classifier (intent.go) misses natural-language procedural questions like
// "what do I do when hydraulic pressure drops?", which spec FEATURE-3 requires
// us to answer. Rather than enumerate domain keywords — the exact brittleness we
// are fixing — this gate is content-agnostic: a real question is either phrased
// as one or long enough to carry intent. It only filters trivial chatter
// (greetings, "thanks") so we do not pay embedding latency on non-questions.
func shouldTryRagFallback(msg string) bool {
	trimmed := strings.TrimSpace(msg)
	if strings.Contains(trimmed, "?") {
		return true
	}
	return len(strings.Fields(trimmed)) >= 4
}

// extractScheduleError returns the error message from a schedule_inspection
// payload where success=false and the "error" field is a non-empty string.
// Returns "" when the payload is not an error (e.g. pending_confirmation or success).
func extractScheduleError(jsonText string) string {
	var probe struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal([]byte(jsonText), &probe); err != nil {
		return ""
	}
	if !probe.Success && probe.Error != "" {
		return probe.Error
	}
	return ""
}

// buildPendingAction checks whether a schedule_inspection tool result is a
// successful Phase 1 response (pending_confirmation=true) and, if so, builds
// the PendingAction that must be stored client-side until the user confirms.
// Returns nil for errors, Phase 2 successes, and non-scheduling results.
func buildPendingAction(jsonText string, ents Entities, reason string) *PendingAction {
	var probe struct {
		PendingConfirmation bool   `json:"pending_confirmation"`
		Summary             string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(jsonText), &probe); err != nil {
		return nil
	}
	if !probe.PendingConfirmation || probe.Summary == "" {
		return nil
	}
	elevatorID := 0
	if len(ents.ElevatorIDs) > 0 {
		elevatorID, _ = strconv.Atoi(ents.ElevatorIDs[0])
	}
	date := ""
	if len(ents.Dates) > 0 {
		date = ents.Dates[0]
	}
	return &PendingAction{
		ElevatorID:     elevatorID,
		InspectionDate: date,
		InspectionType: ents.InspectionType,
		Reason:         reason,
		Summary:        probe.Summary,
	}
}

// cleanValidationError strips Pydantic boilerplate from MCP validation error
// messages so the LLM receives a concise, user-facing description.
// Input: "tool error: 1 validation error for ScheduleInspectionInput\nfield\n  Value error, <msg>"
// Output: "<msg>"
func cleanValidationError(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "Value error, "); ok {
			return after
		}
	}
	return raw
}

// incidentNarrativeCues are experiential / recurrence phrases that mean "has
// this kind of thing happened before?". They mirror the experiential anchors in
// intent.go's RAG keyword group so an experiential RAG question lands on the
// narrative corpus. The list is intentionally substring-based to match the rest
// of this file's style.
//
// No bare "incident" cue here on purpose: a procedural RAG question like
// "what's the procedure for reporting an incident?" already wins IntentRAG via
// "procedure" (1.5) and must route to the maintenance manuals, not the narrative
// corpus. The 8 experiential phrases below fully capture the recurrence intent
// without that false positive.
var incidentNarrativeCues = []string{
	"have we seen",
	"have we had",
	"has this happened",
	"happened before",
	"seen before",
	"ever had",
	"in the past",
	"similar incident",
}

// isIncidentNarrativeQuery reports whether a RAG message should search past
// incident narratives instead of the maintenance manuals. lower must already be
// lowercased.
func isIncidentNarrativeQuery(lower string) bool {
	for _, cue := range incidentNarrativeCues {
		if strings.Contains(lower, cue) {
			return true
		}
	}
	return false
}

// buildMCPArgs maps a classification and original message to an MCP tool name
// and arguments. Called only for data_query, rag, and action intents.
//
// TODO(S3-3): accept allowedTools []string and guard the returned tool name
// against it. Callers (dataAgent, knowledgeAgent, schedulingAgent) should pass
// req.AllowedTools so cross-agent tool calls are rejected before hitting MCP.
func buildMCPArgs(c Classification, msg string) (string, map[string]any) {
	lower := strings.ToLower(msg)
	hasID := len(c.Entities.ElevatorIDs) > 0

	switch c.Intent {
	case IntentDataQuery:
		switch {
		case strings.Contains(lower, "shutdown") || strings.Contains(lower, "shut down") ||
			strings.Contains(lower, "tssa") || strings.Contains(lower, "out of service"):
			return "get_tssa_shutdown_elevators", map[string]any{"limit": 20}

		case hasID && (strings.Contains(lower, "inspection history") ||
			strings.Contains(lower, "past inspection") ||
			strings.Contains(lower, "last inspected")):
			id, _ := strconv.Atoi(c.Entities.ElevatorIDs[0])
			return "get_inspection_history", map[string]any{"elevator_id": id, "limit": 10}

		case hasID && strings.Contains(lower, "incident"):
			id, _ := strconv.Atoi(c.Entities.ElevatorIDs[0])
			return "get_elevator_incidents", map[string]any{"elevator_id": id, "limit": 10}

		case strings.Contains(lower, "follow") || strings.Contains(lower, "overdue"):
			return "get_elevators_needing_followup", map[string]any{"limit": 20}

		case strings.Contains(lower, "incident"):
			return "get_incident_count_last_year", map[string]any{}

		case hasID && strings.Contains(lower, "risk"):
			id, _ := strconv.Atoi(c.Entities.ElevatorIDs[0])
			return "get_elevator_risk", map[string]any{"elevator_id": id}

		default:
			// A query naming a specific elevator but matching no sub-keyword
			// (e.g. "status of elevator 12345") should look up that elevator,
			// not return fleet-wide aggregates.
			if hasID {
				id, _ := strconv.Atoi(c.Entities.ElevatorIDs[0])
				return "get_elevator_risk", map[string]any{"elevator_id": id}
			}
			return "get_fleet_stats", map[string]any{}
		}

	case IntentRAG:
		// RAG splits across two corpora. Experiential / recurrence questions
		// ("have we seen ... incidents?") search past incident narratives;
		// everything else searches the maintenance manuals. The narrative tool
		// uses `limit`; the manual tool keeps `n_results` (pinned contracts).
		if isIncidentNarrativeQuery(lower) {
			return "search_incident_narratives", map[string]any{"query": msg, "limit": 5}
		}
		return "search_maintenance_docs", map[string]any{"query": msg, "n_results": 5}

	case IntentAction:
		// Phase 1 only — confirmed=false returns a summary for the user to review.
		// Phase 2 (confirmed=true) is triggered by detectConfirmation in PostChat.
		args := map[string]any{"confirmed": false, "reason": capReason(msg)}
		if hasID {
			id, _ := strconv.Atoi(c.Entities.ElevatorIDs[0])
			args["elevator_id"] = id
		}
		if len(c.Entities.Dates) > 0 {
			args["inspection_date"] = c.Entities.Dates[0]
		}
		if c.Entities.InspectionType != "" {
			args["inspection_type"] = c.Entities.InspectionType
		}
		return "schedule_inspection", args

	default:
		return "get_fleet_stats", map[string]any{}
	}
}

// maxReasonLen mirrors the MCP server's Pydantic limit on the reason field.
// Capping here turns an over-length scheduling message into a truncated reason
// instead of a hard validation failure on Phase 1.
const maxReasonLen = 500

// capReason trims a scheduling reason to maxReasonLen, cutting on a rune
// boundary so multibyte characters are never split. The message is already
// whitespace-trimmed by the caller, so the result satisfies the server's
// "len(stripped) <= 500" check.
func capReason(s string) string {
	r := []rune(s)
	if len(r) <= maxReasonLen {
		return s
	}
	return string(r[:maxReasonLen])
}

// pendingActionTTL bounds how long a signed confirmation stays valid, so a
// pending_action cannot be replayed after the previewed inspection has left the
// 'Pending' state. Enforced in Phase 2 via PendingAction.ExpiresAt.
const pendingActionTTL = 10 * time.Minute

// chatSigningSecret signs pending scheduling actions (see PendingAction.Signature).
// Set CHAT_SIGNING_SECRET to keep signatures valid across restarts; otherwise an
// ephemeral per-process secret is used, which safely invalidates any pending
// confirmation that spans a restart.
var chatSigningSecret = loadSigningSecret()

func loadSigningSecret() []byte {
	if v := os.Getenv("CHAT_SIGNING_SECRET"); v != "" {
		return []byte(v)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is effectively fatal for signing; fall back to a
		// fixed value so the process still runs (signatures remain consistent
		// within the process, which is all that is required).
		log.Printf("WARNING: crypto/rand unavailable for CHAT_SIGNING_SECRET: %v — using a static fallback", err)
		return []byte("rocket-elevators-static-fallback-secret")
	}
	log.Printf("CHAT_SIGNING_SECRET not set — using an ephemeral signing secret (pending confirmations will not survive a restart)")
	return b
}

// signPendingAction computes the HMAC over the execution fields. Summary is
// intentionally excluded — it is cosmetic and never used to drive the write.
func signPendingAction(pa *PendingAction) string {
	mac := hmac.New(sha256.New, chatSigningSecret)
	fmt.Fprintf(mac, "%d\n%s\n%s\n%s\n%d", pa.ElevatorID, pa.InspectionDate, pa.InspectionType, pa.Reason, pa.ExpiresAt)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyPendingAction reports whether a client-supplied pending_action carries a
// valid signature for its current field values (constant-time comparison).
func verifyPendingAction(pa *PendingAction) bool {
	if pa.Signature == "" {
		return false
	}
	want := signPendingAction(pa)
	return hmac.Equal([]byte(want), []byte(pa.Signature))
}

// detectConfirmation reports whether a message is a scheduling confirmation or
// cancellation. Word-level matching avoids false positives ("know" ≠ "no").
// Called only when a pending_action is present in the request (spec §7.2).
func detectConfirmation(msg string) (isConfirm, isCancel bool) {
	for _, w := range strings.Fields(strings.ToLower(msg)) {
		switch w {
		case "yes", "confirm", "confirmed", "proceed", "approve", "ok", "okay", "sure":
			isConfirm = true
		case "no", "cancel", "nope", "stop", "abort", "decline", "nevermind":
			isCancel = true
		}
	}
	return
}

// ── PostChat handler ──────────────────────────────────────────────────────────
//
// Thin HTTP adapter. Decodes the request, builds an AgentRequest, dispatches
// to Route, and serialises the AgentResponse back to the caller.
func PostChat(w http.ResponseWriter, r *http.Request) {
	var chatReq ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		writeJSON(w, 400, ErrorResponse{Error: "invalid request body"})
		return
	}
	msg := strings.TrimSpace(chatReq.Message)
	if msg == "" {
		writeJSON(w, 400, ErrorResponse{Error: "message is required"})
		return
	}

	// Cap history at 10 turns (20 messages) — drop oldest pair first.
	history := chatReq.History
	for len(history) > 20 {
		history = history[2:]
	}

	ctx, cancel := context.WithTimeout(r.Context(), 330*time.Second)
	defer cancel()

	agentResp := Route(ctx, AgentRequest{
		Message:       msg,
		History:       history,
		PendingAction: chatReq.PendingAction,
	})

	// Log the conversation (best-effort). EnsureConversation does one bounded
	// query so it can return the id in the response; LogTurn never blocks. The
	// agent name comes from the AgentResponse so the logged value matches the
	// multi-agent router vocabulary (general/data/knowledge/scheduling).
	convID := EnsureConversation(r.Context(), chatReq.ConversationID)
	LogTurn(convID, msg, agentResp.Reply, agentResp.AgentName)

	writeJSON(w, 200, ChatResponse{
		Reply:          agentResp.Reply,
		History:        agentResp.UpdatedHistory,
		PendingAction:  agentResp.PendingAction,
		ConversationID: convID,
	})
}
