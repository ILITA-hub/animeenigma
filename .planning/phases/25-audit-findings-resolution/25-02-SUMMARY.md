---
phase: 25-audit-findings-resolution
plan: 02
status: completed
requirement: SCRAPER-HEAL-23
audit_finding: W-INT-02
date: 2026-05-19
---

# Plan 25-02 Summary — Remove stale cacheStream symbol reference

## What changed (single line edit)

`.claude/maintenance-prompt.md` line 256 — the `signed_url_expired`
bullet in the Scraper Playability Regression triage list. Three
substantive changes:

1. Dropped the dead `cacheStream` symbol from the slash-alternative.
2. Changed `client.go` → `cache.go` (where `computeStreamTTL` actually lives).
3. Changed "constant" → "helper" (it's a function, not a const).

### Before:

```
4. `signed_url_expired` → find the stream-cache TTL constant in
   `services/scraper/internal/providers/<name>/client.go` (search for
   `cacheStream` / `computeStreamTTL`) and shorten if the upstream
   signed-URL TTL is now shorter than ours. Tier: `button_fix`.
```

### After:

```
4. `signed_url_expired` → find the stream-cache TTL helper in
   `services/scraper/internal/providers/<name>/cache.go` (search for
   `computeStreamTTL`) and shorten if the upstream signed-URL TTL is
   now shorter than ours. Tier: `button_fix`.
```

## Verification

### Grep contract:

```
$ grep -c cacheStream .claude/maintenance-prompt.md
0
$ grep -c computeStreamTTL .claude/maintenance-prompt.md
1
$ grep -cE "^### (Pattern 6|Pattern 7|Scraper Playability Regression)" .claude/maintenance-prompt.md
3
```

`cacheStream` gone; `computeStreamTTL` retained; all three required
section headings byte-identical.

### Symbol-stability tests:

```
$ cd services/maintenance && go test ./internal/classifier/... \
    -run "TestMaintenancePrompt_" -count=1 -v
=== RUN   TestMaintenancePrompt_FilePresentInWorkingDir
--- PASS: TestMaintenancePrompt_FilePresentInWorkingDir (0.00s)
=== RUN   TestMaintenancePrompt_ContainsPatterns6And7
--- PASS: TestMaintenancePrompt_ContainsPatterns6And7 (0.00s)
=== RUN   TestMaintenancePrompt_AllReasonsCovered
--- PASS: TestMaintenancePrompt_AllReasonsCovered (0.00s)
PASS
ok  	github.com/ILITA-hub/animeenigma/services/maintenance/internal/classifier	0.003s
```

### Full maintenance package:

```
$ cd services/maintenance && go test ./... -count=1
ok  	github.com/ILITA-hub/animeenigma/services/maintenance/internal/classifier	0.004s
ok  	github.com/ILITA-hub/animeenigma/services/maintenance/internal/config	0.002s
ok  	github.com/ILITA-hub/animeenigma/services/maintenance/internal/transport	0.208s
```

All green; no regressions.

## Diff snapshot

```
.claude/maintenance-prompt.md | 2 +-
1 file changed, 1 insertion(+), 1 deletion(-)
```

## Anchor

W-INT-02 (Phase 25 milestone audit, 2026-05-13): operator/bot
guidance referenced a non-existent `cacheStream` symbol; corrected to
the actually-existing `computeStreamTTL` in
`services/scraper/internal/providers/{gogoanime,animepahe}/cache.go`.
SCRAPER-HEAL-23 closed.
