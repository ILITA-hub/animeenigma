"""Dependency-free combined-RSS sampler for the Camoufox/Firefox process tree.

camoufox spawns Firefox (and its content/GPU processes) as CHILDREN of this
python process, so the RAM budget is the RSS of os.getpid() + every descendant.
We read /proc directly (no psutil): /proc/<pid>/statm field 2 is `resident` in
pages; /proc/<pid>/stat field 4 (after the comm field) is the ppid used to walk
the tree. All readers are injectable so the unit tests never touch real /proc.
"""

from __future__ import annotations

import os


def _default_read_stat(pid: int) -> str:
    with open(f"/proc/{pid}/stat", "r") as f:
        return f.read()


def _default_read_statm(pid: int) -> str:
    with open(f"/proc/{pid}/statm", "r") as f:
        return f.read()


def _default_all_pids() -> list[int]:
    out: list[int] = []
    for name in os.listdir("/proc"):
        if name.isdigit():
            out.append(int(name))
    return out


def _ppid_of(stat_line: str) -> int | None:
    """Parse ppid from a /proc/<pid>/stat line. The comm field (field 2) is
    wrapped in parens and may itself contain spaces/parens, so split on the LAST
    ')': everything after it is space-delimited, and ppid is the 2nd such token
    (state, ppid, ...)."""
    rparen = stat_line.rfind(")")
    if rparen == -1:
        return None
    rest = stat_line[rparen + 1:].split()
    if len(rest) < 2:
        return None
    try:
        return int(rest[1])
    except ValueError:
        return None


def _rss_pages(statm_line: str) -> int:
    parts = statm_line.split()
    if len(parts) < 2:
        return 0
    try:
        return int(parts[1])
    except ValueError:
        return 0


def tree_rss_bytes(
    root_pid: int,
    *,
    read_stat=_default_read_stat,
    read_statm=_default_read_statm,
    all_pids=_default_all_pids,
    page_size: int | None = None,
) -> int:
    """Combined RSS (bytes) of root_pid and all of its descendants.

    Builds the pid→ppid map once from a single /proc scan, then sums statm RSS
    for every pid reachable from root_pid. Dead pids (raced away mid-scan) are
    skipped, never fatal — the sampler must not crash the sidecar."""
    if page_size is None:
        page_size = os.sysconf("SC_PAGE_SIZE")

    children: dict[int, list[int]] = {}
    for pid in all_pids():
        try:
            ppid = _ppid_of(read_stat(pid))
        except (OSError, ValueError):
            continue
        if ppid is None:
            continue
        children.setdefault(ppid, []).append(pid)

    total = 0
    stack = [root_pid]
    seen: set[int] = set()
    while stack:
        pid = stack.pop()
        if pid in seen:
            continue
        seen.add(pid)
        try:
            total += _rss_pages(read_statm(pid)) * page_size
        except (OSError, ValueError):
            pass  # process exited between scan and read — skip, don't crash
        stack.extend(children.get(pid, ()))
    return total


def process_tree_rss(
    *,
    root_pid: int | None = None,
    read_stat=_default_read_stat,
    read_statm=_default_read_statm,
    all_pids=_default_all_pids,
    page_size: int | None = None,
) -> int:
    """Combined RSS of THIS process tree (os.getpid() by default)."""
    if root_pid is None:
        root_pid = os.getpid()
    return tree_rss_bytes(
        root_pid,
        read_stat=read_stat,
        read_statm=read_statm,
        all_pids=all_pids,
        page_size=page_size,
    )
