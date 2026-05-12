# UX Reassessment — 2026-05-12 Design Spec

**Status:** Draft, awaiting user approval
**Author:** Claude Opus 4.7
**Spec date:** 2026-05-12
**Predecessor audits:** [2026-04-07](../../issues/ui-audit-2026-04-07.md) (full), [2026-04-17](../../issues/ui-audit-2026-04-17.md) (desktop re-audit), [2026-04-20](../../issues/ui-audit-2026-04-20.md) (mobile re-audit)

## Goal

Establish current UX/a11y state of AnimeEnigma five weeks (392 commits) after the last full audit. Surface new findings on surfaces that have never been audited, verify prior fixes held, and benchmark our UX against leading anime sites (EN + RU markets). **No fixes are applied in this pass** — output is a tiered improvement report.

## Why now

- ~5 weeks since the last full audit; only a mobile re-audit since 2026-04-20.
- Significant new surface area landed: recommendations engine (Trending row, pinned-rec treatment, RecItem top-contributor), Anime English tab, Profile URL-import + server-side watchlist search, Auth instant-QR + native-app deeplink + tg-web fallback, Navbar avatar image rendering, AnimeLib Kodik-fallback drop, Admin recs debug page.
- Carry-over work from 2026-04-20 (Batches G/H/I — 16 findings UA-042…UA-056) hasn't been verified shipped.
- Admin tools (Grafana especially, plus Prometheus / Loki / pgAdmin / Admin recs) have never been included in any audit and the user flags Grafana as visually rough.
- Competitive benchmarking has never been done — we don't know where we stand vs Crunchyroll / animejoy / animevost / yummyanime / 9anime on concrete UX dimensions.

## Scope

### Tier 1 — New surfaces (first-ever audit)

Each gets a full per-view audit (axe-core + DOM probes + screenshots + Nielsen heuristics) at both viewports.

| Surface | Component(s) | What to probe |
|---|---|---|
| Home — Trending-now row | `Home.vue`, `RecItem.vue` | Row label, empty state, horizontal scroll a11y, focus order, pinned-rec border/badge contrast, top_contributor display, `rec_click` instrumentation |
| Home — Up-next row | `Home.vue`, `useRecs` composable | Auth-gated state transitions, refresh on login |
| Anime detail — English tab | `Anime.vue` | Tab a11y (role=tab, aria-selected, aria-controls), default-flip logic, `legacy=1` gating, RU↔EN switch behavior |
| Profile — URL import | `Profile.vue` / import dialog | Accepts URLs vs usernames, error message quality (human-readable failures), validation feedback |
| Profile — watchlist server-side search | watchlist view | List persists during refetch (no flicker), empty/no-results state, debounce feel, server-side accuracy |
| Auth — QR + deeplink + fallback | `Auth.vue`, Telegram QR flow | QR rendering, deeplink trigger on mobile, tg-web fallback path, error states, i18n `@` escaping (UA-NEW candidate) |
| Navbar — avatar image | `Navbar.vue` | Image rendering vs initials fallback, loading state, broken-image fallback, alt text |
| Resume banners — repositioning | `Anime.vue` | New position below player, toned colors, contrast, mobile stacking |

**Excluded from Tier 1:** EnglishPlayer.vue and English-source flows (Universal Anime Scraper still in active development — would generate findings against a moving target).

### Tier 2 — Carry-over verification (Batches G/H/I)

Re-probe each of UA-042…UA-056 against current DOM. Mark each as `✓ Fixed` / `✓ Partial` / `✗ Open` / `↻ Regressed`. Detail:

- UA-042: Home `/schedule` icon link aria-label
- UA-043: Navbar Open/Close menu i18n leak
- UA-044: `Input.vue` `v-bind=$attrs`
- UA-045: Drawer open polish (focus trap, ESC)
- UA-046: Browse Genre placeholder contrast
- UA-047: GenreFilterPopup trigger aria-haspopup
- UA-048: Browse heading-order (sr-only h2)
- UA-050: Anime error state localization ("Failed to fetch anime")
- UA-051: Anime detail dynamic `<title>`
- UA-052: Anime detail `text-white/40` sweep
- UA-053: Navbar drawer role=dialog
- UA-054: Drawer ESC close
- UA-055: Drawer Schedule entry
- UA-056: Drawer z-index over Home

### Tier 3 — Regression sweep

Re-run axe-core on the 4 views that scored zero on 2026-04-20 (Profile, Themes, Schedule, Game) and the 3 prior offenders (Home, Browse, Anime). Capture deltas vs the 2026-04-17 desktop and 2026-04-20 mobile baselines.

### Tier 4 — Admin tools

Each on `admin.animeenigma.ru` (IP-allowlisted) or `/admin/*` SPA paths. Driven from the server's localhost browser session.

| Surface | Audit depth |
|---|---|
| `/admin/recs/:user_id` (`AdminRecs.vue`) | Full a11y + heuristics — never audited |
| `/admin/recs` picker (`AdminRecsPicker.vue`) | Form a11y, error states |
| Grafana (`admin.animeenigma.ru/grafana`) | **UX-focused review** — dashboard inventory, panel choices, naming, layout, color/contrast, mobile usability, time-range defaults, drill-down ergonomics. Recommend Grafana-customizable improvements (dashboard JSON, theme settings, panel configs) — not patches to Grafana itself |
| Prometheus (`/prometheus`) | Lightweight — link sanity, label-name consistency, alert-rule readability |
| Loki (`/loki/ready` + query UI if accessible) | Lightweight — log-stream labels |
| pgAdmin (`/pgadmin`) | Lightweight — note connection presets, saved queries, security posture |
| Admin SPA dashboard | Static HTML at admin root — note structural improvements (icons, search, status indicators) |

**Note:** browser MCP runs in the user's Chrome, not the audit server. Admin IP allowlist (`docker/nginx/admin.conf:7-11`) will block external probes. The spec assumes the audit is driven from a Chrome session inside the allowlisted network (SSH tunnel, VPN, or on the server itself). If that's not feasible, Tier 4 will be deferred and flagged in the report.

### Tier 5 — Competitive benchmark

Side-by-side comparison against five reference sites:

- **Crunchyroll** (`crunchyroll.com`) — global EN leader, large engineering budget
- **animejoy.ru** — RU classic, dense info-rich layout
- **animevost.org** — RU classic, voice-team focused
- **yummyanime.tv** — newer RU, cleaner aesthetic
- **hianime.to** — substituting for 9anime (mirror chain dead per Phase 18 research). Current EN-pirate UX benchmark

Scoring dimensions (1–5 each, with concrete evidence):

| Dimension | What to compare |
|---|---|
| Home / landing | Hero presence, trending strip, continue-watching, density, above-fold value |
| Search & filters | Autocomplete latency/quality, filter taxonomy, results layout, sort options |
| Anime detail | Info density, screenshot/trailer presence, episode list ergonomics, related anime |
| Player UX | Control affordances, quality/sub pickers, episode nav, next-episode CTA, chapter skip, picture-in-picture |
| Watchlist & account | Status taxonomy, custom lists, import/export, sync indicators |
| Recommendations / discovery | Surface count, personalization signal, badging |
| Visual design cohesion | Theme consistency, brand identity, typography hierarchy |
| Mobile UX | Touch targets, drawer pattern, gesture support, viewport behaviors |
| Loading & perceived speed | Skeletons, optimistic UI, scroll-to-content time |
| i18n / locale quality | Translation completeness, RTL hints, locale switcher |

Output: per-dimension table with each site scored + a "what AnimeEnigma should steal" column. Avoid reproducing competitor screenshots; describe observations textually with file path / element selector evidence on our side.

**Constraint:** Competitive analysis is observation-only — no scraping of paywalled content, no account creation, respect each site's terms of service.

## Methodology

Use the framework in `CLAUDE.md > UI/UX Audit Framework` exactly. Key constants:

- **Tooling:** Chrome MCP (`mcp__claude-in-chrome__*`), axe-core 4.10.2 from cdnjs CDN, injected via `<script>` tag
- **Auth:** log in as `ui_audit_bot` via in-page `fetch('/api/auth/login', ...)` so the refresh cookie sets correctly
- **Viewports:** desktop 1280×800, mobile 500×723 (Chrome's clamp)
- **Token discipline:** `read_page` always with `filter:"interactive"`, `depth:8`, `max_chars:20000`; `read_console_messages` and `read_network_requests` always with `pattern`/`urlPattern`, `limit:30`, `clear:true`
- **Citations:** every finding cites code as `(file_path — found via grep "anchor")`; never line numbers from memory
- **Severity scale:** Nielsen 0–3, weighted score per view = `3·cat + 2·major + 1·minor`

## Realistic scenarios

Re-run prior + add new:

**Carry-over (regression check):**
- N1 search → detail → watch
- N2 genre filter → paginate → detail
- N3 mobile hamburger nav switching
- L1 add to watchlist
- L2 status change
- L3 sort by score
- W1 anon player load
- W2 resume from history

**New for 2026-05-12:**
- E1 anon → Home → click rec card → land on anime detail (rec_click instrumentation visible in console)
- E2 logged-in → Profile → watchlist search "narut" → switch a status without losing position
- E3 logged-in → Anime detail → switch RU (Kodik) → EN tab → (do NOT drive player; just confirm tab switches and EnglishPlayer mounts without error)
- E4 mobile → Trending row → horizontal scroll → tap rec
- E5 Auth → QR scan path (use mobile-emulated tab, observe QR refresh + deeplink behavior, capture failure modes)
- A1 admin (in allowlisted browser) → Grafana → poke 3 most-used dashboards → note UX friction
- A2 admin → `/admin/recs` picker → enter username → land on AdminRecs view

## Numbering

Continue from UA-056. Reserve:
- UA-057…UA-079 (23 slots) for Tier 1 (new surfaces)
- UA-080…UA-099 (20 slots) for Tier 4 (admin)
- Tier 5 (competitive) uses CMP-NN numbering, not UA-

Re-use existing UA-042…UA-056 IDs in the verification table — do not renumber.

## Deliverables

```
docs/issues/ui-audit-2026-05-12.md                  # Master report
docs/issues/ui-audit-2026-05-12/
  home.md                                           # Per-view detail (incl. recs rows)
  browse.md
  anime.md                                          # Incl. English tab + resume banner
  profile.md                                        # Incl. URL import + watchlist search
  auth.md                                           # Incl. QR + deeplink + fallback
  navbar.md
  themes.md
  schedule-game.md
  admin-recs.md
  admin-grafana.md
  admin-other.md                                    # Prometheus, Loki, pgAdmin, dashboard
  scenarios.md
  competitive-benchmark.md                          # Tier 5 — full comparison
  axe-raw/                                          # Per-view axe JSON dumps
docs/issues/issues.json                             # Updated with UA-057+, CMP-NN
```

Master report sections (mirrors prior audits):
1. Header (site, locale, account, methodology, scope, tooling)
2. Summary — counts, weighted scores per view, top quick wins, top high-impact fixes, competitive position highlights
3. **Tiered improvement plan** — see "Tiering" below
4. Findings by severity (catastrophic → major → minor → cosmetic)
5. Findings by view (regrouped — UA-057+)
6. Carry-over verification table (UA-042…UA-056)
7. Realistic-scenario findings
8. Admin findings (UA-080+)
9. Competitive benchmark (CMP-NN)
10. axe-core summary table
11. Cross-view consistency
12. Audit notes (token budget, repro hygiene, deferrals)

## Tiering

After all findings are collected, group them into shipping batches. Each batch must be small enough to land in one PR and bounded by file radius.

| Batch label | Definition |
|---|---|
| **Tier A — Catastrophic / blocking** | Severity 3 OR breaks a primary scenario for a real user segment |
| **Tier B — Quick wins** | Severity 2 with ≤ 20 LOC, single-file or single-component fix |
| **Tier C — Major non-trivial** | Severity 2 needing > 20 LOC, multi-file, or design decisions |
| **Tier D — Polish** | Severity 1 — cosmetic, copy, contrast tweaks |
| **Tier E — Strategic / competitive** | Drawn from Tier 5 benchmark — bigger initiatives (e.g. "add picture-in-picture", "redo Home hero", "rebuild Grafana dashboards") with their own scoping |
| **Tier F — Won't fix** | Findings considered and intentionally not actioned, with one-line rationale |

Each batch entry: title, included finding IDs, estimated file radius, estimated LOC, prerequisite (if any), risk notes.

## Checkpoints (3 mandatory user gates)

1. **After first view's dry-run** (Home desktop) — validate per-finding format and per-view doc structure match expectations.
2. **After all per-view audits + admin audits done, before competitive benchmark** — confirm scope is on track and budget is sustainable.
3. **Before commit/push** — final review for redaction, accuracy of citations, completeness of the tiered plan.

## Constraints / "don't do"

- **No fixes applied in this pass.** The deliverable is a report; fixes follow as separate batches.
- **No new accounts** in production — reuse `ui_audit_bot`.
- **No bot-detection bypass** on competitor sites — abort and note in the report if blocked.
- **No competitor screenshots reproduced in the report** — describe observations textually with timestamps.
- **No line numbers from memory** — every code citation comes from a grep run in the same audit pass.
- **No `animeenigma-after-update` invocation** — this is documentation only, nothing deployed.
- **Transient findings filtered** — every finding must repro on ≥ 2 probes.
- **Admin IP allowlist respected** — if browser MCP can't reach admin tools from the user's Chrome session, Tier 4 is deferred with a banner in the report.

## Estimated budget

- Tier 1 (8 surfaces × 2 viewports): ~80–100k tokens
- Tier 2 (16 carry-over checks): ~15k
- Tier 3 (regression sweep, 7 views × 2 viewports, axe only): ~25k
- Tier 4 (admin, 7 surfaces): ~40–60k
- Tier 5 (competitive, 5 sites × 10 dimensions): ~60–80k
- Synthesis + tiering + report writing: ~30k

**Total: ~250–310k tokens.** Larger than any prior audit. **Default plan: split across two sessions** — Session A covers Tiers 1–3 (own-product audit) + checkpoint commit; Session B covers Tiers 4–5 (admin + competitive) + final synthesis. Single-session execution is possible if the model has a 1M context window and budget is acceptable, but the split is the safe default.

## Decisions baked in (resolved without further questions)

1. **9anime substitute** → hianime.to (Phase 18 research already established 9anime's mirror chain is dead).
2. **Two-session split** → adopted as default; the planner can collapse if budget allows.
3. **Admin browser access** → assume the user can route Chrome through an allowlisted IP (SSH tunnel, VPN, or driving from the server's localhost session). If the planner discovers this isn't feasible at runtime, Tier 4 deferral is the documented fallback, with a single-line banner in the report explaining what was skipped and why.
4. **Tier 2 regression bookkeeping** → if a previously-fixed finding (UA-042…UA-056) has regressed (`↻`), it keeps its original UA-NNN in the carry-over table but ALSO gets a fresh entry in the new-findings list (UA-057+) so it shows up in batch tiering. Cross-reference the two with "(regression of UA-NNN)".
