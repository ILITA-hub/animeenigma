# Follow-up session — 2026-05-12 (Chrome reconnected)

After the user restarted Chrome, ~6 additional live probes were completed:
- Hamburger interaction probe (drawer state via direct click)
- Resume-banner probe on a watched anime
- Grafana anonymous probe + dashboard inventory + first-dashboard structure
- Auth/Profile/Anime quick re-probe at near-mobile viewport

Chrome's window-inner-width on this host clamps at **828px** (not the 500px the 2026-04-20 audit used). All mobile-specific layout findings are therefore source-verified, not live-verified at true mobile breakpoints.

## New findings (live-confirmed)

### [UA-110] Resume state-machine renders contradictory banners on a 1-episode anime — Severity 2 (Major) — UX
**View:** `/anime/4cfd00a3-f4d4-4a5e-922e-ee88f6fe8f9b` (Chainsaw Man: Reze — 1 episode in catalog)
**Heuristic:** Nielsen #5 Error prevention + state-machine integrity
**Evidence:**
- Page renders simultaneously: "Продолжить с эп. 12" (Continue from ep 12) **and** "Вы закончили это аниме. Пересмотреть с 1-й серии" (You finished this anime. Re-watch from ep 1).
- Catalog says 1 episode (`1 эп.`); DB has 12 completed episodes for `ui_audit_bot` on this anime (test-data inconsistency).
- The state machine `useResumeStateMachine.ts` exposes `kind` as one of: first-time / watching / finished / not-yet-aired / currently-airing — but the rendered surface shows two states at once.
- Watch_progress data: 12 `completed=true` rows; expected total=1.

**Why it matters:**
1. Even for valid data (12 watched + total 1 should resolve to `finished` only), the UI shows "Resume from ep 12" — which is greater than total. Math is wrong.
2. Two banners with contradictory CTAs is a user-confusion vector.

**Citations:**
- `frontend/web/src/composables/useResumeStateMachine.ts — found via grep "lastWatched" / "kind"`
- `frontend/web/src/views/Anime.vue — found via grep "resume.kind.value"`

**Proposed fix:**
- Cap `lastWatched` at `totalEpisodes` in `deriveLastWatched()` or in the consumer.
- Ensure only one banner renders at a time (use `kind` as a switch, not as additive flags).
- For test-data hygiene: update the seed script to also seed `watch_progress` (currently only seeds `watch_history` — see UA-111).

### [UA-111] `ui_audit_bot` seeded watch_history isn't mirrored to watch_progress — Severity 1 (Minor) — test infra
**Evidence:**
- ui_audit_bot has 3 `watch_history` rows (FMA, JoJo, Frieren) seeded for 2026-04-07.
- But `/api/users/progress/{animeId}` returns `[]` for FMA Brotherhood; watch_progress has 0 rows for FMA.
- Other anime (Chainsaw: Reze) have 12 watch_progress rows but no watch_history.

**Why it matters:** Tests that rely on the resume banner rendering for seeded data are broken because the two data sources don't sync. The seed script `scripts/seed-ui-audit-user.sh` doesn't populate watch_progress.

**Citations:** `scripts/seed-ui-audit-user.sh — found via grep "watch_history"`

**Proposed fix:** Seed script should `INSERT INTO watch_progress` mirroring watch_history rows, OR a backend job should aggregate watch_history → watch_progress. Pick one and document.

### [UA-112] Hamburger button toggles `aria-label` but doesn't set `aria-expanded` after click — Severity 2 (Major) — a11y (LIVE-CONFIRMED UA-053 regression detail)
**View:** Home (any view with navbar)
**Heuristic:** WCAG 4.1.2 Name/Role/Value
**Evidence:**
- Live probe: clicked hamburger; `aria-label` changed from "Open menu" → "Close menu" (✓ state DOES change in DOM); but `aria-expanded` remained `null` (not `"false"` or `"true"`).
- Drawer markup not detectable via `[role="dialog"]` / `[aria-modal]` — confirms UA-053/UA-084 still open.
- Focus did NOT move to drawer after click (`activeElement` stayed `BODY`) — UA-045/UA-054 still open.

**Why it matters:** Screen readers and a11y test tools rely on `aria-expanded` to know drawer state. The label toggle is a partial fix only.

**Citations:** `frontend/web/src/components/layout/Navbar.vue — found via grep "mobileMenuOpen"`

**Proposed fix:** `:aria-expanded="mobileMenuOpen"` on the hamburger button. Combine with UA-053 / UA-054 / UA-080 sweep.

## Grafana / admin findings (LIVE-PROBED)

### [UA-115] **CRITICAL** — Grafana anonymous access is enabled with Admin role on the public internet — Severity 3 (Catastrophic) — **security**
**Surface:** `https://animeenigma.ru/admin/grafana/*`
**Evidence:**
- `docker/docker-compose.yml`:
  - `GF_AUTH_ANONYMOUS_ENABLED: "true"`
  - `GF_AUTH_ANONYMOUS_ORG_ROLE: "Admin"`
  - `GF_AUTH_ANONYMOUS_ORG_NAME: "Main Org."`
- Live probe: navigated to `https://animeenigma.ru/admin/grafana/dashboards` **without authentication** and successfully listed 7 dashboards including operational metrics (Player Health, Watch Activity, Provider Health). The "Sign in" CTA is visible but bypassable.
- The path-routed `animeenigma.ru/admin/*` does NOT enforce the IP allowlist that the legacy `admin.animeenigma.ru` vhost in `docker/nginx/admin.conf` does. (The legacy subdomain blocks external IPs; the path route does not.)

**Why it matters:**
1. **Operational metrics leak**: anyone can browse player health, watch activity, user counts, scraper provider state, and infer business volume.
2. **Anonymous Admin role**: while Grafana login is required for some destructive operations, the `Admin` role grants broad read access to data sources, query API, and folder structure.
3. **PII risk**: depends on what dashboards expose; Watch Activity dashboard panels could include user IDs or anime IDs that, combined with public profile URLs, deanonymize users.
4. This is the **most severe finding of the audit**, exceeding UA-065.

**Citations:**
- `docker/docker-compose.yml — found via grep "GF_AUTH_ANONYMOUS"`
- `docker/nginx/admin.conf — found via grep "IP-based access control"` (the legacy allowlist that the path-route doesn't enforce)
- Live evidence: page text "Welcome to Grafana ... Skip to main content Search or jump to... cmd+k Sign in Home Dashboards" — accessible without credentials.

**Proposed fix:** (ordered by urgency)
1. **Immediate (today):** set `GF_AUTH_ANONYMOUS_ENABLED: "false"` in `docker/docker-compose.yml` and `make redeploy-grafana`. Confirm `/admin/grafana/dashboards` redirects to `/login` for anonymous users.
2. **Short-term:** add an nginx auth-required guard on `location /admin/grafana` in the path-routed config, OR migrate to the IP-allowlisted subdomain (`admin.animeenigma.ru`) and remove the public path.
3. **Audit period:** check Grafana access logs (`docker logs animeenigma-grafana`) for the last 30 days for non-localhost requests to dashboards; assess data exposure.

### [UA-116] Empty "Service Overview" row on AnimeEnigma Monitoring dashboard — Severity 1 (Minor) — Grafana UX
**Surface:** `/admin/grafana/d/animeenigma-services/animeenigma-monitoring`
**Evidence:** Top row reads "Service Overview(0 panels)" — placeholder collapsed-row with no content. Other rows: Video & Parsers (7), User Activity (7), Infrastructure (6), Scheduler (4), 6. Users & Bandwidth (6).
**Why it matters:** Dead UI; users click expecting an overview, get nothing. Either populate or remove.
**Proposed fix:** Either delete the row, or move a 1-row summary (uptime, error rate, request volume) into it as a true "overview at a glance".

### [UA-117] Dashboard row naming inconsistency — Severity 1 (Minor) — Grafana UX
**Evidence:** Six rows: "Service Overview", "Video & Parsers", "User Activity", "Infrastructure", "Scheduler", **"6. Users & Bandwidth"**. The last row has a `6. ` numeric prefix that none of the others have.
**Why it matters:** Suggests the intent was to number all rows (1-6) but only the last one got the prefix; visual inconsistency.
**Proposed fix:** Either prefix all six rows with `1.…6.`, or drop the `6.` prefix entirely.

### [UA-118] Dashboard inventory naming is inconsistent — Severity 1 (Minor) — Grafana UX
**Evidence:** Seven dashboards:
- `AnimeEnigma Monitoring` (PascalCase brand + Title)
- `Content Preferences` (Title Case)
- `Player Health` (Title Case)
- `Preference Resolution` (Title Case)
- **`Rec engine`** (sentence case — odd one out)
- **`Scraper — Provider Health (per stage)`** (service prefix with em-dash + parenthetical)
- `Watch Activity` (Title Case)

**Why it matters:** Inconsistent capitalization and service-prefix style make the list look unfinished. Users scanning the list pause on each variant.
**Proposed fix:** Pick one convention. Suggested: `<Service Domain> — <Aspect>` with Title Case everywhere. E.g., `Recommendations — Engine Health`, `Scraper — Provider Health`, `Players — Health`, `Catalog — Content Preferences`, etc.

### [UA-119] Grafana SPA renders blank for 5-7 seconds with no skeleton or spinner — Severity 1 (Minor) — Grafana UX
**Evidence:** After navigating to `/admin/grafana/dashboards`, the page shows `<body class="theme-dark app-grafana">` with 7 head children but `<body>` text is empty until ~5-7s later when the React tree hydrates. No spinner, no skeleton, no progress indicator.
**Why it matters:** Users see a blank dark page and assume the dashboard failed to load. Compounds frustration on cold loads.
**Proposed fix:** Grafana's own loading state can't be modified directly (it's a third-party app), but the nginx config can serve a static `index.html` with a custom loading placeholder that Grafana's JS replaces on mount. Or: accept this as a Grafana-upstream limitation and add a "Loading dashboards (may take 5-10s)…" hint in the admin landing HTML page.

### [UA-120] Grafana UI is English-only — Severity 0 (Cosmetic) — observation
**Evidence:** Login page shows "Welcome to Grafana / Email or username / Password / Log in". No locale switcher. AnimeEnigma's main app supports RU/EN/JA but the admin Grafana is uniformly English.
**Why it matters:** Cognitive switch for RU admins moving between main site and Grafana. Grafana 9+ does NOT ship native locale switching for the UI chrome — this is upstream limitation.
**Proposed fix:** Live with it. Note in admin onboarding docs that Grafana is English-only.

## Anime view extra finding (LIVE)

### [UA-121] On Chainsaw Man: Reze, 5 color-contrast violations (axe-confirmed) — Severity 2 (Major) — a11y
**Evidence:** Different from FMA (which had 4). Reze page renders the resume banner + finished banner + recommendation row variants and surfaces 5 distinct `color-contrast` violations vs FMA's 4. Same `text-white/40` root pattern but more affected nodes.

This **strengthens UA-052** — the `text-white/40` sweep needs to cover MORE than the 5 instances initially counted; per-view rendering can surface different node counts depending on which features render.

**Proposed fix:** Same global `text-white/40 → /60` sweep recommended in UA-052.

## UA-115 Follow-up — 30-day access log review (2026-05-13)

**Triggered by:** Phase 1 of the `ui-ux-audit` workstream (Tier A catastrophic remediation).

**Closed:** `GF_AUTH_ANONYMOUS_ENABLED` flipped to `"false"` in `docker/docker-compose.yml`; Grafana redeployed. Post-fix probe results:

- `GET https://animeenigma.ru/admin/grafana/api/search` (unauthenticated) → `401 Unauthorized` ✅
- `GET https://animeenigma.ru/admin/grafana/dashboards` (unauthenticated) → returns the SPA shell with `isSignedIn:false` and `orgRole:""` (previously `orgRole:"Admin"`) ✅

**30-day access log review — limitations:**

Container stdout logs are the only access-log source for both Grafana and the path-routing nginx layer (`animeenigma-web`). Both containers were rebuilt during this remediation, and `make redeploy-*` consistently destroys and recreates containers — meaning **stdout logs only retain entries since the most recent container start**. Effective log-retention for both is "since-last-redeploy", which is at the order of hours to days, not 30 days.

What I could check that survives:

| Source | Findings |
|---|---|
| `docker_grafana_data` persistent volume | Contains `grafana.db` SQLite, `dashboards/`, `plugins/`, `alerting/`, `csv/`, `png/`. No log files. |
| `grafana.db` → `login_attempt` table | **0 rows.** No authenticated UI login has ever happened on this instance. |
| `grafana.db` → `session` table | **0 rows.** No authenticated sessions ever created. |
| `grafana.db` → `user_auth_token` table | **0 rows.** No persistent tokens issued. |

**What this tells us:**
1. For the lifetime of this Grafana instance (since `2026-02-08` per volume `_data` directory mtime), every access has been via the anonymous Admin role — there was no other auth flow.
2. The SQLite database itself does NOT record per-request access — Grafana writes those to stdout only, and they're already gone.
3. Therefore, the count and origin of historical anonymous-Admin requests is **unrecoverable** from existing artifacts. We can only confirm the leak existed since 2026-02-08 and is closed as of 2026-05-13.

**No evidence of write-side exploitation** (no datasource edits, no dashboard mutations, no alerting changes) can be derived from in-cluster sources alone. The dashboards directory is read-only-mounted from `docker/grafana/dashboards/` and unchanged (git-tracked); `grafana.db` shows expected schema state for a fresh-default install (no extra orgs, no users).

**Going-forward mitigations recommended (not part of Phase 1 scope):**
1. Add stdout shipping for Grafana → Loki so future audit windows survive redeploys.
2. Add an nginx auth-required guard on `location /admin/grafana` so that even if `GF_AUTH_ANONYMOUS_ENABLED` is ever accidentally flipped back on, an outer layer still blocks.
3. Consider migrating Grafana off the public path route entirely to the IP-allowlisted `admin.animeenigma.ru` vhost.

## Open / deferred items for the next session

- **True 500×723 mobile probe** — this host clamps at 828; needs DPR emulation via Chrome DevTools Protocol (not exposed by the MCP) or a different display.
- **Drive 2-3 more Grafana dashboards** to capture per-dashboard panel-type appropriateness (Stat vs Time-series vs Gauge) and time-range defaults.
- **Try /admin/prometheus + /admin/loki + /admin/pgadmin** for UX consistency check (each is also third-party with limited customization).
- **Temporarily grant admin to ui_audit_bot** to drive `/admin/recs/:user_id` SPA and verify the static code findings (UA-090-101) live.
- ~~Audit Grafana access logs for the last 30 days (UA-115 follow-up).~~ **Done 2026-05-13** — see "UA-115 Follow-up" subsection above. Retention limitation prevents a historical request audit; the leak window is bounded as `2026-02-08 → 2026-05-13`.

## Updated severity counts

- **Catastrophic (3):** UA-065 + **UA-115** (new — Grafana anonymous access)
- **Major (2):** UA-110, UA-112, UA-121 add to the existing list
- **Minor (1):** UA-111, UA-116, UA-117, UA-118, UA-119 add to the existing list
- **Cosmetic (0):** UA-120
