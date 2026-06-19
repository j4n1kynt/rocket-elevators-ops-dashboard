"""Test harness for the Rocket Elevators chatbot (AND-107).

Claude is used as an adversarial judge: the chatbot's reply is never trusted on
its own — it is compared against the ground truth the MCP tools return.

Two consumers (no third-party deps, stdlib only):
  - chat(msg, history, pending) -> hits the deployed Go API POST /api/chat
  - mcp(tool, args)             -> hits the deployed MCP server (ground truth)

Override the endpoints with env vars CHAT_API_URL / MCP_SERVER_URL to point at a
local stack (Go API :8080, MCP server :8765) instead of the Render deployment.

CLI:
  py -3 harness.py chat "How many incidents were reported last year?"
  py -3 harness.py mcp get_inspection_history '{"elevator_id": 60503, "limit": 10}'
"""
import json
import os
import sys
import urllib.request

API = os.environ.get("CHAT_API_URL", "https://rocket-elevators-ops-dashboard.onrender.com")
MCP = os.environ.get("MCP_SERVER_URL", "https://rocket-elevators-ops-dashboard-2.onrender.com")


def chat(message, history=None, pending=None, timeout=300):
    """Ask the deployed chatbot. Returns {reply, history, pending_action}."""
    body = {"message": message, "history": history or []}
    if pending:
        body["pending_action"] = pending
    _, _, raw = _post(API + "/api/chat", body, timeout=timeout)
    return json.loads(raw)


def mcp(tool, args=None, timeout=60):
    """Open a fresh MCP session and call one tool. Returns the parsed tool result.

    MCP Streamable HTTP: initialize -> notifications/initialized -> tools/call,
    JSON-RPC 2.0 over POST /mcp; the server replies with text/event-stream.
    """
    args = args or {}
    # Step 1: initialize — establishes the session id (mcp-session-id header)
    _, headers, _ = _mcp_post({
        "jsonrpc": "2.0", "id": 1, "method": "initialize",
        "params": {"protocolVersion": "2024-11-05", "capabilities": {},
                   "clientInfo": {"name": "harness", "version": "1.0"}},
    }, session=None, timeout=timeout)
    session = headers.get("mcp-session-id") or headers.get("Mcp-Session-Id")
    # Step 2: initialized notification — required before any tool call
    _mcp_post({"jsonrpc": "2.0", "method": "notifications/initialized"},
              session=session, timeout=timeout)
    # Step 3: tools/call
    _, _, raw = _mcp_post({
        "jsonrpc": "2.0", "id": 2, "method": "tools/call",
        "params": {"name": tool, "arguments": args},
    }, session=session, timeout=timeout)
    rpc = _parse_maybe_sse(raw)
    if rpc.get("error"):
        return {"_rpc_error": rpc["error"]}
    content = rpc["result"]["content"][0]["text"]
    try:
        return json.loads(content)
    except json.JSONDecodeError:
        return {"_text": content}


# ── transport helpers ───────────────────────────────────────────────────────────

def _post(url, body, headers=None, timeout=300):
    data = json.dumps(body).encode()
    h = {"Content-Type": "application/json"}
    if headers:
        h.update(headers)
    req = urllib.request.Request(url, data=data, headers=h, method="POST")
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return r.status, dict(r.getheaders()), r.read().decode()


def _mcp_post(body, session, timeout):
    h = {"Content-Type": "application/json",
         "Accept": "application/json, text/event-stream"}
    if session:
        h["mcp-session-id"] = session
    data = json.dumps(body).encode()
    req = urllib.request.Request(MCP + "/mcp", data=data, headers=h, method="POST")
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return r.status, dict(r.getheaders()), r.read().decode()


def _parse_maybe_sse(raw):
    """Return the JSON from an SSE body (first `data:` line) or a plain JSON body."""
    for line in raw.splitlines():
        line = line.strip()
        if line.startswith("data:"):
            return json.loads(line[5:].strip())
    return json.loads(raw.strip())


if __name__ == "__main__":
    cmd = sys.argv[1]
    if cmd == "mcp":
        tool = sys.argv[2]
        args = json.loads(sys.argv[3]) if len(sys.argv) > 3 else {}
        print(json.dumps(mcp(tool, args), indent=2))
    elif cmd == "chat":
        print(json.dumps(chat(sys.argv[2]), indent=2))
    else:
        sys.exit(f"unknown command {cmd!r} — use 'chat' or 'mcp'")
