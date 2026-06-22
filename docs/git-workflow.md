# Git & Deploy Workflow

How code moves from an idea to `origin/main` and into production on this repo.

## TL;DR

```
origin/main ──(cron: ff-only every 10 min)──▶  /data/animeenigma   ← shared base tree
                                                       │              (READ-ONLY MIRROR)
                                                       │
        ① git fetch origin && git worktree add -b <branch> <path> origin/main
                                                       │
        ② code + verify in the worktree   (tests · lint · build · smoke)
                                                       │
        ③ merge to main:  git fetch origin main && git rebase origin/main
                           && git push origin HEAD:main      (retry on race)
                                                       │
        ④ /animeenigma-after-update   ← run AFTER ③, to catch the latest main
                           (lint → build → redeploy → health → changelog → commit → push)
                                                       │
        ⑤ git worktree remove <path> && git worktree prune   ← ONLY after ④ is green
                                                       ▼
                       cron ff-forwards /data/animeenigma to the new origin/main
```

## The golden rule

> **Never make direct changes in `/data/animeenigma` (the `main` base tree) —
> do all work in worktrees. The only exception is `.env` / secrets files**
> (they are git-ignored and host-only, so editing them in place does not create
> git divergence or dirt).

The base tree is a **read-only mirror of `origin/main`**, kept fresh by a cron
(see below). Any code edit or local commit there makes it **dirty or diverged**,
which:

- pauses the auto-sync (it is fast-forward-only and no-ops on divergence — see
  the `skip: DIVERGED` log line), so the tree rots behind `origin/main`, and
- makes it impossible to cleanly commit *just your change* (your diff gets
  tangled with everyone else's uncommitted work — the exact mess this workflow
  exists to prevent).

Keep the base tree pristine and every worktree branches off current code,
fast-forwards cleanly, and the auto-sync keeps working.

## Step by step

### ① Create a worktree off **fresh `origin/main`**

Always base off the freshly-fetched remote ref, never the local `HEAD` (the base
tree can be minutes-to-hours behind even with auto-sync, and is never ahead):

```bash
cd /data/animeenigma
git fetch origin
git worktree add -b <feature-branch> /tmp/ae-<feature> origin/main
cd /tmp/ae-<feature>
# frontend only: bun install   (worktrees don't share node_modules)
```

### ② Code + verify **in the worktree**

Make the change and prove it green *before* it touches `main`:

```bash
go test ./...                         # affected Go services
cd frontend/web && bunx vitest run && bunx vue-tsc --noEmit   # frontend
```

Commit in small atomic commits (conventional-commit style, e.g.
`feat(scope): …`). One change ≈ one worktree ≈ one (or few) commits.

### ③ Merge to main — pull-rebase-push (retry on race)

`origin/main` moves constantly (multiple agents push). Rebase your commits onto
the latest and fast-forward push, retrying if it moved under you:

```bash
for i in 1 2 3 4 5; do
  git fetch origin main \
    && git rebase origin/main \
    && git push origin HEAD:main && break
  echo "push race — retrying ($i)…"; sleep 2
done
```

This keeps history **linear** (no merge commits). Resolve any rebase conflicts
in the worktree and re-run.

### ④ `/animeenigma-after-update` — **after** the push

Run it *after* ③ so it operates on the freshest `main` (catches other agents'
last updates merged in) and deploys the true merged state. It:
lints → builds → `make redeploy-<service>` → health-checks → writes the
user-facing changelog → commits → pushes.

Run it from the worktree. (Its changelog/build commit also pushes — the same
race-retry from ③ applies.)

> Skip `/animeenigma-after-update` only for changes with **no** user-facing or
> deployable surface (pure docs, host-only infra). Those just need ③.

### ⑤ Tear down — **only after ④ is green**

Keep the worktree as a safety net until deploy + health + changelog all
succeeded. Then:

```bash
cd /data/animeenigma
git worktree remove /tmp/ae-<feature>
git worktree prune
git branch -d <feature-branch>      # -D if it was a throwaway
```

If ④ failed (build/health/deploy), fix it **in the still-existing worktree** and
re-run ④ before tearing down.

## Auto-sync of the base tree

A cron fast-forwards `/data/animeenigma` to `origin/main` every 10 minutes:

- **Script:** `/usr/local/bin/animeenigma-git-autosync.sh`
  (source of truth: [`infra/host/animeenigma-git-autosync.sh`](../infra/host/animeenigma-git-autosync.sh))
- **Cron:** `/etc/cron.d/animeenigma-git-sync`
- **Log:** `/var/log/animeenigma-git-sync.log`
- **Safety:** fast-forward-only — never merges (non-ff), rebases, stashes,
  resets, force-updates, or pushes. No-ops (and logs why) on divergence,
  uncommitted-overlap, detached HEAD, or fetch failure. Install/uninstall docs:
  [`infra/host/README.md`](../infra/host/README.md).

If you see `skip: DIVERGED` in the log, the golden rule was broken — someone
committed directly to the base tree. Push or drop those commits to resume sync.

## Deploy notes

- Run deploys (`make redeploy-<service>`) from the **worktree** (or the freshened
  base tree post-merge), never from a dirty base tree. Use
  `COMPOSE_PROJECT_NAME=docker` so it targets the running stack.
- Frontend deploy gate is `vue-tsc` (not bare `tsc`) — it catches `.vue`
  template type errors that vitest misses.

## Why this shape

- **Worktrees isolate** concurrent agents — no shared dirty state, no stepping on
  each other's uncommitted work.
- **Rebase-push** keeps `main` linear and bisectable.
- **After-update last** means you deploy/document the *real* merged HEAD, not a
  stale branch tip.
- **Auto-sync + the golden rule** keep the base tree a clean, current mirror so
  step ① is always trustworthy.
