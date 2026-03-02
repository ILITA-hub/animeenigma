---
allowed-tools: Bash(*), Read(*), Glob(*), Grep(*), AskUserQuestion(*)
description: Auto-detect changed services from git, redeploy them, and commit
---

# CD — Continuous Deployment

Auto-detect uncommitted changes, redeploy affected services, and commit.

## Steps

### 1. Detect uncommitted changes

Find all files changed since the last commit (staged, unstaged, untracked):

```bash
(git diff --name-only HEAD 2>/dev/null; git diff --name-only --cached 2>/dev/null; git ls-files --others --exclude-standard 2>/dev/null) | sort -u
```

If no changes detected, report "Nothing to deploy" and stop.

### 2. Map changes to services

Apply these rules to the changed file paths:

| Path pattern | Action |
|---|---|
| `services/X/**` | Add service `X` to redeploy set |
| `libs/**` | Add ALL Go services: `auth catalog gateway player rooms scheduler streaming themes` |
| `frontend/web/**` | Add `web` to redeploy set |
| `docker/docker-compose.yml` | Warn: "docker-compose.yml changed — may need `make dev-down && make dev` instead of individual redeploys" |
| Anything else (docs, CLAUDE.md, Makefile, etc.) | Skip — no redeployment needed |

Deduplicate the service list.

### 3. Run lint checks

Run lint checks for the affected areas before deploying:

**If Go services or libs changed:**
```bash
cd /data/animeenigma && golangci-lint run ./libs/... ./services/... 2>&1 || true
```
If `golangci-lint` is not installed locally, skip Go lint (CI will catch it).

**If frontend changed:**
```bash
cd /data/animeenigma/frontend/web && bun lint 2>&1
```
Frontend lint must pass with 0 errors (warnings are OK). If there are errors, fix them before deploying.

If any lint check fails, stop and fix the issues before proceeding to deployment.

### 4. Show deployment plan and confirm

Present the list of services to redeploy and why, then ask the user to confirm before proceeding. Example:

> **Services to redeploy:**
> - `gateway` (changed: services/gateway/internal/service/proxy.go)
> - `auth` (changed: libs/authz/jwt.go triggers all services)
>
> Proceed?

### 5. Deploy

Run `make redeploy-<service>` for each service sequentially from the project root. For frontend, use `make redeploy-web`.

If any redeploy fails, stop and report the error. Do not continue to the next service.

### 6. Health check

After all deployments complete:

```bash
make health
```

### 7. Commit

After successful deployment and health check, commit all changes:

1. Stage all changed files: `git add -A`
2. Generate a concise commit message summarizing the changes
3. Include co-authors:
   ```
   Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
   Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
   Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
   ```
4. Create the commit

### 8. Report

Summarize:
- Which services were redeployed
- Health check results
- Commit hash and message
- Any warnings (docker-compose changes, failed deploys)
