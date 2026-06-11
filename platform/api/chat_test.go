package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ollamaChatRequest mirrors the JSON body that the chat handler should POST to
// Ollama's /api/chat endpoint. The mock server decodes into this struct so the
// test can inspect exactly what the handler forwarded.
type ollamaChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// newMockOllama starts a fake Ollama server. If gotReq is not nil, it records
// the request body it receives into gotReq. It always replies with replyText as
// the assistant message, using Ollama's non-streaming /api/chat response shape.
func newMockOllama(t *testing.T, replyText string, gotReq *ollamaChatRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotReq != nil {
			if err := json.NewDecoder(r.Body).Decode(gotReq); err != nil {
				t.Errorf("mock ollama: cannot decode request body: %v", err)
			}
		}
		resp := map[string]any{
			"message": map[string]string{
				"role":    "assistant",
				"content": replyText,
			},
			"done": true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// doChatRequest builds a POST /api/chat request from body, runs it through the
// handler, and returns the recorded response.
func doChatRequest(t *testing.T, handler http.HandlerFunc, body ChatRequest) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// Scenario 1: POST /api/chat with a valid body returns 200 and the bot reply.
func TestChat_ReturnsReply(t *testing.T) {
	const want = "There are 27,659 HIGH-risk elevators."
	mock := newMockOllama(t, want, nil)
	defer mock.Close()

	handler := NewChatHandler(mock.URL, "test-model")

	rec := doChatRequest(t, handler, ChatRequest{
		Message: "How many HIGH risk elevators are there?",
		History: []Message{},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp ChatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Reply != want {
		t.Errorf("Reply = %q, want %q", resp.Reply, want)
	}
}

// Scenario 2: the handler sends the system prompt first, then the past history,
// then the new user message. The response history is the full updated thread.
func TestChat_SendsSystemPromptAndHistory(t *testing.T) {
	var got ollamaChatRequest
	mock := newMockOllama(t, "ack", &got)
	defer mock.Close()

	handler := NewChatHandler(mock.URL, "test-model")

	history := []Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello, how can I help?"},
	}
	const newMsg = "How many elevators are HIGH risk?"

	rec := doChatRequest(t, handler, ChatRequest{
		Message: newMsg,
		History: history,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// First forwarded message must be a non-empty system prompt.
	if len(got.Messages) < 1 {
		t.Fatalf("ollama received no messages")
	}
	if got.Messages[0].Role != "system" || got.Messages[0].Content == "" {
		t.Errorf("first message = %+v, want a non-empty system prompt", got.Messages[0])
	}

	// After the system message: prior history in order, then the new user message.
	want := append([]Message{}, history...)
	want = append(want, Message{Role: "user", Content: newMsg})

	tail := got.Messages[1:]
	if len(tail) != len(want) {
		t.Fatalf("forwarded tail = %+v, want %+v", tail, want)
	}
	for i := range want {
		if tail[i] != want[i] {
			t.Errorf("forwarded message[%d] = %+v, want %+v", i, tail[i], want[i])
		}
	}

	// Response history must be old history + new user + assistant reply.
	var resp ChatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	wantHist := append([]Message{}, history...)
	wantHist = append(wantHist,
		Message{Role: "user", Content: newMsg},
		Message{Role: "assistant", Content: "ack"},
	)
	if len(resp.History) != len(wantHist) {
		t.Fatalf("response history = %+v, want %+v", resp.History, wantHist)
	}
	for i := range wantHist {
		if resp.History[i] != wantHist[i] {
			t.Errorf("response history[%d] = %+v, want %+v", i, resp.History[i], wantHist[i])
		}
	}
}

// Scenario 3: if Ollama is unreachable, the handler returns 503 with a clear
// JSON error message.
func TestChat_OllamaUnavailable(t *testing.T) {
	// Start a server to get a real URL, then close it so connections are refused.
	mock := newMockOllama(t, "unused", nil)
	url := mock.URL
	mock.Close()

	handler := NewChatHandler(url, "test-model")

	rec := doChatRequest(t, handler, ChatRequest{
		Message: "anything",
		History: []Message{},
	})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.Error == "" {
		t.Errorf("error message is empty, want a clear message")
	}
}
