---
name: animeenigma-devops
description: Run AnimeEnigma's guarded Codex git and release workflow without ad-hoc git mutations. Use for any source change requiring a fresh worktree, explicit-path commit with Codex attribution, rebase and non-force push to main, changelog-conflict recovery, service deployment, or cleanup of a completed Codex worktree.
---

# AnimeEnigma DevOps

Read `/data/animeenigma/AGENTS.md` first. Use `bin/ae-codex-git`; never edit the shared mirror or invoke `bin/ae-land.sh` for Codex work.

## Workflow

1. Start from the shared repository:

   ```bash
   bin/ae-codex-git start SHORT-SLUG
   ```

   Continue in the printed `WORKTREE` path.

2. Edit only that worktree and run targeted verification.

3. Add a user-facing changelog entry when required, then land explicit paths:

   ```bash
   bin/ae-codex-git land "fix(scope): concise subject" path/to/file path/to/test
   ```

   The helper appends exactly the required Neymik/Codex trailers, fetches, rebases, and performs a non-force `HEAD:main` push.

4. On a rebase conflict, resolve files, then use:

   ```bash
   bin/ae-codex-git stage path/to/resolved-file
   bin/ae-codex-git continue
   ```

   For changelog-only conflicts:

   ```bash
   bin/ae-codex-git changelog-conflict fix "CHANGELOG MESSAGE"
   bin/ae-codex-git continue
   ```

5. Deploy only impacted services after the commit is on `origin/main`:

   ```bash
   bin/ae-deploy.sh catalog web
   ```

6. Verify the consumer-visible signal and clean only the completed worktree:

   ```bash
   /data/animeenigma/bin/ae-codex-git cleanup /tmp/ae-SLUG-STAMP
   ```

The helper refuses shared-tree mutation, non-`codex/` branches, dirty cleanup, non-ancestor cleanup, force pushes, implicit staging, and invalid conventional subjects.
