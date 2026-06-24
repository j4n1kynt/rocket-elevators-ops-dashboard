.PHONY: test test-go test-python test-integration eval validate-rag help

# ---------------------------------------------------------------------------
# Guards
# ---------------------------------------------------------------------------

# DATABASE_URL must be set in the environment for any target that hits the DB.
# Example: export DATABASE_URL=postgresql://rocketuser:rocketpass@localhost:5432/rocket_elevators
check-db-url:
ifndef DATABASE_URL
	$(error DATABASE_URL is not set. Export it before running DB-backed tests.)
endif

# ---------------------------------------------------------------------------
# Primary targets
# ---------------------------------------------------------------------------

## Run the self-contained suite (Go + isolated Python). No live services needed.
test: test-go test-python

## Build, vet, and test the Go API package (uses httptest fakes — no live services)
test-go:
	@echo "==> Go: build"
	cd platform/api && go build ./...
	@echo "==> Go: vet"
	cd platform/api && go vet ./...
	@echo "==> Go: test"
	cd platform/api && go test ./...

## Run isolated Python test suites (mocked DB/LLM/MCP — no live services needed)
test-python:
	@echo "==> Python: intelligence pipeline tests"
	PYTHONPATH=. ALLOW_EMPTY_CHROMADB=1 MCP_SKIP_CONFIRMATION=1 \
		pytest intelligence/tests/ -v

	@echo "==> Python: MCP validation tests"
	PYTHONPATH=. ALLOW_EMPTY_CHROMADB=1 MCP_SKIP_CONFIRMATION=1 \
		pytest platform/mcp/tools/test_validation.py -v

	@echo "==> Python: MCP RAG search tests"
	PYTHONPATH=. ALLOW_EMPTY_CHROMADB=1 MCP_SKIP_CONFIRMATION=1 \
		pytest platform/mcp/tools/test_rag_search.py -v

	@echo "==> Python: MCP source attribution tests"
	PYTHONPATH=. ALLOW_EMPTY_CHROMADB=1 MCP_SKIP_CONFIRMATION=1 \
		pytest platform/mcp/tools/test_source_attribution.py -v

## Run DB-backed integration tests (requires DATABASE_URL and a running PostgreSQL)
test-integration: check-db-url
	@echo "==> Integration: MCP risk assessment"
	PYTHONPATH=. ALLOW_EMPTY_CHROMADB=1 MCP_SKIP_CONFIRMATION=1 \
		pytest platform/mcp/tools/test_risk_assessment.py -v

	@echo "==> Integration: MCP schedule inspection"
	PYTHONPATH=. ALLOW_EMPTY_CHROMADB=1 MCP_SKIP_CONFIRMATION=1 \
		pytest platform/mcp/tools/test_schedule_inspection.py -v

# ---------------------------------------------------------------------------
# Optional / manual targets
# ---------------------------------------------------------------------------

## Run chatbot eval harnesses (requires full stack: PostgreSQL + Go API + MCP server)
eval: check-db-url
	@echo "==> Eval: run_eval.py"
	cd tests/chatbot && py -3 run_eval.py
	@echo "==> Eval: run_eval2.py"
	cd tests/chatbot && py -3 run_eval2.py

## Validate ChromaDB RAG preprocessing output
validate-rag:
	@echo "==> RAG: validate preprocessing"
	PYTHONPATH=. python intelligence/validate_rag_preprocessing.py

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------

help:
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "  test              Self-contained suite: Go + isolated Python (no live services)"
	@echo "  test-go           Go build, vet, and test (platform/api)"
	@echo "  test-python       Isolated Python suites (mocked DB/LLM/MCP)"
	@echo "  test-integration  DB-backed integration tests (requires DATABASE_URL + PostgreSQL)"
	@echo "  eval              Chatbot eval harnesses (requires full stack running)"
	@echo "  validate-rag      Validate ChromaDB RAG preprocessing output"
	@echo "  help              Show this message"
	@echo ""
	@echo "Required env vars for test-integration / eval:"
	@echo "  DATABASE_URL    e.g. postgresql://rocketuser:rocketpass@localhost:5432/rocket_elevators"
	@echo ""
