---
phase: 25-audit-findings-resolution
plan: 01
status: completed
requirement: SCRAPER-HEAL-22
audit_finding: W-INT-01
date: 2026-05-19
---

# Plan 25-01 Summary — Fix TestGetStreamWithGate_AdDecoy_Skipped parallel-probe race

## What changed (test-only)

`services/scraper/internal/providers/gogoanime/client_gated_test.go` —
rewrote the body of `TestGetStreamWithGate_AdDecoy_Skipped` so the AdDecoy
server now occupies position 2 (vibeplayer / sequential-remainder branch)
instead of position 0 (streamhg / parallel-probe top). All three servers
fail; the test asserts `ErrProviderDown` + both counter increments on
vibeplayer.

Production code at `services/scraper/internal/providers/gogoanime/client.go`
is **byte-identical** before/after (verified `git diff --stat` empty).

## Strategy — A (swap priority order)

Picked Strategy A from the plan's task 1 decision matrix:

- Matches the existing working pattern (sequential-remainder branch
  already proven race-free elsewhere in the file).
- Zero new test infrastructure; FakeProbe untouched.
- The contract being tested (AdDecoy probe → both counters Inc) is
  identical to the original; only the server slot moves.

Strategy B (sync barrier in FakeProbe) was rejected as unnecessary
surface area when topology alone solves the race.

## Diff snapshot

```
services/scraper/internal/providers/gogoanime/client_gated_test.go | 79 ++++++++++++++++------
1 file changed, 60 insertions(+), 19 deletions(-)
```

## Verification logs

### `go test -race -count=10` (the audit's failure window):

```
$ cd services/scraper && go test -race ./internal/providers/gogoanime/... \
    -run TestGetStreamWithGate_AdDecoy_Skipped -count=10
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/gogoanime	1.042s
```

10/10 green, no `DATA RACE` warnings.

### Full gogoanime package `-race -count=3`:

```
$ go test -race ./internal/providers/gogoanime/... -count=3
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/gogoanime	5.270s
```

All `TestGetStreamWithGate_*` tests green; `TestGetStreamWithGate_ParallelTop2`
parallelism still holds; no regressions.

### Extended `-race -count=20` push on the rewritten test:

```
$ go test -race ./internal/providers/gogoanime/... \
    -run TestGetStreamWithGate_AdDecoy_Skipped -count=20
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/gogoanime	1.063s
```

20/20 green.

## Boundary respected

- Production code untouched: `git diff services/scraper/internal/providers/gogoanime/client.go` is empty.
- Counter assertions remain strict equality (`== 1`, no `>= 1`).
- No `runtime.Gosched()` / `time.Sleep` shims anywhere.
- The inline comment block in the rewritten test references W-INT-01,
  CONTEXT.md D2, and the parCancel-vs-Inc race rationale.

## Anchor

W-INT-01 (Phase 25 milestone audit, 2026-05-13). SCRAPER-HEAL-22 closed.
