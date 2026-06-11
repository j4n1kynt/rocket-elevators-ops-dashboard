package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// systemPrompt holds the OpsBot instructions. It is embedded at build time from
// prompts/system_prompt.md, so the path is correct no matter the working
// directory (go test runs in platform/api, go run starts from the project root).
//
//go:embed prompts/system_prompt.md
var systemPrompt string

// ollamaRequestTimeout caps how long we wait for the local model to answer.
const ollamaRequestTimeout = 60 * time.Second

// errOllamaUnreachable marks a failure to reach Ollama (connection refused,
// timeout, DNS). The handler maps it to HTTP 503; other errors map to 500.
var errOllamaUnreachable = errors.New("ollama unreachable")

// ollamaRequest is the body we POST to Ollama's /api/chat endpoint.
type ollamaRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// ollamaResponse is the non-streaming reply from Ollama's /api/chat endpoint.
type ollamaResponse struct {
	Message Message `json:"message"`
}

// ollamaClient talks to a local Ollama server. The base URL is injectable so
// tests can point it at a mock server.
type ollamaClient struct {
	baseURL string
	model   string
	http    *http.Client
}

func newOllamaClient(baseURL, model string) *ollamaClient {
	return &ollamaClient{
		baseURL: baseURL,
		model:   model,
		http:    &http.Client{Timeout: ollamaRequestTimeout},
	}
}

// chat sends the messages to Ollama and returns the assistant reply. A transport
// failure is wrapped with errOllamaUnreachable so the caller can return a 503.
func (c *ollamaClient) chat(messages []Message) (string, error) {
	body, err := json.Marshal(ollamaRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
	})
	if err != nil {
		return "", fmt.Errorf("build model request: %w", err)
	}

	resp, err := c.http.Post(c.baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("%w: %v", errOllamaUnreachable, err)
	}
	defer resp.Body.Close()

	var modelResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelResp); err != nil {
		return "", fmt.Errorf("decode model response: %w", err)
	}
	return modelResp.Message.Content, nil
}

// NewChatHandler builds the POST /api/chat handler. The Ollama base URL and
// model are injected so tests can point the handler at a mock server.
func NewChatHandler(ollamaURL, model string) http.HandlerFunc {
	client := newOllamaClient(ollamaURL, model)

	return func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
			return
		}

		if strings.TrimSpace(req.Message) == "" {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "message is required"})
			return
		}

		messages := buildMessages(systemPrompt, req.History, req.Message)

		reply, err := client.chat(messages)
		if err != nil {
			if errors.Is(err, errOllamaUnreachable) {
				writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "the assistant is currently unavailable; make sure Ollama is running"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "could not generate a reply"})
			return
		}

		// The response history is the prior history plus the new user/assistant pair.
		history := append([]Message{}, req.History...)
		history = append(history,
			Message{Role: "user", Content: req.Message},
			Message{Role: "assistant", Content: reply},
		)

		writeJSON(w, http.StatusOK, ChatResponse{Reply: reply, History: history})
	}
}

// buildMessages assembles the full message slice for the model: the system
// prompt first, the prior history in order, and the new user message last.
func buildMessages(systemPrompt string, history []Message, userMessage string) []Message {
	messages := make([]Message, 0, len(history)+2)
	messages = append(messages, Message{Role: "system", Content: systemPrompt})
	messages = append(messages, history...)
	messages = append(messages, Message{Role: "user", Content: userMessage})
	return messages
}
