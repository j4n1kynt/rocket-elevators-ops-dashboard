package main

import (
	"context"
	"log"
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
		return schedulingAgent(ctx, req)
	}

	c := ClassifyIntent(req.Message, time.Now())
	route := routeIntent(c)

	agent, ok := agents[route.Target]
	if !ok {
		log.Printf("[router] unknown target %q → general (fallback)", route.Target)
		agent = generalAgent
	} else {
		log.Printf("[router] intent=%s confidence=%.2f → %s", c.Intent, c.Confidence, route.Target)
	}

	// Scope the agent to its allowed tools (design §1). An agent with no entry
	// (the general agent) gets an empty list — it does not call MCP tools.
	req.AllowedTools = agentTools[route.Target]

	return agent(ctx, req)
}
