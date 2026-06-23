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
	var gotReq ollamaChatReq

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"message":{"role":"assistant","content":"  hello  "},"done":true}`)
	}))
	defer srv.Close()

	reply, err := callLLM(context.Background(), srv.URL, "sk-test", "minimax-m2.5:cloud", []llmMsg{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("callLLM: %v", err)
	}
	if reply != "hello" {
		t.Errorf("reply: got %q, want %q", reply, "hello")
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth header: got %q, want %q", gotAuth, "Bearer sk-test")
	}
	if gotPath != "/chat" {
		t.Errorf("path: got %q, want %q", gotPath, "/chat")
	}
	if gotReq.Model != "minimax-m2.5:cloud" {
		t.Errorf("model: got %q, want %q", gotReq.Model, "minimax-m2.5:cloud")
	}
}

func TestCallLLMMissingKey(t *testing.T) {
	_, err := callLLM(context.Background(), "http://unused", "", "m", nil)
	if err == nil || !strings.Contains(err.Error(), "OLLAMA_API_KEY is not set") {
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

func TestCallLLMEmptyMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"message":{"role":"assistant","content":""},"done":true}`)
	}))
	defer srv.Close()

	_, err := callLLM(context.Background(), srv.URL, "k", "m", []llmMsg{{Role: "user", Content: "x"}})
	if err == nil || !strings.Contains(err.Error(), "empty content") {
		t.Errorf("expected empty-content error, got %v", err)
	}
}

func TestCallLLMWhitespaceContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"message":{"role":"assistant","content":"  "},"done":true}`)
	}))
	defer srv.Close()

	_, err := callLLM(context.Background(), srv.URL, "k", "m", []llmMsg{{Role: "user", Content: "x"}})
	if err == nil || !strings.Contains(err.Error(), "empty content") {
		t.Errorf("expected empty-content error, got %v", err)
	}
}

func TestSignAndVerifyPendingAction(t *testing.T) {
	pa := &PendingAction{
		ElevatorID:     12345,
		InspectionDate: "2026-07-15",
		InspectionType: "Periodic",
		Reason:         "annual check",
	}
	pa.Signature = signPendingAction(pa)

	if !verifyPendingAction(pa) {
		t.Error("freshly signed pending_action should verify")
	}
}

func TestVerifyPendingActionRejectsTampering(t *testing.T) {
	pa := &PendingAction{
		ElevatorID:     12345,
		InspectionDate: "2026-07-15",
		InspectionType: "Periodic",
		Reason:         "annual check",
	}
	pa.Signature = signPendingAction(pa)

	// Tamper with each execution field; the signature must no longer verify.
	cases := []func(*PendingAction){
		func(p *PendingAction) { p.ElevatorID = 99999 },
		func(p *PendingAction) { p.InspectionDate = "2026-08-01" },
		func(p *PendingAction) { p.InspectionType = "Incident" },
		func(p *PendingAction) { p.Reason = "something else" },
	}
	for i, mutate := range cases {
		tampered := *pa
		mutate(&tampered)
		if verifyPendingAction(&tampered) {
			t.Errorf("case %d: tampered pending_action must not verify", i)
		}
	}
}

func TestVerifyPendingActionRejectsUnsigned(t *testing.T) {
	pa := &PendingAction{ElevatorID: 12345, InspectionDate: "2026-07-15"}
	if verifyPendingAction(pa) {
		t.Error("pending_action with empty signature must not verify")
	}
}

func TestDetectConfirmation(t *testing.T) {
	cases := []struct {
		msg                string
		wantConf, wantCanc bool
	}{
		{"yes", true, false},
		{"confirm", true, false},
		{"yes please", true, false},
		{"ok go ahead", true, false},
		{"no", false, true},
		{"cancel", false, true},
		{"no thanks", false, true},
		{"nevermind", false, true},
		// Neither — must abandon the pending action and reclassify (spec §7.2).
		{"what about elevator 123", false, false},
		{"schedule it for next week", false, false},
		// Word-level matching: "know" must not match "no".
		{"i don't know", false, false},
	}
	for _, c := range cases {
		gotConf, gotCanc := detectConfirmation(c.msg)
		if gotConf != c.wantConf || gotCanc != c.wantCanc {
			t.Errorf("detectConfirmation(%q) = (conf=%t, canc=%t), want (conf=%t, canc=%t)",
				c.msg, gotConf, gotCanc, c.wantConf, c.wantCanc)
		}
	}
}

func TestHasConfidentResults(t *testing.T) {
	cases := []struct {
		name string
		json string
		want bool
	}{
		{"confident hit", `{"total_returned":3,"results":[{"text":"x"}]}`, true},
		{"no confident match", `{"total_returned":0,"message":"No confident matches found","results":[]}`, false},
		{"missing field", `{"results":[]}`, false},
		{"malformed json", `not json at all`, false},
	}
	for _, c := range cases {
		if got := hasConfidentResults(c.json); got != c.want {
			t.Errorf("%s: hasConfidentResults(%q) = %t, want %t", c.name, c.json, got, c.want)
		}
	}
}

func TestCapReason(t *testing.T) {
	if got := capReason("short reason"); got != "short reason" {
		t.Errorf("short reason should pass through unchanged, got %q", got)
	}
	long := strings.Repeat("a", 600)
	if got := capReason(long); len([]rune(got)) != maxReasonLen {
		t.Errorf("over-length reason should be capped to %d runes, got %d", maxReasonLen, len([]rune(got)))
	}
	// Multibyte: 600 runes must cap to exactly maxReasonLen runes, not split bytes.
	multibyte := strings.Repeat("é", 600)
	got := capReason(multibyte)
	if len([]rune(got)) != maxReasonLen {
		t.Errorf("multibyte reason should cap to %d runes, got %d", maxReasonLen, len([]rune(got)))
	}
}
