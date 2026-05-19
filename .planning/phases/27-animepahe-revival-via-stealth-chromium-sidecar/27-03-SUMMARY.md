---
phase: 27
plan: 03
subsystem: docker-compose / scraper boot wiring
tags:
  - docker-compose
  - sidecar
  - healthcheck
  - mem_limit
  - depends_on
  - SCRAPER-HEAL-31
dependency_graph:
  requires:
    - 27-01  # sidecar Dockerfile + /healthz two-layer probe
    - 27-02  # scraper config.AnimePahe.ResolverURL + main.go boot log key
  provides:
    - "compose-level wiring: animepahe-resolver service block + scraper depends_on + SCRAPER_ANIMEPAHE_RESOLVER_URL env"
    - "runtime-enforced 500 MiB memory cap on the sidecar (cgroup-level)"
    - "boot-ordering guarantee: scraper holds in Created until resolver /healthz reports browser=up"
  affects:
    - docker/docker-compose.yml (animepahe-resolver service added; scraper env + depends_on grown; ANIMEPAHE_BASE_URL removed)
tech_stack:
  added: []
  patterns:
    - "docker-compose mem_limit (legacy syntax) + cpus + shm_size for cgroup-enforced resource cap"
    - "depends_on: condition: service_healthy chained on a two-layer healthcheck (HTTP /healthz that internally proves Chromium alive)"
    - "internal docker-compose network DNS for service-to-service URL (no host ports for the sidecar)"
key_files:
  created: []
  modified:
    - docker/docker-compose.yml
decisions:
  - "Kept legacy mem_limit syntax (compose file already uses it for other services) over deploy.resources.limits.memory; runtime inspect confirmed 524288000 bytes cap applied — no fallback needed."
  - "SCRAPER_DEGRADED_PROVIDERS default left UNCHANGED at gogoanime,animepahe per CONTEXT.md D7 — Plan 27-05 owns the flip after Plan 27-04 end-to-end Frieren gate."
  - "Removed ANIMEPAHE_BASE_URL env entry (the code stopped reading it in Plan 27-02). No grace period needed — clean break."
  - "Did NOT expose host port for animepahe-resolver. Sidecar is internal-only per CLAUDE.md 'Service Ports' (reached via animepahe-resolver:3000 docker-compose DNS)."
  - "Did NOT grant cap_add: SYS_ADMIN (T-27-03-02). --no-sandbox is the documented sandbox-bypass; capability grant is unnecessary."
metrics:
  duration_seconds: 190
  completed_at: "2026-05-19T10:48:00Z"
  tasks_completed: 3
  files_touched: 1
  commits: 3
---

# Phase 27 Plan 03: Docker Compose Wiring + Healthcheck Dependency + Mem Limit Summary

**One-liner:** Compose-level wiring of the stealth-Chromium sidecar — adds `animepahe-resolver` service with cgroup-enforced 500 MiB memory cap + two-layer `/healthz` probe; grows scraper's `depends_on` to gate boot on resolver health; clean-breaks the deprecated `ANIMEPAHE_BASE_URL` env in favor of `SCRAPER_ANIMEPAHE_RESOLVER_URL` consumed by Plan 27-02's config path.

## Goal

Wire the Phase 27 sidecar into `docker/docker-compose.yml` so:
1. The `animepahe-resolver` service exists with all six ship-gate directives (mem_limit, cpus, shm_size, init, security_opt seccomp:unconfined, healthcheck on `/healthz`) and **no** `cap_add` / `ports`.
2. The `scraper` service consumes `SCRAPER_ANIMEPAHE_RESOLVER_URL=http://animepahe-resolver:3000` over the docker-compose internal network and refuses to boot until the resolver is healthy.
3. Cold-start ordering is proven empirically (not just statically) — `depends_on: service_healthy` actually holds the scraper.

`SCRAPER_DEGRADED_PROVIDERS` flip is **deliberately deferred to Plan 27-05** per D7 — this plan stops at "the wiring exists and is correct," not "users see animepahe playing yet."

## Tasks Completed

| # | Task | Commit | Files |
|---|---|---|---|
| 1 | Add animepahe-resolver service block | `cd183b9` | docker/docker-compose.yml |
| 2 | Wire scraper depends_on + env, remove deprecated ANIMEPAHE_BASE_URL | `369d6aa` | docker/docker-compose.yml |
| 3 | Cold-compose smoke (manual gate; --allow-empty commit) | `6a1b14d` | (none — runtime gate) |

## Verification Performed

### Static (Tasks 1 & 2)
- `docker compose -f docker/docker-compose.yml config animepahe-resolver` resolves the block with `mem_limit: "524288000"` (500 MiB), `init: true`, `shm_size: "268435456"`, `security_opt: [seccomp:unconfined]`, healthcheck on `wget -q --spider http://localhost:3000/healthz`, `cpus: 0.5`.
- No `cap_add` line under the resolver block (T-27-03-02 mitigated).
- No `ports:` line under the resolver block (internal-only).
- `docker compose -f docker/docker-compose.yml config scraper` resolves with `SCRAPER_ANIMEPAHE_RESOLVER_URL: http://animepahe-resolver:3000` and `depends_on.animepahe-resolver.condition: service_healthy, required: true`.
- `ANIMEPAHE_BASE_URL` removed — `grep` over the compose file returns zero matches.
- `SCRAPER_DEGRADED_PROVIDERS: ${SCRAPER_DEGRADED_PROVIDERS:-gogoanime,animepahe}` line unchanged (D7).
- Whole-file `docker compose config` parses without error.
- Defensive: `go test ./services/scraper/internal/config/ -run TestConfig_AnimepaheResolverURL` — all 3 sub-tests (default / override / invalid) green.

### Runtime cold-start smoke (Task 3)

`docker compose stop scraper animepahe-resolver && docker rm animeenigma-scraper && docker compose build animepahe-resolver scraper && docker compose up -d animepahe-resolver scraper`:

The `up -d` output itself proves the dependency chain:
```
Container animeenigma-animepahe-resolver Started
Container animeenigma-animepahe-resolver Waiting     <-- scraper waits here
Container animeenigma-redis Waiting
Container animeenigma-megacloud-extractor Waiting
Container animeenigma-redis Healthy
Container animeenigma-megacloud-extractor Healthy
Container animeenigma-animepahe-resolver Healthy     <-- only THEN
Container animeenigma-scraper Starting
Container animeenigma-scraper Started
```

Followed by:
- `docker inspect animeenigma-animepahe-resolver --format '{{.HostConfig.Memory}}'` → `524288000` (T-27-03-01 cgroup cap enforced).
- `docker exec animeenigma-scraper env | grep SCRAPER_ANIMEPAHE_RESOLVER_URL` → `http://animepahe-resolver:3000`.
- `docker exec animeenigma-scraper env | grep ANIMEPAHE_BASE_URL` → empty (clean break confirmed).
- Scraper boot log: `scraper service ready ... animepahe_resolver_url=http://animepahe-resolver:3000 ...` — no `animepahe_base_url` key (Plan 27-02 confirmed in production).
- `curl http://localhost:8088/health` → `{"success":true,"data":{"status":"ok"}}` (scraper healthy).
- `wget -qO- http://animepahe-resolver:3000/healthz` (from inside the scraper container) → `{"browser":"up","lastChallengeSolveAt":null,"pageCount":0}` — two-layer probe alive end-to-end (T-27-03-03 mitigated).

## Plan Exit Criteria

All 6 checks from `27-03-PLAN.md` "Plan Exit Criteria" pass:
1. `docker compose config animepahe-resolver` — valid.
2. `docker compose config scraper | grep animepahe-resolver` — present.
3. Runtime mem cap = 524288000.
4. Scraper env has `SCRAPER_ANIMEPAHE_RESOLVER_URL=http://animepahe-resolver:3000`.
5. Scraper env does NOT have `ANIMEPAHE_BASE_URL`.
6. Compose default `SCRAPER_DEGRADED_PROVIDERS:-gogoanime,animepahe` line unchanged.

## Threat Model — Disposition

| Threat ID | STRIDE | Mitigation Status |
|---|---|---|
| T-27-03-01 | DoS via Chromium memory growth | MITIGATED — `mem_limit: 500m` is cgroup-enforced (verified 524288000 bytes at runtime). Restart policy + page-recycle from Plan 27-01 close the loop. |
| T-27-03-02 | Privilege escalation via SYS_ADMIN | MITIGATED — no `cap_add` of any kind. Reviewer checks the diff: `docker/docker-compose.yml` adds 30 lines; zero contain `cap_add`. |
| T-27-03-03 | Scraper races resolver boot | MITIGATED — empirically proven: compose log sequence shows scraper does NOT start until resolver flips healthy. Two-layer `/healthz` probe (browser:"up") makes this Chromium-grade, not just process-grade. |

No new threat surface introduced beyond what the threat register already enumerates.

## Deviations from Plan

None — plan executed exactly as written across all three tasks.

The plan's "if mem_limit didn't apply, fall back to `deploy.resources.limits.memory`" contingency was unnecessary: the legacy `mem_limit` syntax applied cleanly at runtime under Docker Compose v5.0.2 (current host version). No fallback path engaged.

## Known Stubs

None. Compose-layer wiring is operational and complete.

## Threat Flags

None — no new security-relevant surface introduced beyond what the plan's `<threat_model>` declared (which is itself a subset of Plan 27-01's broader sidecar threat register).

## Cross-References

- **Backward:** Plan 27-01 (sidecar source/Dockerfile/healthz route), Plan 27-02 (`AnimePaheConfig.ResolverURL` + `animepahe_resolver_url` boot log key, validation that catches malformed URLs at boot).
- **Forward:** Plan 27-04 — end-to-end Frieren (MAL 52991) gate via the gateway against the now-wired stack. Plan 27-05 — flip `SCRAPER_DEGRADED_PROVIDERS` default to `gogoanime` (removing `animepahe` from the kill-list) after 27-04 passes.

## Self-Check

Verified before writing this summary:
- `docker/docker-compose.yml` modifications committed — `git log` shows `cd183b9` and `369d6aa`.
- `git log --oneline -3` confirms `6a1b14d` (Task 3), `369d6aa` (Task 2), `cd183b9` (Task 1) in worktree branch `worktree-agent-af6fab4683db769b9`.
- Both `animeenigma-animepahe-resolver` and `animeenigma-scraper` containers are `Up (healthy)`.

## Self-Check: PASSED
