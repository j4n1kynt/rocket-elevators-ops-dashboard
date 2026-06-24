Feature: A single command runs the full test suite

  # S3-10 — Automated test suite
  # Command: make test (from repo root)
  # Go package: platform/api — go test ./...

  Scenario: The suite runs with one command
    Given the test suite exists
    When I run the single test command
    Then all routing, quality, edge-case, and error tests run

    # Satisfied by: make test → make test-go (go test ./...) + make test-python
    # All tests use httptest fake servers and t.Setenv — no live services required.


  Scenario: Routing tests check each agent
    Given the test suite is running
    When the routing tests run
    Then they check that queries reach the correct agent

    # Routing — all four agents verified by:
    #   TestRouteIntent           (intent_test.go)  — asserts each IntentDataQuery → mcp_data_tool,
    #                                                  IntentRAG → rag_search,
    #                                                  IntentAction → action_executor,
    #                                                  IntentAdvisory → advisory
    #   TestClassifyIntent        (intent_test.go)  — 27+ cases covering all four intent classes
    #   TestBuildMCPArgsRouting   (intent_test.go)  — verifies correct MCP tool selected per query
    #
    # Per-agent end-to-end routing verified by:
    #   TestKnowledgeAgentProcedureUsesMaintenanceDocs  (agents_test.go) — knowledge agent
    #   TestKnowledgeAgentIncidentQueryUsesNarratives   (agents_test.go) — knowledge agent (RAG split)
    #   TestDataAgentRiskLookupUsesRiskTool             (agents_test.go) — data agent
    #   TestSchedulingAgentPhase1Preview                (agents_test.go) — scheduling agent
    #   TestGeneralAgentNeverCallsMCPTools              (agents_test.go) — general agent


  Scenario: Error recovery is tested
    Given the test suite is running
    When a tool or service is simulated as down
    Then the test checks the chatbot replies gracefully

    # MCP transport failure (connection refused):
    #   TestDataAgentMCPTransportFailureNoRawError   (agents_test.go)
    #     — MCP server is closed before the agent runs; dataAgent must inject
    #       DATA SERVICE UNAVAILABLE into the LLM system message and must not
    #       leak raw Go error strings ("connection refused", "dial tcp").
    #
    # Tool-level error payload:
    #   TestKnowledgeAgentBothCorporaToolErrorNoRawError   (agents_test.go)
    #     — Both search_maintenance_docs and search_incident_narratives return
    #       {"error":true,...}; knowledgeAgent must still reply gracefully and
    #       must not forward the raw error payload to the LLM.
    #
    # Malformed / unsafe LLM output (quality guard + recovery):
    #   TestBuildReplyMalformedLLMOutput   (agents_test.go)
    #     — LLM returns a raw error string or a JSON blob; buildReply must
    #       return the safe fallback message rather than forwarding garbage to the user.
    #
    # MCP HTTP-level errors:
    #   TestInvokeToolHTTPError / TestInvokeToolConnectionRefused   (mcp_client_test.go)
    #     — Verify the MCP client surfaces a typed error for 5xx responses and
    #       unreachable hosts respectively.
