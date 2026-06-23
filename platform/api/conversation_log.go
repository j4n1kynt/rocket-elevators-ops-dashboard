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
// If id > 0, it is returned as-is without touching the database.
// If id == 0, a new row is inserted and its id is returned.
// On any error the function logs a warning and returns 0.
// The caller can still reply to the user normally when this returns 0.
func EnsureConversation(ctx context.Context, id int64) int64 {
	if id > 0 {
		return id
	}

	if db == nil {
		log.Printf("conversation logging: db pool is nil, skipping")
		return 0
	}

	// Short timeout — logging must not slow down the HTTP response.
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

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

		// Insert user message (agent is always NULL for user rows).
		if err := insertMessage(ctx, conversationID, "user", userMessage, nil); err != nil {
			log.Printf("conversation logging: failed to insert user message: %v", err)
			return
		}

		// Insert assistant message (store NULL when agent is empty string).
		agentPtr := nullableString(agent)
		if err := insertMessage(ctx, conversationID, "assistant", reply, agentPtr); err != nil {
			log.Printf("conversation logging: failed to insert assistant message: %v", err)
			return
		}

		// Update the message count — two messages added this turn.
		_, err := db.Exec(ctx,
			`UPDATE conversations SET message_count = message_count + 2 WHERE conversation_id = $1`,
			conversationID,
		)
		if err != nil {
			log.Printf("conversation logging: failed to update message_count: %v", err)
		}
	}()
}

// insertMessage inserts a single message row.
// agent is a pointer so pgx writes SQL NULL when it is nil.
func insertMessage(ctx context.Context, convID int64, role, content string, agent *string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO messages (conversation_id, role, content, agent)
		 VALUES ($1, $2, $3, $4)`,
		convID, role, content, agent,
	)
	return err
}
