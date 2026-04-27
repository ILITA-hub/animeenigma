---
phase: 01-instrumentation-baseline
plan: 07
status: partial
tasks_complete: [0, 1, 3]
tasks_pending: [2, 4]
handoff_to: human
---

# Plan 07 SUMMARY — Docs + Deploy + Verify + Ship

## Status

**Tasks 0, 1, 3 complete inline by orchestrator.**
**Tasks 2 (Grafana human-verify) and Task 4 (animeenigma-after-update skill) pending — handed off to user per checkpoint:human-verify and checkpoint:human-action gates.**

## Task 0 — Loki retention subsection in PROJECT.md ✓

Verified the subsection is already present in `.planning/PROJECT.md`:

| Acceptance check | Result |
|---|---|
| `grep -ic "loki retention" PROJECT.md` ≥ 1 | 3 ✓ |
| `grep -cE "168h\|7 days\|7d" PROJECT.md` ≥ 1 | 4 ✓ |
| `grep -c "31d\|31 days" PROJECT.md` ≥ 1 | 2 ✓ |
| `grep -c "Phase 5\|per-event DB table" PROJECT.md` ≥ 1 | 1 ✓ |
| `docker/loki/loki-config.yml:27-28` still says `168h  # 7 days` | ✓ |

The 31d documentation error from CONTEXT D-06 is corrected in PROJECT.md with the Phase 5 escape-hatch noted.

## Task 1 — Deploy + smoke tests + Grafana restart ✓

### Redeploys

| Step | Exit | Notes |
|---|---|---|
| `make redeploy-player` | 0 | `player:8083 - healthy` |
| `make redeploy-gateway` | 0 | `gateway:8000 - healthy` |
| `make redeploy-web` | 0 | i18n-lint + tsc clean, frontend rebuilt |
| `make health` | 0 | `gateway, auth, catalog, streaming, player, rooms, scheduler` all ✓ |

### Live smoke tests (from `/tmp/plan07-smoke.log`)

| # | Scenario | Expected | Actual | Result |
|---|---|---|---|---|
| 4a | `/metrics` exposes `combo_override_total` + `combo_resolve_total` HELP lines | 2 | 0 before first emit, 2 after smokes 4b/4c | ✓ (counters self-register on first emission per Prometheus client_golang behavior) |
| 4b | Anon `POST /api/preferences/resolve` | 200 | 200 | ✓ |
| 4c | Anon `POST /api/preferences/override` (valid body) | 204 | 204 | ✓ |
| 4d | `dimension=INVALID` (T-01-02 cardinality protection) | 400 | 400 (`"dimension must be language\|player\|team\|episode"`) | ✓ |
| 4e | No JWT, no X-Anon-ID (T-01-01 identity guard) | 400 OR 401 | 400 (`"X-Anon-ID required for unauthenticated requests"`) | ✓ |

### Grafana restart

```
$ docker compose -f docker/docker-compose.yml restart grafana
 Container animeenigma-grafana Started
$ docker compose ... logs grafana --tail 60 | grep -iE "provision|reload"
... msg="Path Provisioning" path=/etc/grafana/provisioning
... msg="starting to provision alerting"
... msg="finished to provision alerting"
```

Provisioning reloaded — new dashboard JSON picked up. Plan acceptance `>= 1 provision/reload line` satisfied.

## Task 2 — Grafana panel renders (human-verify) ⏳ PENDING

User must visually confirm at `https://admin.animeenigma.ru/grafana/d/preference-resolution-v1`:

1. New collapsible row "Auto-Pick Override Rate (Phase 1 Baseline)" visible
2. All 5 sub-panels render (Override Rate stat, by Tier, by Player, by Language/Auth, by Dimension barchart)
3. Stat panel description tooltip mentions "fresh resolves only"
4. Edit panel → PromQL references `combo_override_total` and `combo_resolve_total`
5. Smoke test override should appear under Dimension barchart (`language: 1`)

Optional: Grafana Explore → LogQL `{service="player"} |= "combo_override" | json` over last 1h, verify NO `username` / `Authorization` / token (T-01-03 manual confirmation).

**Resume signal:** "approved" or "issues: <description>".

## Task 3 — Phase 1 follow-up marker in STATE.md ✓

Appended new section + bullet at end of `.planning/STATE.md`:

```markdown
## Phase 1 Follow-ups

- **Phase 1 follow-up:** Capture ≥ 24h baseline override-rate snapshot to .planning/PROJECT.md before Phase 6 starts. Computed via PromQL: rate(combo_override_total[24h]) / rate(combo_resolve_total[24h]), segmented by tier/language/anon/player/dimension. This is ROADMAP success criterion 3 — a phase-gate, not a Phase 1 task. Do not open Phase 6 work until this snapshot is recorded under PROJECT.md § "Baseline override rate".
```

| Acceptance check | Result |
|---|---|
| `grep -c "Phase 1 follow-up"` ≥ 1 | 1 ✓ |
| `grep -c "rate(combo_override_total"` ≥ 1 | 1 ✓ |
| `grep -c "Phase 6"` ≥ 2 (gating phase named) | 2 ✓ |

## Task 4 — animeenigma-after-update skill (human-action) ⏳ PENDING

User invokes the `/animeenigma-after-update` skill which performs:
- a. Lint (`bunx eslint src/`, `bunx tsc --noEmit`, `go vet ./...`)
- b. Build (`bunx vite build` if needed)
- c. Redeploy (already done in Task 1; skill is fast no-op)
- d. `make health`
- e. Update `frontend/web/public/changelog.json` with Phase 1 instrumentation entry (informative + enthusiastic + emojis, schema is array-of-date-groups with `entries[].message`)
- f. Commit with 3 co-authors:
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- g. Push to `origin/main`

Acceptance:
- `git log -1 --pretty=fuller` shows 3 co-authors
- `git status` is clean
- `git rev-list HEAD..@{u} --count` = 0 (or `git status -sb` shows ahead 0 / behind 0)
- `jq -r '.[0].entries[].message' frontend/web/public/changelog.json | grep -ciE "override|baseline|instrumentation"` ≥ 1

**Pre-handoff state of working tree** (these are uncommitted files the skill will pick up):
- `M .claude/maintenance-state.json`
- `M .planning/STATE.md` (updated by Task 3 + roadmap progress)
- `M docs/issues/issues.json` (pre-existing from session start)
- `M frontend/web/public/changelog.json` (pre-existing, will be replaced by skill)

Local main is **23 commits ahead of `origin/main`** with all of Phase 1's work + 1 SUMMARY commit.

## ⚠ PHASE 6 BLOCKER

> **Capture 24h baseline override rate from Grafana on `2026-04-28` (≥ 24h after deploy)** and record under `.planning/PROJECT.md § Baseline override rate` before opening Phase 6 work.
>
> PromQL one-liner:
> `rate(combo_override_total[24h]) / rate(combo_resolve_total[24h])`
>
> Segmented by tier / language / anon / player / dimension.

This is ROADMAP success criterion 3. STATE.md carries the same reminder under `## Phase 1 Follow-ups`.

## Files modified by this orchestrator run

- `.planning/STATE.md` (Task 3 marker + ongoing tracking updates)
- `.planning/phases/01-instrumentation-baseline/01-07-SUMMARY.md` (new)

No source code changes — Task 1 was deploy-only; Task 0 was verify-only; Task 3 was a docs marker append.

## Self-Check

- [x] Tasks 0, 1, 3 acceptance criteria all green
- [x] Live smoke tests pass on production
- [x] Grafana provisioning reloaded
- [x] STATE.md carries Phase 1 follow-up
- [ ] Tasks 2, 4 pending — handed off to user
- [x] No modifications to ROADMAP.md frontmatter beyond progress tracking
- [x] SUMMARY.md committed before handoff
