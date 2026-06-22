# Host automation — `/data/animeenigma` git auto-sync

Keeps the **shared base tree** at `/data/animeenigma` (checked out on `main`)
fast-forwarded to `origin/main` every 10 minutes, so worktrees always branch
off fresh code and the base tree never rots tens of commits behind.

See [`docs/git-workflow.md`](../../docs/git-workflow.md) for the full development
workflow this supports.

## Files

| Repo (source of truth)                   | Installed to                              |
|------------------------------------------|-------------------------------------------|
| `infra/host/animeenigma-git-autosync.sh` | `/usr/local/bin/animeenigma-git-autosync.sh` (mode 755) |
| `infra/host/animeenigma-git-sync.cron`   | `/etc/cron.d/animeenigma-git-sync` (root:root, mode 644) |

## Install / update

```bash
install -m 755 infra/host/animeenigma-git-autosync.sh /usr/local/bin/animeenigma-git-autosync.sh
install -m 644 infra/host/animeenigma-git-sync.cron    /etc/cron.d/animeenigma-git-sync
# cron picks up /etc/cron.d/* automatically; no reload needed.
```

## Safety contract

The script is **fast-forward-only**. It will **never** merge (non-ff), rebase,
stash, reset, force-update, or push. On any of these conditions it logs the
reason and exits 0 having changed nothing:

- HEAD is not on `main` (detached / other branch)
- `git fetch` fails (e.g. transient network)
- the tree has **diverged** (local commits not on `origin/main`)
- a fast-forward is **blocked by uncommitted changes** that overlap incoming files

This makes it safe on the shared tree and compatible with the `git-guard`
PreToolUse hook (`.claude/hooks/git-guard.py`).

> **Divergence pauses sync.** If the log shows `skip: DIVERGED`, someone committed
> directly to the base tree's `main`. Push or drop those commits to resume
> fast-forwarding. The fix is to **never commit directly to `/data/animeenigma`** —
> see `docs/git-workflow.md`.

## Inspect

```bash
tail -f /var/log/animeenigma-git-sync.log     # outcomes, one line per run
/usr/local/bin/animeenigma-git-autosync.sh    # run once, by hand
```
