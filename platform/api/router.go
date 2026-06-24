package main

import (
	"context"
	"log"
	"strings"
	"time"
)

// agents maps each routeIntent target to its handler. Populated in init()
// rather than inline to avoid an initialization cycle: schedulingAgent calls
// Route, which reads agents, which references schedulingAgent.
var agents map[string]AgentFunc

func init() {
	agents = map[string]AgentFunc{
		"advisory":        generalAgent,
		"mcp_data_tool":   dataAgent,
		"rag_search":      knowledgeAgent,
		"action_executor": schedulingAgent,
	}
}

// agentTools is the single source of truth for the MCP tools each route target's
// agent may call (S3-3, design §1). The router copies the matching slice into
// AgentRequest.AllowedTools, and each tool-using agent enforces it before any MCP
// call. The general agent (advisory) has no entry — it answers from its prompt.
var agentTools = map[string][]string{
	"mcp_data_tool": {
		"get_fleet_stats",
		"get_inspection_history",
		"get_elevator_risk",
		"get_elevator_incidents",
		"get_elevators_needing_followup",
		"get_tssa_shutdown_elevators",
		"get_incident_count_last_year",
	},
	"rag_search":      {"search_maintenance_docs", "search_incident_narratives"},
	"action_executor": {"schedule_inspection"},
}

// Route is the single entry point for the multi-agent pipeline. It classifies
// the incoming request, selects an agent, and returns its response.
//
// Scheduling pre-emption (§2.2): if a pending_action is present the message
// goes straight to the scheduling agent — ClassifyIntent is not called.
func Route(ctx context.Context, req AgentRequest) (resp AgentResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[router] panic recovered: %v", r)
			resp = AgentResponse{
				Reply:     "Something went wrong while processing your request. Please try again.",
				AgentName: "router",
			}
		}
	}()

	if req.PendingAction != nil {
		log.Printf("[router] pending_action present → scheduling")
		req.AllowedTools = []string{"schedule_inspection"}
		return schedulingAgent(ctx, req)
	}

	c := contextCarry(ClassifyIntent(req.Message, time.Now()), req.Message, req.History, time.Now())
	route := routeIntent(c)

	agent, ok := agents[route.Target]
	if !ok {
		log.Printf("[router] unknown target %q → general (fallback)", route.Target)
		agent = generalAgent
	} else {
		log.Printf("[router] intent=%s confidence=%.2f → %s", c.Intent, c.Confidence, route.Target)
	}

	// Scope every agent to its allowed tools via the single-source agentTools map
	// (design §1). An agent with no entry (the general agent) gets an empty list
	// and calls no MCP tools. This also covers the scheduling agent — its entry is
	// {"schedule_inspection"}, which schedulingAgent reads via toolAllowed() — so it
	// supersedes the earlier action_executor-only gating (AND-109).
	req.AllowedTools = agentTools[route.Target]

	return agent(ctx, req)
}

// followUpPrefixes are phrases that signal the message continues a prior thought
// rather than starting a new topic. Used by isFollowUp to trigger contextCarry
// for all agent types, not just data queries.
var followUpPrefixes = []string{
	"and for", "and the", "and what", "and how",
	"what about", "how about",
	"also for", "same for",
	"for the same", "how about the", "what about the",
}

// isFollowUp returns true when the message looks like a continuation: it either
// carries an elevator ID (data follow-up) or starts with a conversational
// connector (covers RAG and other agent types too).
func isFollowUp(msg string, c Classification) bool {
	if len(c.Entities.ElevatorIDs) > 0 {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(msg))
	for _, prefix := range followUpPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// contextCarry upgrades a low-confidence advisory classification when the
// message looks like a follow-up (has an elevator ID or starts with a
// conversational connector). It scans history for the most recent non-advisory
// user intent and inherits it, keeping the current entities so the right
// elevator or topic is used.
func contextCarry(c Classification, msg string, history []ChatMessage, now time.Time) Classification {
	if c.Intent != IntentAdvisory || !isFollowUp(msg, c) {
		return c
	}
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role != "user" {
			continue
		}
		prev := ClassifyIntent(history[i].Content, now)
		if prev.Intent != IntentAdvisory {
			c.Intent = prev.Intent
			c.Confidence = prev.Confidence
			c.Signals = prev.Signals
			c.Reason = "context-carry from history: " + prev.Reason
			break
		}
	}
	return c
}
