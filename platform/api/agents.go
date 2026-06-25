package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"
)

// toolAllowed reports whether name appears in the allowed list.
// Returns true when allowed is empty so agents without an explicit list remain
// unrestricted. schedulingAgent always receives a non-empty list from the router,
// so the empty-list short-circuit does not weaken that path.
func toolAllowed(allowed []string, name string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if a == name {
			return true
		}
	}
	return false
}

// looksLikeRawError reports whether reply appears to be a leaked infrastructure
// error string rather than a natural-language answer.
func looksLikeRawError(reply string) bool {
	lower := strings.ToLower(reply)
	// Prefix-only marker: "mcp " is ambiguous — it appears legitimately mid-answer
	// ("the MCP server exposes..."), so it only signals a leak when the reply
	// *starts* with it.
	if strings.HasPrefix(lower, "mcp ") {
		return true
	}
	// Substring markers: pure transport/infrastructure fragments that never occur
	// in a plain-language fleet-ops answer. They must match anywhere because the
	// canonical Go HTTP error leads with the method verb —
	// `Post "URL": dial tcp ...: connectex/connection refused` — so a prefix check
	// alone would miss it and leak the raw error to the user (contract §4.3).
	for _, marker := range []string{"dial tcp", "connectex:", "connection refused", "mcp server unreachable", "llm returned status"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// looksLikeJSON reports whether reply is an unparsed JSON blob (the model
// echoed structured data instead of a natural-language answer). A leading "{"
// is treated as a misfire outright. A leading "[" is ambiguous — markdown links
// ([text](url)) and citations ([1] Smith et al.) also start with "[" — so it is
// only discarded when the whole reply actually parses as JSON.
func looksLikeJSON(reply string) bool {
	trimmed := strings.TrimSpace(reply)
	if strings.HasPrefix(trimmed, "{") {
		return true
	}
	if strings.HasPrefix(trimmed, "[") {
		return json.Valid([]byte(trimmed))
	}
	return false
}

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

	reply, err := callChatLLM(ctx, messages)
	if err != nil {
		log.Printf("[agent] llm call failed: %v", err)
		if strings.Contains(err.Error(), "status 429") {
			return "The model is currently rate-limited. Please wait a moment and try again."
		}
		return "I'm having trouble reaching the assistant right now. Please try again in a moment."
	}
	// Trim first so leading whitespace/newlines don't cause the sanity checks
	// below (all prefix-based) to miss a malformed reply.
	reply = strings.TrimSpace(reply)
	if looksLikeRawError(reply) {
		log.Printf("[agent] llm reply looks like a raw error — discarding: %.120s", reply)
		return "I'm having trouble generating a response right now. Please try again."
	}
	if looksLikeJSON(reply) {
		log.Printf("[agent] llm reply looks like an unparsed JSON blob — discarding: %.120s", reply)
		return "I'm having trouble generating a response right now. Please try again."
	}
	if len(reply) < 20 {
		log.Printf("[agent] llm reply is suspiciously short (%d chars): %s", len(reply), reply)
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
	reply := buildReply(ctx, generalPrompt, "", req.History, req.Message)
	return AgentResponse{
		AgentName:      "general",
		Reply:          reply,
		UpdatedHistory: appendHistory(req.History, req.Message, reply),
	}
}

// ── Data agent ────────────────────────────────────────────────────────────────

// toolInScope reports whether toolName is in the allowed set. It is the guard
// that keeps each agent inside its tool scope (design §1).
func toolInScope(toolName string, allowed []string) bool {
	for _, t := range allowed {
		if t == toolName {
			return true
		}
	}
	return false
}

// dataSummaryPrompt drives the one-line natural-language intro that sits above a
// deterministic data block (hybrid formatting, S3-3). The block is built in Go,
// so the model only writes the intro — it never touches the numbers.
const dataSummaryPrompt = `You are OpsBot, an assistant for Rocket Elevators operations. The user asked a question, and the exact data answer is shown below, already formatted. Write ONE short sentence (20 words or fewer) in plain language to introduce it. Do not repeat the field values. Do not add any fact that is not in the data. Output only the sentence, with no labels and no line breaks.`

// summarizeForUser asks the model for a single short sentence to sit above a
// deterministic data block. The block itself is built in Go, so the model never
// touches the numbers. On any LLM error it returns "" and the caller shows the
// block alone — the data answer is never lost.
func summarizeForUser(ctx context.Context, dataBlock, msg string) string {
	sys := dataSummaryPrompt + "\n\n## Data Answer (already formatted — do not repeat it)\n" + dataBlock
	messages := []llmMsg{
		{Role: "system", Content: sys},
		{Role: "user", Content: msg},
	}
	reply, err := callChatLLM(ctx, messages)
	if err != nil {
		log.Printf("[data] summary llm call failed: %v — showing data block only", err)
		return ""
	}
	line := strings.TrimSpace(reply)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	// The intro is unguarded LLM output sitting above a trusted, Go-built data
	// block. If the model echoed a raw infrastructure error or a JSON blob instead
	// of a sentence, drop it — the caller then shows the deterministic block alone,
	// which is the real answer. Without this the data path would leak exactly what
	// buildReply already guards against on the fallback path.
	if looksLikeRawError(line) || looksLikeJSON(line) {
		log.Printf("[data] summary intro looks malformed — dropping, showing data block only: %.80s", line)
		return ""
	}
	return line
}

func dataAgent(ctx context.Context, req AgentRequest) AgentResponse {
	c := ClassifyIntent(req.Message, time.Now())
	if req.Classification != nil {
		c = *req.Classification
	}
	toolName, mcpArgs := buildMCPArgs(c, req.Message)

	// Scope guard: the data agent may call only the data tools. When the router
	// populates AllowedTools it uses that list; called directly (tests), it falls
	// back to its own canonical set. A tool outside the scope is never called —
	// the agent stays advisory instead (acceptance criteria §2).
	allowed := req.AllowedTools
	if len(allowed) == 0 {
		allowed = agentTools["mcp_data_tool"]
	}

	var dataContext string
	var dataBlock string // deterministic Go-formatted block, when a formatter exists

	if !toolInScope(toolName, allowed) {
		log.Printf("[data] tool %s is out of scope for the data agent — skipping MCP call", toolName)
	} else {
		mcpCtx, mcpCancel := context.WithTimeout(ctx, 25*time.Second)
		result, err := CallMCPTool(mcpCtx, toolName, mcpArgs)
		mcpCancel()
		if err != nil {
			log.Printf("[data] mcp tool %s failed: %v", toolName, err)
			dataContext = "[DATA SERVICE UNAVAILABLE: the PostgreSQL data service could not be reached. " +
				"Tell the user that live fleet data is currently unavailable and you cannot answer their question with real data. " +
				"Do not speculate or answer from general knowledge.]"
		} else if isToolError(result) {
			log.Printf("[data] mcp tool %s returned error payload", toolName)
			dataContext = "[DATA SERVICE ERROR: the data tool returned an error response. " +
				"Tell the user that live fleet data is currently unavailable and you cannot answer their question with real data. " +
				"Do not speculate or answer from general knowledge.]"
		} else if block, ok := formatToolResult(toolName, result); ok {
			dataBlock = block // hybrid: Go owns the exact data block
		} else {
			dataContext = "[DATA SOURCE: PostgreSQL — live fleet data]\n" + result
		}
	}

	// Hybrid path: the model writes one intro line; the Go block stays exact.
	if dataBlock != "" {
		reply := dataBlock
		if line := summarizeForUser(ctx, dataBlock, req.Message); line != "" {
			reply = line + "\n\n" + dataBlock
		}
		return AgentResponse{
			AgentName:      "data",
			Reply:          reply,
			UpdatedHistory: appendHistory(req.History, req.Message, reply),
		}
	}

	// Fallback path: tools without a Go formatter use inject-and-answer.
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
	var primaryHardFailed bool
	if primaryErr != nil {
		log.Printf("[knowledge] primary tool %s failed: %v — trying fallback", primaryTool, primaryErr)
		primaryHardFailed = true
	} else if isToolError(primaryResult) {
		log.Printf("[knowledge] primary tool %s returned error payload — trying fallback", primaryTool)
		primaryHardFailed = true
	} else if hasConfidentResults(primaryResult) {
		log.Printf("[knowledge] primary tool %s matched", primaryTool)
		dataContext = knowledgeSourceLabel(primaryTool) + primaryResult
	}

	var fallbackHardFailed bool
	if dataContext == "" {
		fallbackCtx, fallbackCancel := context.WithTimeout(ctx, 25*time.Second)
		fallbackResult, fallbackErr := CallMCPTool(fallbackCtx, fallbackTool, fallbackArgs)
		fallbackCancel()
		if fallbackErr != nil {
			log.Printf("[knowledge] fallback tool %s failed: %v", fallbackTool, fallbackErr)
			fallbackHardFailed = true
		} else if isToolError(fallbackResult) {
			log.Printf("[knowledge] fallback tool %s returned error payload", fallbackTool)
			fallbackHardFailed = true
		} else if hasConfidentResults(fallbackResult) {
			log.Printf("[knowledge] fallback tool %s matched", fallbackTool)
			dataContext = knowledgeSourceLabel(fallbackTool) + fallbackResult
		}
	}

	if dataContext == "" {
		if primaryHardFailed && fallbackHardFailed {
			log.Printf("[knowledge] both corpus tools unreachable — injecting unavailability notice")
			dataContext = "[DATA SERVICE UNAVAILABLE: the maintenance knowledge base could not be reached. " +
				"Tell the user that documentation search is currently unavailable and you cannot answer their question from maintenance records. " +
				"Do not speculate or answer from general knowledge.]"
		} else {
			log.Printf("[knowledge] no confident results from either corpus — advisory only")
		}
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
			if !toolAllowed(req.AllowedTools, toolName) {
				log.Printf("[scheduling] blocked forbidden tool %q — only schedule_inspection is allowed", toolName)
				dataContext = "[ACTION VALIDATION ERROR]\nThat request cannot be handled through the scheduling workflow. No action was taken — please try again."
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
