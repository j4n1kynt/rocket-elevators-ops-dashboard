// S3-8: Conversation analytics read endpoints.
// Three read-only handlers for the conversation analytics page:
//   - GetConversations:      GET /api/conversations
//   - GetConversationByID:   GET /api/conversations/{id}
//   - GetConversationStats:  GET /api/conversations/stats

package main

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// canonicalAgents are the agents the current router produces. They always
// appear in the stats agent distribution (at 0 when absent), but the ?agent=
// filter accepts any value present in the data — older logged conversations
// may carry legacy agent names (e.g. "advisory", "mcp_data_tool").
var canonicalAgents = []string{"data", "knowledge", "scheduling", "general"}

// truncateTitle trims s and truncates it to 120 runes, appending "…" when cut.
func truncateTitle(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > 120 {
		return string(runes[:120]) + "…"
	}
	return s
}

// fmtTimestamp formats a time.Time value as RFC3339 UTC.
func fmtTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// GetConversations handles GET /api/conversations.
// Validation returns 400 before any DB access.
func GetConversations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// ── Parameter parsing & validation (FIRST — no DB calls before this) ──

	page := 1
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSON(w, 400, ErrorResponse{Error: "page must be a positive integer"})
			return
		}
		page = n
	}

	limit := 20
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			writeJSON(w, 400, ErrorResponse{Error: "limit must be between 1 and 100"})
			return
		}
		limit = n
	}

	search := q.Get("q")
	// The agent filter accepts any value: it filters to conversations that have
	// at least one assistant message by that agent. An unknown value simply
	// matches nothing (empty list), which is the honest result. Values are always
	// used as parameterised args, never interpolated.
	agentFilter := q.Get("agent")

	// ── Build WHERE clause (all conditions use parameterised args) ──

	var conds []string
	var args []any

	if search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		conds = append(conds, fmt.Sprintf(
			`EXISTS (
				SELECT 1 FROM messages ms
				WHERE ms.conversation_id = c.conversation_id
				  AND LOWER(ms.content) LIKE $%d
			)`, len(args)))
	}
	if agentFilter != "" {
		args = append(args, agentFilter)
		conds = append(conds, fmt.Sprintf(
			`EXISTS (
				SELECT 1 FROM messages ms
				WHERE ms.conversation_id = c.conversation_id
				  AND ms.role = 'assistant'
				  AND ms.agent = $%d
			)`, len(args)))
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	// ── Count total matching conversations ──

	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM conversations c %s`, where)
	var total int
	if err := db.QueryRow(r.Context(), countSQL, args...).Scan(&total); err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}

	conversations := []ConversationSummary{}
	offset := (page - 1) * limit

	if offset < total {
		// Append LIMIT and OFFSET as the next positional args.
		args = append(args, limit)
		limitArg := len(args)
		args = append(args, offset)
		offsetArg := len(args)

		querySQL := fmt.Sprintf(`
			WITH msg_agg AS (
				SELECT
					conversation_id,
					COUNT(*)                                       AS message_count,
					MAX(created_at)                               AS last_message_at,
					ARRAY_AGG(DISTINCT agent ORDER BY agent)
						FILTER (WHERE agent IS NOT NULL)           AS agents,
					MIN(created_at) FILTER (WHERE role = 'user') AS first_user_at,
					MIN(message_id) FILTER (WHERE role = 'user') AS first_user_id
				FROM messages
				GROUP BY conversation_id
			),
			first_user_msg AS (
				SELECT DISTINCT ON (m.conversation_id)
					m.conversation_id,
					m.content AS title
				FROM messages m
				WHERE m.role = 'user'
				ORDER BY m.conversation_id, m.created_at ASC, m.message_id ASC
			)
			SELECT
				c.conversation_id,
				c.started_at,
				COALESCE(ma.last_message_at, c.started_at) AS last_activity_at,
				COALESCE(ma.message_count, 0)              AS message_count,
				COALESCE(ma.agents, ARRAY[]::text[])       AS agents,
				COALESCE(fum.title, 'New conversation')    AS title
			FROM conversations c
			LEFT JOIN msg_agg ma ON ma.conversation_id = c.conversation_id
			LEFT JOIN first_user_msg fum ON fum.conversation_id = c.conversation_id
			%s
			ORDER BY last_activity_at DESC, c.conversation_id DESC
			LIMIT $%d OFFSET $%d`,
			where, limitArg, offsetArg,
		)

		rows, err := db.Query(r.Context(), querySQL, args...)
		if err != nil {
			writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var (
				convID         int64
				startedAt      time.Time
				lastActivityAt time.Time
				messageCount   int
				agents         []string
				title          string
			)
			if err := rows.Scan(&convID, &startedAt, &lastActivityAt, &messageCount, &agents, &title); err != nil {
				writeJSON(w, 500, ErrorResponse{Error: "scan failed"})
				return
			}
			if agents == nil {
				agents = []string{}
			}
			conversations = append(conversations, ConversationSummary{
				ConversationID: convID,
				StartedAt:      fmtTimestamp(startedAt),
				LastActivityAt: fmtTimestamp(lastActivityAt),
				MessageCount:   messageCount,
				Agents:         agents,
				Title:          truncateTitle(title),
			})
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, 500, ErrorResponse{Error: "row iteration failed"})
			return
		}
	}

	writeJSON(w, 200, ConversationListResponse{
		Total:         total,
		Page:          page,
		Limit:         limit,
		Conversations: conversations,
	})
}

// GetConversationByID handles GET /api/conversations/{id}.
func GetConversationByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")

	if !isNumeric(idStr) {
		writeJSON(w, 400, ErrorResponse{Error: "Invalid conversation ID format. ID must be numeric."})
		return
	}

	convID, _ := strconv.ParseInt(idStr, 10, 64)

	// ── Fetch summary fields for the conversation ──

	var (
		startedAt      time.Time
		lastActivityAt time.Time
		messageCount   int
		agents         []string
		title          string
	)
	err := db.QueryRow(r.Context(), `
		WITH msg_agg AS (
			SELECT
				conversation_id,
				COUNT(*)                                       AS message_count,
				MAX(created_at)                               AS last_message_at,
				ARRAY_AGG(DISTINCT agent ORDER BY agent)
					FILTER (WHERE agent IS NOT NULL)           AS agents
			FROM messages
			WHERE conversation_id = $1
			GROUP BY conversation_id
		),
		first_user_msg AS (
			SELECT content
			FROM messages
			WHERE conversation_id = $1 AND role = 'user'
			ORDER BY created_at ASC, message_id ASC
			LIMIT 1
		)
		SELECT
			c.started_at,
			COALESCE(ma.last_message_at, c.started_at) AS last_activity_at,
			COALESCE(ma.message_count, 0)              AS message_count,
			COALESCE(ma.agents, ARRAY[]::text[])       AS agents,
			COALESCE((SELECT content FROM first_user_msg), 'New conversation') AS title
		FROM conversations c
		LEFT JOIN msg_agg ma ON ma.conversation_id = c.conversation_id
		WHERE c.conversation_id = $1`,
		convID,
	).Scan(&startedAt, &lastActivityAt, &messageCount, &agents, &title)

	// No row → the conversation does not exist (404). Any other error is a real
	// DB failure (500) — do not mask it as "not found".
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, 404, ErrorResponse{Error: "Conversation not found.", ConversationID: idStr})
		return
	}
	if err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}

	if agents == nil {
		agents = []string{}
	}

	// ── Fetch messages ordered by created_at ASC, message_id ASC ──

	msgRows, err := db.Query(r.Context(), `
		SELECT message_id, role, content, agent, created_at
		FROM messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC, message_id ASC`,
		convID,
	)
	if err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}
	defer msgRows.Close()

	messages := []Message{}
	for msgRows.Next() {
		var (
			msgID     int64
			role      string
			content   string
			agent     *string
			createdAt time.Time
		)
		if err := msgRows.Scan(&msgID, &role, &content, &agent, &createdAt); err != nil {
			writeJSON(w, 500, ErrorResponse{Error: "scan failed"})
			return
		}
		messages = append(messages, Message{
			MessageID: msgID,
			Role:      role,
			Content:   content,
			Agent:     agent,
			CreatedAt: fmtTimestamp(createdAt),
		})
	}
	if err := msgRows.Err(); err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "row iteration failed"})
		return
	}

	writeJSON(w, 200, ConversationDetail{
		ConversationID: convID,
		StartedAt:      fmtTimestamp(startedAt),
		LastActivityAt: fmtTimestamp(lastActivityAt),
		MessageCount:   messageCount,
		Agents:         agents,
		Title:          truncateTitle(title),
		Messages:       messages,
	})
}

// GetConversationStats handles GET /api/conversations/stats.
func GetConversationStats(w http.ResponseWriter, r *http.Request) {
	// ── Total conversations and messages ──

	var totalConversations, totalMessages int
	err := db.QueryRow(r.Context(), `
		SELECT
			(SELECT COUNT(*) FROM conversations),
			(SELECT COUNT(*) FROM messages)`,
	).Scan(&totalConversations, &totalMessages)
	if err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}

	avgMsgs := 0.0
	if totalConversations > 0 {
		raw := float64(totalMessages) / float64(totalConversations)
		// Round to 1 decimal place.
		avgMsgs = math.Round(raw*10) / 10
	}

	// ── Agent distribution — count assistant messages per agent ──
	// Count every agent present in the data (including legacy values from early
	// logging), so the analytics hide nothing. The 4 canonical agents are
	// seeded at 0 so they always appear even before any traffic.

	agentDist := map[string]int{}
	for _, a := range canonicalAgents {
		agentDist[a] = 0
	}
	distRows, err := db.Query(r.Context(), `
		SELECT agent, COUNT(*) AS cnt
		FROM messages
		WHERE role = 'assistant'
		  AND agent IS NOT NULL
		GROUP BY agent`,
	)
	if err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}
	defer distRows.Close()

	for distRows.Next() {
		var agentName string
		var cnt int
		if err := distRows.Scan(&agentName, &cnt); err != nil {
			writeJSON(w, 500, ErrorResponse{Error: "scan failed"})
			return
		}
		agentDist[agentName] = cnt
	}
	if err := distRows.Err(); err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "row iteration failed"})
		return
	}

	// ── Activity by day — conversations started per calendar day, last 30 days ──

	dayRows, err := db.Query(r.Context(), `
		SELECT
			TO_CHAR(started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS day,
			COUNT(*) AS cnt
		FROM conversations
		WHERE started_at >= NOW() - INTERVAL '30 days'
		GROUP BY day
		ORDER BY day ASC`,
	)
	if err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "database query failed"})
		return
	}
	defer dayRows.Close()

	activityByDay := []DayActivity{}
	for dayRows.Next() {
		var day string
		var cnt int
		if err := dayRows.Scan(&day, &cnt); err != nil {
			writeJSON(w, 500, ErrorResponse{Error: "scan failed"})
			return
		}
		activityByDay = append(activityByDay, DayActivity{Date: day, Conversations: cnt})
	}
	if err := dayRows.Err(); err != nil {
		writeJSON(w, 500, ErrorResponse{Error: "row iteration failed"})
		return
	}

	// Sort ascending by date (already sorted by SQL ORDER BY, but be defensive).
	sort.Slice(activityByDay, func(i, j int) bool {
		return activityByDay[i].Date < activityByDay[j].Date
	})

	writeJSON(w, 200, ConversationStats{
		TotalConversations:         totalConversations,
		TotalMessages:              totalMessages,
		AvgMessagesPerConversation: avgMsgs,
		AgentDistribution:          agentDist,
		ActivityByDay:              activityByDay,
	})
}
