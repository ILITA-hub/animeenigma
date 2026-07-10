# Host automation

Host-level automation for the `/data/animeenigma` production box. Each unit's
source of truth lives here in `infra/host/` and is **installed** to the host
paths below (the installed copies are what actually run).

## 1. git auto-sync

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
- the tree has **diverged** (local commits not on `origin/main`) — `skip: DIVERGED`
- the tree is in a **conflict state** (unmerged index entries from a stranded merge/rebase/autostash, often with no `MERGE_HEAD`) — `skip: CONFLICTED`
- a merge/rebase/cherry-pick is **in progress** — `skip: IN-PROGRESS`
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

---

## 2. daily provider-recovery operator

A `systemd` **timer** that runs one autonomous headless Claude Code session per
day. The session adopts a single unhealthy EN scraper provider, diagnoses the
root cause, attempts a real recovery (an honest `probe-result` verdict and/or a
small worktree fix + `/animeenigma-after-update`), verifies it end-to-end,
appends to [`docs/issues/provider-recovery-log.md`](../../docs/issues/provider-recovery-log.md),
and posts one report to the Telegram admin chat. The canonical task prompt is
`provider-recovery-prompt.md`.

**Full autonomy (authorized by the project owner):** the session runs with
`--dangerously-skip-permissions`, so it can edit code in worktrees, run
`make redeploy-*`, and `git push` to `main` with no human in the loop. The
committed `git-guard` + `ds-lint` hooks in `.claude/` still apply. `IS_SANDBOX=1`
in the unit lets the flag run as root. Stop it any time with
`systemctl disable --now animeenigma-provider-recovery.timer`.

### Files

| Repo (source of truth)                              | Installed to                                                        |
|-----------------------------------------------------|---------------------------------------------------------------------|
| `infra/host/animeenigma-provider-recovery.sh`       | `/usr/local/bin/animeenigma-provider-recovery.sh` (mode 755)        |
| `infra/host/provider-recovery-prompt.md`            | `/usr/local/share/animeenigma/provider-recovery-prompt.md` (mode 644) |
| `infra/host/animeenigma-provider-recovery.service`  | `/etc/systemd/system/animeenigma-provider-recovery.service` (mode 644) |
| `infra/host/animeenigma-provider-recovery.timer`    | `/etc/systemd/system/animeenigma-provider-recovery.timer` (mode 644)   |

### Install / update

```bash
install -m 755 infra/host/animeenigma-provider-recovery.sh /usr/local/bin/animeenigma-provider-recovery.sh
install -d -m 755 /usr/local/share/animeenigma
install -m 644 infra/host/provider-recovery-prompt.md /usr/local/share/animeenigma/provider-recovery-prompt.md
install -m 644 infra/host/animeenigma-provider-recovery.service /etc/systemd/system/animeenigma-provider-recovery.service
install -m 644 infra/host/animeenigma-provider-recovery.timer   /etc/systemd/system/animeenigma-provider-recovery.timer
systemctl daemon-reload
systemctl enable --now animeenigma-provider-recovery.timer
```

### Configure (optional, host-only)

The model defaults to `sonnet`. Override per-host without editing the unit:

```bash
echo 'RECOVERY_MODEL=opus' >> /etc/animeenigma/provider-recovery.env   # mkdir -p /etc/animeenigma first
systemctl restart animeenigma-provider-recovery.timer
```

Other knobs (env or `/etc/animeenigma/provider-recovery.env`): `RECOVERY_MODEL`,
`RECOVERY_LOG`, `RECOVERY_PROMPT_FILE`, `CLAUDE_BIN`, `ANIMEENIGMA_REPO`.

### Inspect / run

```bash
systemctl list-timers animeenigma-provider-recovery.timer    # next scheduled run
/usr/local/bin/animeenigma-provider-recovery.sh --check      # prerequisites only, NO agent run
systemctl start animeenigma-provider-recovery.service        # trigger a real run now
journalctl -u animeenigma-provider-recovery.service -f       # follow a run
tail -f /var/log/animeenigma-provider-recovery.log           # run log (START/END + checks)
```

> **Why a host timer and not a cloud routine?** The operator needs production-only
> surfaces a cloud sandbox can't reach: the Docker-network-only catalog internal
> endpoints (`localhost:8081/internal/*`), the git-ignored Telegram secrets in
> `docker/.env`, `make logs-scraper`, and the live containers. Like the
> maintenance bot, it must run on the box.

---

## 3. Walpurgis no Kaiten release watcher

A weekly cron that reminds the owner (one Telegram admin-chat message per
signal, via the **maintenance** bot creds in `docker/maintenance.env`) to
download «Madoka Magica: Walpurgisnacht Rising» (shiki/MAL 48820) once it is
actually obtainable. Two one-shot signals, deduplicated through
`/var/lib/animeenigma/walpurgis-watch.state`:

1. **premiere** — catalog status flips off `announced` OR the JP theatrical
   date (2026-08-28) passes. FYI only (torrents at that point are camrips).
2. **torrent** — library search (`/api/library/search`, Jackett→Nyaa/AnimeTosho)
   surfaces a non-camrip ≥1080p (or ≥2 GiB) release. Actionable: enqueue the
   download (movie ⇒ set `episode=1` on the `library_jobs` row — the filename
   carries no episode number for the detector).

After both signals have fired the script exits immediately; the cron entry can
stay installed forever. Requested by the owner 2026-07-03.

### Files

| Repo (source of truth)                        | Installed to                                              |
|-----------------------------------------------|------------------------------------------------------------|
| `infra/host/animeenigma-walpurgis-watch.sh`   | `/usr/local/bin/animeenigma-walpurgis-watch.sh` (mode 755) |
| `infra/host/animeenigma-walpurgis-watch.cron` | `/etc/cron.d/animeenigma-walpurgis-watch` (mode 644)       |

### Install / update

```bash
install -m 755 infra/host/animeenigma-walpurgis-watch.sh   /usr/local/bin/animeenigma-walpurgis-watch.sh
install -m 644 infra/host/animeenigma-walpurgis-watch.cron /etc/cron.d/animeenigma-walpurgis-watch
```

### Inspect

```bash
tail /var/log/animeenigma-walpurgis-watch.log      # one line per fired signal
cat /var/lib/animeenigma/walpurgis-watch.state     # which signals already notified
/usr/local/bin/animeenigma-walpurgis-watch.sh      # run once, by hand (idempotent)
```

---

## 4. maintenance-gate helper (shared)

A sourceable bash helper so host automations (git auto-sync, provider-recovery,
docker prune, and future routines) can check policy-service's per-routine
maintenance gate without each re-implementing the `curl`+`jq` read. Same
`(policy,health)`-style gate that backs `libs/maintenancegate` on the Go side,
reached at its host-published address (`127.0.0.1:8098`).

**FAIL-OPEN by design:** any error — policy-service unreachable, non-200,
unparseable body — is treated as "enabled", so a policy-service outage can
never silently pause a host routine. The gate can only *stop* a routine when
it explicitly answers `enabled:false`.

### Interface

```bash
# shellcheck source=infra/host/animeenigma-maint-gate.sh
source /usr/local/lib/animeenigma/maint-gate.sh

maint_gate_enabled <routine_id>        # return 0 = run, 1 = gate says stop
maint_gate_setting <routine_id> <key>  # prints a settings value (empty on any miss)
maint_status <routine_id> <ok:0|1> <summary>  # fire-and-forget status POST; never fails the caller
```

`MAINT_POLICY_BASE` (default `http://localhost:8098`) overrides the policy-service
address, e.g. for local testing.

### Files

| Repo (source of truth)                    | Installed to                                       |
|--------------------------------------------|-----------------------------------------------------|
| `infra/host/animeenigma-maint-gate.sh`     | `/usr/local/lib/animeenigma/maint-gate.sh` (mode 644) |

### Install / update

```bash
install -d -m 755 /usr/local/lib/animeenigma
install -m 644 infra/host/animeenigma-maint-gate.sh /usr/local/lib/animeenigma/maint-gate.sh
```

### Use from a script

```bash
# shellcheck source=/dev/null
source /usr/local/lib/animeenigma/maint-gate.sh

if ! maint_gate_enabled git_autosync; then
  echo "skip: gated by policy-service"; exit 0
fi

# ... do the work ...

maint_status git_autosync 0 "fast-forwarded to abc1234"
```

### Manual smoke (host-access only)

```bash
source /usr/local/lib/animeenigma/maint-gate.sh
maint_gate_enabled git_autosync && echo RUN
maint_gate_setting disk_prune high_water_pct     # -> 80
```
