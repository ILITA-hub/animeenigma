---
allowed-tools: Bash(*), Read(*), Glob(*), Grep(*), Edit(*), Write(*), AskUserQuestion(*)
description: After any implementation — redeploy services, update changelog, commit & push
---

# AnimeEnigma After-Update

Run this after any implementation work to deploy, update the user-facing changelog, commit, and push.

**Run end-to-end without stopping for confirmation.** This skill is non-interactive: it lints, deploys, changelogs, commits, and pushes in one uninterrupted pass. Do NOT ask the user "want me to run this?" or "confirm the redeploy list?" — the user invoking after-update (or the assistant auto-invoking it after finishing an update) IS the confirmation. The ONLY reasons to pause are a hard failure: a lint error you can't auto-fix, a failed redeploy, or a failed health check. Surface those; otherwise drive all the way to a pushed commit.

**Auto-invoke:** After completing any implementation work (feature, fix, refactor, UI tweak), the assistant should run this skill automatically without being asked. The user should not have to call it every time. (Batching exception still applies: for a run of several small planned changes, do them all, then ONE after-update at the end — no changelog spam.)

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

List the services to redeploy and why (one line, for the record), then redeploy them immediately — do NOT pause for confirmation.

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

**Writing style — Russian Trump-mode (mandatory default):** Bombastic, self-aggrandizing, emoji-led, but factually grounded. Write for anime fans, not developers. The technical claim must stay true to what actually changed — only the *tone* is theatrical.

Trump-mode signature elements (use 2-4 per entry, not all at once — keep it punchy, not parody-soup):

- **ALL-CAPS emphasis** on one or two key adjectives/nouns per entry: `ВЕЛИКОЛЕПНАЯ архитектура`, `ОГРОМНАЯ перемотка`, `КАТАСТРОФА была`, `НИКОГДА больше`, `БОЛЬШАЯ починка`, `ВСЁ работает`.
- **Signature closers**, pick one per entry: `Поверьте мне.` · `Никто другой так не делает!` · `МЫ сделали. Никто другой не сделал!` · `Лучший X. Лучший!` · `ВЕЛИКОЛЕПНО.` · `Грандиозно.`
- **Self-aggrandizing claim**: `МЫ это нашли. Никто другой не заметил.` / `Многие просили — МЫ сделали.` / `Только МЫ можем.`
- **Bombastic comparatives**: `лучшая защита`, `лучшая перемотка`, `грандиозный фильтр`.
- **Drama-then-fix arc** for `fix` entries: name the disaster ("КАТАСТРОФА была"), then the heroic resolution ("Теперь — ВЕЛИКОЛЕПНО").
- **Slang intensifiers** `ЖОСКО` and `КАЙФ` — drop them as ALL-CAPS verdicts: `ЖОСКО` for raw power/brutal-impressiveness of what we built (`Сделали ЖОСКО.`, `ЖОСКО починили.`), `КАЙФ` for the user-felt pleasure of the result (`Смотреть — один КАЙФ.`, `Теперь КАЙФ, а не плеер.`). They also work as standalone one-word closers: `ЖОСКО.` / `КАЙФ.`
- **Emojis stay** at the start (🎉 🔧 ⚡ 🎌 🐛 📐 🧭 🔔 etc.) — they're the visual hook.
- **Length cap ≈ 180 chars** per entry; longer is fine when the technical content earns it (see the 2026-05-19 AnimePahe batch in the same file as the gold-standard reference).

Examples (good):
- "🎉 AnimePahe ОЖИЛ! Английская озвучка снова идёт через animepahe — Frieren тянется все 28 эпизодов без единого 403. ВЕЛИКОЛЕПНАЯ архитектура. Никто другой так не делает!"
- "🔧 Субтитры в AnimeLib плеере. БОЛЬШАЯ починка. Плавные субтитры снова работают. МЫ это починили. Поверьте мне."
- "⚡ Страницы грузятся быстрее благодаря оптимизированному кэшированию. Лучшая скорость. Лучшая!"
- "🐛 Перемотка в EN-плеере зависала на 0:00 — КАТАСТРОФА была. МЫ нашли баг в hls.js, никто другой не заметил. Починили ЖОСКО. Теперь смотреть — один КАЙФ."

Anti-examples (do NOT ship these — old "informative + enthusiastic" tone):
- ❌ "🎉 Новая лента аниме-новостей из Telegram прямо на главной! Будьте в курсе всех анонсов"
- ❌ "🔧 Исправлен рендеринг субтитров в AnimeLib плеере — плавные субтитры снова работают!"

If a change is too small to bear Trump-mode (one-line config tweak), prefer a short Trump-mode entry over no entry — never silently fall back to a friendly tone.

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
