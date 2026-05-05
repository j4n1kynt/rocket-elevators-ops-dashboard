## Entry – Updating README with Claude Code

**Task:** Update README.md to meet Task 1 requirements.

**Prompt used:**
"Update the existing README.md to include the project name, a one-paragraph description, and a list of the four main directories with explanations. Keep it concise and limited to Module 101."

**What worked:**
Claude Code produced a clean, well-structured README that clearly listed the required directories and aligned with the project scope. The concise prompt helped avoid unnecessary features or future assumptions.

**What didn’t work or was unexpected:**
In an earlier attempt, a less structured prompt led Claude to mention features not yet implemented, which required manual correction, in addition to creating a new README.md instead of modifying the existing one.

**What I’d change next time:**
Be explicit about constraints (e.g., module scope, no future features) to reduce over-generation.

**Lesson learned:**
Structured, constraint-driven prompts lead to more accurate documentation updates than open-ended requests, especially when modifying existing files instead of generating new ones.

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

## Entry – Dashboard Specification Iteration

**Task:** Write a technical specification for the Rocket Elevators Operations Dashboard (Task 3).

**Initial prompt used:**
"Write a technical dashboard specification in plain English based on the operations manager’s request, including page layout, summary cards, and a detail table with columns, data types, and display formats."

**What worked:**
Claude Code generated a clear and well-structured first version of the dashboard specification. The layout, summary metrics, and table columns closely matched the operations manager’s request, and the level of detail was sufficient for implementation without follow-up questions. In particular, the metric calculations and table definitions were strong and easy to understand.

**What didn’t work or was unexpected:**
The initial output was more detailed than necessary for a “keep it simple” requirement. It included layout specifics, interaction details, and dataset assumptions that went beyond what was explicitly requested. This made the spec feel closer to a final UI design rather than an initial technical specification.

**What I changed:**
I provided corrective feedback to simplify the document, remove over-specific details, reduce assumptions about the dataset, and focus strictly on the operations manager’s stated needs. Claude Code successfully revised the specification while preserving its overall structure.

**Lesson learned:**
AI can produce very complete outputs, but completion is not the same as correctness for a given context. Clear constraints and targeted feedback are more effective than regenerating content from scratch. Iterating on AI output helped me align the specification with business priorities instead of unnecessary technical precision.


## Entry – Refining Dashboard Specification Based on Real Datasets

**Task:** Write and finalize the technical dashboard specification for the Rocket Elevators Operations Dashboard (Task 3).

**Initial prompt / interaction:**
I initially asked Claude Code to generate a dashboard specification based on the operations manager’s request, focusing on layout, summary metrics, and a detail table. The first versions of the specification were business-aligned but relied on assumed or generic field names (e.g., “Last Inspection Date,” “Elevator Type”) rather than confirmed dataset fields.

**What worked:**
Claude Code was very effective at translating the business request into a clear dashboard structure (sidebar, summary cards, detail table) and at defining metrics and layout clearly in plain English. Iterating on the specification through targeted prompts helped progressively align the document with both stakeholder needs and technical constraints.

**What didn’t work or was unexpected:**
The initial specification did not fully reflect the actual structure of the available datasets. After reviewing `license.csv` and `inspection.csv`, it became clear that some assumptions about available fields were incorrect or incomplete. A clarification from the course tutor confirmed that table columns must come directly from the real data files, not from reasonable guesses.

**What I changed:**
I revised the specification to explicitly review and reference the real datasets (`license.csv`, `inspection.csv`, and later `installed.json`). Table columns were updated to map directly to existing fields, summary metrics were tied to concrete data sources, and joins between datasets were documented using `ElevatingDevicesNumber`. Fields that do not exist (or are not present for all records) were clearly documented as data limitations instead of being invented.

**Lesson learned:**
A dashboard specification should be business-driven but data-aware. Reviewing real datasets early helps avoid invalid assumptions, while still keeping the specification focused on stakeholder needs rather than raw data exploration. Iterating on the spec based on new information (including tutor feedback) was more effective than trying to design a “perfect” spec in one pass.
``