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
	"strings"
	"time"
)

// systemPromptBase is the OpsBot system prompt (PROMPT-1 / EVAL-1 deliverable),
// embedded at build time. The Dockerfile must COPY platform/api/prompts so this
// file is present during `go build`.
//
// OpsBot is advisory and educational only: it has NO live data access and does
// not look up individual elevators. This matches the behavior validated in
// docs/system-prompt-evaluation.md — no fleet data is injected into the prompt.
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

// ── PostChat handler ──────────────────────────────────────────────────────────
//
// Advisory-only: the model receives the OpsBot system prompt, the conversation
// history, and the latest user message. No live fleet data is fetched or injected
// — OpsBot answers from its embedded domain knowledge and directs users to the
// dashboard for any live-data request, exactly as validated in EVAL-1.
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

	// Cap history at 10 turns (20 messages) — drop oldest pair first
	history := req.History
	for len(history) > 20 {
		history = history[2:]
	}

	messages := []ollamaMsg{{Role: "system", Content: systemPromptBase}}
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
