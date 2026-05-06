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
