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

### 2. Simplify (quality cleanup)

Before linting and deploying, run the **`simplify`** skill over the changes this update produced to fold in reuse / simplification / efficiency / altitude cleanups. Invoke **`/simplify`** — it reviews the changed code and applies behavior-preserving quality fixes to the working tree. It does NOT hunt for bugs (that's `/code-review`); it only cleans up.

- **Default depth = ONE cleanup agent.** Invoke `/simplify` with a single review agent that folds all four angles (reuse / simplification / efficiency / altitude) into one pass. The built-in skill fans out to 4 agents by default — for a normal update that is ~4× the tokens for the same one-or-two findings, so override it down to 1. (This session's data: 4 agents = ~48% of the whole run for a 30-line diff.)
- **Big update → offer the fuller pass first.** If the diff is large — roughly **>150 changed lines**, **>5 non-test files**, OR it spans **multiple services/subsystems** — propose the full 4-agent (one-per-angle) simplify and run it only if the owner opts in. Otherwise stay at 1 agent.
- Run it **once**, on the diff this update produced, and let it apply its fixes. If the work was already committed (e.g. done in a worktree), simplify's fixes land as new uncommitted changes that step 6 (commit) then captures as a follow-up cleanup.
- Run it **before** step 3 (lint) and step 4 (deploy) so the cleaned-up version is what gets linted, deployed, and committed — never deploy first and simplify after.
- **Skip it only** when this update touched no code (docs / `CLAUDE.md` / changelog-only changes) — there is nothing to simplify.
- Don't separately re-verify simplify's output here — the lint/build step (3) and health check (4) catch any regression.

### 3. Verify (lint + build + tests) — terse, script-driven

**Frontend** (`frontend/web/**` changed) — ONE script runs DS-lint + eslint(touched) + `bun run build` (type truth) + vitest(touched) and prints ~5 status lines (dumps only the failing gate's log):
```bash
bin/ae-fe-verify.sh <touched-file ...>   # no args → derives touched files from `git diff HEAD`
```
This REPLACES the old separate `bun lint` + `bun run build` — do NOT also run them. If `/frontend-verify` was **just** run green earlier in this session, skip this (identical gates — don't double-build). Run `frontend/web/scripts/i18n-lint.sh` + the locale vitest specs ONLY when locale JSON changed.

**Go** (`services/**` or `libs/**` changed):
```bash
cd /data/animeenigma && golangci-lint run ./libs/... ./services/... 2>&1 || true   # skip if not installed
```

Fix any failure before proceeding. Docs / `CLAUDE.md` / script-only changes → nothing to verify here.

### 4. Update the changelog for LastUpdates.vue — user-facing changes only

**Skip entirely for internal-only changes** (tooling / bin scripts / docs / CLAUDE.md / pure refactors with no user-visible effect) — users don't care, and a no-user-impact change needs no web redeploy either.

Generate user-facing changelog entries and prepend them to **`frontend/web/changelog.full.json`** (the full-history source of truth — NOT the served file).

The served file `frontend/web/public/changelog.json` is **generated** from the full history (latest 30 entries only) and is what `LastUpdates.vue`'s Changelog tab + the backend spotlight "Latest News" card actually fetch. We trim it because it's downloaded whole on every page load — the full history is hundreds of KB while consumers render only the newest handful. Always edit `changelog.full.json`, then regenerate the served file (see Rules below). Never hand-edit `public/changelog.json`.

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
- **Signature closers**, pick one per entry: `Поверьте мне.` · `Никто другой так не делает!` · `МЫ сделали. Никто другой не сделал!` · `Лучший X. Лучший!` · `ВЕЛИКОЛЕПНО.` · `Грандиозно.` · `Нигде в мире такого нет.`
- **Self-aggrandizing claim**: `МЫ это нашли. Никто другой не заметил.` / `Многие просили — МЫ сделали.` / `Только МЫ можем.`
- **Bombastic comparatives**: `лучшая защита`, `лучшая перемотка`, `грандиозный фильтр`.
- **Drama-then-fix arc** for `fix` entries: name the disaster ("КАТАСТРОФА была"), then the heroic resolution ("Теперь — ВЕЛИКОЛЕПНО").
- **Slang intensifiers** `ЖОСКО`, `КАЙФ`, `ВАЙБ` — drop them as ALL-CAPS verdicts:
  - `ЖОСКО` = raw power/brutal-impressiveness of what we built. Use it as an **adverb BEFORE the verb**: `ЖОСКО сделали.`, `ЖОСКО починили.`, `ЖОСКО переписали.` — NOT `Сделали ЖОСКО`. Also fine standalone: `ЖОСКО.`
  - `КАЙФ` = the user-felt pleasure of the result: `Смотреть — один КАЙФ.`, `Теперь КАЙФ, а не плеер.`, standalone closer `КАЙФ.`
  - `ВАЙБ` = the atmosphere/feel of a feature or the whole platform: `Какой ВАЙБ!`, `ВАЙБ — имперский.`, `Новый дизайн — чистый ВАЙБ.`, `Поймали ВАЙБ.`
  - `BEST` = English caps drop-in for `Лучший`, used randomly/occasionally instead of it: `BEST плеер. BEST!`, `BEST перемотка в индустрии.`, `Лучший X. BEST!` Mix it in — don't replace every `Лучший`.
- **Emojis stay** at the start (🎉 🔧 ⚡ 🎌 🐛 📐 🧭 🔔 etc.) — they're the visual hook.
- **Length cap ≈ 180 chars** per entry; longer is fine when the technical content earns it (see the 2026-05-19 AnimePahe batch in the same file as the gold-standard reference).

Examples (good):
- "🎉 AnimePahe ОЖИЛ! Английская озвучка снова идёт через animepahe — Frieren тянется все 28 эпизодов без единого 403. ВЕЛИКОЛЕПНАЯ архитектура. Никто другой так не делает!"
- "🔧 Субтитры в AnimeLib плеере. БОЛЬШАЯ починка. Плавные субтитры снова работают. МЫ это починили. Поверьте мне."
- "⚡ Страницы грузятся быстрее благодаря оптимизированному кэшированию. Лучшая скорость. Лучшая!"
- "🐛 Перемотка в EN-плеере зависала на 0:00 — КАТАСТРОФА была. МЫ нашли баг в hls.js, никто другой не заметил. ЖОСКО починили. Теперь смотреть — один КАЙФ."
- "🎨 Новый дизайн главной — неоновый Токио, чистый ВАЙБ. BEST дизайн в индустрии. Никто другой так не делает!"

Anti-examples (do NOT ship these — old "informative + enthusiastic" tone):
- ❌ "🎉 Новая лента аниме-новостей из Telegram прямо на главной! Будьте в курсе всех анонсов"
- ❌ "🔧 Исправлен рендеринг субтитров в AnimeLib плеере — плавные субтитры снова работают!"

If a change is too small to bear Trump-mode (one-line config tweak), prefer a short Trump-mode entry over no entry — never silently fall back to a friendly tone.

**Add entries with the script** (handles today's-group merge + served-file regen; no hand-editing JSON):
```bash
bin/ae-changelog-add.sh fix "🔧 …" feature "🎉 …"      # one or more <type> "<message>" pairs
```
It appends to today's group in `changelog.full.json` (creates it if absent), then regenerates `public/changelog.json` (latest 30). Commit BOTH files (step 5 does). Never hand-edit `public/changelog.json`; never trim `changelog.full.json`.

### 5. Commit & push (land on `main`)

Deploy builds from `main`, so land there FIRST. One script stages, commits (co-authors auto-appended), rebases onto `origin/main`, and pushes `HEAD:main`:
```bash
printf '%s\n' 'fix(scope): conventional subject' '' 'Optional body.' | bin/ae-land.sh <file ...>
```
Pass explicit paths (the worktree index may hold unrelated hunks). On a **rebase conflict** it STOPS and prints the conflicted files — resolve them (for `changelog.*`, prefer re-running `bin/ae-changelog-add.sh` so entries land atop the incoming group), then `git add <files> && GIT_EDITOR=true git rebase --continue && git push origin HEAD:main`. It never force-pushes.

### 6. Deploy (only if a service changed)

Map changes → services (step 1). If nothing under `services/**`, `libs/**`, or `frontend/web/**` that affects the running app changed (docs / scripts / CLAUDE.md / changelog-less internal work), **skip deploy.** Otherwise, one script ff-syncs the shared tree to `origin/main`, redeploys each service, and checks the container is up (no full 14-service `make health`):
```bash
bin/ae-deploy.sh web            # e.g. frontend;  or:  bin/ae-deploy.sh catalog gateway
```
`libs/**` → redeploy ALL Go services (`auth catalog gateway player rooms scheduler streaming themes`). It refuses to force-sync a dirty/diverged shared tree — resolve that manually if it reports it. For a broad multi-service check after Go changes you may still run `make health`.

### 7. Report

Summarize:
- What was implemented (human-readable)
- Which services were redeployed (or "no deploy — internal-only")
- Verify + container-up results
- Changelog entries added (or "skipped — internal-only")
- Commit hash and message
- Push status
