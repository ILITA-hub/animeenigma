# UI Audit Test User (DO NOT DELETE)

> Extracted from `CLAUDE.md` (2026-06-03) to keep the root guidelines under the context-size budget. This account is permanent infrastructure for the [UI/UX Audit Framework](ui-audit-framework.md), integration tests, and Playwright e2e tests.

Permanent test account for automated UI/UX audits, integration tests, and Playwright e2e tests:

- **Username**: `ui_audit_bot`
- **Public ID**: `ui-audit-bot`
- **Profile URL**: `https://animeenigma.ru/user/ui-audit-bot`
- **API Key**: stored in `docker/.env` as `UI_AUDIT_API_KEY` (not committed)
- **Seeded with**: 8 anime_list entries (mixed statuses), 3 watch_history rows, 3 theme_ratings
- **Password login is enabled** — `audit_bot_test_password_2026` (set 2026-04-07 so audits can use the standard `/api/auth/login` flow with refresh-cookie semantics). Treat this as an automation account, not a human one.

**Permanent infrastructure** — recreating loses seeded state and breaks any e2e tests depending on stable IDs. Re-seeding is idempotent via `scripts/seed-ui-audit-user.sh`.

To use in tests:
```bash
curl -H "Authorization: Bearer $UI_AUDIT_API_KEY" https://animeenigma.ru/api/...
```

To refresh stale data (e.g. before a new audit):
```bash
./scripts/seed-ui-audit-user.sh
```

To rotate the API key (lost the previous one):
```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma \
  -c "UPDATE users SET api_key_hash = NULL WHERE username = 'ui_audit_bot';"
./scripts/seed-ui-audit-user.sh
# New key printed in the banner — save to docker/.env
```
