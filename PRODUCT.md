# Product

## Register

product

## Users

Rocket Elevators operations staff — primarily the operations manager — monitoring a
licensed elevator fleet across Ontario. They work at a desk, on a wide screen, often
under time pressure. Their context is task-driven, not exploratory: they open the
dashboard to answer a specific question (fleet status at a glance, which elevators are
overdue or high-risk, the full profile of one device) and act on it.

## Product Purpose

A single operations dashboard that replaces scrolling through spreadsheets. It unifies
license, inspection, incident, alteration, and ML-risk data into one view so staff can:
see fleet health at a glance (summary cards, risk donut), find any elevator fast
(search, filter, sort, paginate), read a full per-elevator profile (detail panel), and
act on the riskiest devices (critical alerts, schedule inspections via the OpsBot chat).
Success looks like: the right elevator and its risk are found in seconds, and no one
needs to open a raw CSV.

## Brand Personality

Calm, precise, trustworthy. The feel of a good instrument panel: quiet by default, with
state shown clearly the moment it matters. Three words: **calm, precise, dependable**.
The interface should disappear into the task. Confidence comes from clarity and
consistency, not decoration. Mono-numeric readouts (IBM Plex Mono) give data an
instrument-panel feel; the slate shell stays in the background so the data leads.

## Anti-references

- **Consumer SaaS marketing.** No gradients, hero-metric templates, glassmorphism,
  decorative illustrations, or flashy color. This is an internal operations tool.
- **Generic admin template.** Avoid the AdminLTE / Bootstrap look — identical card
  grids, heavy drop shadows, default-everything components.
- **Cluttered legacy enterprise.** Avoid the dense, gray, hard-to-read
  government-portal feel. Hierarchy must stay clear even at high data density.

## Design Principles

- **The data leads, the chrome recedes.** Color and weight serve the numbers and
  states; the shell stays quiet.
- **State at a glance.** Green/amber/red carry meaning (status, risk, outcome) and are
  never decorative. Blue is reserved for interaction (focus, selection) so it can never
  be confused with data.
- **Earned familiarity over surprise.** Standard product affordances (side nav, top bar,
  data table, detail panel). The tool should feel obvious, not clever.
- **Density with clarity.** Show a lot, but keep visual hierarchy sharp so nothing reads
  as cluttered.
- **Consistency screen to screen.** Same badge system, same card vocabulary, same
  control shapes across Overview, Fleet, and Alerts.

## Accessibility & Inclusion

Target **WCAG 2.1 AA**. Body text ≥ 4.5:1 contrast, large text ≥ 3:1; visible
keyboard focus on every control (already present via a global focus-visible ring);
`prefers-reduced-motion` honored for page transitions and any future motion. Color is
never the only signal — status and risk use a text label plus color, not color alone,
to stay readable for color-blind users.
