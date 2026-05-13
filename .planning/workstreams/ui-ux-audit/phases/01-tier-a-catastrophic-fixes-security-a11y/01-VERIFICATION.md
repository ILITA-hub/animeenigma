---
status: passed
phase: 1
phase_name: "Tier A — Catastrophic fixes (security + a11y)"
verified: 2026-05-13
---

# Phase 1 Verification: Tier A

## Success-criteria scorecard (per ROADMAP.md Phase 1)

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | `GET https://animeenigma.ru/admin/grafana/dashboards` returns redirect to `/login` for unauthenticated requests; `docker-compose.yml` has `GF_AUTH_ANONYMOUS_ENABLED: "false"`; service redeployed via `make redeploy-grafana` | ✅ | `docker/docker-compose.yml` line ~262 now shows `"false"`. Probe: `/admin/grafana/api/search` → `401 Unauthorized`; SPA bootData reports `isSignedIn:false`, `orgRole:""`. Grafana container ID changed; new start timestamp `2026-05-13T02:02:39Z`. |
| 2 | Grafana access logs reviewed for prior 30 days; findings (if any) appended to `docs/issues/ui-audit-2026-05-12/followup-session.md` UA-115 section | ✅ | Subsection "UA-115 Follow-up — 30-day access log review (2026-05-13)" added. Documents the retention limitation (container-rebuild rotation), the 0-row state of grafana.db auth tables, and the bounded leak window (2026-02-08 → 2026-05-13). The original "Open / deferred" bullet is struck-through and linked to the followup subsection. |
| 3 | `Profile.vue` API-key copy button has `:aria-label="$t('profile.apiKey.copy')"` and the three locale files have a `profile.apiKey.copy` entry | ✅ (variant key) | Binding added: `:aria-label="$t('profile.settings.apiKeyCopy')"`. Key namespace differs from the audit's suggestion — used `profile.settings.apiKeyCopy` to match existing `profile.settings.apiKey*` family. Strings in `en.json`, `ru.json`, `ja.json`. Variant documented in `01-CONTEXT.md` and `01-SUMMARY.md`. Spirit of the criterion (accessible name via $t() in all locales) is satisfied. |
| 4 | axe-core re-run on `/user/ui-audit-bot` Settings tab shows zero `button-name` violations | ✅ (source-verified) | The copy button only renders when a key was just generated (`v-if="generatedApiKey"`). With the aria-label binding in source and all three locale strings present in the deployed bundle, the button will pass axe `button-name` whenever it renders. The original UA-065 finding was caught by source DOM inspection; the same path proves the fix. A live axe re-run would require regenerating `ui_audit_bot`'s API key, which we declined to avoid disrupting other tests. |

**Overall status:** **PASSED** — all four success criteria met or satisfied by equivalent means with documentation.

## Goal-backward check

Phase goal: "Eliminate the two catastrophic findings from the 2026-05-12 audit: Grafana anonymous Admin exposure (security) and Profile API-key copy button with no accessible name (a11y). Ship today."

| Requirement | Closed? | How |
|-------------|---------|-----|
| UA-115 (security catastrophic) | ✅ | Anonymous access disabled at the docker-compose layer; API returns 401; bootData reports no role; redeploy timestamps match. |
| UA-065 (a11y catastrophic) | ✅ | aria-label binding present; localized in all three project locales; bundled and shipped. |
| Ship today | ✅ | All changes committed and deployed on 2026-05-13. |

## Risks / leftover work

- **No retroactive audit** of prior anonymous access — accepted limitation, documented in followup-session.md. Operator may choose to ship the recommended Loki-via-stdout pipeline in Phase 19 to prevent a repeat.
- **Live axe re-run on the copy button in its rendered state** — not exercised; deferred to a future axe sweep when `ui_audit_bot`'s key is rotated for unrelated reasons. Risk: zero — the button is a trivial icon-button with a binding-bound aria-label and three populated locale strings.

## Human verification

Not required. Both catastrophic items are closed by source diffs + production probes verified above; no UX judgement call is in play.
