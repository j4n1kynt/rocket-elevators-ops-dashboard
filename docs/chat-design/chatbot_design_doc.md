# Chat Widget — Design Document

**Feature:** CHAT-1  
**Branch:** `feature/design-1-chat-widget-design`  
**Status:** Design

> **⚠️ Superseded by EVAL-1 for the shipped CHAT-2 build.** This document describes the original **data-grounded** vision (live fleet queries via intent detection + context injection). The implemented chatbot instead follows the **OpsBot advisory** model validated in `docs/system-prompt-evaluation.md`: it is **advisory/educational only, with no live data access** — it explains regulations, terminology, and risk concepts, and redirects all live-data requests to the dashboard. Sections describing live-data querying (2 "Included" data items, §3.2 intent responsibilities, §3.4 Context Injection, §3.9) reflect the original design, **not** current behavior. See `dashboard_spec.md §5` for the authoritative shipped spec.

---

## 1. Purpose

The chat widget gives operations staff a natural-language interface to query the Ontario elevator fleet without leaving the dashboard. A user can ask "show me HIGH risk elevators in Toronto" or "when was elevator 10054 last inspected?" and receive a direct, data-grounded answer — while the fleet table remains visible behind the panel.

The widget is a companion to the existing dashboard, not a replacement for the table or detail panel. It offers a faster path to answers that would otherwise require several filter and sort interactions.

---

## 2. Scope

**Included:**
- Floating chat panel embedded in the existing dashboard page
- Natural-language queries answered from live fleet data
- Fleet-level queries (totals, risk distribution, overdue inspections)
- Single-elevator queries (status, inspection history, risk assessment)
- Suggested starter prompts to guide new users
- Urgency ranking — "Which elevator needs attention the most right now?" combines risk score, days since last inspection, and unresolved orders into a single prioritized answer
- Inspection outcome trends — "Has elevator 10054 been getting better or worse over time?" summarizes the inspection history trajectory rather than showing only the latest result

**Excluded:**
- Authentication or per-user conversation history
- Write operations — the chat is read-only
- Queries against data not yet exposed by the Go API
- Multi-turn memory beyond the current browser session

---

## 3. Architecture

### 3.1 Overview

```
Browser (HTMX)
    │
    │  POST /chat  (form fields: message, history)
    ▼
Flask  (localhost:5000)
    │
    │  POST /api/chat  (JSON body: {message, history})
    ▼
Go API  (localhost:8080)
    │
    ├─ Retrieve relevant fleet context
    │    └─ intent detection → elevators, stats, inspections, risk
    │
    │  POST http://localhost:11434/api/chat
    ▼
Ollama  (local LLM service)
    │
    │  buffered response
    ▼
Go API  (returns JSON: {reply, history})
    │
    ▼
Flask  (renders HTML fragment: reply bubble + OOB history field)
    │
    ▼
Browser  (HTMX appends reply bubble, overwrites history hidden field)
```

The browser never communicates directly with the Go API or Ollama. Flask is the intermediary — it forwards the request to the Go API, receives JSON back, and renders the HTML fragment that HTMX swaps into the page.

### 3.2 Go API Chat Endpoint

**Route:** `POST /api/chat`

**Request body:**
```json
{
  "message": "How many HIGH risk elevators are there?",
  "history": [
    { "role": "user", "content": "..." },
    { "role": "assistant", "content": "..." }
  ]
}
```

**Responsibilities:**
1. Parse the incoming message and history
2. Determine which fleet data is relevant (stats, elevator lookup, inspections, risk)
3. Query the Go API's own data layer for that context
4. Compose a prompt containing the system instructions, context data, and conversation history
5. Forward the composed prompt to Ollama (`POST localhost:11434/api/chat`)
6. Return the model's response as JSON

**Response body:**
```json
{
  "reply": "There are 27,659 HIGH-risk elevators in the current fleet, representing approximately 64% of total devices.",
  "history": [
    { "role": "user", "content": "How many HIGH risk elevators are there?" },
    { "role": "assistant", "content": "There are 27,659 HIGH-risk elevators..." }
  ]
}
```

The `history` array is the full updated conversation (appended with the latest user/assistant pair). Flask uses it to overwrite the client-side hidden field via an OOB swap — the Go API does not store history between requests.

**Error responses:**

| Code | Condition |
|---|---|
| `503` | Ollama is unreachable |
| `500` | Internal data fetch or formatting failure |

### 3.3 Ollama Integration

The Go API communicates with Ollama using its REST API at `http://localhost:11434`. The model used is configurable via environment variable (`OLLAMA_MODEL`); the default is `mistral:latest` (mistral:7b), selected in EVAL-1 (`docs/system-prompt-evaluation.md`). Because mistral:7b cold start exceeds 300s, the Go API warms the model with a small request on startup.

Ollama runs locally on the same machine as the Go API. No API key is used — Ollama is called directly without authentication. This is intentional for a local-only deployment where all services (`Flask :5000`, `Go API :8080`, `Ollama :11434`) run on the same developer machine and are not exposed to external networks.

The Go API sends a `POST /api/chat` request to Ollama with:
- A system message containing the operator role, response rules, and the fleet context snapshot
- The full conversation history
- The user's latest message

Responses are buffered (non-streaming) for the initial implementation to keep the HTMX partial swap simple.

### 3.4 Context Injection

Before calling Ollama, the Go API retrieves a concise data snapshot relevant to the user's message and injects it into the system prompt. This grounds the model's answer in live fleet data.

The Go API uses lightweight keyword-based intent detection to decide which data to fetch — it does not fetch all four sources on every request. The incoming message is scanned for signals before any database call is made.

| Intent | Signals | Data fetched |
|---|---|---|
| Fleet-wide | "how many", "total", "fleet", "distribution", "summary" | `GET /api/fleet/stats` |
| Elevator lookup | location name, region, or keyword without a numeric ID | `GET /api/elevators?q={term}&limit=5` |
| Single elevator | numeric elevator ID present in the message | `GET /api/elevators/{id}` + `/inspections` + `/risk` |
| Alerts / critical | "alert", "critical", "out of service", "urgent" | `GET /api/fleet/alerts` |
| No match | none of the above signals detected | No data fetched — see below |

The context snapshot is formatted as compact JSON and prepended to the system message. If a data fetch fails, the system message notes the unavailability so the model can report it rather than hallucinate.

**Unmatched queries:** When the message does not match any intent, no fleet data is fetched. The system message includes a note stating that no relevant context was found. The model is instructed to respond with: "I can help with fleet summaries, elevator lookups, inspection history, and critical alerts. Could you rephrase your question?" — it does not attempt to answer from its training data.

### 3.5 System Prompt

The authoritative system prompt is `platform/api/prompts/system_prompt.md` — the hardened "OpsBot" prompt from PROMPT-1 / EVAL-1, embedded into the Go API binary at build time and used **verbatim**. It defines OpsBot's identity, Ontario elevator domain knowledge, tone, boundaries, and edge-case handling.

OpsBot is **advisory and educational only** and has **no live data access** (this is the EVAL-1-validated behavior — no fleet data is injected). Its boundaries:

- No live data access; no specific elevator lookups — redirect live-data requests to the dashboard.
- No regulatory/procedural advice; no fabrication; no identity override; emergencies → 911.
- Concise, professional answers from its embedded domain knowledge; match format to the question.
- Use risk levels (`HIGH`, `MEDIUM`, `LOW`, `UNKNOWN`) exactly as defined.

---

## 4. User Interaction Model

### 4.1 Entry Point

A floating action button (FAB) appears in the bottom-right corner of every dashboard page. Clicking it opens the chat panel without reloading the page or disrupting the dashboard view.

Opening and closing the panel does not trigger a page navigation, change the URL, or fire a browser history event. The panel is toggled purely via a CSS `hidden` class — no route change, no full-page swap, no HTMX `hx-push-url`. Closing the panel (via `[×]` or a future outside-click handler) restores the dashboard to its previous state without losing any in-progress filters, sort order, or open elevator detail panel.

### 4.2 Panel Layout

The chat panel overlays the dashboard anchored to the bottom-right corner. It does not push or reflow the table or sidebar.

```
┌──────────────────────────────────┐
│  Fleet Assistant            [×]  │  ← header
├──────────────────────────────────┤
│                                  │
│  [assistant]  What would you     │
│               like to know about │
│               the elevator fleet?│
│                                  │
│  ┌──────────────────────────┐    │
│  │ Suggested:               │    │
│  │ [Fleet summary] [Alerts] │    │
│  └──────────────────────────┘    │
│                                  │
│  [user]  Show HIGH risk          │
│          elevators in Ottawa     │
│                                  │
│  [assistant]  Found 47 HIGH-risk │
│               elevators in       │
│               Ottawa. Top 5 by   │
│               risk score: …      │
│                                  │
├──────────────────────────────────┤
│  [                          ] ▶  │  ← input + send
└──────────────────────────────────┘
```

**Panel size:** 380 px wide × 520 px tall.

**Layering:** The panel uses `position: fixed` and sits above all dashboard content via `z-index`. No backdrop or overlay is applied — the dashboard remains fully visible and interactive behind the panel.

**Dashboard visibility:** The 380 px panel width leaves the majority of the dashboard uncovered. The fleet table, summary cards, and fleet health panel remain readable and interactive while the chat is open. The panel does not dim or disable the rest of the page.

### 4.3 Suggested Prompts

When the conversation is empty, three clickable chips appear above the input:

- "Fleet risk summary"
- "Elevators with overdue inspections"
- "Critical alerts"

Clicking a chip populates and submits the input immediately.

### 4.4 Message Types

| Type | Alignment | Visual |
|---|---|---|
| User message | Right | Solid accent background, white text |
| Assistant message | Left | White card, light gray border |
| Error message | Left | Red-tinted card, muted error text |
| Loading indicator | Left | Animated ellipsis while awaiting reply |

### 4.5 Message List and Scrolling

Messages are rendered in chronological order, oldest at the top and newest at the bottom. The message list occupies the scrollable middle area of the panel between the header and the input bar.

The message list scrolls independently — it has its own `overflow-y: auto` scroll container. The rest of the dashboard does not scroll when the user scrolls inside the panel.

When a new message is appended (user send or assistant reply), the scroll position is kept at the bottom via the browser's default `overflow-anchor: auto` behaviour. If the user has scrolled up to read history, the position is preserved — the view does not force-jump to the latest message.

The loading indicator (animated ellipsis) appears at the bottom of the list while the assistant reply is pending and is replaced in-place by the actual reply bubble once the response arrives.

### 4.6 Input Area

The input area is a single-line text field with placeholder text `Ask about the fleet…`. It is pinned to the bottom of the panel above the panel border.

| Behaviour | Detail |
|---|---|
| After send | The field clears immediately when the message is submitted |
| While loading | The field and send button are both disabled until the assistant reply arrives |
| Send button | Disabled when the field is empty or while a reply is pending |
| Max length | Hard cap of 500 characters — enforced via `maxlength="500"` on the input element. Typing stops at 500 with no error message or counter shown. |

The send button uses the same dark navy as the panel header to maintain visual consistency with the existing dashboard palette.

### 4.7 Panel States

| State | Description |
|---|---|
| Empty | Panel just opened, no messages sent. Suggested prompt chips are visible above the input (4.3). |
| Loading | A message has been submitted and a reply is pending. Input and send button are disabled. Loading indicator (animated ellipsis) appears at the bottom of the message list. |
| Active | Conversation in progress. Input and send button are enabled. Suggested chips are hidden. |
| Error — reply failed | The Go API returned a `500`. An error bubble appears in the message list. Input re-enables so the user can retry. |
| Error — Ollama unreachable | The Go API returned a `503`. An error bubble reads "The assistant is currently unavailable. Make sure Ollama is running." Input re-enables. |
| Cleared | User clicked "Clear". Message list is empty and suggested chips reappear, returning to the Empty state. |

---

## 5. Visual Design

### 5.1 Palette

The widget reuses the dashboard's existing palette (defined in `docs/dashboard_spec.md 4.2`) without adding new colors.

| Element | Color |
|---|---|
| FAB background | Dark navy (`#0f172a`) |
| Panel header | Dark navy (`#0f172a`), white text |
| Panel body background | Light gray (`#f1f5f9`) |
| User message bubble | Green (`#16a34a`), white text |
| Assistant message bubble | White, `#e2e8f0` border |
| Error bubble | `#fef2f2`, dark red text |
| Input bar | White |
| Send button | Dark navy |

### 5.2 Typography

The widget inherits the dashboard's base font (Tailwind's default sans-serif stack). No new typefaces are introduced.

| Element | Size | Weight |
|---|---|---|
| Panel header | `text-sm` | `font-semibold` |
| Message bubble | `text-sm` | `font-normal` |
| Suggested chips | `text-xs` | `font-medium` |
| Input field | `text-sm` | `font-normal` |

Elevator IDs and numeric scores use `font-mono` inline within message bubbles.

### 5.3 Spacing

Message bubbles have `px-3 py-2` internal padding and `rounded-lg` corners. Consecutive messages are separated by `gap-3`. The panel header and input bar use `px-4 py-3`. The panel itself has a `shadow-lg` elevation and `rounded-t-xl` top corners.

### 5.4 Risk Badges in Responses

When the assistant references a risk level, the same colored badge from the fleet table is rendered inline in the message bubble:

- `HIGH` → red badge
- `MEDIUM` → amber badge
- `LOW` → green badge

Elevator IDs and numeric scores are rendered in a monospace style, matching their appearance in the table.

---

## 6. HTMX Integration

The widget follows the project convention of no custom JavaScript. All interactivity uses HTMX attributes.

| Interaction | HTMX mechanism |
|---|---|
| Sending a message | `hx-post="/api/chat"` on the form |
| Appending the reply | `hx-swap="beforeend"` on the message list container |
| Loading indicator | `hx-indicator` pointing to an ellipsis element |
| Opening the panel | `hx-on:click` on the FAB toggles a CSS `hidden` class |
| Closing the panel | `hx-on:click` on `[×]` toggles the same class |
| Submitting a chip | Each chip is a `<button>` in the form with a preset `value` |

---

## 7. Session State

Conversation history is maintained client-side as a hidden field in the chat form. On each submission, the browser sends the full history array to Flask alongside the message. Flask forwards it to the Go API, which appends the new user/assistant pair and returns the updated array in the JSON response. Flask then renders an HTML fragment containing the reply bubble and an OOB swap that overwrites the hidden history field in the browser. This keeps both Flask and the Go API fully stateless — no session storage required on either side.

History is capped at 10 turns (20 messages) to bound the payload and Ollama context window usage. When the cap is reached, the oldest user/assistant pair is dropped. A "Clear" button in the panel header resets the hidden history field to empty.

---

## 8. Constraints & Decisions

| Decision | Rationale |
|---|---|
| Ollama as the LLM backend | Runs locally; no external API dependency or cost |
| No API key for Ollama | Local-only deployment — all services on the same machine; key management adds friction with no benefit in this context |
| Go API as the relay | Centralizes data access; keeps Ollama off the client; consistent with existing architecture |
| No custom JavaScript | Project convention — all interactivity via HTMX |
| History stored client-side | Keeps the Go API stateless; avoids session infrastructure |
| History capped at 10 turns | Bounds Ollama context and request payload size |
| Results capped at 5 per response | Chat answers must be scannable; full pagination belongs in the table |
| Non-streaming responses | Simplifies HTMX partial swap; streaming deferred to a future iteration |

---

## 9. Security Limitations

The following limitations apply to this implementation. They are acceptable for a local development deployment but must be addressed before any network-exposed deployment.

| Limitation | Risk | Notes |
|---|---|---|
| No authentication on `/api/chat` | Anyone who can reach `:8080` can query the fleet via chat | Acceptable on localhost; critical if the Go API is ever port-forwarded or deployed |
| No rate limiting | The endpoint can be flooded, saturating Ollama and making the dashboard unresponsive | Deferred to a future security pass |
| Ollama binds to `:11434` on all interfaces by default | Ollama may be reachable on the local network, not just `127.0.0.1`, allowing direct calls that bypass the Go API relay | Mitigate by binding Ollama to `127.0.0.1` explicitly in production |
| Prompt injection | A crafted user message can attempt to override the system prompt; small local models have weak instruction following | The system prompt (3.5) provides partial mitigation but is not a reliable security boundary |
| Client-side history tampering | The history hidden field can be edited in browser DevTools, allowing a user to prime the model with fabricated context | Acceptable for a trusted internal tool; not suitable for public-facing deployments |
| Context snapshot exposure | The fleet data injected into the system prompt could be extracted via prompt injection | Mitigated by the read-only, non-sensitive nature of the data (no PII in the fleet dataset) |

---

## 10. Out of Scope

- Voice input
- Export of chat history
- Alerts or push notifications triggered by chat queries
- Admin UI for configuring the system prompt or Ollama model
- Rate limiting (deferred to a future security pass)
