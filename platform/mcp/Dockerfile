# MCP server for the Rocket Elevators chatbot (AND-107).
# FastMCP Streamable HTTP server. The Go API connects to it as an MCP client.
#
# Build from the REPO ROOT, not platform/mcp/ — the COPY paths below are
# relative to the repo root:
#   docker build -f platform/mcp/Dockerfile .
# On Render: leave "Root Directory" empty and set Dockerfile Path to
# "platform/mcp/Dockerfile".
FROM python:3.12-slim

WORKDIR /app

# Install CPU-only torch first. On Linux the default torch pulls the full
# multi-GB CUDA stack, which is useless on Render (no GPU) and bloats the image.
# Installing it from the CPU index means sentence-transformers reuses it below.
# Pinned to the version verified on the CPU index for py3.12 (keeps builds
# reproducible and compatible with sentence-transformers==5.6.0).
RUN pip install --no-cache-dir torch==2.12.1 --index-url https://download.pytorch.org/whl/cpu

# Install the rest of the deps (torch is already satisfied, so no CUDA pull).
COPY platform/mcp/requirements.txt platform/mcp/requirements.txt
RUN pip install --no-cache-dir -r platform/mcp/requirements.txt

# Bake the embedding model into the image so there is no ~125MB download on
# each cold start. The model must match EMBEDDING_MODEL in platform/mcp/rag.py.
RUN python -c "from sentence_transformers import SentenceTransformer; SentenceTransformer('BAAI/bge-small-en-v1.5')"

# App code (platform is a package) and the committed ChromaDB index.
COPY platform/__init__.py platform/__init__.py
COPY platform/mcp/ platform/mcp/
COPY data/chromadb/ data/chromadb/

ENV PYTHONUNBUFFERED=1

# Render injects $PORT at runtime; server.py reads MCP_PORT or PORT (default 8765).
EXPOSE 8765

CMD ["python", "-m", "platform.mcp.server"]
