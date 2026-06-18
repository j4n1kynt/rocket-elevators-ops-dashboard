package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallLLMSuccess(t *testing.T) {
	var gotAuth, gotPath string
	var gotReq openAIChatReq

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"  hello  "}}]}`)
	}))
	defer srv.Close()

	reply, err := callLLM(context.Background(), srv.URL, "sk-test", "qwen/test", []llmMsg{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("callLLM: %v", err)
	}
	if reply != "hello" {
		t.Errorf("reply: got %q, want %q", reply, "hello")
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth header: got %q, want %q", gotAuth, "Bearer sk-test")
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path: got %q, want %q", gotPath, "/chat/completions")
	}
	if gotReq.Model != "qwen/test" {
		t.Errorf("model: got %q, want %q", gotReq.Model, "qwen/test")
	}
}

func TestCallLLMMissingKey(t *testing.T) {
	_, err := callLLM(context.Background(), "http://unused", "", "m", nil)
	if err == nil || !strings.Contains(err.Error(), "API_KEY is not set") {
		t.Errorf("expected missing-key error, got %v", err)
	}
}

func TestCallLLMNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"message":"rate limited"}}`)
	}))
	defer srv.Close()

	_, err := callLLM(context.Background(), srv.URL, "k", "m", []llmMsg{{Role: "user", Content: "x"}})
	if err == nil || !strings.Contains(err.Error(), "status 429") {
		t.Errorf("expected status 429 error, got %v", err)
	}
}

func TestCallLLMNoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	_, err := callLLM(context.Background(), srv.URL, "k", "m", []llmMsg{{Role: "user", Content: "x"}})
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Errorf("expected no-choices error, got %v", err)
	}
}

func TestCallLLMEmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"  "}}]}`)
	}))
	defer srv.Close()

	_, err := callLLM(context.Background(), srv.URL, "k", "m", []llmMsg{{Role: "user", Content: "x"}})
	if err == nil || !strings.Contains(err.Error(), "empty content") {
		t.Errorf("expected empty-content error, got %v", err)
	}
}
