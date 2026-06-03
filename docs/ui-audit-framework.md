# UI/UX Audit Framework

> Extracted from `CLAUDE.md` (2026-06-03) to keep the root guidelines under the context-size budget. Use this framework for any future UI/UX audit on AnimeEnigma. The permanent test account it depends on is documented in [`ui-audit-test-user.md`](ui-audit-test-user.md). Reference: first audit `docs/issues/ui-audit-2026-04-07.md`.

## Methodology

Combined approach (no single technique catches everything):

1. **Static heuristic review** — Nielsen's 10 heuristics applied to screenshots + DOM
2. **Automated a11y scan** — axe-core injected via CDN script tag (loads cleanly under animeenigma.ru's CSP) and `axe.run()` over the document
3. **Per-view interaction probe** — Tab×5 (focus visibility), Esc, scroll, mid-view resize, back-nav, click below fold. Catches interaction bugs static screenshots miss.
4. **Realistic user scenarios** — drive 4-6 end-to-end flows that mirror what real users actually do (search → watch, list management, resume from history, switch player). Catches workflow gaps static review misses.
5. **Cross-view consistency sweep** — compare button styles, modal patterns, loading skeletons, focus rings across all captured views

## Tooling

- **Browser:** Chrome MCP (`mcp__claude-in-chrome__*`)
  - One browser session, no true parallelism — but parallelize JS inspection across multiple tabs in single tool-call batches
  - Use `update_plan` first to register intended domains (one-time consent)
- **axe-core:** load from `https://cdnjs.cloudflare.com/ajax/libs/axe-core/4.10.2/axe.min.js` via injected `<script>` tag in `javascript_tool` (verified to bypass CSP on animeenigma.ru)
- **Auth:** log in as `ui_audit_bot` via `fetch('/api/auth/login', ...)` from inside the page so the refresh cookie gets set properly. Inject the JWT into `localStorage.token` and the user object into `localStorage.user` (matches `frontend/web/src/stores/auth.ts:42-43`)
- **Token discipline (mandatory):**
  - `read_page` → always pass `filter: "interactive"`, `depth: 8`, `max_chars: 20000`
  - `read_console_messages` → always pass `pattern`, `limit: 30`, `clear: true`
  - `read_network_requests` → always pass `urlPattern`, `limit: 30`, `clear: true`
  - Write findings to disk view-by-view, not at end (context survives crashes)

## Severity scale (Nielsen 0-3)

| Level | Meaning | Weight for per-view score |
|---|---|---|
| 0 — Cosmetic | Fix only if extra time | 0 |
| 1 — Minor | Low priority | 1 |
| 2 — Major | High priority | 2 |
| 3 — Catastrophic | Must fix before next release | 3 |

Per-view score = `3*catastrophic + 2*major + 1*minor`. Replaces a raw count which would always make Watch.vue look worst by virtue of being the most complex.

## Citation rules (no hallucinated line numbers)

Every finding cites code as `(file_path — found via grep "anchor string")`. The anchor string MUST be a verbatim string from a `Grep` call performed in the same audit pass.

✅ `frontend/web/src/views/Home.vue — found via grep "Continue Watching"`
❌ `frontend/web/src/views/Home.vue:142` (line number from memory — forbidden)

If a finding can't be anchored to a real grep result, the citation is `(no code anchor found — visual evidence only)`.

## Per-finding template

```markdown
##### [UA-NNN] One-line title — Severity N (label) — category

**View:** Which view + viewport
**Heuristic:** Nielsen #N or category
**Evidence:**
- Concrete observation 1 (DOM query, axe rule, screenshot reference)
- Concrete observation 2
- Cross-reference to seeded data / DB state if relevant

**Why it matters:**
1. User-impact statement
2. Accessibility / SEO / consistency / etc.

**Citations:**
- `path/to/file.vue — found via grep "anchor"`

**Proposed fix:** Concrete steps the implementer can take.
```

## Realistic user scenarios (drive these in addition to per-view static audit)

For AnimeEnigma specifically, the highest-value scenarios are:

**Navigation scenarios:**
- N1: Search for a specific anime → open detail → start watching
- N2: Browse by genre filter → paginate → open detail
- N3: Mobile: switch between Home / Browse / Profile via the hamburger or persistent nav

**List management scenarios:**
- L1: From anime detail, add to watchlist with status "watching"
- L2: From watchlist view, change status (watching → completed)
- L3: View watchlist filtered by status, sort by score

**Watching scenarios:**
- W1: Anonymous: visit Watch view → does the player load without auth?
- W2: Logged in: resume an in-progress episode from watch history
- W3: Switch player or translation mid-episode (Kodik = limited, others = full control)

Each scenario gets its own findings sub-section with friction points, dead ends, missing affordances, and any observed errors.

## Output structure

Single markdown file: `docs/issues/ui-audit-YYYY-MM-DD.md` with sections:
- Header (site, locale, account, methodology, scope, tooling)
- Summary (counts, weighted scores, top quick wins, top high-impact fixes)
- Findings by severity (catastrophic → major → minor → cosmetic)
- Findings by view (same items, regrouped for "fix this view" sessions)
- Realistic-scenario findings (one section per scenario)
- axe-core raw output per view
- Cross-view inconsistencies
- Audit notes (token budget consumed, transient findings filtered, bot detection, truncations)

## Checkpoints (3 mandatory user gates)

1. After dry-run on first view — validate format
2. After all per-view audits complete (before scenarios) — confirm on track
3. Before commit/push — final review for redaction, accuracy, completeness

## Don't do

- Don't cite line numbers from memory — only from grep calls in the same pass
- Don't mark a finding as "real" if it only reproduces once (transient — must repro on 2+ navigations)
- Don't bypass bot detection — abort the audit and report which view tripped it
- Don't auto-invoke `animeenigma-after-update` after the audit — wait for explicit user "ship it"
- Don't create accounts in production for the audit; reuse `ui_audit_bot`
