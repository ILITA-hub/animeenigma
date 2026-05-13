# Phase 1: Tier A — Catastrophic fixes (security + a11y) - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, narrow concrete scope, no architectural grey areas)

<domain>
## Phase Boundary

Eliminate the two catastrophic findings from the 2026-05-12 UX audit:

1. **UA-115 / UX-01 — security**: Grafana anonymous Admin access exposed on the public internet via `https://animeenigma.ru/admin/grafana/`. Anonymous role currently defaults to `Admin` per `docker/docker-compose.yml`. Must be closed plus a 30-day access-log review.
2. **UA-065 / UX-02 — a11y**: Profile API-key copy button in `frontend/web/src/views/Profile.vue` has no accessible name; screen readers announce nothing. Fix with an `:aria-label="$t('profile.apiKey.copy')"` binding plus a `profile.apiKey.copy` key in all three locale files (`en.json`, `ru.json`, `ja.json`).

Out of scope for this phase: every other audit finding (Tier B/C/D/E batches).

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

All implementation choices are at Claude's discretion. Success criteria in ROADMAP.md Phase 1 are concrete enough that there are no architectural grey areas:

- **Grafana close-down**: set `GF_AUTH_ANONYMOUS_ENABLED: "false"` in `docker/docker-compose.yml` (single env var); redeploy via `make redeploy-grafana`; verify the public URL redirects to `/login` for unauthenticated requests; review the prior 30 days of access logs and append findings (if any) to `docs/issues/ui-audit-2026-05-12/followup-session.md` under a UA-115 section.
- **API-key copy aria-label**: add the `aria-label` binding to the existing copy button in `Profile.vue`; add the `profile.apiKey.copy` translation key to `en.json`, `ru.json`, and `ja.json`. Russian/English/Japanese wording to be short imperative ("Copy API key" / "Скопировать API-ключ" / "APIキーをコピー").
- **Verification**: axe-core re-run on the Settings tab of `/user/ui-audit-bot` must show zero `button-name` violations.

### Locked from ROADMAP

- The two catastrophic items must close in the same phase (single PR, ship-today framing).
- Anonymous access disabled at the docker-compose layer, not at the nginx/proxy layer (env var is the authoritative knob).

</decisions>

<code_context>
## Existing Code Insights

- `docker/docker-compose.yml` — `grafana` service block contains the `GF_AUTH_ANONYMOUS_*` env vars currently enabling Admin-role anonymous access.
- `frontend/web/src/views/Profile.vue` — already exists with a Settings tab and API-key copy button; the button currently has icon-only content with no accessible text.
- `frontend/web/src/locales/{en,ru,ja}.json` — i18n catalog (three locales). Pattern: nested object, dot-separated keys via `$t()`.
- `make redeploy-grafana` — established command for restarting Grafana with new env (see CLAUDE.md service-table redeploy commands).
- `docs/issues/ui-audit-2026-05-12/followup-session.md` — exists; UA-115 section is the destination for the access-log review note.

</code_context>

<specifics>
## Specific Ideas

- Grafana log review: scope to `/data/animeenigma/docker/logs/grafana/` (or whatever the compose-mounted log dir is) plus nginx-side `/admin/grafana/*` request logs from the last 30 days. Focus on writes/state-mutating endpoints (POST/PUT/DELETE), unusual UA strings, and anonymous sessions that touched datasource or alert config. If no logs survive 30 days (rotation), document that limitation in the UA-115 followup note.
- For the aria-label: don't rename or restructure the button — just add the binding. Keep the existing icon and click-handler intact.

</specifics>

<deferred>
## Deferred Ideas

None — phase scope is intentionally narrow (two catastrophic items, ship-today).

</deferred>
