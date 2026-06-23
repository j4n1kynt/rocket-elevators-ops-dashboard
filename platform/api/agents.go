package main

import (
	"context"
	"log"
	"strings"
	"time"
)

// buildReply assembles the message list and calls the LLM. Returns the reply
// text or a plain-language error string — never a Go error (§4.3).
func buildReply(ctx context.Context, systemPrompt string, dataContext string, history []ChatMessage, msg string) string {
	systemContent := systemPrompt
	if dataContext != "" {
		systemContent += "\n\n## Live Data Context\n" + dataContext
	}
	messages := []llmMsg{{Role: "system", Content: systemContent}}
	for _, h := range history {
		messages = append(messages, llmMsg{Role: h.Role, Content: h.Content})
	}
	messages = append(messages, llmMsg{Role: "user", Content: msg})

	reply, err := callLLM(ctx, getOllamaBaseURL(), getOllamaKey(), getOllamaModel(), messages)
	if err != nil {
		log.Printf("[agent] llm call failed: %v", err)
		if strings.Contains(err.Error(), "status 429") {
			return "The model is currently rate-limited. Please wait a moment and try again."
		}
		return "I'm having trouble reaching the assistant right now. Please try again in a moment."
	}
	return reply
}

// appendHistory returns a new history slice with the current turn appended.
func appendHistory(history []ChatMessage, msg, reply string) []ChatMessage {
	return append(history,
		ChatMessage{Role: "user", Content: msg},
		ChatMessage{Role: "assistant", Content: reply},
	)
}

// ── General agent ─────────────────────────────────────────────────────────────

func generalAgent(ctx context.Context, req AgentRequest) AgentResponse {
	var dataContext string

	if shouldTryRagFallback(req.Message) {
		mcpCtx, mcpCancel := context.WithTimeout(ctx, 25*time.Second)
		defer mcpCancel()
		result, err := CallMCPTool(mcpCtx, "search_maintenance_docs", map[string]any{"query": req.Message, "n_results": 5})
		if err != nil {
			log.Printf("[general] rag fallback failed: %v — staying advisory", err)
		} else if isToolError(result) {
			log.Printf("[general] rag fallback returned error payload — staying advisory")
		} else if hasConfidentResults(result) {
			log.Printf("[general] rag fallback matched maintenance docs")
			dataContext = "[DATA SOURCE: maintenance documentation]\n" + result
		}
	}

	reply := buildReply(ctx, generalPrompt, dataContext, req.History, req.Message)
	return AgentResponse{
		AgentName:      "general",
		Reply:          reply,
		UpdatedHistory: appendHistory(req.History, req.Message, reply),
	}
}

// ── Data agent ────────────────────────────────────────────────────────────────

func dataAgent(ctx context.Context, req AgentRequest) AgentResponse {
	c := ClassifyIntent(req.Message, time.Now())
	toolName, mcpArgs := buildMCPArgs(c, req.Message)

	var dataContext string
	mcpCtx, mcpCancel := context.WithTimeout(ctx, 25*time.Second)
	defer mcpCancel()
	result, err := CallMCPTool(mcpCtx, toolName, mcpArgs)
	if err != nil {
		log.Printf("[data] mcp tool %s failed: %v — falling back to advisory", toolName, err)
	} else if isToolError(result) {
		log.Printf("[data] mcp tool %s returned error payload — falling back to advisory", toolName)
	} else {
		dataContext = "[DATA SOURCE: PostgreSQL — live fleet data]\n" + result
	}

	reply := buildReply(ctx, dataPrompt, dataContext, req.History, req.Message)
	return AgentResponse{
		AgentName:      "data",
		Reply:          reply,
		UpdatedHistory: appendHistory(req.History, req.Message, reply),
	}
}

// ── Knowledge agent ───────────────────────────────────────────────────────────

// knowledgeSourceLabel returns the correct data context tag for each knowledge
// tool. Keeping labels distinct ensures the model cites source_name accurately
// and never attributes documentation to the live fleet database.
func knowledgeSourceLabel(toolName string) string {
	if toolName == "search_incident_narratives" {
		return "[DATA SOURCE: incident narratives]\n"
	}
	return "[DATA SOURCE: maintenance documentation]\n"
}

func knowledgeAgent(ctx context.Context, req AgentRequest) AgentResponse {
	lower := strings.ToLower(req.Message)

	// Scope: this agent may only call the two knowledge search tools.
	// Route through isIncidentNarrativeQuery to pick the primary corpus;
	// fall back to the other if the primary returns nothing confident.
	primaryTool := "search_maintenance_docs"
	primaryArgs := map[string]any{"query": req.Message, "n_results": 5}
	fallbackTool := "search_incident_narratives"
	fallbackArgs := map[string]any{"query": req.Message, "limit": 5}
	if isIncidentNarrativeQuery(lower) {
		primaryTool, primaryArgs, fallbackTool, fallbackArgs = fallbackTool, fallbackArgs, primaryTool, primaryArgs
	}

	var dataContext string

	// Each corpus call gets its own 25s budget so a slow primary never starves
	// the fallback (embedding queries on constrained CPU can take ~8s warm).
	primaryCtx, primaryCancel := context.WithTimeout(ctx, 25*time.Second)
	primaryResult, primaryErr := CallMCPTool(primaryCtx, primaryTool, primaryArgs)
	primaryCancel()
	if primaryErr != nil {
		log.Printf("[knowledge] primary tool %s failed: %v — trying fallback", primaryTool, primaryErr)
	} else if isToolError(primaryResult) {
		log.Printf("[knowledge] primary tool %s returned error payload — trying fallback", primaryTool)
	} else if hasConfidentResults(primaryResult) {
		log.Printf("[knowledge] primary tool %s matched", primaryTool)
		dataContext = knowledgeSourceLabel(primaryTool) + primaryResult
	}

	if dataContext == "" {
		fallbackCtx, fallbackCancel := context.WithTimeout(ctx, 25*time.Second)
		fallbackResult, fallbackErr := CallMCPTool(fallbackCtx, fallbackTool, fallbackArgs)
		fallbackCancel()
		if fallbackErr != nil {
			log.Printf("[knowledge] fallback tool %s failed: %v — advisory only", fallbackTool, fallbackErr)
		} else if isToolError(fallbackResult) {
			log.Printf("[knowledge] fallback tool %s returned error payload — advisory only", fallbackTool)
		} else if hasConfidentResults(fallbackResult) {
			log.Printf("[knowledge] fallback tool %s matched", fallbackTool)
			dataContext = knowledgeSourceLabel(fallbackTool) + fallbackResult
		}
	}

	if dataContext == "" {
		log.Printf("[knowledge] no confident results from either corpus — advisory only")
	}

	reply := buildReply(ctx, knowledgePrompt, dataContext, req.History, req.Message)
	return AgentResponse{
		AgentName:      "knowledge",
		Reply:          reply,
		UpdatedHistory: appendHistory(req.History, req.Message, reply),
	}
}

// ── Scheduling agent ──────────────────────────────────────────────────────────

func schedulingAgent(ctx context.Context, req AgentRequest) AgentResponse {
	var dataContext string
	var pendingAction *PendingAction

	if req.PendingAction != nil {
		isConfirm, isCancel := detectConfirmation(req.Message)
		pa := req.PendingAction

		// Ambiguous reply — default to the safe choice: never write on ambiguity.
		if isConfirm && isCancel {
			isConfirm = false
		}

		switch {
		case isConfirm:
			if !verifyPendingAction(pa) {
				log.Printf("[scheduling] confirmation rejected: invalid signature elevator=%d", pa.ElevatorID)
				dataContext = "[ACTION VALIDATION ERROR]\nThe pending scheduling request could not be verified — it may have expired or been altered. No action was taken and nothing was written. Ask the user to start the scheduling request again."
			} else if pa.ExpiresAt > 0 && time.Now().Unix() > pa.ExpiresAt {
				log.Printf("[scheduling] confirmation rejected: expired elevator=%d", pa.ElevatorID)
				dataContext = "[ACTION VALIDATION ERROR]\nThis scheduling confirmation has expired. No action was taken and nothing was written. Ask the user to start the scheduling request again."
			} else {
				log.Printf("[scheduling] confirmation=yes elevator=%d date=%s", pa.ElevatorID, pa.InspectionDate)
				phase2Args := map[string]any{
					"confirmed":       true,
					"elevator_id":     pa.ElevatorID,
					"inspection_date": pa.InspectionDate,
					"reason":          pa.Reason,
				}
				if pa.InspectionType != "" {
					phase2Args["inspection_type"] = pa.InspectionType
				}
				mcpCtx, mcpCancel := context.WithTimeout(ctx, 10*time.Second)
				defer mcpCancel()
				if result, err := CallMCPTool(mcpCtx, "schedule_inspection", phase2Args); err != nil {
					log.Printf("[scheduling] phase2 failed: %v", err)
					errMsg := cleanValidationError(err.Error())
					dataContext = "[ACTION VALIDATION ERROR]\n" + errMsg + "\nDo NOT show a confirmation prompt. Tell the user what is wrong."
				} else if errMsg := extractScheduleError(result); errMsg != "" {
					log.Printf("[scheduling] phase2 error: %s", errMsg)
					dataContext = "[ACTION VALIDATION ERROR]\n" + errMsg + "\nDo NOT show a confirmation prompt. Tell the user what is wrong."
				} else {
					dataContext = "[DATA SOURCE: PostgreSQL — live fleet data]\n" + result
				}
			}

		case isCancel:
			log.Printf("[scheduling] confirmation=cancelled elevator=%d", pa.ElevatorID)
			dataContext = "[ACTION CANCELLED]\nThe user cancelled the inspection scheduling. Confirm that no action was taken and no database write occurred."

		default:
			// Confirmation abandoned — clear pending state and re-dispatch through
			// Route so the correct agent and prompt handle the message (spec §7.2).
			log.Printf("[scheduling] confirmation=abandoned elevator=%d — re-dispatching", pa.ElevatorID)
			return Route(ctx, AgentRequest{
				Message: req.Message,
				History: req.History,
			})
		}
	} else {
		// Phase 1 scheduling
		c := ClassifyIntent(req.Message, time.Now())

		if len(c.Entities.ElevatorIDs) == 0 || len(c.Entities.Dates) == 0 {
			dataContext = "[ACTION NEEDS MORE INFO]\nThe user wants to schedule an inspection but did not provide both an elevator ID and a date. Ask them for whichever is missing before proceeding. Do not invent values."
		} else {
			toolName, mcpArgs := buildMCPArgs(c, req.Message)
			mcpCtx, mcpCancel := context.WithTimeout(ctx, 10*time.Second)
			defer mcpCancel()
			if toolName != "schedule_inspection" {
				log.Printf("[scheduling] blocked forbidden tool %q — only schedule_inspection is allowed", toolName)
				dataContext = "[ACTION VALIDATION ERROR]\nInternal routing error: the scheduling agent may only call schedule_inspection. No action was taken. Please try again or contact support."
			} else if result, err := CallMCPTool(mcpCtx, toolName, mcpArgs); err != nil {
				log.Printf("[scheduling] mcp tool %s failed: %v", toolName, err)
				errMsg := cleanValidationError(err.Error())
				dataContext = "[ACTION VALIDATION ERROR]\n" + errMsg + "\nDo NOT show a confirmation prompt. Tell the user what is wrong and ask them to correct it."
			} else if errMsg := extractScheduleError(result); errMsg != "" {
				log.Printf("[scheduling] mcp tool %s validation error: %s", toolName, errMsg)
				dataContext = "[ACTION VALIDATION ERROR]\n" + errMsg + "\nDo NOT show a confirmation prompt. Tell the user what is wrong and ask them to correct it."
			} else {
				pendingAction = buildPendingAction(result, c.Entities, capReason(req.Message))
				if pendingAction != nil {
					pendingAction.ExpiresAt = time.Now().Add(pendingActionTTL).Unix()
					pendingAction.Signature = signPendingAction(pendingAction)
				}
				dataContext = "[DATA SOURCE: PostgreSQL — live fleet data]\n" + result
			}
		}
	}

	reply := buildReply(ctx, schedulingPrompt, dataContext, req.History, req.Message)
	return AgentResponse{
		AgentName:      "scheduling",
		Reply:          reply,
		PendingAction:  pendingAction,
		UpdatedHistory: appendHistory(req.History, req.Message, reply),
	}
}
