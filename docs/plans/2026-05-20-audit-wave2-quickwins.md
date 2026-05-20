# Audit Wave 2 — Quick Wins

**Date:** 2026-05-20
**Predecessor:** `docs/plans/2026-05-19-audit-wave1-fixes.md`
**Revision:** 1 — reviewed by code-reviewer subagent 2026-05-20. Tightened: T1 trailing-comma instruction explicit; T2 fresh-DB acceptance added; T3 streaming inline workaround + stale comment cleanup + expanded test cases; T4 exiter mechanism + zap caller-skip handling pinned.
**Goal:** Clear five small audit follow-ups in a single execution session. Each task = one commit, low blast radius, no architectural decisions.
**Workflow:** subagent-driven (implementer → spec-reviewer → code-quality reviewer per task).

## Aggregate metrics

- **UXΔ:** `+1 (Better)` — five small quality/correctness cleanups
- **CDI:** `0.07 * 9` — (small distribution spread × small shift) × Fib 9 across the wave
- **MVQ:** `Sprite 88%/92%` — tight, well-scoped, low-slop cleanup wave

## Plan-time verification (done 2026-05-20)

| Claim | Status |
|---|---|
| `videojsError` keys exist only in locales, no src references | ✅ confirmed via `grep -rn videojsError frontend/web/src/` (only 3 hits, all in locales) |
| Redundant `idx_watch_progress_user_id` still tagged in domain model | ✅ confirmed at `services/player/internal/domain/watch.go:32` |
| Gateway `CORS_ORIGINS` parsed without trim/empty-filter | ✅ confirmed `strings.Split(getEnv("CORS_ORIGINS", ""), ",")` at `services/gateway/internal/config/config.go:105` — produces `[""]` when unset |
| Streaming `PROXY_ALLOWED_DOMAINS` same shape | ✅ confirmed at `services/streaming/internal/config/config.go:64` |
| Rooms env-list parser already correct | ✅ confirmed at `services/rooms/internal/config/config.go:42-50` (proper trim+filter loop from Wave 1 T1) |
| Maintenance has 6 `Fatalw` call sites | ✅ confirmed (lines 37, 44, 54, 57, 65, 90 in `cmd/maintenance/main.go`) |
| Gateway `FATAL:` prefix on non-fatal warning | ✅ confirmed at `services/gateway/internal/config/config.go:125` |

---

## Tasks

### WV2-T1 — Drop dead `videojsError` i18n keys

**Trace:** Audit follow-up "Dead videojsError strings in locales"
**Why:** Wave 1 T6 removed video.js from the bundle. Three translation entries remain orphaned across `en.json`, `ru.json`, `ja.json`.

**Verify-first:**
```bash
grep -rn videojsError frontend/web/src/ # must return only the 3 locale lines
```

**Files:**
- `frontend/web/src/locales/en.json` (line 269)
- `frontend/web/src/locales/ru.json` (line 269)
- `frontend/web/src/locales/ja.json` (line 269)

**Change:** In each file, delete the `videojsError` line AND remove the trailing comma from the preceding `playbackError` line — `videojsError` is the last key in the `errors` block, so the prior line's trailing comma must go or the JSON is invalid. After each edit, validate via `python -m json.tool < <file>` or rely on `bun run build` failing on malformed JSON.

**Acceptance:**
- `bun run build` succeeds
- `bunx tsc --noEmit` clean
- Post-change `grep -rn videojsError frontend/web/src/` empty

**Commit:**
```
chore(frontend): drop dead videojsError i18n keys

Video.js was removed in Wave 1 T6; these three locale entries are no
longer referenced anywhere in src/. Removing orphan translations.
```

**Metrics:** UXΔ `0 (Ambiguous)` · CDI `0.005 * 1` · MVQ `Sprite 95%/95%`

---

### WV2-T2 — Drop redundant `idx_watch_progress_user_id` standalone index

**Trace:** Audit follow-up "Redundant standalone index (covered by leading column of compound)"
**Why:** Wave 1 T5 created `idx_watch_progress_user_anime_ep (user_id, anime_id, episode_number)`. Its leading column already serves `WHERE user_id = $1` queries via Postgres prefix matching. The standalone secondary just adds write overhead and storage cost per row.

**Files:**
- `services/player/internal/domain/watch.go:32` — remove `;index:idx_watch_progress_user_id` from the GORM tag
- `services/player/cmd/player-api/main.go` — add idempotent `DROP INDEX IF EXISTS idx_watch_progress_user_id` after the existing compound-index `CREATE` block (mirrors T5's pattern)

**Verify-first:**
```sql
\d watch_progress  -- both indexes present before change
```

**Acceptance:**
- Post-restart `\d watch_progress` shows only `idx_watch_progress_user_anime_ep`
- `EXPLAIN ANALYZE SELECT * FROM watch_progress WHERE user_id = '...'` uses the compound index
- Player service starts cleanly, no AutoMigrate errors
- Fresh-DB path (no pre-existing index): AutoMigrate creates only `idx_watch_progress_user_anime_ep` since the `index:` tag is gone; the explicit `DROP INDEX IF EXISTS` is a no-op in this case

**Commit:**
```
perf(player): drop redundant idx_watch_progress_user_id

Wave 1 T5 added the compound unique index on
(user_id, anime_id, episode_number). Postgres can serve user_id-only
queries from its leading column via prefix matching, making the
standalone secondary pure write overhead.

Removes the `index:` GORM tag and drops the existing index
idempotently on next service boot.
```

**Metrics:** UXΔ `+1 (Better)` · CDI `0.024 * 2` · MVQ `Sprite 85%/90%`

---

### WV2-T3 — Centralize comma-list env parsing; fix gateway/streaming correctness bug

**Trace:** Audit follow-up "Env-list parsing duplicated across rooms/gateway/streaming configs"
**Why (escalation found during planning):** gateway uses bare `strings.Split(raw, ",")` which produces `[""]` (one empty string element) when the env var is unset — silently inserting an empty allowed-origin at startup. Streaming has the same bare Split *plus* an inline `[""]→[]` workaround on lines 65-67 carrying a misleading "Empty means allow all" comment (the proxy actually fail-closes on an empty list — verified against `libs/videoutils/proxy.go` and its tests). Rooms parses correctly (Wave 1 T1 fixed it inline with proper trim+filter). The duplication is dangerous because one site's bug is invisible from the others, and the stale streaming comment will mislead the next reader.

**Approach:**
1. New helper `httputil.ParseCommaList(raw string) []string` — trims whitespace, drops empty elements.
2. Replace three call sites with the helper.
3. Single TDD-driven commit.

**TDD:**
1. **Red** — `libs/httputil/parselist_test.go` covering: empty input (`""` → `nil`/`[]`), single value (`"a"` → `["a"]`), multiple values (`"a,b,c"`), leading/trailing whitespace (`" a , b "`), trailing comma (`"a,"`), leading comma (`",a"`), lone comma (`","` → empty), internal empty (`"a,,b"` → `["a","b"]`), whitespace-only (`" "` → empty).
2. **Green** — `libs/httputil/parselist.go` implementing `ParseCommaList`.
3. **Migrate** — update three sites; ALSO delete the misleading "Empty means allow all" comment in `services/streaming/internal/config/config.go:65-67` (it's wrong — the proxy fail-closes on empty).

**Files:**
- `libs/httputil/parselist.go` (new, ~10 lines)
- `libs/httputil/parselist_test.go` (new)
- `services/gateway/internal/config/config.go:105`
- `services/streaming/internal/config/config.go:64`
- `services/rooms/internal/config/config.go:42-50` (replace inline loop with helper call)

**Note:** No new lib added — `libs/httputil` already a dep of all three services. Avoids the go.work / go.mod / Dockerfile triple-update overhead per project convention.

**Acceptance:**
- `go test ./libs/httputil/...` passes
- Each service compiles + boots
- Manual smoke: `CORS_ORIGINS=""` (or unset) → `cfg.CORSOrigins == nil` (or empty), not `[""]`
- Manual smoke: `CORS_ORIGINS="a, ,b "` → `["a", "b"]`

**Commit:**
```
fix(httputil): centralize comma-list env parsing, fix empty-string bug

Gateway and streaming both used bare strings.Split to parse
comma-separated env vars, which produces [""] (one empty element) for
an unset/empty value — silently inserting an empty allowed-origin or
allowed-domain at startup.

Adds httputil.ParseCommaList(raw): trims whitespace, skips empty
elements. Replaces three duplicated parsers (gateway CORS_ORIGINS,
streaming PROXY_ALLOWED_DOMAINS, rooms ALLOWED_WS_ORIGINS).
```

**Metrics:** UXΔ `+1 (Better)` · CDI `0.03 * 3` · MVQ `Sprite 80%/85%`

---

### WV2-T4 — Flush logger before fatal exit in maintenance

**Trace:** Audit follow-up "Sync()-on-Fatalw skip in maintenance (T7 minor)"
**Why:** Zap's `Fatal` calls `os.Exit(1)` directly, bypassing any deferred `logger.Sync()`. For containerized services this is usually fine — container stdout is captured. For the maintenance daemon (host-native binary, no log aggregator), the final fatal log line can vanish into the void. Operator gets a silent restart with no root cause.

**Approach:**
1. Add `Logger.FatalSync(msg string, fields ...interface{})` to `libs/logger`. Implementation:
   - Call `l.Sync()` first (best-effort; ignore error since stderr/stdout sync sometimes returns EINVAL on non-tty).
   - Log at ERROR level (NOT `Fatalw` — zap's `Fatalw` calls `os.Exit` internally, defeating exiter injection).
   - Call `l.exiter(1)` where `exiter func(int)` is a private field defaulting to `os.Exit`.
   - Use `l.base.WithOptions(zap.AddCallerSkip(1))` (or equivalent on the sugared logger) so the log line's caller field points at the *caller* of `FatalSync`, not at `FatalSync` itself. The existing logger already registers `AddCallerSkip(1)` for sugared paths (verified at `libs/logger/logger.go:43`); `FatalSync` wraps one more frame so it needs `+1` on top.
2. Expose a test-only setter `setExiter(fn func(int))` (unexported, accessible from a same-package `*_test.go`) so tests can inject a recorder without leaking the seam into prod APIs.
3. Replace all 6 `log.Fatalw(...)` sites in `services/maintenance/cmd/maintenance/main.go` with `log.FatalSync(...)`.
4. Leave other services alone (containerized, stdout captured by docker, lower priority).

**TDD:**
1. **Red** — `libs/logger/fatalsync_test.go`: inject a fake exiter `func(int)` into the logger, verify that calling `FatalSync` triggers `Sync()` before the exiter is called. Verify the message reaches the output sink.
2. **Green** — implement.
3. **Migrate** maintenance.

**Files:**
- `libs/logger/logger.go` (or new `fatalsync.go`)
- `libs/logger/fatalsync_test.go`
- `services/maintenance/cmd/maintenance/main.go` (6 call sites)

**Acceptance:**
- `go test ./libs/logger/...` passes
- `bin/maintenance` rebuilt and run with a forced failure: final log line visible on stderr
- Exit code remains 1 on failure

**Commit:**
```
fix(logger,maintenance): flush logger before fatal exit

Zap's Fatal calls os.Exit(1) directly, bypassing deferred Sync(). For
maintenance (host-native binary, no log aggregator) the final fatal
line can be lost — operator sees a restart with no root cause.

Adds Logger.FatalSync which Sync()s before exiting. Migrates 6 Fatalw
sites in the maintenance daemon. Other services keep Fatalw for now
(container stdout captures their output cleanly).
```

**Metrics:** UXΔ `0 (Ambiguous)` · CDI `0.01 * 2` · MVQ `Sprite 80%/85%`

---

### WV2-T5 — Reword gateway DevMode `FATAL:` prefix → `WARN:`

**Trace:** Audit follow-up "Misleading FATAL: stderr prefix in gateway's DevMode guard"
**Why:** Wave 1 T4 added a guard at `services/gateway/internal/config/config.go:125`:
```go
fmt.Fprintf(os.Stderr, "FATAL: DEV_MODE=true is forbidden when ENVIRONMENT=%q — forcing DevMode=false\n", cfg.Environment)
```
The code does **not** exit — it sets `DevMode=false` and continues. Labeling the line `FATAL:` misleads operators into thinking gateway crashed.

**File:**
- `services/gateway/internal/config/config.go:125`

**Change:** `"FATAL:"` → `"WARN:"` in the stderr `Fprintf` call. No behavioral change.

**Acceptance:**
- Setting `DEV_MODE=true ENVIRONMENT=production` shows `WARN:` not `FATAL:`
- `make redeploy-gateway` succeeds, service comes up healthy

**Commit:**
```
fix(gateway): correct DevMode guard stderr label from FATAL to WARN

The guard sets DevMode=false and continues — the process does not
exit. Calling the line FATAL: misleads operators into thinking the
gateway crashed. Renames to WARN: to match actual behavior.
```

**Metrics:** UXΔ `+1 (Better)` · CDI `0.002 * 1` · MVQ `Sprite 95%/95%`

---

## Execution discipline

- One commit per task with the three Co-Authored-By trailers (`Claude Opus 4.6`, `0neymik0`, `NANDIorg`).
- **Never** `git add -A` or `git add .` — branch carries pre-existing dirty state that belongs to the user. Stage exactly the files each task changes.
- Skip after-update (deploy + changelog + push) per task. Bundle into one batch via `/animeenigma-after-update` after all five tasks land.
- Each task gets a spec-compliance review + a code-quality review before the next starts.
- If a task surfaces unexpected scope, mark `DONE_WITH_CONCERNS` and stop — do not silently expand.

## Out of scope (intentional)

- Any task with architectural surface area (S3/C2/C3/watch_history/backup-restore-CI) — each gets its own dedicated plan file.
- Touching other services' `Fatalw` calls (only maintenance gets `FatalSync` migration).
- Adding a new `libs/` module (avoid the go.work / go.mod / Dockerfile triple-update for a 10-line helper).

## Post-Wave 2 checklist

- [ ] Plan reviewed by code-reviewer subagent before kickoff (per `feedback_plan_review_pattern.md`)
- [ ] WV2-T1 → review → merge
- [ ] WV2-T2 → review → merge
- [ ] WV2-T3 → review → merge
- [ ] WV2-T4 → review → merge
- [ ] WV2-T5 → review → merge
- [ ] Bundle `/animeenigma-after-update` (redeploy player + gateway + maintenance binary + web bundle; changelog entry)
- [ ] Push to `origin/main`
