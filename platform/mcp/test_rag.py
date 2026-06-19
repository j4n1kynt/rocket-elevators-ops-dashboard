"""
Unit tests for rag.warm_rag — the startup warm-up that moves the embedding-model
cold-start off the request path.

The lazy loaders are mocked so the test runs without a live ChromaDB or the
~125 MB sentence-transformer model.

Run: pytest platform/mcp/test_rag.py -v
"""

from unittest.mock import MagicMock, patch

import pytest

from platform.mcp import rag


def test_warm_rag_loads_client_and_model_and_encodes():
    model = MagicMock()
    with patch("platform.mcp.rag._get_client") as get_client, \
         patch("platform.mcp.rag._get_model", return_value=model) as get_model:
        rag.warm_rag()

    get_client.assert_called_once()
    get_model.assert_called_once()
    # A trivial encode is required: SentenceTransformer defers some weight init
    # until the first encode, not construction.
    model.encode.assert_called_once()
    assert model.encode.call_args.args[0] == "warmup"


def test_warm_rag_propagates_loader_failure():
    # ChromaDB not populated → _get_client raises. warm_rag must surface it so the
    # caller (server lifespan) can log and treat it as non-fatal.
    with patch("platform.mcp.rag._get_client", side_effect=RuntimeError("no chromadb")):
        with pytest.raises(RuntimeError, match="no chromadb"):
            rag.warm_rag()
