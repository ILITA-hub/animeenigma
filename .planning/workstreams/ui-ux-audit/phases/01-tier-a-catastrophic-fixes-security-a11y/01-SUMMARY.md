# Phase 1 Summary: Tier A — Catastrophic fixes (security + a11y)

**Completed:** 2026-05-13
**Plan:** 01-PLAN.md
**Outcome:** Both catastrophic findings closed. Single atomic plan, no waves.

## Changes shipped

### UA-115 — Grafana anonymous Admin closed (security)

- `docker/docker-compose.yml` — `GF_AUTH_ANONYMOUS_ENABLED` flipped from `"true"` to `"false"` with an inline comment recording the change date and rationale. Kept the now-no-op `GF_AUTH_ANONYMOUS_ORG_ROLE` / `GF_AUTH_ANONYMOUS_ORG_NAME` lines for grep-ability.
- `make redeploy-grafana` executed. Post-deploy probes:
  - `GET https://animeenigma.ru/admin/grafana/api/search` (unauthenticated) → `401 Unauthorized`
  - `GET https://animeenigma.ru/admin/grafana/dashboards` (unauthenticated) → SPA shell now reports `isSignedIn:false`, `orgRole:""` (was `Admin`)
- 30-day access log review appended to `docs/issues/ui-audit-2026-05-12/followup-session.md` under the new "UA-115 Follow-up" subsection. **Key finding:** container stdout is the only access-log source and rotates on every redeploy, so a strict 30-day historical audit is unrecoverable. Grafana SQLite `login_attempt`, `session`, `user_auth_token` all have 0 rows, confirming there was never any authenticated user — every access during the leak window was via the anonymous Admin role. The leak window is bounded at `2026-02-08 → 2026-05-13`. Forward-looking mitigations (Loki shipping, nginx auth-guard, subdomain migration) are documented but out of Phase 1 scope.

### UA-065 — Profile API-key copy button accessible name (a11y)

- `frontend/web/src/views/Profile.vue` — added `:aria-label="$t('profile.settings.apiKeyCopy')"` to the copy `<button>` on the generated-key panel (around line 740). Icon and click-handler untouched.
- `frontend/web/src/locales/en.json` — added `"apiKeyCopy": "Copy API key"`.
- `frontend/web/src/locales/ru.json` — added `"apiKeyCopy": "Скопировать API-ключ"`.
- `frontend/web/src/locales/ja.json` — added `"apiKeyCopy": "APIキーをコピー"`.
- Key namespace is `profile.settings.apiKeyCopy` rather than the audit's suggested `profile.apiKey.copy`, to stay consistent with the surrounding `profile.settings.apiKey*` family. Discussed in `01-CONTEXT.md`.
- `make redeploy-web` executed. Post-deploy probe of the deployed JS bundle (`/assets/index-9gFJAsoq.js`) shows all three translations bundled.

## Verification

See `01-VERIFICATION.md` for the success-criteria scorecard.

## Files touched

```
docker/docker-compose.yml                              # +3 / -1 (env var flip + comment)
frontend/web/src/views/Profile.vue                      # +1 (aria-label binding)
frontend/web/src/locales/en.json                        # +1
frontend/web/src/locales/ru.json                        # +1
frontend/web/src/locales/ja.json                        # +1
docs/issues/ui-audit-2026-05-12/followup-session.md     # +30 (UA-115 followup subsection)
.planning/workstreams/ui-ux-audit/phases/01-tier-a-catastrophic-fixes-security-a11y/
  01-CONTEXT.md      (new)
  01-PLAN.md         (new)
  01-SUMMARY.md      (this file)
  01-VERIFICATION.md (new)
```

## Notes for downstream phases

- Phase 6 (Navbar drawer a11y) and Phase 7 (`Input.vue` $attrs) will land aria-* attributes through similar patterns. The `profile.settings.apiKeyCopy` key style is the reference for future i18n keys that exist purely to feed aria-labels.
- The Loki-shipping mitigation for Grafana logs is suggested in Phase 19 (Grafana dashboard rebuild) if the operator wants to make the access-log audit story repeatable.
