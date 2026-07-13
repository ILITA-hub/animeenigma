# AnimeEnigma — Codex Operating Rules

Read this file first. It is the entry point for Codex work in this repository.

## Non-negotiable safety rules

1. `/data/animeenigma` is the shared `main` mirror, kept current by a fast-forward-only host routine. **Never edit, commit, switch branches, reset, stash, or rebase in this tree.** Create a worktree from fresh `origin/main` for every source change.
2. Treat `docker/.env`, `frontend/web/.env`, Kustomize secret env files, credentials, tokens, user reports, and production data as sensitive. Do not print, copy, commit, or add them to context files.
3. A live operation is not a substitute for a code fix. Use the incident lane only for a known, reversible mitigation; make source changes in a worktree and land them through the release lane.
4. Never set a feedback report to `resolved`. Codex may use the guarded helper for `new`, `in_progress`, `ai_done`, or `not_relevant` only when the task authorizes it.
5. Do not tear down, prune, modify, or reuse another agent's worktree. Existing worktrees are concurrent work, not disposable workspace.

## Commit attribution

For every Codex-created commit, retain the configured Git author and append exactly these trailers:

```
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: Codex <noreply@openai.com>
```

Do not add other co-authors unless the owner explicitly requests them. The repository's existing `bin/ae-land.sh` is shared Claude workflow infrastructure; do not alter it for Codex attribution. Append these trailers explicitly when making a Codex commit.

## Read the linked context before acting

- [Project context](.codex/CONTEXT.md): architecture, current source-of-truth map, and task routing.
- [Operations and incidents](.codex/OPERATIONS.md): production authority, verification, and rollback rules.
- [After-update lifecycle](.codex/AFTER_UPDATE.md): required worktree-to-production cycle.
- [Service impact map](.codex/SERVICE_MAP.md): current deployable services and path-to-impact rules.

## Default execution model

- Read/research: the shared base tree is safe to inspect.
- Code change: fresh worktree → smallest patch → targeted verification → review → commit/rebase/push.
- Release/deploy: only from a clean, current shared tree after the commit is on `origin/main`.
- Live incident: diagnose with the consumer-visible signal; mitigate only within the authority matrix in `.codex/OPERATIONS.md`; preserve evidence and follow through to verification.

Within repository guidance, prioritize this file, then the linked Codex files, then the most current code/script behavior over older prose documentation. System, developer, and direct user instructions remain higher priority.
