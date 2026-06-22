---
name: Rocket Elevators Operations Dashboard
description: Calm, precise instrument panel for an Ontario elevator fleet
colors:
  shell-900: "#0f172a"
  shell-800: "#1e293b"
  shell-700: "#334155"
  ink: "#1e293b"
  ink-body: "#334155"
  ink-soft: "#475569"
  muted: "#94a3b8"
  line-strong: "#cbd5e1"
  line: "#e2e8f0"
  line-soft: "#f1f5f9"
  page: "#f1f5f9"
  surface: "#ffffff"
  surface-alt: "#f8fafc"
  state-good: "#16a34a"
  state-good-bg: "#dcfce7"
  state-good-ink: "#15803d"
  state-warn: "#d97706"
  state-warn-bg: "#fef3c7"
  state-warn-ink: "#b45309"
  state-bad: "#dc2626"
  state-bad-bg: "#fee2e2"
  state-bad-ink: "#b91c1c"
  interaction: "#2563eb"
  interaction-bg: "#eff6ff"
typography:
  metric:
    fontFamily: "IBM Plex Mono, ui-monospace, SFMono-Regular, Menlo, monospace"
    fontSize: "2.25rem"
    fontWeight: 700
    lineHeight: 1.1
    letterSpacing: "-0.01em"
  title:
    fontFamily: "IBM Plex Sans, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 600
    lineHeight: 1.4
    letterSpacing: "normal"
  body:
    fontFamily: "IBM Plex Sans, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: "normal"
  label:
    fontFamily: "IBM Plex Sans, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: "0.05em"
  readout:
    fontFamily: "IBM Plex Mono, ui-monospace, SFMono-Regular, Menlo, monospace"
    fontSize: "0.75rem"
    fontWeight: 500
    lineHeight: 1.4
    letterSpacing: "-0.01em"
rounded:
  sm: "4px"
  md: "8px"
  pill: "9999px"
spacing:
  xs: "8px"
  sm: "12px"
  md: "20px"
  lg: "28px"
components:
  card:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.md}"
    padding: "20px"
  badge-good:
    backgroundColor: "{colors.state-good-bg}"
    textColor: "{colors.state-good-ink}"
    rounded: "{rounded.sm}"
    padding: "2px 8px"
  badge-warn:
    backgroundColor: "{colors.state-warn-bg}"
    textColor: "{colors.state-warn-ink}"
    rounded: "{rounded.sm}"
    padding: "2px 8px"
  badge-bad:
    backgroundColor: "{colors.state-bad-bg}"
    textColor: "{colors.state-bad-ink}"
    rounded: "{rounded.sm}"
    padding: "2px 8px"
  button-primary:
    backgroundColor: "{colors.shell-900}"
    textColor: "{colors.surface}"
    rounded: "{rounded.md}"
    padding: "8px 12px"
  button-primary-hover:
    backgroundColor: "{colors.shell-700}"
  nav-link-active:
    backgroundColor: "{colors.shell-700}"
    textColor: "{colors.surface}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-body}"
    rounded: "{rounded.sm}"
    padding: "6px 8px"
---

# Design System: Rocket Elevators Operations Dashboard

## 1. Overview

**Creative North Star: "The Instrument Panel"**

This dashboard reads like a calibrated readout, not a website. The numbers lead and
everything else gets out of their way. A dark slate shell frames bright white data
surfaces; monospaced numerals (IBM Plex Mono) give every count, score, and ID the feel
of a gauge you can trust. Color is never decoration — green, amber, and red carry
operational meaning (status, risk, inspection outcome) and nothing else. Blue is held in
reserve for interaction (focus, selection) so it can never be mistaken for data.

The system is calm by default and loud only where it must be. A high-risk badge or an
overdue date stands out precisely because the surrounding chrome is quiet. Density is a
feature: operations staff scan many rows fast, so the layout favors clear hierarchy over
whitespace for its own sake. Depth is flat — white surfaces separated by hairline slate
borders and a single whisper of shadow, never stacked drop shadows.

This system explicitly rejects three looks. It is **not consumer SaaS marketing** (no
gradients, hero-metric templates, glassmorphism, or decorative illustration). It is
**not a generic admin template** (no identical card grids, no heavy shadows, no
default-everything). And it is **not cluttered legacy enterprise** (hierarchy stays
sharp even at high data density).

**Key Characteristics:**
- Mono numerals for all data readouts; sans for everything else.
- Semantic green / amber / red only; blue only for interaction.
- Flat surfaces, hairline borders, one soft shadow at most.
- Dark shell, bright content — a control surface, not a brochure.
- Density with clear hierarchy.

## 2. Colors

A restrained slate neutral base, lit by a tight three-color state vocabulary and a single
interaction blue.

### Primary
- **Shell Slate** (#0f172a): The sidebar, chat header, and primary buttons. The dark
  frame that holds the bright data surfaces. Hover lifts to **Shell Slate 700** (#334155),
  which also marks the active nav link.

### Secondary
- **Signal Green** (#16a34a): "Good" state — active licenses, low risk, passed
  inspections. Also the metric color on the Active Elevators card.
- **Signal Amber** (#d97706): "Attention" state — by-request licenses, medium risk,
  follow-up inspections, licenses expiring soon (only when the count is > 0).
- **Signal Red** (#dc2626): "Risk" state — overdue inspections, high risk, failed
  inspections. The strongest color on the page; used sparingly.

### Tertiary
- **Interaction Blue** (#2563eb): Keyboard focus ring and selected-row accent
  (#eff6ff fill, blue left edge). Reserved for interaction only — never used for data.

### Neutral
- **Ink** (#1e293b) / **Body Ink** (#334155): Headings and body text on white.
- **Soft Ink** (#475569): Monospaced IDs and secondary table text.
- **Muted** (#94a3b8): Uppercase labels, placeholders, "—" empty markers.
- **Line** (#e2e8f0) / **Strong Line** (#cbd5e1): Card, table, and panel borders;
  scrollbar thumb.
- **Page** (#f1f5f9): The app background behind all surfaces.
- **Surface** (#ffffff) / **Surface Alt** (#f8fafc): Cards, table, panels; alt is the
  row-hover and chat message background.

### Named Rules
**The Two-Channel Color Rule.** Green / amber / red mean *data state*. Blue means
*interaction*. The two channels never cross: blue is never a status, and a state color is
never a focus ring or a selection. This is why a user can trust the colors at a glance.

**The Quiet-Chrome Rule.** The slate shell and neutrals fill ~90% of every screen. A
state color earns its place only by attaching to a real signal.

## 3. Typography

**Display / UI Font:** IBM Plex Sans (with ui-sans-serif, system-ui fallback)
**Data / Mono Font:** IBM Plex Mono (with ui-monospace, Menlo fallback)

**Character:** One humanist sans for all interface text, paired with its monospaced
sibling for data. The pairing is intentional contrast — sans for language, mono for
numbers — so readouts line up in tabular columns and feel like instrument values, not
prose.

### Hierarchy
- **Metric** (mono, 700, ~2.25rem / `text-4xl`, tabular nums): The big card numbers.
  The largest element on any screen.
- **Title** (sans, 600, ~1.125rem): Panel and section headings.
- **Body** (sans, 400, 0.875rem / `text-sm`): Table cells, panel text, chat messages.
  Tables may run dense and wide (120ch+ is fine).
- **Label** (sans, 600, 0.75rem / `text-xs`, uppercase, +0.05em tracking): Card labels
  and table headers. Always muted slate-400.
- **Readout** (mono, 500, 0.75rem, tabular nums, -0.01em): Inline IDs, license numbers,
  risk scores, legend counts.

### Named Rules
**The Mono-for-Data Rule.** If it is a number a user reads as a value — a count, a
score, an ID, a date — it is set in IBM Plex Mono with tabular figures. Language is
always IBM Plex Sans. Never mix the two roles.

**The Muted-Label Rule.** Labels and column headers are small, uppercase, tracked, and
muted (slate-400). They name the data without competing with it.

## 4. Elevation

The system is **flat by default**. Surfaces are white, separated from the slate page by a
single hairline border (1px, #e2e8f0) and at most one soft, low shadow (`shadow-sm`).
Depth comes from tone (page-gray behind, white surface in front) and from borders, not
from stacked shadows. There is no glassmorphism and no heavy drop shadow anywhere.

### Shadow Vocabulary
- **Card rest** (`box-shadow: 0 1px 2px rgba(0,0,0,0.05)` — Tailwind `shadow-sm`): The
  only ambient shadow. Lifts cards and panels just off the page.
- **Hover lift** (`shadow` — slightly stronger): Used only on interactive cards (the
  Active Elevators drill-down) to confirm they are clickable.

### Named Rules
**The Flat-By-Default Rule.** A surface is flat at rest with a hairline border. Shadow
appears only as a whisper on cards, and a step stronger only as hover feedback on a
clickable surface. If a shadow blur is 16px or more, it is wrong here.

## 5. Components

### Buttons
- **Shape:** Rounded (8px / `rounded-lg` for actions; the round 56px FAB for the chat).
- **Primary:** Shell Slate (#0f172a) background, white text. Used for the chat send
  button and the chat FAB.
- **Hover / Focus:** Background lifts to Shell Slate 700 (#334155); a 2px blue
  focus-visible ring appears on keyboard focus only. Disabled drops to ~40% opacity.

### Chips
- **Style:** Pill (`rounded-full`), white background, 1px slate-300 border, slate-600
  text. Used for the chat suggested-prompt chips.
- **State:** Hover deepens the border to slate-500 and the text to slate-800.

### Cards / Containers
- **Corner Style:** 8px (`rounded-lg`).
- **Background:** White (#ffffff) on the slate page (#f1f5f9).
- **Shadow Strategy:** `shadow-sm` at rest (see Elevation). Interactive cards add a
  stronger `shadow` plus a slate-300 border on hover.
- **Border:** 1px slate-200 (#e2e8f0).
- **Internal Padding:** 20px (`p-5`).

### Inputs / Fields
- **Style:** 1px slate-200 border, 4px radius, white background, slate-700 text, slate-400
  placeholder. The table search input is borderless and rides inside the controls bar.
- **Focus:** Border shifts to slate-400; the global 2px blue ring shows on keyboard focus.

### Navigation
- **Style:** Vertical sidebar links in the dark shell. Sans, 0.875rem, medium weight,
  with a 16px leading icon.
- **States:** Default slate-300 text; hover fills slate-800 with white text; **active**
  fills slate-700 with white text (4px radius). One link is always marked active.

### Status & Risk Badges (signature component)
The core data component. A small inline badge: 4px radius, `px-2 py-0.5`, 0.75rem medium
text, a tinted background, matching ink, and a 1px ring one step darker.
- **Good** (green-100 bg / green-700 text / green-200 ring): ACTIVE, LOW risk, Passed.
- **Attention** (amber-100 / amber-700 / amber-200): BY REQUEST, MEDIUM risk, Follow up.
- **Risk** (red-100 / red-700 / red-200): cancelled, HIGH risk, Fail.
- **None:** a muted slate-400 "—", never an empty cell.

### Risk Donut (signature component)
A pure-CSS `conic-gradient` ring (no JavaScript, no chart library), 128px, with an inset
white hole carrying the total device count in mono. Segment colors match the badge
scheme: green / amber / red / slate-400 for no-prediction.

### Selected Table Row (signature component)
The selected row gets a blue-50 (#eff6ff) fill and a 3px Interaction Blue inset on the
left edge — the one place blue marks state, and it marks *selection*, not data.

## 6. Do's and Don'ts

### Do:
- **Do** set every data value (count, score, ID, date) in IBM Plex Mono with tabular
  figures; keep IBM Plex Sans for all language.
- **Do** use green / amber / red only to signal real state, and keep blue (#2563eb) for
  interaction only — focus rings and selection.
- **Do** keep surfaces flat: white fill, 1px slate-200 border, at most `shadow-sm`.
- **Do** pair every color signal with a text label (the badge shows "HIGH", not just
  red) so the meaning survives for color-blind users — WCAG 2.1 AA.
- **Do** keep labels and table headers small, uppercase, tracked, and muted (slate-400).
- **Do** show a muted "—" for missing data, never a blank cell.

### Don't:
- **Don't** ship consumer-SaaS marketing patterns: no gradients, no hero-metric
  template, no glassmorphism, no decorative illustration. This is an internal ops tool.
- **Don't** drift toward a generic admin template: no identical icon-card grids, no heavy
  drop shadows, no default-everything components.
- **Don't** recreate the cluttered legacy-enterprise look: keep hierarchy sharp even when
  the table is dense.
- **Don't** use blue for any data value, and don't use green / amber / red for focus,
  hover, or selection. The two color channels never cross.
- **Don't** use a shadow blur of 16px or more, or stack shadows for depth.
- **Don't** set body or label text in light gray below 4.5:1 contrast "for elegance".
