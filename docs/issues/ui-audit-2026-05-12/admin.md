# Admin tools — Tier 4 findings

**Audit mode:** Code-based (browser disconnected before live admin probes; only the SPA admin pages were code-reviewable. Grafana / Prometheus / Loki / pgAdmin require a live browser session inside the allowlisted network — deferred).

## AdminRecs SPA (`/admin/recs`, `/admin/recs/:user_id`)

`ui_audit_bot` has `role: user`, not `admin` — even with a live browser, the route guard at `router/index.ts` would redirect home. The audit below is from source.

### AdminRecsPicker.vue findings

| ID | Title | Severity |
|---|---|---|
| **UA-090** | "Use my account" button has no aria-label; only `$t(...)` text — when slot is empty (e.g. translation missing), button announces nothing | 2 major (a11y) |
| **UA-091** | No focus management on form errors (no `aria-live` for validation, no focus return to input) | 1 minor (a11y) |
| **UA-092** | No loading indicator during navigation after submit (clicking the picker silently routes; slow networks make it look frozen) | 1 minor (UX) |

### AdminRecs.vue findings

| ID | Title | Severity |
|---|---|---|
| **UA-093** | Signal columns `S1`…`S5` are hardcoded plain text (not translatable; no tooltip explaining what each signal is) | 2 major (i18n + clarity) |
| **UA-094** | Recs table has no `<caption>`, no `aria-label`, no semantic markup for screen readers | 2 major (a11y) |
| **UA-095** | Expandable rows fire only on `@click` — no keyboard support (Enter/Space) and no `aria-expanded`/`tabindex` | 2 major (a11y) |
| **UA-096** | Error states: only 403 maps to a friendly i18n key; 401 / 500 / 429 / 503 / timeout all fall through to raw axios error | 1 minor (UX) |
| **UA-097** | No empty-state help when a user has zero recs (main table just shows blank) | 1 minor (UX) |
| **UA-098** | Mobile horizontal-scroll table has no `aria-label` or visual scroll affordance (shadow on right edge) | 1 minor (a11y + UX) |
| **UA-099** | Recompute button success/failure has no toast — user can't tell if it succeeded | 1 minor (UX) |

### Router guard (`requiresAdmin: true`)

| ID | Title | Severity |
|---|---|---|
| **UA-100** | Non-admin users are silently redirected to `/` with no toast, no `?reason=…` query, no breadcrumb explaining why | 2 major (UX) |
| **UA-101** | 401 (not authenticated) and 403 (authenticated but not admin) are conflated in the guard — same redirect for both, no distinguishing signal | 1 minor (UX) |

## Grafana / Prometheus / Loki / pgAdmin — deferred

Driven from `animeenigma.ru/admin/grafana` (path-routed, not the `admin.animeenigma.ru` subdomain mentioned in CLAUDE.md). Requires a live browser session and Chrome MCP reconnect to complete.

**Pre-flight observations from `docker/nginx/admin.conf` + container `ps`:**
- Grafana container: `animeenigma-grafana` on port 3004 (internal). Public via `animeenigma.ru/admin/grafana`. Redirects `localhost:3004/login` → `https://animeenigma.ru/admin/grafana/login` confirmed.
- Prometheus, Loki containers running. pgAdmin is configured in `admin.conf` but the live URL `animeenigma.ru/admin/pgadmin` returns 404 — pgAdmin may not be exposed in the production routing, only via the legacy `admin.animeenigma.ru` subdomain.

**Recommended next step:** On Chrome reconnect, drive `animeenigma.ru/admin/grafana/d/scraper-health` (the new Phase 17-04 dashboard) and 2-3 other dashboards, then capture UX findings on:
- Dashboard inventory + naming consistency
- Panel choices (graph vs gauge vs stat) appropriateness
- Default time ranges (1h vs 6h vs 24h vs 7d)
- Color choices vs Grafana dark theme
- Mobile responsiveness (likely poor — Grafana's mobile UX is industry-known weak)
- Drill-down ergonomics
- Variable / dropdown clarity

**Provisional recommendations (without live probe):**
- Audit dashboard list — likely names are inconsistent (some title-case, some lowercase, some prefix the team/service).
- Add row links for cross-dashboard navigation (Grafana 9.x feature).
- Switch any Stat panels to Time-series where the time dimension matters.
- Standardize time range default to 6h (current default behavior is whatever was last selected).

These are speculation pending live audit — capture in Tier E (Strategic) of the master report.

## Audit notes

- Live admin browser probes (Grafana especially) were the explicit user ask. Defer to the next session when Chrome reconnects.
- ui_audit_bot would need temporary `role: admin` to drive AdminRecs SPA in a browser. Code review captures the static a11y/UX gaps anyway; live probe would mainly verify focus order and loading-state behavior.
