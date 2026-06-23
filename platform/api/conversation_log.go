// S3-7 Step 4: Conversation logging module.
// Provides two functions used by the chat handler:
//   - EnsureConversation: creates a new conversation row when needed.
//   - LogTurn: saves the user and assistant messages in the background.

package main

import (
	"context"
	"log"
	"time"
)

// EnsureConversation returns the conversation id for this turn.
// If id == 0, a new row is inserted and its id is returned.
// If id > 0, the row must still exist: a stale id (DB reset, deleted row, or an
// edited hidden field) would make every later insert fail the foreign key and
// silently disable logging for the whole session. When the row is missing we
// create a fresh conversation instead and return its id.
// On any error the function logs a warning and returns 0.
// The caller can still reply to the user normally when this returns 0.
//
// This is the one synchronous step on the chat path: the HTTP response must
// carry the conversation id, so it cannot be deferred to the background. The
// timeout below bounds how long it can block; LogTurn is the part that never
// blocks the reply.
//
// Note (S3-7 design decision): the dashboard has no login or sessions, so any
// client may read or append to any conversation. The conversation id is not an
// ownership token — it only groups turns. This is intentional for an internal
// tool where all users share the same view.
func EnsureConversation(ctx context.Context, id int64) int64 {
	if db == nil {
		log.Printf("conversation logging: db pool is nil, skipping")
		return 0
	}

	// Short timeout — logging must not slow down the HTTP response. A single-row
	// read or insert that takes longer than this means the DB is already in
	// trouble, so giving up fast is better than holding the reply.
	tctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// Reuse a client-supplied id only when its row still exists.
	if id > 0 {
		var exists bool
		err := db.QueryRow(tctx,
			`SELECT EXISTS(SELECT 1 FROM conversations WHERE conversation_id = $1)`, id,
		).Scan(&exists)
		if err != nil {
			// Could not verify (transient DB error): keep the id as-is rather than
			// create a duplicate conversation on every error.
			log.Printf("conversation logging: could not verify conversation %d: %v", id, err)
			return id
		}
		if exists {
			return id
		}
		log.Printf("conversation logging: conversation %d not found, creating a new one", id)
	}

	var newID int64
	err := db.QueryRow(tctx,
		`INSERT INTO conversations DEFAULT VALUES RETURNING conversation_id`,
	).Scan(&newID)
	if err != nil {
		log.Printf("conversation logging: failed to create conversation: %v", err)
		return 0
	}

	return newID
}

// LogTurn saves the user message and the assistant reply for a conversation.
// It runs in the background and never blocks the caller.
// A failed write is logged and dropped — the chat is never affected.
// agent is the handler name for the assistant row; the user row always uses NULL.
//
// Both messages are written in a single multi-row INSERT. One statement is
// atomic in PostgreSQL, so a turn is all-or-nothing: there is no way to store
// the user row without the assistant row. It is also one round-trip, which
// keeps pressure off the connection pool.
//
// Failed turns are logged too: PostChat always reaches this call because Route
// returns a fallback reply on an LLM error or a recovered panic instead of an
// early return. So a rate-limit or provider error is still recorded (with the
// fallback text as the assistant reply), which is what conversation monitoring
// needs to see.
func LogTurn(conversationID int64, userMessage, reply, agent string) {
	if conversationID == 0 {
		return
	}

	go func() {
		// Catch any unexpected panic so a logging bug can never crash the server.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("conversation logging: panic in LogTurn goroutine: %v", r)
			}
		}()

		// Use a fresh context — the HTTP request may already be done.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Insert both rows at once. The user row always stores NULL for agent;
		// the assistant row stores the handler name (or NULL when empty).
		_, err := db.Exec(ctx,
			`INSERT INTO messages (conversation_id, role, content, agent)
			 VALUES ($1, 'user', $2, NULL),
			        ($1, 'assistant', $3, $4)`,
			conversationID, userMessage, reply, nullableString(agent),
		)
		if err != nil {
			log.Printf("conversation logging: failed to insert turn: %v", err)
		}
	}()
}
