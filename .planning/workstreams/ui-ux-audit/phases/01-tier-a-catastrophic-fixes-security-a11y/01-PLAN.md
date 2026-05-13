# Phase 1 Plan: Tier A — Catastrophic fixes (security + a11y)

**Status:** Active
**Plan #:** 1 (single atomic plan)
**Created:** 2026-05-13

## Goal

Close UA-115 (Grafana anonymous Admin exposure) and UA-065 (Profile API-key copy button accessible name). Ship today.

## Tasks

### 1. Disable Grafana anonymous access (UA-115)

- [ ] Edit `docker/docker-compose.yml` — flip `GF_AUTH_ANONYMOUS_ENABLED: "true"` → `"false"`. Leave `GF_AUTH_ANONYMOUS_ORG_ROLE` / `GF_AUTH_ANONYMOUS_ORG_NAME` in place (they become no-ops when ENABLED is false; minimal diff for reviewability).
- [ ] Run `make redeploy-grafana` to apply.
- [ ] Probe `https://animeenigma.ru/admin/grafana/dashboards` unauthenticated → must redirect to `/login` (302/303) or 401.

### 2. Grafana access log review (UA-115 follow-up)

- [ ] `docker logs --since 720h animeenigma-grafana` (30 days, if retained).
- [ ] Cross-check nginx access logs for `/admin/grafana/*` requests in the same window.
- [ ] Categorize: localhost/internal vs external; reads vs writes; identifiable PII exposure (user IDs in dashboard URLs/queries).
- [ ] Append findings to `docs/issues/ui-audit-2026-05-12/followup-session.md` under a new `### UA-115 Follow-up — 30-day access log review (2026-05-13)` subsection beneath the existing UA-115 finding. If logs were rotated and the full 30 days aren't available, document the retention limitation.

### 3. Profile API-key copy button accessible name (UA-065)

- [ ] Edit `frontend/web/src/views/Profile.vue` — add `:aria-label="$t('profile.settings.apiKeyCopy')"` to the copy `<button>` at line 740. Keep the icon/click-handler untouched. (Using `profile.settings.apiKeyCopy` rather than the audit's suggested `profile.apiKey.copy` to stay consistent with the surrounding `profile.settings.apiKey*` key family.)
- [ ] Add the `apiKeyCopy` key under `profile.settings` in:
  - `frontend/web/src/locales/en.json` — `"Copy API key"`
  - `frontend/web/src/locales/ru.json` — `"Скопировать API-ключ"`
  - `frontend/web/src/locales/ja.json` — `"APIキーをコピー"`

### 4. Verification

- [ ] `make redeploy-web` after frontend i18n + Vue change.
- [ ] Hit `https://animeenigma.ru/admin/grafana/dashboards` anonymously → expect redirect to `/login` and no dashboard content.
- [ ] Hit `https://animeenigma.ru/user/ui-audit-bot` Settings tab → confirm copy button DOM has the `aria-label` attribute set to the localized string.
- [ ] Run axe-core scan on the same Settings tab → zero `button-name` violations.

### 5. Artifacts

- [ ] Write `01-SUMMARY.md` recording diff list + verification results.
- [ ] Write `01-VERIFICATION.md` with success-criteria scorecard.
- [ ] Commit with co-authors per CLAUDE.md.

## Risk / Rollback

- **Grafana redeploy may briefly take dashboards offline** (~10s). Acceptable; admins are notified out-of-band if hit during.
- If access logs reveal a real exposure (non-localhost writes / config changes / datasource edits), open a P0 ticket and escalate immediately.
- Rollback path: revert env var to `"true"` and `make redeploy-grafana`. The aria-label change is a pure additive frontend tweak with no rollback risk.

## Files touched (estimated)

```
docker/docker-compose.yml                        # 1 line
frontend/web/src/views/Profile.vue                # 1 line (copy button)
frontend/web/src/locales/en.json                  # 1 line
frontend/web/src/locales/ru.json                  # 1 line
frontend/web/src/locales/ja.json                  # 1 line
docs/issues/ui-audit-2026-05-12/followup-session.md  # appended subsection
.planning/workstreams/ui-ux-audit/phases/01-*/01-{CONTEXT,PLAN,SUMMARY,VERIFICATION}.md
```
