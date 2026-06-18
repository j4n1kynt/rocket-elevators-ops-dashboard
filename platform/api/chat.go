package main

import (
	"bytes"
	"context"
	_ "embed"
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

// systemPromptBase is the OpsBot system prompt (PROMPT-1 / EVAL-1 deliverable),
// embedded at build time. The Dockerfile must COPY platform/api/prompts so this
// file is present during `go build`.
//
//go:embed prompts/system_prompt.md
var systemPromptBase string

func getOpenRouterBaseURL() string {
	if v := os.Getenv("OPENROUTER_BASE_URL"); v != "" {
		return v
	}
	return "https://openrouter.ai/api/v1"
}

func getOpenRouterModel() string {
	if v := os.Getenv("OPENROUTER_MODEL"); v != "" {
		return v
	}
	return "qwen/qwen3-next-80b-a3b-instruct:free"
}

func getOpenRouterKey() string {
	return os.Getenv("OPENROUTER_API_KEY")
}

// llmClient is shared across all calls so TCP connections to the provider are
// pooled. No Timeout is set — callers pass a context that governs the deadline.
var llmClient = &http.Client{}

// ── OpenRouter client (OpenAI-compatible) ──────────────────────────────────────

type llmMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatReq struct {
	Model    string   `json:"model"`
	Messages []llmMsg `json:"messages"`
	Stream   bool     `json:"stream"`
}

// openAIChatResp matches the OpenAI/OpenRouter response shape: the reply lives
// in choices[0].message.content, and an "error" object is returned on failure.
type openAIChatResp struct {
	Choices []struct {
		Message llmMsg `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func callLLM(ctx context.Context, baseURL, apiKey, model string, messages []llmMsg) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY is not set")
	}

	payload, err := json.Marshal(openAIChatReq{Model: model, Messages: messages, Stream: false})
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
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

	var result openAIChatResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("llm error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
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
		// confirmed=false is intentional: this sprint only supports the Phase 1
		// preview (tool returns a summary for the user to review). Phase 2
		// (confirmed=true → actual INSERT) requires a multi-turn confirmation
		// flow and is tracked as a future task.
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
// prompt, then calls the LLM (OpenRouter). Falls back to advisory-only on MCP error.
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
	messages := []llmMsg{{Role: "system", Content: systemContent}}
	for _, h := range history {
		messages = append(messages, llmMsg{Role: h.Role, Content: h.Content})
	}
	messages = append(messages, llmMsg{Role: "user", Content: msg})

	ctx, cancel := context.WithTimeout(r.Context(), 330*time.Second)
	defer cancel()
	reply, err := callLLM(ctx, getOpenRouterBaseURL(), getOpenRouterKey(), getOpenRouterModel(), messages)
	if err != nil {
		log.Printf("llm call failed: %v", err)
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			writeJSON(w, 503, ErrorResponse{Error: "The assistant took too long to respond. Please try again."})
		case errors.Is(err, context.Canceled):
			// client disconnected — nothing to write
		case strings.Contains(err.Error(), "API_KEY is not set"):
			writeJSON(w, 503, ErrorResponse{Error: "OPENROUTER_API_KEY is not set. Configure it before using the chat."})
		case strings.Contains(err.Error(), "status 401"):
			writeJSON(w, 503, ErrorResponse{Error: "OpenRouter rejected the API key (401). Check OPENROUTER_API_KEY."})
		case strings.Contains(err.Error(), "status 429"):
			writeJSON(w, 503, ErrorResponse{Error: "OpenRouter rate limit reached (429). Free models are limited — wait and retry."})
		case strings.Contains(err.Error(), "unreachable") || strings.Contains(err.Error(), "connection refused"):
			writeJSON(w, 503, ErrorResponse{Error: "The LLM provider is unreachable. Check your network or OPENROUTER_BASE_URL."})
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
