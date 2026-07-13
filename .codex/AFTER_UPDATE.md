# Codex After-Update Lifecycle

This adapts the useful Claude after-update intent while retaining current service scope and explicit production authority.

## 0. Classify

- Docs/internal metadata: verify links/format; no deploy or user changelog.
- Code/UI: full release lane.
- No-source operational incident: use `OPERATIONS.md`; do not invent a release.
- Data/security/host change: explicit approval and rollback before mutation.

## 1. Isolate and assess

1. Fetch `origin` from the shared tree and create a unique worktree from fresh `origin/main`.
2. Edit only in that worktree; check for unrelated changes.
3. Determine impact through `SERVICE_MAP.md`, code consumers, API contracts, and deployment configuration.
4. For a report with a build SHA, inspect that build and then determine whether current `main` already fixes it.

## 2. Implement and simplify

Make the smallest complete change. Review the patch once for duplication, needless complexity, avoidable work, and violated abstractions; apply only task-scoped behavior-preserving cleanup. Routine changes do not require parallel agents.

## 3. Verify

- Go: affected module tests; relevant library consumers; lint/build proportional to risk.
- Frontend: `bin/ae-fe-verify.sh` for touched paths; locale lint/parity only for locale changes; real `bun run build` is type truth.
- API/generation: generate intentionally and review generated diff; verify clients and servers.
- Deploy/config: relevant static guards; never guess real secrets.

Fix failures in the worktree and re-run the failed gate. Do not land an unverified fix just to unblock a deploy.

## 4. Changelog

Only user-visible changes get an entry. Use the existing guarded helper for `frontend/web/changelog.full.json`; never hand-edit generated `public/changelog.json`. If the served changelog changes, include `web` in deployment assessment.

## 5. Land

Commit only explicit task paths with a conventional subject and the required Neymik/Codex co-author trailers from `AGENTS.md`. Do not use `bin/ae-land.sh` for Codex commits: it is shared Claude workflow infrastructure and appends a different attribution set. Fetch/rebase onto `origin/main`, resolve conflicts in the worktree, and fast-forward push—never force-push. On a push race, rebase and re-verify as needed; never use the shared tree to resolve it.

## 6. Deploy

1. Derive services from `SERVICE_MAP.md`, never the old fixed eight-service mapping.
2. Confirm Compose production or explicitly authorized Kubernetes environment.
3. Confirm the shared tree is clean and at the landed revision. Otherwise stop; never force-sync.
4. Use the repository’s serialized deployment path and deploy only justified services.
5. If deployment fails, retain the worktree and correct/revert from there.

## 7. Verify and close

Confirm workload/container state, then the original user/alert-facing signal—not only localhost liveness. Report behavior, checks, deployed services, changelog status, commit, target, residual risk, and skipped checks. Only then remove your own worktree.

## Stop conditions

Stop for direction on missing production authority, an unfixable hard gate, dirty/diverged shared tree, unapproved destructive data/secret/host/Kubernetes work, or any feedback transition to `resolved`.
