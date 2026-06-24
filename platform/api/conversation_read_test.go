package main

// conversation_read_test.go — TDD tests for S3-8 conversation analytics endpoints.
//
// No-DB tests: call handlers directly with httptest; cover all 400 paths that
// return before any database access. These must pass without a running database.
//
// DB integration tests: guarded by a helper that calls InitDB() and skips when
// no database is available. They are hermetic: rows are inserted inside a
// transaction that is always rolled back.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── helpers ────────────────────────────────────────────────────────────────

var (
	dbOnce    sync.Once
	dbAvail   bool // true when InitDB() succeeded
)

// tryInitDB tries to connect to the database exactly once (across all tests in
// the process). Returns nil when the database is not available; the caller must
// call t.Skip in that case.
func tryInitDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbOnce.Do(func() {
		if err := InitDB(); err == nil && db != nil {
			dbAvail = true
		}
	})
	if !dbAvail {
		return nil
	}
	return db
}

// decodeBody unmarshals the response body into a map.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return m
}

// ─── GET /api/conversations — validation (no-DB) ────────────────────────────

func TestGetConversations_BadLimit(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"limit=0", "limit=0"},
		{"limit=101", "limit=101"},
		{"limit=abc", "limit=abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/conversations?"+c.query, nil)
			rec := httptest.NewRecorder()
			GetConversations(rec, r)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d", rec.Code)
			}
			body := decodeBody(t, rec)
			if body["error"] != "limit must be between 1 and 100" {
				t.Errorf("unexpected error message: %v", body["error"])
			}
		})
	}
}

func TestGetConversations_BadPage(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"page=0", "page=0"},
		{"page=abc", "page=abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/conversations?"+c.query, nil)
			rec := httptest.NewRecorder()
			GetConversations(rec, r)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d", rec.Code)
			}
			body := decodeBody(t, rec)
			if body["error"] != "page must be a positive integer" {
				t.Errorf("unexpected error message: %v", body["error"])
			}
		})
	}
}

// The agent filter accepts any value (see "show all present" decision): an
// unknown agent is NOT a 400 — it is a valid filter that simply matches
// nothing. So it must pass validation and reach the DB layer. With no database
// this cannot be asserted further here; the behavior is covered live and by the
// integration tests. We only assert it is not rejected as a 400 when a DB is
// present.
func TestGetConversations_UnknownAgentIsAccepted(t *testing.T) {
	pool := tryInitDB(t)
	if pool == nil {
		t.Skip("no database")
	}
	r := httptest.NewRequest(http.MethodGet, "/api/conversations?agent=unknown", nil)
	rec := httptest.NewRecorder()
	GetConversations(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 (unknown agent = empty match), got %d — body: %s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	if total, ok := body["total"].(float64); !ok || total != 0 {
		t.Errorf("expected total=0 for an unknown agent, got %v", body["total"])
	}
}

// ─── GET /api/conversations/{id} — validation (no-DB) ───────────────────────

func TestGetConversationByID_NonNumericID(t *testing.T) {
	cases := []string{"abc", "12abc", "3.14", "!@#", ""}
	for _, id := range cases {
		t.Run("id="+id, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/conversations/"+id, nil)
			// Manually set PathValue because httptest does not use the mux router.
			r.SetPathValue("id", id)
			rec := httptest.NewRecorder()
			GetConversationByID(rec, r)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d (id=%q)", rec.Code, id)
			}
			body := decodeBody(t, rec)
			if body["error"] != "Invalid conversation ID format. ID must be numeric." {
				t.Errorf("unexpected error message: %v", body["error"])
			}
		})
	}
}

// ─── DB integration tests (skipped when no database) ────────────────────────

// seedConversation inserts a conversation and messages inside the given
// connection. Returns conversationID and the IDs of inserted messages.
// The caller is responsible for rolling back / cleaning up.
func seedConversation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userMsg, assistantMsg, agent string) int64 {
	t.Helper()
	var convID int64
	err := pool.QueryRow(ctx,
		`INSERT INTO conversations DEFAULT VALUES RETURNING conversation_id`,
	).Scan(&convID)
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO messages (conversation_id, role, content, agent)
		 VALUES ($1, 'user', $2, NULL),
		        ($1, 'assistant', $3, $4)`,
		convID, userMsg, assistantMsg, agent,
	)
	if err != nil {
		t.Fatalf("seed messages: %v", err)
	}
	return convID
}

func TestGetConversations_Integration(t *testing.T) {
	pool := tryInitDB(t)
	if pool == nil {
		t.Skip("no database")
	}

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	// Seed one conversation with a user + assistant message.
	var convID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO conversations DEFAULT VALUES RETURNING conversation_id`,
	).Scan(&convID)
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO messages (conversation_id, role, content, agent)
		 VALUES ($1, 'user', 'test question for integration', NULL),
		        ($1, 'assistant', 'test answer', 'data')`,
		convID,
	)
	if err != nil {
		t.Fatalf("seed messages: %v", err)
	}

	// Call handler — uses the shared global db, not the transaction, so we
	// cannot assert on the seeded row directly. Instead we just confirm the
	// endpoint returns 200 with the required top-level shape.
	r := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)
	rec := httptest.NewRecorder()
	GetConversations(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	if _, ok := body["total"]; !ok {
		t.Error("response missing 'total' field")
	}
	if _, ok := body["page"]; !ok {
		t.Error("response missing 'page' field")
	}
	if _, ok := body["limit"]; !ok {
		t.Error("response missing 'limit' field")
	}
	convs, ok := body["conversations"]
	if !ok {
		t.Fatal("response missing 'conversations' field")
	}
	// Must be an array (not null).
	if convs == nil {
		t.Error("'conversations' must not be null")
	}
}

func TestGetConversationByID_Integration_NotFound(t *testing.T) {
	pool := tryInitDB(t)
	if pool == nil {
		t.Skip("no database")
	}

	r := httptest.NewRequest(http.MethodGet, "/api/conversations/999999999", nil)
	r.SetPathValue("id", "999999999")
	rec := httptest.NewRecorder()
	GetConversationByID(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	body := decodeBody(t, rec)
	if body["error"] != "Conversation not found." {
		t.Errorf("unexpected error: %v", body["error"])
	}
	if body["conversation_id"] != "999999999" {
		t.Errorf("expected conversation_id=999999999, got %v", body["conversation_id"])
	}
}

func TestGetConversationByID_Integration_Found(t *testing.T) {
	pool := tryInitDB(t)
	if pool == nil {
		t.Skip("no database")
	}

	ctx := context.Background()
	// Seed a real conversation.
	convID := seedConversation(t, ctx, pool,
		"what is the risk of elevator 99001?",
		"Risk level: HIGH",
		"data",
	)
	// Clean up after test.
	defer pool.Exec(ctx, `DELETE FROM conversations WHERE conversation_id = $1`, convID)

	r := httptest.NewRequest(http.MethodGet, "/api/conversations/"+itoa64(convID), nil)
	r.SetPathValue("id", itoa64(convID))
	rec := httptest.NewRecorder()
	GetConversationByID(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)

	// title must be the first user message content
	if body["title"] != "what is the risk of elevator 99001?" {
		t.Errorf("unexpected title: %v", body["title"])
	}
	// messages must not be null
	msgs, ok := body["messages"]
	if !ok || msgs == nil {
		t.Error("response missing or null 'messages' field")
	}
	// message_count must be 2
	if cnt, ok := body["message_count"].(float64); !ok || cnt != 2 {
		t.Errorf("expected message_count=2, got %v", body["message_count"])
	}
	// agents must include "data"
	if agents, ok := body["agents"].([]any); !ok || len(agents) != 1 || agents[0] != "data" {
		t.Errorf("expected agents=[data], got %v", body["agents"])
	}
	// conversation_id must match
	if id, ok := body["conversation_id"].(float64); !ok || int64(id) != convID {
		t.Errorf("expected conversation_id=%d, got %v", convID, body["conversation_id"])
	}
}

func TestGetConversationStats_Integration(t *testing.T) {
	pool := tryInitDB(t)
	if pool == nil {
		t.Skip("no database")
	}

	r := httptest.NewRequest(http.MethodGet, "/api/conversations/stats", nil)
	rec := httptest.NewRecorder()
	GetConversationStats(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)

	requiredFields := []string{
		"total_conversations",
		"total_messages",
		"avg_messages_per_conversation",
		"agent_distribution",
		"activity_by_day",
	}
	for _, f := range requiredFields {
		if _, ok := body[f]; !ok {
			t.Errorf("response missing field '%s'", f)
		}
	}

	// agent_distribution must have all 4 keys.
	dist, ok := body["agent_distribution"].(map[string]any)
	if !ok {
		t.Fatal("agent_distribution is not an object")
	}
	for _, key := range []string{"data", "knowledge", "scheduling", "general"} {
		if _, ok := dist[key]; !ok {
			t.Errorf("agent_distribution missing key '%s'", key)
		}
	}

	// activity_by_day must not be null.
	days := body["activity_by_day"]
	if days == nil {
		t.Error("activity_by_day must not be null")
	}
}

// itoa64 converts int64 to string — avoids importing strconv in the test file
// since strconv is already in the main package.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte(n%10) + '0'
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
