# AI Workflow Baseline – Module 101

This document describes my current baseline workflow when using AI tools, prior to starting development on the Rocket Elevators Operations Dashboard. The goal is to capture how I actually use AI today, including strengths, limitations, and habits, before intentionally improving my workflow throughout the course.

---

## Tools

I currently use the following AI tools:

- **Claude Code (VS Code extension)**: My primary tool for software development tasks, including code generation, repository-aware changes, and documentation updates.
- **Gemini**: Used mainly for information retrieval, conceptual clarification, theory validation, and occasionally for image editing or fact validation.
- **ChatGPT**: Used primarily to generate or refine prompts for other generative AI tools rather than as a direct problem‑solving assistant.
- **GitHub Copilot**: Used in professional contexts because it is the company-standard tool, but not my preferred choice for complex development tasks.

I use Gemini and Claude Code most frequently. ChatGPT and Copilot are used less often, mainly due to perceived limitations in output quality for my typical use cases.

I intentionally stopped using Copilot as a primary tool after participating in a small university experiment comparing output quality versus number of interactions. Copilot ranked lowest in that comparison, and I also found its lack of project-level context and heavy chat interface inefficient for sustained work.

---

## Use Cases

I typically rely on AI tools in the following concrete scenarios:

- When I have a clear idea of what I want to build but want to validate whether I am missing edge cases or architectural considerations.
- When debugging errors that are not immediately obvious.
- When performing repetitive tasks such as generating similar code or test structures.
- When clarifying the purpose or behavior of an unfamiliar block of code.

A concrete example: while implementing a GET endpoint based on two database queries, I asked the AI to generate a similar endpoint following the existing project patterns. I used the output as a reference to align with established conventions, fix query formatting, generate unit tests, and draft Swagger documentation.

---

## Typical Interaction

I usually decide to use AI only after I fully understand the task and the desired outcome. I start by defining the role the AI should take (e.g., assistant developer, documentation helper) and providing a base prompt with clear constraints.

My prompts often include existing queries, models, or patterns that must be followed, and I explicitly instruct the AI not to modify certain elements without confirmation.

Sessions typically last between one and two hours, depending on repository complexity, integration issues, or quality constraints such as SonarQube findings. I rarely copy outputs directly; instead, I read the response carefully and then manually implement or rewrite the solution to internalize the logic and maintain control.

---

## What Works Well

AI tools are most effective for me when used to:

- Generate initial project scaffolding
- Draft documentation
- Create unit tests
- Replicate existing patterns observed in a repository

I trust AI the most when it generates code based on already implemented code, as it tends to extract the most relevant characteristics and apply them consistently in new components.

I am comfortable receiving code or explanations intended for review rather than direct copy‑paste, and I often provide feedback or request adjustments before applying changes.

---

## What Does Not Work / Frustrations

The main frustrations in my current AI workflow are:

- Receiving excessive or indirect information that was not requested
- AI failing to challenge non-viable options that I propose
- Hallucinated information that is not supported by the actual repository or project context

When this happens, I usually prefer starting a new conversation instead of continuing an existing one, as context degradation often leads to wasted tokens and poorer results.

---

## Confidence Level

My confidence in AI-generated outputs depends heavily on the model being used. On a scale from 1 to 5, my confidence level for **Claude Code** is **4**, while it is significantly lower for Copilot.

I verify AI output by:
- Carefully reviewing the logic
- Running and testing the code locally
- Comparing behavior against benchmarks or expectations

I have encountered multiple situations—particularly with Copilot—where an output appeared correct but failed in practice, which has influenced my overall level of trust.