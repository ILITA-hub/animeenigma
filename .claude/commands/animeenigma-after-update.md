---
allowed-tools: Bash(*), Read(*), Glob(*), Grep(*), Edit(*), Write(*), AskUserQuestion(*)
description: After any implementation — redeploy services, update changelog, commit & push
---

# AnimeEnigma After-Update

Run this after any implementation work to deploy, update the user-facing changelog, commit, and push.

## Steps

### 1. Gather context

**Option A — Context from conversation (preferred):**
If you just finished implementing something, you already know what changed. Use that knowledge directly — list the affected files and services.

**Option B — No context (fallback):**
Detect uncommitted changes:

```bash
(git diff --name-only HEAD 2>/dev/null; git diff --name-only --cached 2>/dev/null; git ls-files --others --exclude-standard 2>/dev/null) | sort -u
```

If no changes detected, report "Nothing to deploy" and stop.

**Map changes to services:**

| Path pattern | Action |
|---|---|
| `services/X/**` | Add service `X` to redeploy set |
| `libs/**` | Add ALL Go services: `auth catalog gateway player rooms scheduler streaming themes` |
| `frontend/web/**` | Add `web` to redeploy set |
| `docker/docker-compose.yml` | Warn: "docker-compose.yml changed — may need `make dev-down && make dev`" |
| Anything else (docs, CLAUDE.md, Makefile, scripts, etc.) | Skip — no redeployment needed |

Deduplicate the service list.

### 2. Lint & build checks

**If Go services or libs changed:**
```bash
cd /data/animeenigma && golangci-lint run ./libs/... ./services/... 2>&1 || true
```
If `golangci-lint` is not installed locally, skip Go lint.

**If frontend changed:**
```bash
cd /data/animeenigma/frontend/web && bun lint 2>&1
```
Frontend lint must pass with 0 errors. If there are errors, **fix them** before deploying.

If any lint check fails, fix the issues before proceeding.

### 3. Deploy

Present the list of services to redeploy and why. Ask the user to confirm.

Run `make redeploy-<service>` for each service sequentially from `/data/animeenigma`. For frontend, use `make redeploy-web`.

If any redeploy fails, stop and fix the error.

After all deployments:
```bash
make health
```

### 4. Update changelog.json for LastUpdates.vue

Generate user-facing changelog entries for `frontend/web/public/changelog.json`.

This file is loaded by the `LastUpdates.vue` Changelog tab to show users what's new.

**Format:**
```json
[
  {
    "date": "YYYY-MM-DD",
    "entries": [
      { "type": "feature", "message": "..." },
      { "type": "fix", "message": "..." }
    ]
  }
]
```

**Types:** `feature`, `fix`, `perf`

**Language:** Always write entries in **Russian**.

**Writing style:** Informative + enthusiastic with emojis. Write for anime fans, not developers.

Examples:
- "🎉 Новая лента аниме-новостей из Telegram прямо на главной! Будьте в курсе всех анонсов"
- "🔧 Исправлен рендеринг субтитров в AnimeLib плеере — плавные субтитры снова работают!"
- "⚡ Ускорена загрузка страниц благодаря оптимизированному кэшированию"

**Rules:**
- If today's date group already exists at the top, merge new entries into it
- Otherwise, prepend a new date group
- Keep total entry count under ~50 (trim oldest groups if needed)
- Read the existing file first, update it, write back

### 5. Commit & push

1. Stage all changed files: `git add -A`
2. Generate a conventional commit message summarizing the changes
3. Include co-authors:
   ```
   Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
   Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
   Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
   ```
4. Create the commit
5. Push: `git push`

### 6. Report

Summarize:
- What was implemented (human-readable)
- Which services were redeployed
- Health check results
- Changelog entries added
- Commit hash and message
- Push status
