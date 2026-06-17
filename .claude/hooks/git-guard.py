#!/usr/bin/env python3
"""
PreToolUse Bash guard: block destructive git ops in the SHARED MAIN worktree.

Why: /data/animeenigma is one mutable checkout shared by many concurrent agents.
`git reset --hard`, `git stash push/pop`, `git checkout <old> -- .`, switching
branches, or `git restore --source=<old>` run *there* strand the working tree at
an older state for everyone (the recurring "orphaned revert" / "stray branch"
clutter). Those operations are fine inside a per-agent `git worktree`; this guard
only fires when the effective target is the main worktree root.

Mechanism: reads the PreToolUse JSON on stdin, inspects the Bash command, and
exits 2 (deny, stderr shown to the agent) when a destructive op targets the main
tree. Override per-command with `AE_ALLOW_DESTRUCTIVE_GIT=1`. FAIL-OPEN: any
unexpected error exits 0 so a guard bug can never block normal commands.
"""
import sys, json, os, re, shlex, subprocess

MAIN_TREE = os.path.realpath("/data/animeenigma")
OVERRIDE = "AE_ALLOW_DESTRUCTIVE_GIT=1"


def resolve_toplevel(path):
    try:
        r = subprocess.run(["git", "-C", path, "rev-parse", "--show-toplevel"],
                           capture_output=True, text=True, timeout=5)
        if r.returncode == 0 and r.stdout.strip():
            return os.path.realpath(r.stdout.strip())
    except Exception:
        pass
    return None


def is_main_tree(target):
    top = resolve_toplevel(target)
    if top is not None:
        return top == MAIN_TREE
    try:
        return os.path.realpath(target) == MAIN_TREE
    except Exception:
        return False


def classify(tokens):
    """Return a human reason if this git token list is destructive, else None."""
    i = 0
    while i < len(tokens) and re.match(r'^[A-Za-z_][A-Za-z0-9_]*=', tokens[i]):
        i += 1  # skip leading inline env assignments
    if i >= len(tokens) or tokens[i] != "git":
        return None
    args = tokens[i + 1:]
    sub, rest, j = None, [], 0
    while j < len(args):  # skip git global opts, keep going to the subcommand
        a = args[j]
        if a in ("-C", "-c") and j + 1 < len(args):
            j += 2
            continue
        if a.startswith("-"):
            j += 1
            continue
        sub, rest = a, args[j + 1:]
        break
    if sub is None:
        return None
    flags = [t for t in rest if t.startswith("-")]
    nonflags = [t for t in rest if not t.startswith("-")]

    if sub == "reset" and "--hard" in rest:
        return "git reset --hard"
    if sub == "stash":
        verb = next((t for t in rest if not t.startswith("-")), "push")
        if verb in ("create", "show", "list"):
            return None  # non-mutating
        return f"git stash {verb}".strip()  # bare/push/pop/apply/save/drop/clear
    if sub == "switch":
        return "git switch (branch change)" if nonflags else None
    if sub == "checkout":
        if "--" in rest:  # pathspec form: safe unless restoring from a non-HEAD ref
            ref = [t for t in rest[:rest.index("--")] if not t.startswith("-")]
            if ref and ref[0] not in ("HEAD", "@"):
                return "git checkout <ref> -- (historical file restore)"
            return None
        if nonflags or any(f in ("-b", "-B") for f in flags):
            return "git checkout (branch/commit switch)"
        return None
    if sub == "restore":
        src = None
        for k, t in enumerate(rest):
            if t.startswith("--source="):
                src = t.split("=", 1)[1]
            elif t == "--source" and k + 1 < len(rest):
                src = rest[k + 1]
        if src is not None and src not in ("HEAD", "@"):
            return "git restore --source=<old>"
        return None
    return None


def target_dir(toks, cur):
    if "-C" in toks:
        k = toks.index("-C")
        if k + 1 < len(toks):
            tp = toks[k + 1]
            return tp if os.path.isabs(tp) else os.path.normpath(os.path.join(cur, tp))
    return cur


def main():
    p = json.load(sys.stdin)
    if p.get("tool_name") != "Bash":
        return 0
    cmd = (p.get("tool_input") or {}).get("command", "")
    if not cmd or "git" not in cmd:
        return 0
    if OVERRIDE in cmd:
        return 0
    cur = p.get("cwd") or os.getcwd()
    for seg in re.split(r'&&|\|\||;|\||\n', cmd):
        seg = seg.strip()
        if not seg:
            continue
        try:
            toks = shlex.split(seg)
        except Exception:
            toks = seg.split()
        if not toks:
            continue
        if toks[0] == "cd" and len(toks) >= 2:  # track cwd across the pipeline
            path = toks[1]
            cur = path if os.path.isabs(path) else os.path.normpath(os.path.join(cur, path))
            continue
        reason = classify(toks)
        if reason and is_main_tree(target_dir(toks, cur)):
            sys.stderr.write(
                f"BLOCKED by git-guard: `{reason}` in the SHARED MAIN worktree ({MAIN_TREE}).\n"
                f"This is what strands the tree at an old state for every agent.\n"
                f"Run it in a dedicated worktree instead:\n"
                f"  git worktree add /tmp/ae-fix origin/main && cd /tmp/ae-fix && <your git op>\n"
                f"If you genuinely mean to run it in the main tree, prefix with {OVERRIDE} .\n"
            )
            return 2
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception:
        sys.exit(0)  # FAIL-OPEN: never block normal commands on a guard bug
