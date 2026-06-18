package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// systemPromptBase is the OpsBot system prompt (PROMPT-1 / EVAL-1 deliverable),
// embedded at build time. The Dockerfile must COPY platform/api/prompts so this
// file is present during `go build`.
//
//go:embed prompts/system_prompt.md
var systemPromptBase string

func getOllamaURL() string {
	if v := os.Getenv("OLLAMA_URL"); v != "" {
		return v
	}
	return "http://localhost:11434"
}

func getOllamaModel() string {
	if v := os.Getenv("OLLAMA_MODEL"); v != "" {
		return v
	}
	return "mistral:latest"
}

// ollamaClient is shared across all calls so TCP connections to Ollama are
// pooled. No Timeout is set — callers pass a context that governs the deadline.
var ollamaClient = &http.Client{}

// ── Ollama client ─────────────────────────────────────────────────────────────

type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatReq struct {
	Model    string      `json:"model"`
	Messages []ollamaMsg `json:"messages"`
	Stream   bool        `json:"stream"`
}

type ollamaChatResp struct {
	Message ollamaMsg `json:"message"`
}

func callOllama(ctx context.Context, baseURL, model string, messages []ollamaMsg) (string, error) {
	payload, err := json.Marshal(ollamaChatReq{Model: model, Messages: messages, Stream: false})
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ngrok-skip-browser-warning", "true")

	resp, err := ollamaClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", err
		}
		return "", fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result ollamaChatResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return strings.TrimSpace(result.Message.Content), nil
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

// buildMCPArgs maps a classification and original message to an MCP tool name
// and arguments. Called only for data_query, rag, and action intents.
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
		return "search_maintenance_docs", map[string]any{"query": msg, "n_results": 5}

	case IntentAction:
		args := map[string]any{"confirmed": false, "reason": msg}
		if hasID {
			id, _ := strconv.Atoi(c.Entities.ElevatorIDs[0])
			args["elevator_id"] = id
		}
		if len(c.Entities.Dates) > 0 {
			args["inspection_date"] = c.Entities.Dates[0]
		}
		return "schedule_inspection", args

	default:
		return "get_fleet_stats", map[string]any{}
	}
}

// ── PostChat handler ──────────────────────────────────────────────────────────
//
// Classifies the user message, calls the appropriate MCP tool for data_query /
// rag / action intents, injects the result as live context into the system
// prompt, then calls Ollama. Falls back to advisory-only on MCP error.
func PostChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, ErrorResponse{Error: "invalid request body"})
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeJSON(w, 400, ErrorResponse{Error: "message is required"})
		return
	}

	classification := ClassifyIntent(msg, time.Now())
	route := routeIntent(classification)
	log.Printf("chat intent=%s confidence=%.2f route=%s stub=%t reason=%q",
		classification.Intent, classification.Confidence, route.Target, route.Stub, classification.Reason)

	var dataContext string
	if classification.Intent == IntentAction &&
		(len(classification.Entities.ElevatorIDs) == 0 || len(classification.Entities.Dates) == 0) {
		// Don't call the write tool with missing required fields — it would
		// ValidationError → silent advisory. Tell the model to ask for them.
		dataContext = "[ACTION NEEDS MORE INFO]\nThe user wants to schedule an inspection but did not provide both an elevator ID and a date. Ask them for whichever is missing before proceeding. Do not invent values."
	} else if classification.Intent == IntentDataQuery ||
		classification.Intent == IntentRAG ||
		classification.Intent == IntentAction {
		toolName, mcpArgs := buildMCPArgs(classification, msg)
		mcpCtx, mcpCancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer mcpCancel()
		if result, err := CallMCPTool(mcpCtx, toolName, mcpArgs); err != nil {
			log.Printf("mcp tool %s failed: %v — falling back to advisory", toolName, err)
		} else if isToolError(result) {
			log.Printf("mcp tool %s returned an error payload: %s — falling back to advisory", toolName, result)
		} else {
			dataContext = "[DATA SOURCE: PostgreSQL — live fleet data]\n" + result
		}
	}

	// Cap history at 10 turns (20 messages) — drop oldest pair first
	history := req.History
	for len(history) > 20 {
		history = history[2:]
	}

	systemContent := systemPromptBase
	if dataContext != "" {
		systemContent += "\n\n## Live Data Context\n" + dataContext
	}
	messages := []ollamaMsg{{Role: "system", Content: systemContent}}
	for _, h := range history {
		messages = append(messages, ollamaMsg{Role: h.Role, Content: h.Content})
	}
	messages = append(messages, ollamaMsg{Role: "user", Content: msg})

	ctx, cancel := context.WithTimeout(r.Context(), 330*time.Second)
	defer cancel()
	reply, err := callOllama(ctx, getOllamaURL(), getOllamaModel(), messages)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			writeJSON(w, 503, ErrorResponse{Error: "The assistant took too long to respond. Please try again."})
		case errors.Is(err, context.Canceled):
			// client disconnected — nothing to write
		case strings.Contains(err.Error(), "unreachable") || strings.Contains(err.Error(), "connection refused"):
			writeJSON(w, 503, ErrorResponse{Error: "Ollama is unreachable. Make sure Ollama is running."})
		default:
			writeJSON(w, 500, ErrorResponse{Error: "assistant failed to respond"})
		}
		return
	}

	updatedHistory := append(history,
		ChatMessage{Role: "user", Content: msg},
		ChatMessage{Role: "assistant", Content: reply},
	)

	writeJSON(w, 200, ChatResponse{Reply: reply, History: updatedHistory})
}

// WarmUpOllama sends a tiny request at startup so Ollama loads the model into
// memory. mistral:7b cold start exceeds 300s (EVAL-1); doing this once at boot
// keeps the first real user message fast. Safe to call in a goroutine.
func WarmUpOllama() {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	model := getOllamaModel()
	log.Printf("ollama warm-up starting (model %s)…", model)
	if _, err := callOllama(ctx, getOllamaURL(), model, []ollamaMsg{
		{Role: "user", Content: "ping"},
	}); err != nil {
		log.Printf("ollama warm-up failed (first chat may be slow): %v", err)
		return
	}
	log.Printf("ollama warm-up complete — model %s is resident", model)
}
