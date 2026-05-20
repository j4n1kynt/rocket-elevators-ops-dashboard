# AI Interaction Log — Module 101

## Summary: Evolution of AI-Assisted Development Through Module 101

Over the course of Module 101, my understanding of how to effectively collaborate with AI shifted from viewing it primarily as a content generator to recognizing it as a partner in iterative, data-driven design. This evolution was driven by concrete friction points and surprises that forced me to re-examine my assumptions about prompting, specification writing, and verification.

### Prompting: From Open-Ended Requests to Constraint-Driven Precision

Early interactions taught me that prompting quality directly correlates to output precision. My initial README prompt was loose—it generated features that hadn't been implemented yet and created unnecessary content. Reframing the task with explicit constraints (module scope, no future assumptions) eliminated over-generation. This pattern repeated across multiple tasks: structured prompts with clear boundaries consistently outperformed open-ended requests.

But prompting is not one-size-fits-all. I discovered that the three main techniques—zero-shot, few-shot, and chain-of-thought—serve different purposes. Zero-shot prompting provided fast baselines for straightforward classification tasks. Few-shot prompting excelled when edge cases required contextual grounding (e.g., interpreting ambiguous license statuses). Chain-of-thought prompting was essential for complex reasoning and for catching misinterpretations hidden in technically correct results.

This last lesson emerged painfully: my data analysis task produced a result claiming 100% of licenses were expired. The calculation was correct—mathematically sound—but operationally meaningless because the dataset is historical. Only chain-of-thought prompting surfaced this insight, forcing explicit reasoning about data provenance. Simpler techniques accepted the dataset as current. The lesson: data interpretation requires context, and reasoning-focused prompts help distinguish calculations from meaningful insights.

### From Specifications as Documents to Specifications as Control

Specifications started as a way to document requirements. They evolved into something more powerful: a single source of truth that could drive both implementation and visual refinement without manual code edits.

This evolution was enabled by a critical upstream shift: verifying data early. My initial dashboard specification assumed field names without checking the actual datasets. When I aligned the specification with real schemas—confirming which fields existed, how they were named, and what gaps existed—the specification became genuinely actionable. A specification grounded in real data constraints is more valuable than one built on reasonable assumptions.

Once the specification was data-aware, it became an effective control mechanism. Instead of editing HTML directly when visual design needed improvement, I updated the specification with branding guidance and regenerated the entire dashboard. This kept intent and implementation aligned and prevented the inconsistencies that manual tweaks introduce. The specification became the single source of truth, and regeneration became the standard workflow.

### The Role of Data Verification as a Foundation

Data exploration is not separate from downstream work—it is prerequisite work. When I verified `ElevatingDevicesNumber` as a unique identifier, validated that ACTIVE and BY REQUEST were the operational statuses, and confirmed schema alignment across datasets, I was not just exploring. I was establishing foundations that everything downstream depended on.

This upfront verification accelerated implementation. The dashboard specification could be written confidently because the data constraints were known. The static HTML could be generated without follow-up questions. The prompting lab analysis could be interpreted correctly because I understood the dataset's provenance.

Conversely, skipping verification created rework: initial spec assumptions about field availability required correction. The solution was not to be more careful with guesses but to verify systematically before writing specifications.

### Project Structure and Workflow Payoff

An early decision to define clear directory structure (data/, docs/, intelligence/, platform/) seemed routine but paid surprising dividends. Having a settled structure eliminated hesitation when creating new artifacts. New notebooks, specifications, and dashboards had an obvious home, reducing context-switching friction. This reinforces a broader principle: upfront investment in scaffolding pays off quickly when the actual work begins.

### Honest Failures and What They Revealed

Two significant friction points stood out. First, I initially over-generated content when prompts were too open-ended. The README generated features not yet implemented. The initial dashboard specification included design details that belonged in a UI wireframe, not a technical spec. These failures traced directly to prompts that lacked constraints. The fix was not to be smarter about reading AI output but to be more deliberate about what I asked for.

Second, tooling friction appeared when working with Jupyter notebooks. Notebook cell edits required verification after completion. This was frustrating in the moment but served a deeper purpose: it reinforced the importance of validating structured outputs rather than assuming they are correct.

### Implications for Future Modules

These patterns will shape how I approach AI-assisted work going forward:

1. **Specifications should be complete from the start.** Branding, visual hierarchy, and design decisions should be specification-driven, not afterthoughts. This prevents visual iteration from becoming a series of manual code edits.

2. **Data verification precedes specification writing.** Before describing what the dashboard should display, I need to know exactly what data is available, what constraints exist, and how datasets join.

3. **Prompting is technique selection, not a single skill.** I will continue to use chain-of-thought for reasoning tasks, few-shot for edge cases, and zero-shot as a baseline, rather than defaulting to any single approach.

4. **Iteration on AI output is more efficient than regeneration from scratch.** When feedback is clear and targeted, guiding AI through revisions produces better results than asking for a complete redo. This was true for the README and the dashboard specification alike.

5. **Structured verification is not optional.** Before accepting AI-generated content—especially for data analysis or calculations—I should verify outputs against source data and ask clarifying questions about context and assumptions.

The overall shift is from treating AI as a tool that produces complete, polished artifacts to treating it as a collaborator in an iterative process where clarity of intent, early verification, and targeted feedback produce better results than hoping for the perfect first attempt.

---

## Entry – Monorepo Setup: Defining Project Structure for Module 101

**Task:** Set up the initial monorepo structure, directory layout, and project organization to support Module 101 work.

**Interaction summary:**
I worked with Claude Code to define a clear directory structure (data/, docs/, intelligence/) and establish initial project documentation before writing specifications or dashboards. This required translating loose course requirements into a concrete, usable repository layout without over-engineering for future modules.

**What worked:**
The straightforward separation of concerns made it immediately clear where datasets, specifications, notebooks, and logs belonged. This structure reduced friction in later tasks, as new artifacts could be placed confidently without revisiting organizational decisions.

**What was unexpected:**
Defining the structure early significantly accelerated later tasks. Having a settled structure eliminated hesitation and context-switching when moving between specification, dashboard development, and data analysis.

**Lesson learned:**
Upfront investment in project structure pays off quickly. Repository organization should emerge from the actual scope of work (Module 101), not from generic best practices aimed at hypothetical future needs.

---

## Entry – Updating README with Claude Code

**Task:** Update README.md to meet Task 1 requirements.

**Prompt used:**
"Update the existing README.md to include the project name, a one-paragraph description, and a list of the four main directories with explanations. Keep it concise and limited to Module 101."

**What worked:**
Claude Code produced a clean, well-structured README that clearly listed the required directories and aligned with the project scope. The concise prompt helped avoid unnecessary features or future assumptions.

**What didn't work or was unexpected:**
In an earlier attempt, a less structured prompt led Claude to mention features not yet implemented, which required manual correction, in addition to creating a new README.md instead of modifying the existing one.

**What I'd change next time:**
Be explicit about constraints (e.g., module scope, no future features) to reduce over-generation.

**Lesson learned:**
Structured, constraint-driven prompts lead to more accurate documentation updates than open-ended requests, especially when modifying existing files instead of generating new ones.

---

## Entry – Refining AI Workflow Baseline

**Task:** Create the AI workflow baseline document for Module 101.

**Interaction summary:**
While drafting the initial version of the AI workflow baseline, I realized the content felt too superficial and did not accurately reflect my real usage of AI tools. I reframed the task by answering more detailed, self‑reflective questions before structuring the document.

**What worked:**
Answering the questions in Spanish and from real experience resulted in a more honest and technically accurate baseline. This helped surface patterns in how I actually rely on AI during development work.

**What was unexpected:**
I underestimated how much nuance I had developed in my AI usage until I was forced to articulate it explicitly.

**Lesson learned:**
Baseline documentation is most valuable when it reflects real behavior, not an idealized workflow. Taking time to reflect before structuring the document leads to more meaningful results.

---

## Entry – License Dataset Analysis: Identifier Selection, Filtering, and Validation

**Task:** Analyze the license dataset (intelligence/license_analysis.ipynb) to identify a unique elevator identifier and create a clean, operational subset of the data.

**Interaction summary:**
Using pandas, I verified ElevatingDevicesNumber as the unique identifier, examined the distribution of LICENSESTATUS values, applied deliberate filtering decisions (keeping ACTIVE and BY REQUEST), and validated that uniqueness was preserved after filtering. I also extracted geographic information and created a license expiration timeline visualization.

**What worked:**
Treating identifier selection as a verification step rather than an assumption ensured safe cross-dataset joins later. Filtering to operational statuses produced a dataset aligned with the dashboard's operational focus. Post-filter validation confirmed data integrity was preserved.

**What was unexpected:**
The license expiration timeline revealed most expirations clustered in earlier years, indicating the dataset is historical. This insight later helped correctly interpret prompting lab results that showed all licenses as expired.

**What I'd change next time:**
Document the filtering rationale (why ACTIVE and BY REQUEST) even earlier to make future reuse of the dataset more explicit.

**Lesson learned:**
Data exploration should answer concrete operational questions before enabling downstream artifacts. Verifying assumptions early avoids compounding errors and strengthens confidence in data-driven decisions.

---

## Entry – Cross-Dataset Schema Alignment: Validating Join Fields Across Datasets

**Task:** Validate schema relationships across license.csv, inspection.csv, and installed.json to support a data-aware dashboard specification.

**Interaction summary:**
I confirmed that all relevant datasets share a common elevator identifier (ElevatingDevicesNumber, with a naming variation in installed.json) and documented this join explicitly in the dashboard specification. This ensured the spec reflected real data constraints rather than assumptions.

**What worked:**
Explicitly documenting joins and dataset sources in the specification created a single source of truth and prevented the dashboard implementation from guessing join logic or field availability.

**What didn't work or was unexpected:**
Initial spec drafts assumed certain fields existed without verification. Comparing against real schemas exposed mismatches in field names and formats that required correction.

**What I'd change next time:**
Verify dataset schemas before writing the first version of the specification to avoid rework.

**Lesson learned:**
Data schemas are constraints, not suggestions. Grounding specifications in real schemas early is more efficient than correcting implementation errors later.

---

## Entry – Dashboard Specification Iteration

**Task:** Write a technical specification for the Rocket Elevators Operations Dashboard (Task 3).

**Initial prompt used:**
"Write a technical dashboard specification in plain English based on the operations manager's request, including page layout, summary cards, and a detail table with columns, data types, and display formats."

**What worked:**
Claude Code generated a clear and well-structured first version of the dashboard specification. The layout, summary metrics, and table columns closely matched the operations manager's request, and the level of detail was sufficient for implementation without follow-up questions. In particular, the metric calculations and table definitions were strong and easy to understand.

**What didn't work or was unexpected:**
The initial output was more detailed than necessary for a "keep it simple" requirement. It included layout specifics, interaction details, and dataset assumptions that went beyond what was explicitly requested. This made the spec feel closer to a final UI design rather than an initial technical specification.

**What I changed:**
I provided corrective feedback to simplify the document, remove over-specific details, reduce assumptions about the dataset, and focus strictly on the operations manager's stated needs. Claude Code successfully revised the specification while preserving its overall structure.

**Lesson learned:**
AI can produce very complete outputs, but completion is not the same as correctness for a given context. Clear constraints and targeted feedback are more effective than regenerating content from scratch. Iterating on AI output helped me align the specification with business priorities instead of unnecessary technical precision.

---

## Entry – Refining Dashboard Specification Based on Real Datasets

**Task:** Write and finalize the technical dashboard specification for the Rocket Elevators Operations Dashboard (Task 3).

**Initial prompt / interaction:**
I initially asked Claude Code to generate a dashboard specification based on the operations manager's request, focusing on layout, summary metrics, and a detail table. The first versions of the specification were business-aligned but relied on assumed or generic field names (e.g., "Last Inspection Date," "Elevator Type") rather than confirmed dataset fields.

**What worked:**
Claude Code was very effective at translating the business request into a clear dashboard structure (sidebar, summary cards, detail table) and at defining metrics and layout clearly in plain English. Iterating on the specification through targeted prompts helped progressively align the document with both stakeholder needs and technical constraints.

**What didn't work or was unexpected:**
The initial specification did not fully reflect the actual structure of the available datasets. After reviewing `license.csv` and `inspection.csv`, it became clear that some assumptions about available fields were incorrect or incomplete. A clarification from the course tutor confirmed that table columns must come directly from the real data files, not from reasonable guesses.

**What I changed:**
I revised the specification to explicitly review and reference the real datasets (`license.csv`, `inspection.csv`, and later `installed.json`). Table columns were updated to map directly to existing fields, summary metrics were tied to concrete data sources, and joins between datasets were documented using `ElevatingDevicesNumber`. Fields that do not exist (or are not present for all records) were clearly documented as data limitations instead of being invented.

**Lesson learned:**
A dashboard specification should be business-driven but data-aware. Reviewing real datasets early helps avoid invalid assumptions, while still keeping the specification focused on stakeholder needs rather than raw data exploration. Iterating on the spec based on new information (including tutor feedback) was more effective than trying to design a "perfect" spec in one pass.

---

## Entry – Iterating on Static Dashboard via Specification

**Task:** Generate and refine a static HTML operations dashboard based on a written specification (Task 4).

**Initial interaction:**
After finalizing the dashboard specification, I asked Claude Code to generate a static HTML dashboard using the spec as the only source of truth. The first version correctly reflected the layout, metrics, table structure, and real dataset fields, confirming that the specification was sufficiently clear and actionable.

**What worked:**
Using the specification as the single input worked very well. Claude Code was able to generate a complete static dashboard without asking follow-up questions, including summary metrics, a detailed table, and placeholder data sourced from the real datasets. This validated the clarity and completeness of the spec.

**What didn't work or was unexpected:**
While the initial HTML matched the specification functionally, the visual presentation was very neutral. Once the data alignment issues were solved, it became clear that visual hierarchy and branding could be improved to make key metrics stand out more clearly for an operations audience.

**What I changed:**
Instead of editing the HTML directly, I updated the dashboard specification to include explicit guidance on branding, color usage, and visual emphasis (e.g., status badges, overdue highlights). I then asked Claude Code to regenerate the HTML using the updated spec, which resulted in a more polished and professional dashboard while preserving the original structure.

**Lesson learned:**
Iteration does not stop at functionality. Visual clarity and branding are also part of the product, and they can be effectively controlled through a well-written specification. Treating the spec as the single source of truth made it easier to evolve the dashboard design without introducing inconsistencies or manual HTML edits.

---

## Entry – Dashboard UI Polish: Specification-Driven Visual Refinement

**Task:** Refine the visual design of the static operations dashboard using specification-driven iteration.

**Interaction summary:**
After delivering a functionally correct dashboard, I recognized that visual hierarchy and branding could be improved. Instead of editing HTML directly, I updated the dashboard specification with branding and styling guidance, then regenerated the HTML from the revised spec.

**What worked:**
Keeping the specification as the single source of truth allowed visual changes to propagate cleanly to the HTML. This maintained alignment between design intent and implementation and avoided manual, one-off code edits.

**What didn't work or was unexpected:**
Visual design was initially treated as separate from specification work. In practice, design decisions are part of the product and benefit from being specification-driven as well.

**What I'd change next time:**
Include basic branding and visual hierarchy guidance in the initial specification.

**Lesson learned:**
Specifications should encode both functional and visual decisions. A spec-driven regeneration workflow scales better than direct code modifications.

---

## Entry – Module 101 Prompting Lab: Three Tasks, Three Techniques

**Task:** Develop a Module 101 prompting lab notebook (intelligence/prompting_lab.ipynb) comparing three prompting strategies (zero-shot, few-shot, and chain-of-thought) across three data analysis tasks using the Rocket Elevators license dataset.

**Interaction approach:**
For each of the three tasks — (1) classification of LICENSESTATUS values, (2) calculation of expired license percentage, and (3) open-ended geographic analysis — I ran the same task using all three prompting techniques. Each prompt and its corresponding output were copied into structured notebook cells, followed by a comparison and reflection section analyzing trade-offs between techniques.

**What worked:**
Applying the same task across different prompting styles made the strengths of each technique visible. Zero-shot prompting provided fast, reasonable baselines. Few-shot prompting worked particularly well when edge cases existed (for example, interpreting ambiguous statuses like "BY REQUEST" or framing urban vs. rural observations). Chain-of-thought prompting was most effective for complex reasoning, as it forced explicit step-by-step logic and made conclusions easier to verify and explain.

Using a notebook structure helped enforce discipline: separating prompts from outputs and completing each section sequentially reduced scope creep and made it clear when a task was actually finished.

**What didn't work or was unexpected:**
The calculation task produced a surprising result: 100% of licenses appeared expired when compared to today's date. While mathematically correct, this initially seemed like a data quality issue. Interpreting the result required recognizing that the dataset represents a historical snapshot. Chain-of-thought prompting naturally surfaced this explanation, while zero-shot and few-shot outputs needed additional contextual clarification.

Tooling friction also occurred when writing to notebook cells, requiring verification of cell contents after edits. While not directly related to prompting, this reinforced the importance of validating outputs when working with structured formats.

**What I'd change next time:**
For larger prompting labs, I would fully scaffold the notebook structure (all prompt and output sections) before generating responses. For calculation tasks involving time-based data, I would proactively flag dataset context to avoid misinterpreting correct-but-misleading results.

**Lesson learned:**
Prompting techniques are complementary. Chain-of-thought excels at reasoning and explanation but adds little value to simple classification. Few-shot prompting is most effective when edge cases need contextual grounding. Zero-shot prompting is fast and often sufficient as a baseline. Comparing techniques side-by-side in a notebook made these differences clear and measurable.

---

## Entry – License Expiration Interpretation: Correct Results vs. Meaningful Insights

**Task:** Interpret the result that 100% of licenses appeared expired during the prompting lab analysis.

**Interaction summary:**
Zero-shot and few-shot prompting correctly calculated that all licenses were expired based on today's date. This result seemed suspicious until chain-of-thought prompting surfaced the key insight: the dataset represents a historical snapshot rather than current operational data.

**What worked:**
Chain-of-thought prompting forced explicit reasoning about data context and provenance, clarifying that the result was mathematically correct but operationally misleading.

**What didn't work or was unexpected:**
Simpler prompting techniques accepted the dataset as current, producing outputs that required significant contextual reframing afterward.

**What I'd change next time:**
Explicitly include dataset context (timeframe, scope) in prompts for time-based calculations.

**Lesson learned:**
Data interpretation requires context. Reasoning-focused prompting techniques help distinguish correct calculations from meaningful insights.

## Entry – Evaluator Simulation as a Pre‑Submission Quality Gate

**Task:** Review Extra‑Mile tasks using an evaluator mindset prior to final submission.

**Interaction summary:**  
Both Extra‑Mile tasks were submitted to Claude Code with explicit evaluator‑level constraints: strict checklist review, no invented work, and no overlap with mandatory tasks. The AI acted as an adversarial reviewer rather than a collaborator, confirming Extra‑Mile 1 as fully compliant and identifying a specific evaluation risk in Extra‑Mile 2.

**What worked:**  
Framing the AI as a critical assessor surfaced issues that were not obvious during implementation. The feedback went beyond syntax and structure, focusing on whether the task fulfilled the evaluator’s intent.

**What was unexpected:**  
Extra‑Mile 2 passed every formal requirement but still carried a substantive evaluation risk due to lack of variance in the second dimension. This distinction would likely have been missed without an evaluator‑style review.

**Lesson learned:**  
Using AI as a simulated evaluator is a powerful quality gate. It shifts the role of AI from builder to critic and helps identify risks that only appear when judging intent rather than correctness.

## Entry – Diagnosing Structurally Correct but Conceptually Weak Designs

**Task:** Evaluate the intent of a two‑column visualization that was technically correct.

**Interaction summary:**  
Extra‑Mile 2 initially met all structural requirements (groupby, unstack, labels, legend, paragraph) but failed to introduce meaningful variation because the second grouping column (Province) contained only one value after filtering.

**What worked:**  
The evaluator‑style feedback correctly distinguished between structural compliance and analytical value. The chart was valid code, but it did not meaningfully extend insight beyond a single‑column view.

**What was unexpected:**  
This revealed a subtle failure mode: code that is correct, well‑written, and fully compliant can still undermine the purpose of the task.

**Lesson learned:**  
Meeting the letter of a requirement is not always sufficient. Visualizations must introduce real explanatory power, not just satisfy structural criteria.

## Entry – Minimal Targeted Correction over Full Rewrite

**Task:** Correct Extra‑Mile 2 based on evaluator feedback with minimal scope change.

**Interaction summary:**  
To resolve the identified risk, the second grouping dimension was changed from Province to ExpiryYear, a column already derived earlier in the notebook. No new logic or datasets were introduced.

**What worked:**  
Replacing a single dimension immediately restored meaningful variation to the visualization. Reusing an existing column avoided unnecessary complexity and preserved notebook integrity.

**What was unexpected:**  
A very small change completely eliminated the evaluation risk, reinforcing that large rewrites are often unnecessary when intent is clearly understood.

**Lesson learned:**  
The smallest change that satisfies evaluator intent is often the best one. Targeted corrections preserve stability and reduce the chance of introducing new issues.

## Entry – Final Re‑Review as a Submission Checkpoint

**Task:** Re‑audit Extra‑Mile tasks after applying corrections.

**Interaction summary:**  
After correcting Extra‑Mile 2, both bonus tasks were re‑reviewed against the full evaluation checklist. The re‑review confirmed that all requirements were now fully met, with no remaining risks.

**What worked:**  
The review‑correct‑re‑review loop ensured confidence that the fix was complete and did not introduce regressions, despite changes in grouping orientation.

**Lesson learned:**  
A deliberate submission workflow—review, identify risk, correct, re‑review—reduces uncertainty and prevents last‑minute surprises. Passing once is not the same as being submission‑ready.

## AND-2 Task 1: CLAUDE.md Setup — Subagent Explore for repo discovery

**Date:** 2026-05-11

**Goal:** Create a minimal root-level CLAUDE.md (<30 lines) with accurate repo structure and /data inventory.

**Prompt (paraphrased):**
"Use the Explore subagent (quick) to inspect the repository and return only: top-level structure, list of /data files with formats, and a one-sentence project purpose. Do not paste raw file contents."

**What went right:**
- The subagent returned a concise summary (top-level folders + /data file inventory + purpose) without dumping file contents.
- This preserved my main session context and reduced noise before drafting CLAUDE.md.

**What went wrong / risks noticed:**
- The returned project purpose was longer than needed for the Task 1 requirement (I needed a shorter, single-sentence purpose).
- Exploration could have been overkill if it produced long outputs, so I constrained it tightly.

**Context management decision (why subagent):**
I used Explore to keep repo scanning, file reads, and any dead ends out of the main context window. I only brought back a short summary to draft the initial CLAUDE.md.

**What I will change next time:**
- Ask Explore to produce the purpose as a strict single sentence (max ~20–25 words).
- Request the /data inventory in a fixed bullet format to copy directly into CLAUDE.md.

## AND-2 Task 3: Dynamic Dashboard — Context Reset and Server-Rendered Summary Cards

**Date:** 2026-05-13

**Context / Situation**  
While implementing HTMX-based filtering and sorting for the dashboard table, the Summary Cards at the top of the page stopped displaying values and showed placeholder dashes ("—"). Table interactivity via HTMX was functioning correctly.

**Prompt (paraphrased)**  
"Why did the summary cards stop showing values after migrating from a JavaScript-based prototype to an HTMX + server-rendered dashboard?"

**What the output got right**  
The analysis correctly identified that the cards were previously populated by custom JavaScript in the static prototype. After removing custom JS to comply with Task 3 requirements, the `/` route was still serving `index.html` as a static file via `send_from_directory`, so no server-side rendering or data injection was occurring. As a result, the cards had no mechanism to receive values.

**What went wrong / limitation identified**  
Although HTMX was correctly implemented for the table, the dashboard shell (`/`) was not rendered as a template. This meant summary metrics were never calculated or injected by the server, leaving the cards empty. This was not an HTMX issue, but an architectural gap introduced during the migration away from client-side JavaScript.

**Context management decision**  
At this point, the session context had grown to ~40% of the available window and response quality began to degrade. I deliberately used `/compact` to preserve the current state (HTMX table working, root cause of missing card values identified) while discarding resolved exploration and reducing noise before planning the fix.

**Design decision taken**  
To remain compliant with course constraints (no direct HTML edits, spec-driven workflow, no custom JavaScript), I chose to implement server-rendered summary cards:
- Update `docs/dashboard_spec.md` to explicitly define summary cards as server-rendered metrics.
- Regenerate `platform/index.html` from the updated spec, replacing placeholders with template variables.
- Modify the server to compute metrics from `elevator_fleet.csv` and render the dashboard using server-side templating.

**What I would do differently next time**  
When migrating from a static prototype to server-rendered interactivity, I would proactively audit which UI elements depend on client-side logic and plan equivalent server-rendered behavior earlier, instead of discovering missing functionality after removing JavaScript.

## AND-2 Task 3: Context Management — Using /compact to Maintain Output Quality

**Date:** 2026-05-13

**Goal:** Maintain response quality during a long, multi-file Task 3 implementation involving server logic, HTMX behavior, and spec alignment.

**Interaction summary:**
During Task 3, the conversation context grew to approximately 40% of the available window while iterating on server logic, HTMX interactivity, and dashboard semantics. At that point, responses began to lose precision and reference earlier decisions less reliably.

**What worked:**
- Using `/compact` preserved the essential state of the task (HTMX table working, server-rendered cards identified as missing, scope mismatch diagnosed).
- The context reset reduced noise from earlier exploration and allowed focused reasoning on how to complete Task 3 without reintroducing JavaScript or breaking the spec-driven workflow.

**What was unexpected:**
- Response degradation became noticeable before reaching the context limit, reinforcing that context quality degrades gradually, not only at hard limits.

**Design decision:**
I deliberately used `/compact` to reset the conversation state once the architectural direction was clear. This ensured subsequent guidance focused only on unresolved issues (server-rendered cards and semantic alignment), rather than rehashing solved problems.

**Lesson learned:**
For long tasks involving multiple files and design decisions, proactive context management is necessary. Using `/compact` early enough improves solution quality and reduces iteration time.

## AND-2 Task 3: Data Pipeline for Server-Rendered Dashboard

**Date:** 2026-05-13

**Goal:** Create a data preparation script (platform/prepare_data.py) that replicates the filtering logic from Module 1 Task 6c and produces elevator_fleet.csv as the authoritative source for all dashboard operations.

**Interaction summary:**
I asked Claude Code to design a Python data pipeline that:
- Loads three source datasets (license.csv, inspection.csv, installed.json) with consistent string typing
- Replicates the ACTIVE + BY REQUEST filtering from Module 1 Task 6c
- Normalizes date formats (DD-MMM-YY → YYYY-MM-DD) for consistent sorting and display
- Deduplicates inspection records to keep only the most recent per elevator
- Joins datasets on ElevatingDevicesNumber and outputs a clean CSV

**What worked:**
- The script correctly filtered 43,002 records (42,665 ACTIVE + 337 BY REQUEST) from a larger source dataset
- Date normalization using pd.to_datetime with explicit format strings handled format variation without data loss
- The deduplication strategy (sort descending, drop_duplicates keep='first') correctly extracted the latest inspection per elevator
- Output column naming aligned directly with template variable expectations in index.html

**What was unexpected:**
- The Unicode encoding issue (→ character in print statement) surfaced Windows PowerShell's cp1252 limitations; this required using plain ASCII (→ became ->) instead of assuming UTF-8 was available in terminal output
- Despite the encoding issue, the data pipeline itself executed successfully and produced correct output

**Design decision:**
prepare_data.py became the single source of truth for data quality. All downstream components (Flask server, summary card metrics, table filtering) depend on this cleaned CSV rather than re-implementing filter logic in multiple places. This centralization prevents inconsistency and makes the filtering logic auditable in one location.

**Lesson learned:**
Data pipelines should be script-first, not ad hoc. Encoding issues are environmental, not logical errors; they don't invalidate the core logic. The pipeline's output shape directly influences what template variables and server-side logic become possible.

---

## AND-2 Task 3: HTMX Table Interactivity — Migration from JavaScript Prototype

**Date:** 2026-05-13

**Goal:** Replace the static JavaScript-driven table prototype with an HTMX-based, server-rendered approach for filtering, search, and sorting.

**Interaction summary:**
I worked with Claude Code to redesign the dashboard interactivity model:
- Removed the entire JavaScript block from the static prototype (const data array, sortTable(), renderTable() functions)
- Added HTMX attributes to search input, filter dropdowns, and sortable headers
- Implemented a two-channel HTMX pattern: filters/search target #tableBody (innerHTML swap), sort buttons target #fleetTable (outerHTML swap)
- Defined clear HX-Target header inspection logic in server-side code to route responses to the correct swap target

**What worked:**
- The HTMX directive syntax (hx-get, hx-target, hx-swap, hx-trigger, hx-include) was expressive enough to encode all filtering, search, and sort behavior without custom JavaScript
- The two-swap pattern (innerHTML for filters, outerHTML for sort) correctly distinguished lightweight row-only updates from full-table replacements with updated button URLs
- Search debouncing (hx-trigger="keyup changed delay:300ms") provided responsive UX without page reloads

**What was unexpected:**
- The HTMX approach required no custom JavaScript whatsoever, eliminating an entire class of client-side state management bugs
- Unicode arrow characters (↑↓↕) in Python required careful handling but rendered correctly in browser HTML (no terminal encoding issues)

**Design decision:**
Approach A (server returns updated button URLs) was chosen over Approach B (client remembers sort state) because it keeps all state on the server and requires zero client-side logic. Each sort button click toggles the URL parameter, and the server recalculates the opposite direction. This trades a full table HTML return for complete statelessness on the client.

**Lesson learned:**
HTMX shifts interactivity from client-side logic to server-side response shape. The two different swap targets (innerHTML vs. outerHTML) let the server choose response granularity (rows only vs. full table) based on the type of update, reducing payload size and simplifying the client.

---

## AND-2 Task 3: Flask Server Architecture — HTML Fragments Over JSON

**Date:** 2026-05-13

**Goal:** Build a Flask backend (platform/server.py) that serves HTML fragments instead of JSON responses, enabling HTMX's innerHTML and outerHTML swapping.

**Interaction summary:**
I designed a Flask server with two endpoints:
- GET / renders the dashboard shell with server-calculated summary card metrics
- GET /table accepts filter, search, and sort parameters; applies them server-side; and returns either <tr> rows or a full <table> based on the HX-Target header

**What worked:**
- Returning Jinja2-rendered HTML fragments (not JSON) made HTMX swapping trivial; the browser receives ready-to-insert markup
- Checking request.headers.get("HX-Target") allowed a single /table endpoint to serve two different response shapes
- The is_overdue() helper and compute_metrics() function isolated business logic from routing, making calculations testable and reusable
- Parsing dates server-side via pd.to_datetime ensured correct chronological sorting regardless of input format

**What was unexpected:**
- HTML fragment responses eliminated the need for a separate JSON API and JavaScript template rendering on the client
- The server's full table generation (build_full_table()) required no client-side state; the next sort direction is embedded in the button URL on every response

**Design decision:**
Jinja2 templating was chosen over JSON because it keeps the rendering logic server-side. This maintains consistency: the server controls both the markup structure and the data it contains. No duplication of template logic across client and server.

**Lesson learned:**
When the client is Hypertext (HTML), serving HTML fragments is simpler and more consistent than serving JSON and rendering it on the client. HTMX's swap semantics reduce cognitive overhead: innerHTML is simpler than managing client-side state or diffing JSON updates.

---

## AND-2 Task 3: Dashboard Title and Scope Alignment

**Date:** 2026-05-13

**Goal:** Ensure the dashboard title and subtitle accurately reflect the operational subset (ACTIVE and BY REQUEST) rather than implying the full Ontario elevator registry.

**Interaction summary:**
During spec refinement, I updated the dashboard title and subtitle to clarify scope:
- Title: "Fleet Overview" → "Operational Fleet Overview"
- Subtitle: "Ontario elevator registry — HTMX-driven, server-rendered" → "Active and by-request licensed devices — server-rendered, HTMX-driven"

This was informed by earlier license_analysis.ipynb verification that confirmed ACTIVE and BY REQUEST were the only operational statuses included in the dashboard.

**What worked:**
- Renaming the title made the operational subset immediately clear to users
- Updating the subtitle removed the misleading reference to "full registry" which would have included CANCELLED_NOT_RENEWED and other non-operational statuses
- The change was spec-driven, so regenerating index.html from the updated spec automatically applied the new text

**What was unexpected:**
- The subtitle change required no code modifications; only text edits to the specification and subsequent HTML regeneration
- This reinforced that the spec-as-source-of-truth approach keeps UI text (not just layout) in sync with documented intent

**Design decision:**
Title accuracy was treated as part of specification correctness, not as a minor UI detail. By anchoring the title to the spec, future readers and maintainers immediately understand the dashboard's scope without needing to trace the data pipeline.

**Lesson learned:**
UI text like titles and subtitles should be specification-driven, just as layout and interactivity are. Clear, scoped titles reduce user confusion and improve documentation quality.

---
## AND-2 Task4: Claude Code Setup: Statusline Configuration with Custom jq Formatter

**Date:** 2026-05-14

**Task:** Configure the Claude Code statusline to display real-time context window usage, token counts, cache statistics, and session cost metrics.

**Interaction summary:**
I provided a custom jq formatting script that transforms Claude Code's session JSON data into a human-readable status line. The script extracts model name, context usage percentage, input/output tokens, cache read/write statistics, and cumulative session cost, then formats them as a single pipe-separated line. Used the statusline-setup agent to integrate this script into Claude Code's configuration.

**What worked:**
The jq script successfully formats all desired metrics in a concise, readable format. The script uses explicit null coalescing (`// default_value`) to gracefully handle missing fields, preventing errors when certain metrics are not yet available (e.g., before the first API call when context percentage is unknown). The statusline-setup agent discovered that configuration infrastructure already existed at a specific project location and applied two small improvements to handle edge cases better.

**What didn't work or was unexpected:**
Initial navigation required clarification—I had to provide the full file path because standard shell config files don't exist on Windows systems. The agent initially asked whether I was using WSL or wanted to manually define metrics, but once provided the jq script, it successfully integrated it into existing configuration.

**Design decision:**
jq was chosen for the formatter because it allows flexible field extraction and transformation while remaining portable across platforms. The script returns "?" for metrics that haven't been calculated yet (e.g., `?%` before the first message), making the statusline useful throughout the session lifecycle rather than only after API calls.

**Lesson learned:**
Claude Code's configuration is highly customizable—status display can be driven by structured data transformation scripts. Using explicit null checks and default values in formatters makes them robust to evolving session states, where some metrics may be unavailable or null at different points in the conversation.

**What I'd change next time:**
Include the full file path when requesting statusline configuration setup on Windows, rather than expecting the agent to locate standard shell config files that don't exist on that platform.
---

## AND-2 Task 5: ETL Pipeline — Dataset Merging and Integration

**Date:** 2026-05-14

**Goal:**  
Build a unified ETL pipeline that merges license, installed, alteration, and inspection datasets into a single, consistent fleet dataset while handling schema mismatches and one-to-many relationships.

**AI Techniques Used:**  
- **/compact** was used between merge steps to reduce context size and preserve key decisions such as row counts, join keys, and filtering logic.
- Claude Code was used interactively to validate merge strategies, resolve schema inconsistencies (column names and data types), and confirm handling of one-to-many relationships.

**What worked:**  
Breaking the pipeline into three explicit merge stages made it easier to reason about data loss and row multiplication. Using data-driven evidence (row counts and unique key analysis) helped justify decisions such as left joins and inspection deduplication.

**What was unexpected:**  
Schema inconsistencies across datasets (e.g., different naming and types for elevator identifiers) required explicit normalization. Additionally, inspection date parsing required careful handling to avoid silent errors when converting to datetime.

**Lesson learned:**  
In ETL workflows, documenting reasoning and context-management decisions is as important as the final output. Explicitly tracking row counts and using /compact strategically prevents confusion when working with large, multi-step pipelines.

---

## AND-2 Task 6: NLP Analysis — Subagent Research and Text Cleaning Implementation

**Date:** 2026-05-14

**Goal:**  
Build an NLP analysis notebook (intelligence/nlp_analysis.ipynb) that performs incident narrative clustering using text cleaning, TF-IDF vectorization, and K-means clustering, grounded in a research-driven method selection process.

**AI Techniques Used:**  
- **Explore subagent** was used to conduct a comprehensive comparison of LDA vs TF-IDF + clustering across six dimensions (document length, interpretability, computational cost, hyperparameter sensitivity, unknown k behavior, and final recommendation).
- **Claude Code** was used iteratively to fix encoding issues, resolve field name mismatches, handle None values, and integrate text cleaning logic into the notebook workflow.

**Interaction Summary:**

1. **Research Phase (Subagent):** Spun up an Explore agent to compare LDA and TF-IDF + clustering for short incident narratives (avg 12.6 words). The subagent produced a detailed analysis covering all six required dimensions, concluding that TF-IDF + K-means was superior for this use case due to short document length incompatibility with LDA's assumptions.

2. **Notebook Structure:** Created the research findings markdown cell at the top of the notebook, documenting the comparison and explicit justification for method choice (3–5 sentences referencing document length and hyperparameter robustness).

3. **Text Cleaning Implementation:** Added a complete text cleaning section (cells 5–8) before TF-IDF vectorization:
   - NLTK setup cell with required downloads (stopwords, wordnet, pos_tagger)
   - clean_text() function implementing: lowercasing, punctuation removal, tokenization, stopword filtering, and **lemmatization** (not stemming) using WordNetLemmatizer
   - DataFrame creation and cleaning application with sample before/after display
   - Updated TF-IDF vectorizer to use the cleaned narrative column

4. **Debugging and Fixes:**
   - Fixed encoding issues in research markdown (replaced fancy dashes with ASCII hyphens)
   - Corrected field name from 'narrative' to 'Reported occurrence narrative' (matched actual JSON structure)
   - Added None value handling using `(inc.get(...) or '')` pattern to prevent AttributeError during TF-IDF vectorization
   - Maintained all existing parameters (max_features=3000, min_df=2, max_df=0.8, ngram_range=(1,2))

**What worked:**  
- The Explore subagent provided a well-reasoned, multi-dimensional comparison that justified the method choice clearly and concretely.
- The research findings were documented as a graded deliverable before implementation, establishing clear justification for the approach.
- Text cleaning implementation was simple and readable, using standard NLTK patterns without over-engineering.
- Iterative debugging (field names, None handling, encoding) was straightforward once root causes were identified from error messages.
- Preserving the clustering workflow unchanged meant the text cleaning step was additive, not disruptive.

**What was unexpected:**  
- Encoding issues in the notebook emerged from how PowerShell handled special characters (fancy dashes became corrupted UTF-8 sequences). This required replacing special characters with ASCII equivalents rather than assuming UTF-8 would render correctly throughout the pipeline.
- The incident.json field naming ('Reported occurrence narrative' vs. assumed 'narrative') required explicit verification against the actual dataset structure. Initial assumptions about field names would have produced empty strings and silent failures.
- Some incident records had None values for the narrative field, which TF-IDF couldn't process directly. The `or ''` fallback pattern prevented AttributeError without losing data integrity.

**Design decision:**  
Lemmatization was chosen over stemming because it normalizes words to meaningful base forms (e.g., "running" → "run") rather than truncating (e.g., "runn"). For short incident narratives (5–15 words on average), preserving semantic meaning is more important than aggressive reduction.

Text cleaning was implemented as an **explicit preprocessing step** before vectorization, rather than relying on TF-IDF's built-in stop_words parameter. This made the cleaning logic auditable, repeatable, and transparent for future analysis or refinement.

**Lesson learned:**  
- **Subagent research is a graded deliverable.** Using Explore to conduct method comparison early establishes credible justification before implementation begins. This prevents post-hoc rationalization and surfaces trade-offs explicitly.
- **Field name verification is non-negotiable.** Assumptions about dataset structure must be verified against actual schemas before writing extraction logic. Silent failures (empty strings from misnamed fields) are harder to debug than explicit errors.
- **None handling in data pipelines must be deliberate.** Using fallback patterns (`or ''`) prevents downstream errors while maintaining data integrity. Different techniques (dropna(), or '', conversion to string) have different implications and should be chosen consciously.
- **Encoding issues are environmental, not logical errors.** PowerShell's cp1252 limitations don't invalidate UTF-8 content; they require pragmatic workarounds (ASCII equivalents in markdown). This distinction helps avoid unnecessary rework.

**What I'd change next time:**  
- Scaffold the full notebook structure (research section, cleaning section, vectorization, clustering) before generating any code, to reduce iteration on insertion points and encoding.
- Request field name verification from Explore when conducting initial dataset reconnaissance, rather than discovering mismatches during implementation.
- Use explicit None checks and fallback patterns earlier in the notebook to surface data quality issues before they cascade to downstream cells.

---

## AND-2 Task 6: NLP Analysis — Context Management via /compact

**Date:** 2026-05-14

**Goal**  
Perform NLP analysis on incident narratives to identify operational safety patterns using clustering, while maintaining output quality across multiple iterative steps within the same task.

**Context management decision**  
During Task 6, the analysis required multiple stages (data exploration, NLP method selection, text cleaning, vectorization, clustering, and interpretation). As the session progressed, the context window began to fill with intermediate outputs (statistics, TF‑IDF shapes, silhouette scores, cluster term lists), which risked degrading response quality.

To ensure sufficient context for high‑quality reasoning while continuing work on the same task, `/compact` was deliberately used to reduce noise and preserve only the critical decisions and evidence needed to proceed.

**What was preserved after /compact**
- Dataset characteristics: ~2,446 incident reports with ~2,445 non‑null narratives.
- Evidence that narratives are very short (average ~12.6 words), influencing method choice.
- Chosen NLP approach: TF‑IDF vectorization combined with K‑Means clustering (not LDA).
- Clustering configuration (TF‑IDF parameters, K selection via silhouette score).
- Requirement to explicitly add a full text‑cleaning step (lowercasing, punctuation removal, stop‑word removal, lemmatization) before analysis.
- Need for labeled clusters, visualization, and a concrete operational summary.

**Why /compact was appropriate**  
The task was not complete, and clearing context entirely would have required re‑establishing decisions already justified by data evidence. Using `/compact` allowed continuation of the same analytical task with a clean context window while preserving the most important assumptions, parameters, and evaluation requirements.

**Lesson learned**  
For multi‑stage NLP workflows, proactive context management is essential. Strategic use of `/compact` helps maintain analytical quality when working with iterative exploration and modeling steps, without losing alignment with task requirements or previously validated decisions.
## AND-2 Task 6: NLP Analysis — Cluster/Summary Alignment Fix

**Date:** 2026-05-14

**Goal**  
Ensure that the incident pattern summary accurately reflects the actual clustering results produced by the NLP pipeline, maintaining strict alignment between computed outputs and narrative interpretation.

**AI techniques used**  
- Used Claude Code in a strict evaluation role to review the notebook for consistency between clustering outputs and the final summary section.
- Leveraged iterative AI-assisted inspection of cluster characteristics (top TF‑IDF terms and cluster sizes) to realign interpretation with computed results.

**What worked**  
A strict evaluator-style review immediately exposed a mismatch between the cluster statistics displayed in the notebook and the summary narrative describing incident patterns. Re-extracting cluster sizes and top terms directly from the clustering output eliminated assumptions and ensured the summary was fully data-driven.

**What was unexpected**  
A minor preprocessing change (adding an NLTK tokenization dependency) propagated through tokenization, TF‑IDF vectorization, and K‑Means clustering, completely reorganizing cluster assignments without producing runtime errors. The notebook structure and parameters remained unchanged, but the semantic meaning of clusters shifted significantly.

**Design decision**  
Rather than reverting preprocessing changes or forcing clusters to match an earlier interpretation, the summary was rewritten to describe the actual clustering results shown in the notebook. Clusters were grouped into four higher-level operational categories (falls & injuries, water & flooding, door system issues, and mechanical failures) based strictly on top terms and cluster sizes, ensuring that all reported counts summed exactly to the total number of incidents.

**Lesson learned**  
In NLP workflows, preprocessing choices have cascading effects that can silently alter downstream results. Narrative summaries must always be traceable to concrete model outputs, and any change to preprocessing requires revalidation of all interpretive sections. Treating the summary as a derived artifact—rather than a static explanation—prevents evaluation-blocking inconsistencies.

---

## AND-2 Task 7: Executive Report — 6-Phase Structured Report Production

**Date:** 2026-05-15

**Goal**  
Produce a complete executive report and presentation for AND-2 Task 7, integrating findings from the ETL pipeline (Task 5) and NLP analysis (Task 6) into a single, evidence-based document with verified cost data, actionable recommendations, and a timed presentation script.

**AI techniques used**  
- **6-phase structured prompt** — The report task was decomposed into explicit phases: pre-work validation, outline and content plan, report writing, validation checklist, presentation script, and final review. Each phase produced a verifiable artifact before the next began.
- **Evaluator-style validation** — Phase 4 used a 14-criterion checklist to simulate a grader review before finalizing the report, catching issues (generic language, placeholder text) before submission.
- **Cross-session context continuity** — The task spanned multiple sessions; `/compact` was used to preserve critical decisions while discarding resolved exploration, and the session summary allowed seamless continuation without re-establishing context.

**Interaction summary**  
The report was built in two distinct stages. In the first stage, a 6-phase prompt produced the full report and presentation as chat output, synthesizing verified statistics from `intelligence/etl_pipeline.ipynb` and `intelligence/nlp_analysis.ipynb` (52,031 rows, 2,446 incidents, 4 hazard categories, 51 high-alteration elevators). In the second stage, existing documents (`docs/executive_report.md` and `intelligence/executive_report_task7.ipynb`) contained fabricated cost numbers that had never been captured from the real status bar. These were identified, replaced with verified data extracted from status bar screenshots, and cross-validated to ensure both documents were consistent.

**What worked**  
- The 6-phase structure prevented scope creep: each phase had a defined output and a clear entry condition. This made it immediately visible when a phase was complete and what the next step required.  
- The pre-work validation phase (Phase 1) surfaced that cost data for Tasks 1–6 had never been individually captured. Acknowledging this gap explicitly — rather than estimating — kept the report factually defensible.  
- Evaluator-style checklists (Phase 4) caught issues not visible during writing: recommendations that referenced data correctly but lacked a concrete action verb, and a visualization reference that pointed to the right notebook but not the specific cell.  
- Status bar screenshots (`less_expended_session.png`, `more_expended_session.png`) provided exact verified values ($0.2412 and $3.5225) that replaced all fabricated cost figures across both documents simultaneously.

**What was unexpected**  
- Both `docs/executive_report.md` and `intelligence/executive_report_task7.ipynb` already contained a cost table with fabricated numbers (~$1.50, ~$1.80, ~$2.20 per task, totaling ~$7.90) that had never been verified against any real measurement. These figures appeared plausible but were entirely invented. Identifying and replacing them required reading every cost-related cell in both documents before making any edits.  
- Markdown table formatting broke silently: row content was split across multiple lines during a prior edit, which rendered correctly in raw text but broke the table display entirely in markdown viewers. The fix was a full section rewrite rather than a targeted cell edit.  
- The actual cost difference between the two sessions was larger than expected: $0.2412 (Haiku 4.5) vs. $3.5225 (Sonnet 4.6) — a 14x multiplier driven almost entirely by model selection, not session length or task complexity.

**Design decision**  
Cost reporting was restricted to exactly two verified data points — the minimum and maximum — with no interpolation for Tasks 1–6. This was a deliberate choice to keep the report factually honest rather than statistically convenient. Any per-task estimate would have required labeling it as estimated, which would have undermined the report's credibility on the one dimension where real evidence existed.

Images were embedded directly in `docs/executive_report.md` (not just referenced) so that the visual evidence appears inline with the cost table rather than requiring the reader to locate a separate file. Both screenshots were copied to `docs/images/` to keep all report assets co-located with the document.

**Lesson learned**  
A document that looks complete can still contain fabricated data. Plausible-looking numbers that are never traced to a source measurement are indistinguishable from real ones until explicitly checked. For any section that cites metrics — especially cost, performance, or counts — tracing each value to its source before writing is more efficient than correcting fabricated values after the document exists.

Model selection is the dominant cost variable in AI-assisted development workflows. The 14x cost difference between Haiku 4.5 and Sonnet 4.6 for comparable session scopes demonstrates that task-model matching (using the cheapest model sufficient for the task) has more cost impact than any context management technique. `/compact` and subagent delegation reduce secondary cost drivers but cannot compensate for an unnecessary model upgrade.

**What I'd change next time**  
- Capture the exact session cost from the status bar at the end of each task session, not just when prompted. A one-line note with the final cost value at session close would have made the cost section trivial to populate.  
- Run a fabrication check on any document section that contains numeric claims before finalizing. Ask explicitly: "Is each number in this section traceable to a real measurement or a verified source output?"  
- For multi-document reports (markdown + notebook), maintain a single authoritative cost table in one file and reference it from the other, rather than duplicating the same values across both documents independently.

---

## AND-2 Task 7: Cost Data Integrity — Replacing Fabricated Numbers with Verified Evidence

**Date:** 2026-05-15

**Goal**  
Replace all fabricated cost estimates in `docs/executive_report.md` and `intelligence/executive_report_task7.ipynb` with the two verified data points extracted from actual status bar screenshots.

**AI techniques used**  
- **Cross-document consistency enforcement** — All 5 cost-related cells in the notebook (cell-3, cell-5, cell-7, cell-9, cell-11) and the corresponding section in the markdown file were updated in a single coordinated pass to eliminate the risk of partial updates leaving the two documents out of sync.
- **Screenshot-as-evidence** — Status bar screenshots (`less_expended_session.png`, `more_expended_session.png`) were used as the authoritative source for cost values and context percentages, with exact figures read from the image rather than recalled from memory.

**What worked**  
Updating all 5 notebook cells in parallel (using multiple `NotebookEdit` calls in a single pass) ensured that the placeholder `"Highest (in progress)"` was eliminated everywhere simultaneously. Sequential cell-by-cell updates would have created windows where the two documents were temporarily inconsistent.

**What was unexpected**  
The status bar screenshots revealed that the previously assumed final cost ($2.52) was incorrect. The actual captured value was $3.5225 for the Sonnet 4.6 session — a difference significant enough to matter in a cost analysis. Reading the value from the screenshot rather than relying on memory prevented this discrepancy from persisting into the final report.

**Lesson learned**  
Screenshots are more reliable than memory for exact cost values. When the status bar captures a value like `$3.5225`, that precision matters — rounding to `$3.52` or misremembering as `$2.52` produces a factually wrong cost table. The correct workflow is: take the screenshot at session end, read the exact value from the image, then write it into the document.

---

## AND-2 Task 2: Data Model — Field Selection and Join Key Validation

**Date:** 2026-05-11

**Task:** Define the Elevator entity data model for the operations dashboard specification, selecting fields, data types, and join logic from three source datasets.

**Interaction summary:**
I used Claude Code to review the actual schemas of `license.csv`, `inspection.csv`, and `installed.json` before writing the data model section of the dashboard specification. The goal was to ensure that every field in the data model mapped directly to a real column in a real dataset, rather than relying on assumed or idealized field names.

**Join key decision:**
`ElevatingDevicesNumber` was identified as the canonical join key across all three datasets. The field exists under slightly different names in each source:
- `license.csv`: `ElevatingDevicesNumber`
- `inspection.csv`: `ElevatingDevicesNumber`
- `installed.json`: `Elevating devices number` (spacing variation)

Explicit string normalization (`.astype(str).str.strip()`) was required before joining to prevent silent mismatches caused by whitespace or type differences. This was verified prior to writing the spec rather than discovered during implementation.

**Schema inconsistencies encountered:**
- `installed.json` uses a different column name with a space (`Elevating devices number`) compared to the other two datasets.
- Date fields in `license.csv` (`LICENSEEXPIRYDATE`) and `inspection.csv` (`Latest_INSPECTION_Date`) required explicit datetime parsing due to mixed format variation across records.
- The inspection dataset has a one-to-many relationship with the license dataset; this required the `last_inspection_date` and `last_inspection_outcome` fields to be derived from the most recent record per elevator, not sourced directly.

**Field selection reasoning:**
Fields were selected strictly from verified dataset columns. Derived fields (`location_city_region`, `last_inspection_date`, `last_inspection_outcome`) were explicitly marked as derived in the specification, and fields that do not exist for all records (e.g., Elevator Type for devices not in `installed.json`) were documented as data limitations rather than excluded from the model.

**Lesson learned:**
Data model documentation is most reliable when it is written after schema verification, not before. Documenting join keys, type mismatches, and one-to-many relationships explicitly in the specification prevents downstream implementation surprises and provides a traceable record of deliberate design decisions.

## AND-103 Task 1: Interaction Specification (SDD)

**Prompt used**
"Help me create and refine an Interaction Specification for my existing dashboard using the six SDD elements. 
The goal is to define detail panel, filter/search, and sorting behavior, including edge cases and interaction conflicts. 
Then improve the specification to make it precise and unambiguous without rewriting the structure."

**What worked**
Using a structured prompt that clearly defined constraints and scope allowed the specification to be built iteratively and remain aligned with the actual dashboard behavior. The process made it easier to identify missing details, particularly in interaction conflicts and UI structure.

**What didn’t work / issues**
The initial version of the specification was conceptually correct but not precise enough for implementation. Some behaviors (like search matching and detail panel structure) were implicitly defined rather than explicitly specified, requiring an additional refinement step.

**What I would change next time**
I initially mixed implementation details (HTMX attributes) into the specification. 
I corrected this by focusing on observable behavior rather than implementation, aligning the spec with SDD principles.
I would write the first version of the specification with more explicit structural detail, especially for UI layout and edge cases, to reduce the need for iterative refinement. This task reinforced the importance of precision in spec-driven development.

## AND-103 Task 2: Server Tests and Detail Endpoint (TDD)

**Prompt used**
"Generate pytest tests for existing endpoints (/, /table) and then extend the test suite with tests for a new /elevator endpoint before implementing it. 
After tests are written, implement the endpoint to satisfy the tests using data from merged and inspection datasets."

**What worked**
Using clear, structured prompts enabled the creation of a comprehensive test suite that validated filtering, sorting, and search behavior. Writing tests before implementing the endpoint clarified requirements and ensured the implementation was guided by expected outcomes rather than assumptions.

**What didn’t work / issues**
Some environment-related issues occurred (Flask installation, pytest setup), and initial assumptions about HTML parsing needed refinement to handle both fragment and full-table responses. However, these issues helped improve test robustness.

**What I would change next time**
I would verify environment setup and dependencies earlier before running tests. I would also define expected response formats more explicitly when writing tests to avoid ambiguity. This task confirmed that TDD improves development clarity and reduces debugging effort.

## AND-103 Task 3: Interaction Specification Audit and Refinement (SDD)

**Prompt used**
"Act as a strict spec auditor and identify all sections in my dashboard specification that violate the SDD principle of describing WHAT (behavior) instead of HOW (implementation), including references to HTMX, endpoints, and backend logic. Provide corrections for each violation."

**What worked**
The audit clearly identified places where the specification unintentionally mixed implementation details with behavior definitions. It specifically highlighted the misuse of HTMX attributes, endpoint references, and backend logic descriptions inside interaction sections. 

This helped isolate a key issue: although the specification was structurally complete (all six SDD elements were present), it was not fully aligned with SDD principles because it prescribed HOW the system should work instead of describing observable behavior.

**What didn’t work / issues**
The initial specification incorrectly included implementation-level details such as HTMX attributes (e.g., hx-get, hx-swap), endpoint names, and backend concepts (e.g., “update backend”), particularly in Task Breakdown and Prior Decisions sections. This reduced the generality of the spec and tied it to a specific implementation approach.

Additionally, some verification criteria referred to internal mechanics (e.g., “server calls”) instead of user-observable outcomes, making them less appropriate as SDD validation points.

**What I would change next time**
I would ensure from the initial drafting phase that all interaction descriptions are written in terms of observable system behavior rather than implementation details. Specifically, I would avoid referencing tools, frameworks, endpoints, or code-level constructs and instead describe how the user experiences the system.

This task reinforced that a high-quality SDD spec must be technology-agnostic and focused entirely on outcomes, constraints, and verifiable behavior. Implementation details should be deferred to the development phase, not embedded in the specification.

## AND-103 Task 3: Over-implementation and Spec Alignment

**Prompt used**
"Implement the elevator detail panel interaction using HTMX, based on the interaction specification."

**What worked**
The detail panel was successfully implemented and correctly integrated with the backend endpoint. The interaction worked as expected: selecting a row displayed the elevator’s information dynamically without a full page reload.

**What didn’t work / issues**
The implementation introduced unnecessary complexity beyond what the specification required. This included adding explicit state tracking (selected_id), out-of-band updates, and an additional endpoint to control panel behavior. 

While these solutions worked technically, they were not required by the specification and made the system more complex than necessary. Additionally, I initially considered explicitly defining state tracking behavior in the specification itself.

However, I realized that including this kind of detail would introduce implementation concerns into the spec, violating the SDD principle of separating WHAT (behavior) from HOW (implementation).

**What I would change next time**
I would strictly follow the specification as a boundary and implement only the minimum behavior required. Instead of introducing state tracking or additional mechanisms, I would rely on the behavior already defined in the spec—specifically, that the panel closes when the selected elevator is no longer visible.
