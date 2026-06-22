# Maintenance Bot Worktree Isolation — Design

**Date:** 2026-06-22
**Status:** Draft (awaiting owner spec-review)
**Related:** `docs/git-workflow.md` (golden rule), memory `project_autosync_cron_workflow_doc`, `feedback_no_direct_changes_to_main_tree`

## Problem

The maintenance bot (`services/maintenance`, systemd `animeenigma-maintenance`, **root**, `WorkingDirectory=/data/animeenigma`) executes auto-fixes by spawning Claude Code (`claude -p`) with **`cmd.Dir = /data/animeenigma`** (the shared base tree) and an allowlist including `git add/commit/push/checkout/revert` + `Skill`. The prompt (`.claude/maintenance-prompt.md`) tells it to run `/animeenigma-after-update` (lint → redeploy → changelog → commit → push).

So **every fix edits, builds, commits, pushes, and re-syncs inside the shared base tree.** This is the primary source of:
- base-tree churn (uncommitted edits left behind),
- stray local commits / divergence,
- the `rebase --autostash` conflict state that **wedged the ff-only auto-sync** (it paused for an hour; see the 2026-06-22 incident),
- plus the Go process live-writing `.claude/maintenance-state.json` (already gitignored) and `docs/issues/issues.json` (still tracked → churns + can ff-block).

The bot predates and directly contradicts the golden rule ("never make direct changes in `/data/animeenigma`; work in worktrees").

## Goals

1. The bot's fix execution MUST NOT dirty or git-mutate the base tree.
2. Each fix runs in an isolated, throwaway worktree off **fresh `origin/main`**, flushed after the cycle (success or failure).
3. Deploy still works — and must still build from the base tree on `main` (per `/animeenigma-after-update`'s own rule that `make redeploy-*` builds from `/data/animeenigma`, not a worktree).
4. The base tree stays a clean, ff-able mirror so the auto-sync never wedges because of the bot.
5. No loss of capability: analyze, fix, test, redeploy, commit, push, escalate all still work.

## Non-goals

- Rewriting the classifier / Telegram / Grafana ingestion.
- Changing the risk-tiering or admin-approval (Telegram button) model.
- Changing *which* fixes are attempted.
- Changing the binary-in-git deploy model for `bin/maintenance` (see Open Decisions).

## Design

### Fix lifecycle (new)

For each approved fix (currently `Dispatcher.ExecuteFix`, called from `cmd/maintenance/main.go:~1208`):

1. **Provision** — `git -C <base> fetch origin main`; `git -C <base> worktree add -b maint/<fix-id> <WT> origin/main`, where `WT = ${MAINT_WORKTREE_BASE:-/tmp/ae-maint}/<fix-id>`. Optionally symlink `frontend/web/node_modules` from the base tree into `WT` to skip `bun install` on frontend fixes.
2. **Fix** — spawn Claude with `cmd.Dir = WT` (instead of `projectRoot`). Claude edits, tests, and commits **in the worktree**. Allowed-tools unchanged except the deploy split below.
3. **Push** — push the worktree branch to `origin/main` with a fetch→rebase→push **retry loop** (origin/main moves under concurrent agents).
4. **Deploy** — from the base tree on `main`: the Go wrapper brings the base tree to `origin/main` (`git -C <base> merge --ff-only origin/main`, relying on the base tree being clean — see Hygiene) and runs `make redeploy-<svc>` there. The deploy still builds from `/data/animeenigma`, preserving after-update's contract.
5. **Flush** — `git -C <base> worktree remove --force <WT>` + `git -C <base> worktree prune`, in a `defer` so it always runs.

The **analysis** phase (`--permission-mode auto`, read-only) can stay in the base tree (it makes no changes) or move to the worktree for uniformity; recommend the worktree for uniformity and so its scratch reads see fresh `origin/main`.

### Go changes (`services/maintenance`)

- **`internal/worktree/` (new small package):** `Provision(base, id) (dir string, cleanup func(), err error)` — fetch + `worktree add` + optional node_modules symlink; `cleanup()` does `worktree remove --force` + `prune`. Unit-tested against a temp git repo.
- **`internal/dispatcher/claude.go`:** `invoke()` gains a `workdir` param (defaults to `projectRoot`); `ExecuteFix` runs in the provisioned worktree. The fetch→rebase→push retry can live here or in the prompt (see Prompt).
- **`cmd/maintenance/main.go`:** wrap the `ExecuteFix` call with `Provision`/`cleanup`; after a successful push, run the base-tree ff + `make redeploy-<svc>` deploy step. Startup sweep: prune stale `${MAINT_WORKTREE_BASE}/*` from prior crashes.
- **`internal/config/config.go`:** add `MAINT_WORKTREE_BASE` (default `/tmp/ae-maint`), `MAINT_WORKTREE_ISOLATION` feature flag (default **off** for safe rollout), `COMPOSE_PROJECT_NAME` passthrough.

### Prompt changes (`.claude/maintenance-prompt.md`)

- Remove hardcoded `cd /data/animeenigma/...` (those point at the **base tree** even when Claude's CWD is a worktree); use CWD-relative paths.
- Replace the plain `git push` / `git checkout HEAD -- <file>` rollback guidance with worktree-aware equivalents and the fetch→rebase→push retry loop.
- State the worktree → push → deploy split explicitly, and link `docs/git-workflow.md`.
- Note that deploy is performed from the base tree on `main` (by the Go wrapper) — Claude commits + pushes; it does not `make redeploy` from inside the worktree.

### Deploy-from-base-tree plumbing

- Ensure `COMPOSE_PROJECT_NAME=docker` in `docker/maintenance.env` so `make redeploy-*` targets the running stack regardless of CWD.
- The base tree must be clean to ff before deploy → see Hygiene.

### Hygiene (supporting changes)

- **`docs/issues/issues.json`** — live-written by the Go state manager in the base tree → churn + ff-block vector. **Open Decision (below).**
- **`/animeenigma-after-update`** — change the final plain `git push` (`.claude/commands/animeenigma-after-update.md:150`) to the fetch→rebase→push retry loop, matching `docs/git-workflow.md`.

## Open Decisions (resolve at spec-review)

1. **`docs/issues/issues.json` handling:**
   - **I-A (simplest, kills churn):** gitignore it as host-local runtime state (like `maintenance-state.json`). Cost: the issue log (`AUTO-xxx`) stops being tracked in git going forward.
   - **I-B (preserves history):** the bot commits issues.json changes through the worktree flow. Cost: more machinery + more commits (per issue update). **Recommendation: I-A** unless the in-git issue log is valued.
2. **`bin/maintenance`** is currently a **committed** binary (latest origin/main commit rebuilds it) — that's how the host gets the new binary via git ff. Leaving as-is. *Optional, separate:* switch to build-on-host + gitignore the binary (stops the only remaining artifact churn, but changes the deploy model). Out of scope unless you want it.

## Risks & mitigations

- **Deploy-from-worktree pitfall** — `make redeploy` must build from the base tree on `main`, NOT the worktree (else a separate compose stack / stale code). Mitigation: explicit base-tree deploy step; `COMPOSE_PROJECT_NAME=docker` pinned.
- **node_modules cost** for frontend fixes in a fresh worktree — symlink the base tree's `frontend/web/node_modules` (read reuse) or accept `bun install`.
- **Base tree dirty at deploy** blocks the ff — resolved by the issues.json decision (and bin/maintenance staying committed/clean).
- **Live service** — rollout via `make build-maintenance` + `systemctl restart animeenigma-maintenance`; gate behind `MAINT_WORKTREE_ISOLATION` (default off) so we can revert instantly.
- **Worktree leak** on crash — `defer` cleanup + startup sweep of `${MAINT_WORKTREE_BASE}/*`.
- **git-guard** — worktree ops are exempt; the base-tree `merge --ff-only` is an allowed (non-destructive) verb.

## Testing / verification

- **Unit:** `worktree.Provision/cleanup` (create → exists → remove, idempotent prune); deploy-dir resolution; prompt-path relativity check.
- **Integration (dry-run):** a fake fix that edits one file → assert base tree `git status` stays empty throughout, the worktree is created then removed, and the commit lands on the `maint/<id>` branch.
- **Manual:** trigger one low-risk fix with the flag on → verify (a) base tree clean before/during/after, (b) commit on `origin/main`, (c) deploy succeeded + health green, (d) worktree flushed, (e) `/var/log/animeenigma-git-sync.log` stays healthy (no `CONFLICTED`/`DIVERGED`).

## Scoring (per `.planning/CONVENTIONS.md`)

- **UXΔ = +1 (Better)** — no end-user surface change; removes the recurring wedge that stalls deploys + the base-tree churn that burns agent time.
- **CDI = 0.06 * 13** — spread across the maintenance service (new worktree pkg + dispatcher + main loop + config), the prompt, and two doc/hygiene tweaks; moderate shift; Effort_Fib 13.
- **MVQ = Griffin 85%/80%** — disciplined isolation that guards the shared tree; low slop once the worktree lifecycle is in place.

## Rollout

1. Implement in a worktree; `make build-maintenance`.
2. Ship with `MAINT_WORKTREE_ISOLATION=false`; force one low-risk fix to validate the path end-to-end.
3. Flip the flag true in `docker/maintenance.env`; `systemctl restart animeenigma-maintenance`.
4. Watch one full cycle (maintenance journal + autosync log); confirm the base tree stays clean.
