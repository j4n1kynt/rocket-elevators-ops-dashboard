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

	c := ClassifyIntent(req.Message, time.Now())
	route := routeIntent(c)

	agent, ok := agents[route.Target]
	if !ok {
		log.Printf("[router] unknown target %q → general (fallback)", route.Target)
		agent = generalAgent
	} else {
		log.Printf("[router] intent=%s confidence=%.2f → %s", c.Intent, c.Confidence, route.Target)
	}

	// allowedTools gates which MCP tools each agent's Go handler may call
	// (design §2). Scheduling is the only agent with a write tool, so it is
	// the only one explicitly listed here. Other agents enforce their own
	// scoping internally (knowledgeAgent, dataAgent, generalAgent).
	if route.Target == "action_executor" {
		req.AllowedTools = []string{"schedule_inspection"}
	}

	return agent(ctx, req)
}
