# Project Conventions

Durable rules for how planning, scoping, and feature work is described in this repo. Every new plan, phase, feature, or significant change MUST follow these — they apply to humans, sub-agents, and orchestrators alike.

---

## Effort & impact metrics — no days, no hours, no sprints

**Adopted 2026-05-19.** Time-effort units (days, hours, sprints, ideal-engineer-time) are NOT used in this project. Three dimensional metrics replace them. Every plan or feature SHOULD be scored on all three; they answer different questions and stay separable.

The user reserves the right to adjust weights, examples, and wording over time — when calibration shifts, update this doc and the matching memory file (`/root/.claude/projects/-data-animeenigma/memory/feedback_no_days_metric.md`).

---

### 1. UXΔ — UX Delta Score

How much, and in which direction, user experience shifts when a feature ships.

- **Range:** `-5` to `+5`, signed
- **Sub-signals** (weighted, normalized to `-5/+5` each):
  - Task completion time change — **40%**
  - User error rate change — **30%**
  - Subjective satisfaction shift (survey or session replay) — **30%**
- **Direction label** (always include):
  - `Better` — aggregate `+1 to +5` *and* sub-signals agree
  - `Worse` — aggregate `-5 to -1` *and* sub-signals agree
  - `Ambiguous` — aggregate is `0`, OR sub-signals disagree (e.g. faster completion but more errors) *regardless* of aggregate
- **Report format:** `UXΔ = +3 (Better)` or `UXΔ = -1 (Ambiguous)`

---

### 2. CDI — Coherence Disruption Index

How much the change disturbs the existing system's coherence, amplified by effort. Reported as **two numbers separated by `*`** (like MVQ keeps creature + two percentages); the multiplication is the conceptual model — effort amplifies disruption — but the values stay legible separately.

- **Form:** `CDI = (Distribution Spread × Coherence Shift) * Effort`
- **Left side (A factor):** `Distribution Spread × Coherence Shift`
  - **Distribution Spread:** `touched_components / total_components`, range `0.0–1.0`. Count every component: API layer, schema, UI module, lib, config file.
  - **Coherence Shift:** qualitative `1–5`
    - `1` — extends existing pattern
    - `3` — new but compatible pattern
    - `5` — contradicts existing pattern, forces refactoring
- **Right side (E factor):** Fibonacci Effort rating: `1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 233`. Larger if needed ("up to hundreds").
- **Intuition behind the `* E` amplifier:** effort acts as a risk amplifier. A change that touches few components and stays within an existing pattern but takes huge effort is *more disruptive than its A-factor suggests* — rollback cost scales with effort, and unknown-unknowns scale with how long the work occupies the codebase. The Fibonacci scale is intentionally non-linear to discourage false precision.

#### Effort calibration anchors

| Fib | Anchor description |
|-----|---------------------|
| `1` | trivial single-line tweak |
| `2` | small isolated function edit |
| `3` | simple partial-refactor or mostly code-style changes |
| `5` | small new feature with tests |
| `8` | standard new feature confined to one module |
| `13` | medium feature, multi-module |
| `21` | large feature, cross-service |
| `34` | significant phase-of-work |
| `55` | research + really big isolated change with tests |
| `89` | milestone-scale |
| `144` | epic |
| `233+` | should be split before committing |

#### User-supplied canonical examples

- `CDI = 0.01 * 55` — research + really big isolated change with tests
- `CDI = 0.8 * 3` — simple partial-refactor (mostly code-style changes)

#### Report format

`CDI = 0.02 * 13` — read as "small spread, compatible pattern, medium multi-module effort."

**Do NOT pre-multiply into a single number.** The two-number form is the rule. The product `0.26` would hide which factor is driving the score.

---

### 3. MVQ — Mythic Vibe Quotient 🐉

Assigns a mythic creature whose personality represents the feature's vibe, plus two percentages: how well the feature embodies that archetype, and how strongly it resists slop.

#### Creature taxonomy

| Creature | Archetype |
|----------|-----------|
| **Phoenix** 🔥 | Transformative, rises-from-ashes, renewal energy |
| **Griffin** 🦅 | Elegant hybrid; combines disparate things into one form |
| **Kraken** 🐙 | Massive scope, overwhelming but impressive |
| **Sprite / Pixie** ✨ | Small, delightful, whimsical polish |
| **Basilisk** 🐍 | Dangerous complexity, powerful but risky to look at directly |
| **Dragon** 🐲 | Ambitious, showy, high-impact centerpiece |

Pick the single creature whose archetype best matches the feature's intrinsic energy. If two fit equally, prefer the one that better describes the feature's *failure mode* (Basilisk over Dragon for risky-ambitious; Sprite over Griffin for small-elegant).

#### Two percentages

- **Creature Vibe Match (`0–100%`):** how well does the feature actually embody the assigned creature's archetype? A feature tagged Phoenix that genuinely reinvents a user flow scores `~90%`. One that just renames buttons under the same tag scores `~15%`.
- **Slop Resistance (`0–100%`):** how much of the feature feels intentional and crafted vs. generic and phoned-in? High slop = AI-autocompleted design with no human soul.

#### Report format

`MVQ = Griffin 82%/91%` — read as "elegantly combines two things, vibe match 82%, resists slop at 91%."

A bad feature might be `MVQ = Basilisk 30%/25%` — complex and dangerous, doesn't deliver on the archetype, and most of it feels sloppy.

---

## Worked examples — Phase 28 (calibration anchor for the three metrics together)

| Item | UXΔ | CDI | MVQ |
|------|-----|-----|-----|
| Miruro obfuscation spike | `0 (Ambiguous)` | `0.02 * 34` | `Basilisk 75%/90%` |
| AnimeFever embed recon | `0 (Ambiguous)` | `0.01 * 3` | `Sprite 60%/85%` |
| AnimeFever provider lift | `+2 (Better)` | `0.02 * 13` | `Griffin 85%/80%` |
| New embed extractors | `+1 (Better)` | `0.015 * 8` | `Sprite 70%/80%` |
| Miruro provider lift (conditional) | `+2 (Better)` | `0.04 * 21` | `Phoenix 70%/85%` |
| 9anime.me.uk lift | `0 (Ambiguous)` | `0.075 * 13` | `Basilisk 40%/30%` |
| Dropdown polish + after-update | `+3 (Better)` | `0.015 * 5` | `Sprite 88%/92%` |

When in doubt about a score, compare against these anchors and pick the closest match.

---

## How to apply

1. **Every new PLAN file** — score the plan as a whole on all three metrics, in its front-matter or top-of-doc summary. Where individual tasks differ materially, score them per-task too.
2. **Every new phase CONTEXT.md** — score the phase aggregate (or list per-plan scores; phase aggregate is the sum/blend across plans, judgment call).
3. **Every feature in CHANGELOG.json** — include the triple in the changelog body when it's a meaningful user-facing entry. Not required for purely-internal commits.
4. **Time-BOXES are not effort estimates** — a "spike kill-switch if no convergence in N agent-sessions" is a real workflow cutoff and should stay as a cutoff phrase. Do NOT translate it into UXΔ/CDI/MVQ. Conversely, "this task will take ~N days" IS an estimate and MUST be reframed to the three metrics.
5. **Sub-agents** (gsd-planner, gsd-executor, gsd-code-reviewer, etc.) read this file when scoring. If a sub-agent ships a plan with day-estimates, push back and re-score before merging.
6. **Calibration updates** — when the user revises weights, examples, or wording, update both this file AND the matching memory file (`feedback_no_days_metric.md`) in the same commit so the convention stays in sync across the orchestrator's memory and the repo.

---

## Cross-references

- Memory mirror (orchestrator-side): `/root/.claude/projects/-data-animeenigma/memory/feedback_no_days_metric.md`
- CLAUDE.md pointer: under `## Code Conventions` → `### Effort & impact metrics`
