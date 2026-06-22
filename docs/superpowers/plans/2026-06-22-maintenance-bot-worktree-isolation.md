# Maintenance Bot Worktree Isolation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the maintenance bot execute every auto-fix inside a per-cycle throwaway git worktree (off fresh `origin/main`, flushed after each cycle), with the Go wrapper owning push + base-tree fast-forward + deploy — so the bot stops churning/wedging the shared base tree.

**Architecture:** Today `dispatcher.ExecuteFix` runs `claude -p` with `cmd.Dir = /data/animeenigma` and lets Claude commit/push/deploy in the base tree. New flow (behind `MAINT_WORKTREE_ISOLATION`): Go provisions a worktree → Claude edits/tests/**commits there only** → Go pushes it to `origin/main` (rebase-retry) → Go fast-forwards the base tree and runs `make redeploy-<target>` from it → Go flushes the worktree. Git mutation and deploy move OUT of the LLM's hands.

**Tech Stack:** Go 1.x (`services/maintenance`), `os/exec`, git worktrees, Claude Code CLI, systemd, Make.

## Global Constraints

- Go service layout per CLAUDE.md (`internal/{config,dispatcher,...}`); shared logger `github.com/ILITA-hub/animeenigma/libs/logger`.
- No new third-party deps; use stdlib `os/exec`, `path/filepath`, `os`.
- The base tree is `PROJECT_ROOT` (default `/data/animeenigma`); deploy MUST build from it on `main` (per `/animeenigma-after-update`).
- All new git mutation uses `git -C <dir>` (never rely on process CWD); worktree ops are git-guard-exempt.
- Feature-flagged: `MAINT_WORKTREE_ISOLATION` default **false** (old behavior unchanged when off).
- Tests: `go test ./...` from `services/maintenance`; no network/live-API in unit tests (use temp git repos + a local bare remote).
- Rollout: `make build-maintenance` + `systemctl restart animeenigma-maintenance` (host systemd, runs as root).

---

## File Structure

- `internal/config/config.go` — add `WorktreeIsolation bool`, `WorktreeBase string`.
- `internal/gitops/gitops.go` (new) — `Run`, `Provision`, `Cleanup`, `SweepStale`, `PushToMain`. One responsibility: git/worktree mechanics.
- `internal/deploy/deploy.go` (new) — `FastForwardAndRedeploy`. One responsibility: base-tree ff + `make redeploy`.
- `internal/dispatcher/claude.go` — `invoke`/`ExecuteFix` accept a `workdir`.
- `cmd/maintenance/main.go` — wire provision→fix→push→deploy→cleanup in `applyFix`; startup sweep.
- `.claude/maintenance-prompt.md` — worktree-aware, commit-only, CWD-relative.
- `.gitignore` + untrack `docs/issues/issues.json`.
- `.claude/commands/animeenigma-after-update.md` — push → rebase-retry loop.

---

### Task 1: Config — feature flag + worktree base

**Files:**
- Modify: `services/maintenance/internal/config/config.go`
- Test: `services/maintenance/internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.WorktreeIsolation bool`, `Config.WorktreeBase string` (default `/tmp/ae-maint`).

- [ ] **Step 1: Write the failing test**

Add to `config_test.go`:
```go
func TestLoad_WorktreeDefaults(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "x")
	t.Setenv("TELEGRAM_ADMIN_CHAT_ID", "1")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WorktreeIsolation {
		t.Errorf("WorktreeIsolation default = true, want false")
	}
	if cfg.WorktreeBase != "/tmp/ae-maint" {
		t.Errorf("WorktreeBase = %q, want /tmp/ae-maint", cfg.WorktreeBase)
	}
}

func TestLoad_WorktreeIsolationEnabled(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "x")
	t.Setenv("TELEGRAM_ADMIN_CHAT_ID", "1")
	t.Setenv("MAINT_WORKTREE_ISOLATION", "true")
	t.Setenv("MAINT_WORKTREE_BASE", "/tmp/custom")
	cfg, _ := Load()
	if !cfg.WorktreeIsolation {
		t.Error("WorktreeIsolation = false, want true")
	}
	if cfg.WorktreeBase != "/tmp/custom" {
		t.Errorf("WorktreeBase = %q, want /tmp/custom", cfg.WorktreeBase)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/maintenance && go test ./internal/config/ -run TestLoad_Worktree -v`
Expected: FAIL (compile error: `cfg.WorktreeIsolation` undefined).

- [ ] **Step 3: Add the fields + loading**

In `config.go`, add to the `Config` struct (after `TestMode bool`):
```go
	// WorktreeIsolation runs each auto-fix in a throwaway git worktree off
	// origin/main instead of mutating the shared base tree (PROJECT_ROOT).
	// Default false (legacy in-base behavior) for safe rollout.
	WorktreeIsolation bool
	// WorktreeBase is the parent dir for per-fix worktrees (one subdir per fix).
	WorktreeBase string
```
In `Load()`'s returned `&Config{...}` (after `TestMode: ...`):
```go
		WorktreeIsolation: getEnvBool("MAINT_WORKTREE_ISOLATION", false),
		WorktreeBase:      getEnv("MAINT_WORKTREE_BASE", "/tmp/ae-maint"),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/maintenance && go test ./internal/config/ -run TestLoad_Worktree -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/maintenance/internal/config/config.go services/maintenance/internal/config/config_test.go
git commit -m "feat(maintenance): add MAINT_WORKTREE_ISOLATION + MAINT_WORKTREE_BASE config"
```

---

### Task 2: gitops package — Run + worktree lifecycle

**Files:**
- Create: `services/maintenance/internal/gitops/gitops.go`
- Test: `services/maintenance/internal/gitops/gitops_test.go`

**Interfaces:**
- Produces:
  - `func Run(ctx context.Context, dir string, args ...string) (string, error)` — runs `git -C dir <args>`, returns trimmed stdout.
  - `type Worktree struct { Dir, Branch string }`
  - `func Provision(ctx context.Context, base, parentDir, id string) (*Worktree, error)` — fetch origin main in `base`, `worktree add -b maint/<id> <parentDir>/<id> origin/main`.
  - `func (w *Worktree) Cleanup(ctx context.Context, base string) error` — `worktree remove --force <dir>` + `worktree prune` + `branch -D`.
  - `func SweepStale(ctx context.Context, base, parentDir string) error` — prune + remove leftover `<parentDir>/*` dirs.

- [ ] **Step 1: Write the failing test**

`gitops_test.go` (uses a temp repo + bare remote so no network):
```go
package gitops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initRepoWithRemote makes base/ tracking a bare origin with one commit on main.
func initRepoWithRemote(t *testing.T) (base string) {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	base = filepath.Join(root, "base")
	run := func(dir string, args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := exec.Command("git", "init", "--bare", "-b", "main", bare).Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	run(base, "init", "-b", "main")
	run(base, "remote", "add", "origin", bare)
	if err := os.WriteFile(filepath.Join(base, "f.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(base, "add", ".")
	run(base, "commit", "-m", "init")
	run(base, "push", "-u", "origin", "main")
	return base
}

func TestProvisionAndCleanup(t *testing.T) {
	ctx := context.Background()
	base := initRepoWithRemote(t)
	parent := filepath.Join(t.TempDir(), "wts")

	wt, err := Provision(ctx, base, parent, "fix1")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if _, err := os.Stat(wt.Dir); err != nil {
		t.Fatalf("worktree dir missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt.Dir, "f.txt")); err != nil {
		t.Fatalf("worktree not checked out: %v", err)
	}
	if err := wt.Cleanup(ctx, base); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(wt.Dir); !os.IsNotExist(err) {
		t.Fatalf("worktree dir still present after cleanup")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/maintenance && go test ./internal/gitops/ -v`
Expected: FAIL (package/functions not defined).

- [ ] **Step 3: Implement `gitops.go`**

```go
// Package gitops centralizes the maintenance bot's git/worktree mechanics so
// fix execution never mutates the shared base tree (PROJECT_ROOT). Everything
// uses `git -C <dir>` — process CWD is irrelevant.
package gitops

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Run executes `git -C dir <args>` and returns trimmed stdout (or an error
// carrying combined output).
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Worktree is a provisioned per-fix checkout off origin/main.
type Worktree struct {
	Dir    string
	Branch string
}

// Provision fetches origin/main in base and adds a fresh worktree at
// parentDir/id on a new branch maint/<id> pointing at origin/main.
func Provision(ctx context.Context, base, parentDir, id string) (*Worktree, error) {
	if _, err := Run(ctx, base, "fetch", "origin", "main"); err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return nil, err
	}
	dir := filepath.Join(parentDir, id)
	branch := "maint/" + id
	// Best-effort: drop a stale worktree/branch from a prior crashed cycle.
	_, _ = Run(ctx, base, "worktree", "remove", "--force", dir)
	_, _ = Run(ctx, base, "branch", "-D", branch)
	if _, err := Run(ctx, base, "worktree", "add", "-b", branch, dir, "origin/main"); err != nil {
		return nil, fmt.Errorf("worktree add: %w", err)
	}
	return &Worktree{Dir: dir, Branch: branch}, nil
}

// Cleanup force-removes the worktree, prunes, and deletes the branch. Safe to
// call multiple times.
func (w *Worktree) Cleanup(ctx context.Context, base string) error {
	if _, err := Run(ctx, base, "worktree", "remove", "--force", w.Dir); err != nil {
		// fall through to prune even if remove failed
		_, _ = Run(ctx, base, "worktree", "prune")
		_, _ = Run(ctx, base, "branch", "-D", w.Branch)
		return err
	}
	_, _ = Run(ctx, base, "worktree", "prune")
	_, _ = Run(ctx, base, "branch", "-D", w.Branch)
	return nil
}

// SweepStale prunes dangling worktree refs and removes any leftover dirs under
// parentDir (from prior crashes). Call once at startup.
func SweepStale(ctx context.Context, base, parentDir string) error {
	_, _ = Run(ctx, base, "worktree", "prune")
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		dir := filepath.Join(parentDir, e.Name())
		_, _ = Run(ctx, base, "worktree", "remove", "--force", dir)
		_ = os.RemoveAll(dir)
	}
	_, _ = Run(ctx, base, "worktree", "prune")
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/maintenance && go test ./internal/gitops/ -v`
Expected: PASS (`TestProvisionAndCleanup`).

- [ ] **Step 5: Commit**

```bash
git add services/maintenance/internal/gitops/
git commit -m "feat(maintenance): gitops worktree lifecycle (Provision/Cleanup/SweepStale)"
```

---

### Task 3: gitops — PushToMain (rebase-retry)

**Files:**
- Modify: `services/maintenance/internal/gitops/gitops.go`
- Test: `services/maintenance/internal/gitops/gitops_push_test.go`

**Interfaces:**
- Consumes: `Run`, `Worktree` (Task 2).
- Produces: `func PushToMain(ctx context.Context, dir string, attempts int) (string, error)` — in worktree `dir`: loop {`fetch origin main` → `rebase origin/main` → `push origin HEAD:main`}; returns the pushed short SHA.

- [ ] **Step 1: Write the failing test**

`gitops_push_test.go` (reuses `initRepoWithRemote`; makes a commit in a worktree, pushes, asserts origin/main advanced):
```go
package gitops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPushToMain(t *testing.T) {
	ctx := context.Background()
	base := initRepoWithRemote(t)
	parent := filepath.Join(t.TempDir(), "wts")
	wt, err := Provision(ctx, base, parent, "push1")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	t.Cleanup(func() { _ = wt.Cleanup(ctx, base) })

	// commit a change in the worktree
	if err := os.WriteFile(filepath.Join(wt.Dir, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", wt.Dir}, args...)...)
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	commit("add", ".")
	commit("commit", "-m", "add new.txt")

	sha, err := PushToMain(ctx, wt.Dir, 3)
	if err != nil {
		t.Fatalf("PushToMain: %v", err)
	}
	if sha == "" {
		t.Fatal("empty sha")
	}
	// origin/main now contains new.txt
	got, err := Run(ctx, base, "cat-file", "-e", "origin/main:new.txt")
	_ = got
	if err == nil {
		// fetch first so base sees the new origin/main
	}
	if _, err := Run(ctx, base, "fetch", "origin", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, base, "cat-file", "-e", "origin/main:new.txt"); err != nil {
		t.Fatalf("new.txt not on origin/main: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/maintenance && go test ./internal/gitops/ -run TestPushToMain -v`
Expected: FAIL (`PushToMain` undefined).

- [ ] **Step 3: Implement `PushToMain`**

Append to `gitops.go`:
```go
// PushToMain pushes the worktree's HEAD to origin/main, rebasing onto the
// latest origin/main and retrying on a push race. Returns the pushed short SHA.
func PushToMain(ctx context.Context, dir string, attempts int) (string, error) {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if _, err := Run(ctx, dir, "fetch", "origin", "main"); err != nil {
			lastErr = err
			continue
		}
		if _, err := Run(ctx, dir, "rebase", "origin/main"); err != nil {
			// abort a half-applied rebase before the next attempt
			_, _ = Run(ctx, dir, "rebase", "--abort")
			lastErr = err
			continue
		}
		if _, err := Run(ctx, dir, "push", "origin", "HEAD:main"); err != nil {
			lastErr = err
			continue
		}
		sha, err := Run(ctx, dir, "rev-parse", "--short", "HEAD")
		if err != nil {
			return "", err
		}
		return sha, nil
	}
	return "", fmt.Errorf("push to main failed after %d attempts: %w", attempts, lastErr)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/maintenance && go test ./internal/gitops/ -v`
Expected: PASS (both worktree + push tests).

- [ ] **Step 5: Commit**

```bash
git add services/maintenance/internal/gitops/
git commit -m "feat(maintenance): gitops PushToMain with rebase-retry"
```

---

### Task 4: deploy package — fast-forward base tree + redeploy

**Files:**
- Create: `services/maintenance/internal/deploy/deploy.go`
- Test: `services/maintenance/internal/deploy/deploy_test.go`

**Interfaces:**
- Consumes: `gitops.Run`.
- Produces: `func FastForwardAndRedeploy(ctx context.Context, base, target string, log *logger.Logger) error` — `git -C base merge --ff-only origin/main` then `make -C base redeploy-<target>` (skip make when `target == ""`). Uses `runMake` (a package var so tests can stub it).

- [ ] **Step 1: Write the failing test**

`deploy_test.go` (stub the make runner; use a real temp repo for the ff):
```go
package deploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

func TestFastForwardAndRedeploy_RunsMakeTarget(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	// minimal git repo so the ff step is a no-op success (already up to date)
	mustGit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", base}, args...)...)
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustGit("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(base, "x"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit("add", ".")
	mustGit("commit", "-m", "i")
	mustGit("update-ref", "refs/remotes/origin/main", "HEAD") // ff is a no-op

	var gotTarget string
	runMake = func(_ context.Context, dir, target string) error {
		gotTarget = target
		return nil
	}
	t.Cleanup(func() { runMake = defaultRunMake })

	if err := FastForwardAndRedeploy(ctx, base, "gateway", logger.NewNop()); err != nil {
		t.Fatalf("FastForwardAndRedeploy: %v", err)
	}
	if gotTarget != "gateway" {
		t.Errorf("make target = %q, want gateway", gotTarget)
	}
}
```
> If `logger.NewNop()` does not exist, substitute the project's standard test logger constructor (check `libs/logger`); the dispatcher tests already construct one — mirror that.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/maintenance && go test ./internal/deploy/ -v`
Expected: FAIL (package not defined).

- [ ] **Step 3: Implement `deploy.go`**

```go
// Package deploy fast-forwards the base tree to origin/main and rebuilds the
// affected service from it. Deploy MUST build from the base tree (PROJECT_ROOT)
// on main — `make redeploy-*` reads the shared tree, not a worktree.
package deploy

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/gitops"
)

// runMake is a package var so tests can stub the (slow, side-effectful) make call.
var runMake = defaultRunMake

func defaultRunMake(ctx context.Context, dir, target string) error {
	cmd := exec.CommandContext(ctx, "make", "-C", dir, "redeploy-"+target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("make redeploy-%s: %w (%s)", target, err, string(out))
	}
	return nil
}

// FastForwardAndRedeploy brings the base tree to origin/main (ff-only) and runs
// `make redeploy-<target>` from it. A blank target skips the rebuild (e.g.
// restart-only fixes the dispatcher handled differently).
func FastForwardAndRedeploy(ctx context.Context, base, target string, log *logger.Logger) error {
	if _, err := gitops.Run(ctx, base, "fetch", "origin", "main"); err != nil {
		return fmt.Errorf("deploy fetch: %w", err)
	}
	if _, err := gitops.Run(ctx, base, "merge", "--ff-only", "origin/main"); err != nil {
		return fmt.Errorf("base tree ff (is it dirty/diverged?): %w", err)
	}
	if target == "" {
		log.Infow("deploy: ff-only, no redeploy target")
		return nil
	}
	log.Infow("deploy: redeploying", "target", target)
	return runMake(ctx, base, target)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/maintenance && go test ./internal/deploy/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/maintenance/internal/deploy/
git commit -m "feat(maintenance): deploy pkg — ff base tree + make redeploy-<target>"
```

---

### Task 5: Dispatcher — run fix in a given workdir

**Files:**
- Modify: `services/maintenance/internal/dispatcher/claude.go`
- Test: `services/maintenance/internal/dispatcher/autofix_test.go` (extend)

**Interfaces:**
- Consumes: existing `Dispatcher`.
- Produces: `func (d *Dispatcher) ExecuteFixIn(ctx context.Context, fix domain.PendingFix, workdir string) (*domain.AnalysisResult, error)`. Existing `ExecuteFix` delegates with `workdir = d.projectRoot` (back-compat). `invoke` gains a `workdir` param used for `cmd.Dir`.

- [ ] **Step 1: Write the failing test**

Add to `autofix_test.go` a check that `invoke` honors an explicit workdir. Since `invoke` shells out to the real `claude` binary, assert at the wiring level instead: verify `ExecuteFixIn` exists and that with a bogus claude path it still uses `workdir` (it will fail to start, but cmd.Dir is set before Start). Minimal compile-level guard:
```go
func TestExecuteFixIn_Exists(t *testing.T) {
	d := New("/nonexistent/claude", t.TempDir(), "", "sonnet", "opus", 5, testLogger(t))
	_, err := d.ExecuteFixIn(context.Background(), domain.PendingFix{}, t.TempDir())
	if err == nil {
		t.Fatal("expected error from missing claude binary")
	}
}
```
> Use the same `testLogger` helper the existing dispatcher tests use; if none, construct the project's standard logger.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/maintenance && go test ./internal/dispatcher/ -run TestExecuteFixIn -v`
Expected: FAIL (`ExecuteFixIn` undefined).

- [ ] **Step 3: Thread `workdir` through `invoke`**

In `claude.go`: change `invoke` signature to `func (d *Dispatcher) invoke(ctx context.Context, prompt, model, workdir string, extraAllowedTools []string)` and set `cmd.Dir = workdir`. Update the three call sites:
- `Analyze`: `return d.invoke(ctx, prompt, d.model, d.projectRoot, nil)`
- `ExecuteFix`: keep as a thin wrapper:
```go
func (d *Dispatcher) ExecuteFix(ctx context.Context, fix domain.PendingFix) (*domain.AnalysisResult, error) {
	return d.ExecuteFixIn(ctx, fix, d.projectRoot)
}

// ExecuteFixIn runs the fix with Claude's CWD set to workdir (a worktree when
// isolation is enabled). Claude edits/tests/commits there; it does NOT push or
// deploy — the caller (Go) owns push + base-tree ff + redeploy.
func (d *Dispatcher) ExecuteFixIn(ctx context.Context, fix domain.PendingFix, workdir string) (*domain.AnalysisResult, error) {
	prompt := d.buildFixPrompt(fix)
	model := d.model
	if fix.FixPlan.Type == domain.FixCodeFix {
		model = d.codeModel
	}
	return d.invoke(ctx, prompt, model, workdir, allowedFixTools)
}
```
- Any report-analysis call site found via `grep -n "d.invoke(" claude.go`: pass `d.projectRoot` as the new `workdir` arg.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/maintenance && go test ./internal/dispatcher/ -v`
Expected: PASS (new test + existing dispatcher tests still green).

- [ ] **Step 5: Commit**

```bash
git add services/maintenance/internal/dispatcher/claude.go services/maintenance/internal/dispatcher/autofix_test.go
git commit -m "feat(maintenance): dispatcher ExecuteFixIn(workdir) for worktree-isolated fixes"
```

---

### Task 6: Wire the isolated lifecycle into applyFix

**Files:**
- Modify: `services/maintenance/cmd/maintenance/main.go` (`applyFix`, ~line 1201; startup near `dispatcher.New` ~line 101)

**Interfaces:**
- Consumes: `gitops.Provision/Cleanup/PushToMain/SweepStale`, `deploy.FastForwardAndRedeploy`, `cfg.WorktreeIsolation`, `cfg.WorktreeBase`, `cfg.Claude.ProjectRoot`.

- [ ] **Step 1: Add a fix-id helper + startup sweep**

Near the top of `main.go` add imports for the two new packages. After config load / before the poller starts (near `dispatcher.New`, ~line 101), when isolation is on, sweep stale worktrees:
```go
if cfg.WorktreeIsolation {
	if err := gitops.SweepStale(context.Background(), cfg.Claude.ProjectRoot, cfg.WorktreeBase); err != nil {
		log.Warnw("worktree startup sweep failed", "error", err)
	}
}
```

- [ ] **Step 2: Branch `applyFix` on the flag**

In `applyFix`, replace the single `ExecuteFix` call (currently inside `runInterruptible` at ~1207-1209) with isolation-aware logic. When `s.cfg.WorktreeIsolation` is true:
```go
var result *domain.AnalysisResult
var err error
if s.cfg.WorktreeIsolation {
	wt, perr := gitops.Provision(ctx, s.cfg.Claude.ProjectRoot, s.cfg.WorktreeBase, sanitizeID(issueID))
	if perr != nil {
		log.Errorw("worktree provision failed", "issue_id", issueID, "error", perr)
		reply(fmt.Sprintf("<b>❌ Fix failed</b> (%s)\nworktree provision: %s", approver, truncateForTelegram(perr.Error())))
		s.fb.TrySetStatus(fix.FeedbackID, feedback.StatusInProgress)
		return
	}
	defer func() {
		if cerr := wt.Cleanup(context.Background(), s.cfg.Claude.ProjectRoot); cerr != nil {
			log.Warnw("worktree cleanup failed", "issue_id", issueID, "dir", wt.Dir, "error", cerr)
		}
	}()
	result, err = s.runInterruptible(ctx, replyToID, "Applying fix "+issueID, func(c context.Context) (*domain.AnalysisResult, error) {
		return s.disp.ExecuteFixIn(c, fix, wt.Dir)
	})
	if err == nil {
		if _, perr := gitops.PushToMain(ctx, wt.Dir, 5); perr != nil {
			err = fmt.Errorf("push to main: %w", perr)
		} else if derr := deploy.FastForwardAndRedeploy(ctx, s.cfg.Claude.ProjectRoot, deployTarget(fix), log); derr != nil {
			err = fmt.Errorf("deploy: %w", derr)
		}
	}
} else {
	result, err = s.runInterruptible(ctx, replyToID, "Applying fix "+issueID, func(c context.Context) (*domain.AnalysisResult, error) {
		return s.disp.ExecuteFix(c, fix)
	})
}
```
Keep the existing `elapsed`, error-handling, and success blocks below unchanged (they consume `result, err`).

- [ ] **Step 3: Add the two small helpers**

At the bottom of `main.go`:
```go
// sanitizeID makes an issue ID safe as a path/branch segment.
func sanitizeID(id string) string {
	r := strings.NewReplacer("/", "-", " ", "-", ":", "-")
	s := r.Replace(strings.TrimSpace(id))
	if s == "" {
		s = "fix"
	}
	return s
}

// deployTarget maps a fix plan to a `make redeploy-<target>` service, or "" to
// skip the rebuild (restart/docker_pull/retry_job handle their own side effects
// inside the fix and need no source rebuild).
func deployTarget(fix domain.PendingFix) string {
	switch fix.FixPlan.Type {
	case domain.FixCodeFix, domain.FixRedeploy:
		return fix.FixPlan.Target
	default:
		return ""
	}
}
```

- [ ] **Step 4: Build + run the maintenance test suite**

Run: `cd services/maintenance && go build ./... && go test ./...`
Expected: PASS (all packages compile and tests green).

- [ ] **Step 5: Commit**

```bash
git add services/maintenance/cmd/maintenance/main.go
git commit -m "feat(maintenance): isolate fixes in per-cycle worktree; Go owns push+deploy"
```

---

### Task 7: Rewrite the fix prompt to be worktree-aware

**Files:**
- Modify: `.claude/maintenance-prompt.md`

**Interfaces:** none (LLM instructions).

- [ ] **Step 1: Update the apply-path guidance**

Replace the `/animeenigma-after-update`-centric apply instructions (around lines 96-100 and the Auto-Edit Selector Workflow git steps ~280-321) with worktree-aware, commit-only guidance. Key edits:
- State up front: *"Your working directory is an isolated git worktree off the latest `origin/main`. Make all edits here. Do NOT `cd /data/animeenigma`, do NOT push, and do NOT deploy — after you commit, the maintenance service pushes your commit to `main` and redeploys from the base tree."*
- Replace absolute `cd /data/animeenigma/services/scraper && ...` with CWD-relative `cd services/scraper && ...` (or no `cd`).
- Replace the `git push` / `/animeenigma-after-update` steps with: *"Stage and commit your change with a conventional-commit message + the standard co-authors. Stop after the commit."*
- Replace rollback guidance `git checkout HEAD -- <file>` with: *"on any failure, leave the worktree uncommitted or `git restore <file>` and report failure — the worktree is discarded, so no cleanup is needed."*
- Remove `Skill` reliance for deploy; keep `Bash(go test:*)`, `Bash(bunx:*)` etc. for in-worktree verification.

- [ ] **Step 2: Tighten the allowlist (claude.go) to match commit-only**

In `dispatcher/claude.go` `allowedFixTools`, remove `Bash(git push:*)`, `Bash(git checkout:*)`, `Bash(git revert:*)`, and `Skill` (push/deploy now belong to Go). Keep `git add/commit/diff/status/log`, `Edit`, `Write`, `Bash(go ...)`, `Bash(bun*)`, `Bash(make:*)` only if still needed for build-checks (keep `go build`/`go test`; drop `make:*` since deploy is Go-side). Update `autofix_test.go` if it asserts on the tool list.

- [ ] **Step 3: Verify prompt symbol tests still pass**

Run: `cd services/maintenance && go test ./internal/classifier/ -run Prompt -v`
Expected: PASS (the `maintenance_prompt_symbols_test.go` guard).

- [ ] **Step 4: Commit**

```bash
git add .claude/maintenance-prompt.md services/maintenance/internal/dispatcher/claude.go services/maintenance/internal/dispatcher/autofix_test.go
git commit -m "feat(maintenance): worktree-aware commit-only fix prompt + tightened allowlist"
```

---

### Task 8: Stop tracking docs/issues/issues.json (decision I-A)

**Files:**
- Modify: `.gitignore`
- Untrack: `docs/issues/issues.json`

- [ ] **Step 1: Untrack + ignore**

```bash
git rm --cached docs/issues/issues.json
```
Add to `.gitignore` (next to the maintenance-state.json rule):
```
# Maintenance bot: issue log (live-written by the maintenance service; host-local
# runtime state — tracking it churns the shared base tree). Decision I-A, 2026-06-22.
docs/issues/issues.json
```

- [ ] **Step 2: Verify it's ignored**

Run: `git check-ignore docs/issues/issues.json`
Expected: prints `docs/issues/issues.json`.

- [ ] **Step 3: Commit**

```bash
git add .gitignore
git commit -m "chore(git): stop tracking docs/issues/issues.json (bot runtime state, I-A)"
```

> **Host transition note (post-merge, like the maintenance-state.json cross):** after this lands on origin/main, the base tree must cross the tracked→untracked transition without losing the live file: `cp -a docs/issues/issues.json /tmp/issues.bak && rm -f docs/issues/issues.json && git -C /data/animeenigma merge --ff-only origin/main && cp -a /tmp/issues.bak docs/issues/issues.json`. Documented in the rollout section.

---

### Task 9: after-update push → rebase-retry

**Files:**
- Modify: `.claude/commands/animeenigma-after-update.md` (~line 150)

- [ ] **Step 1: Replace the bare push step**

Change step 5 `Push: git push` to the rebase-retry loop from `docs/git-workflow.md`:
```bash
for i in 1 2 3 4 5; do
  git fetch origin main && git rebase origin/main && git push origin HEAD:main && break
  echo "push race — retry ($i)"; sleep 2
done
```

- [ ] **Step 2: Commit**

```bash
git add .claude/commands/animeenigma-after-update.md
git commit -m "docs(after-update): push via fetch-rebase-push retry loop"
```

---

## Self-Review

**Spec coverage:** Goals 1-2 (isolation + flush) → Tasks 2,6; deploy-from-base (Goal 3) → Task 4,6; clean mirror (Goal 4) → Tasks 6,8; no capability loss (Goal 5) → Tasks 5,6,7. Prompt alignment → Task 7. issues.json I-A → Task 8. after-update → Task 9. Feature flag/rollout → Tasks 1,6. All spec sections mapped.

**Placeholder scan:** No TBD/TODO; every code step has real code. The two "if helper X doesn't exist, mirror the existing test logger" notes are concrete fallbacks, not placeholders (the executor confirms the project's logger constructor in `libs/logger`).

**Type consistency:** `gitops.Run/Provision/Cleanup/PushToMain/SweepStale`, `Worktree{Dir,Branch}`, `deploy.FastForwardAndRedeploy(ctx,base,target,log)`, `dispatcher.ExecuteFixIn(ctx,fix,workdir)`, `cfg.WorktreeIsolation/WorktreeBase`, helpers `sanitizeID`/`deployTarget` — names consistent across Tasks 2-6.

## Rollout

1. Implement Tasks 1-9 in a worktree; `make build-maintenance` (host-native binary).
2. Commit the rebuilt `bin/maintenance` (binary-in-git deploy model is unchanged) and push.
3. Cross the issues.json transition on the host (Task 8 note).
4. Keep `MAINT_WORKTREE_ISOLATION=false`; force one low-risk fix and confirm legacy path still works.
5. Set `MAINT_WORKTREE_ISOLATION=true` in `docker/maintenance.env`; `systemctl restart animeenigma-maintenance`.
6. Trigger one low-risk fix; verify: base tree `git status` clean throughout, commit on `origin/main`, deploy succeeded + `make health` green, worktree flushed, `/var/log/animeenigma-git-sync.log` shows no `CONFLICTED`/`DIVERGED`.
7. Revert plan: flip the flag back to `false` + restart.
