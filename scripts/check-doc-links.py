#!/usr/bin/env python3
"""Validate links in maintained Markdown without fetching external URLs."""

from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import unquote, urlsplit


INLINE_LINK = re.compile(r"!?\[[^\]]*\]\(([^)\n]+)\)")
REFERENCE_LINK = re.compile(r"^\s*\[[^\]]+\]:\s*(\S+)")
FENCE = re.compile(r"^\s*(`{3,}|~{3,})")
LINE_SUFFIX = re.compile(r"^(.*):\d+(?::\d+)?$")
IGNORED_PREFIXES = ("#", "/", "mailto:", "tel:", "data:", "javascript:")
IGNORED_PARTS = ("*", "{", "}", "$", "<", ">")
IGNORED_DIRECTORIES = (".planning/", ".claude/")


def repository_root() -> Path:
    result = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        check=True,
        capture_output=True,
        text=True,
    )
    return Path(result.stdout.strip()).resolve()


def markdown_files(root: Path) -> list[Path]:
    result = subprocess.run(
        ["git", "ls-files", "-co", "--exclude-standard", "-z", "--", "*.md"],
        cwd=root,
        check=True,
        capture_output=True,
    )
    files = []
    for raw in result.stdout.split(b"\0"):
        if not raw:
            continue
        relative = raw.decode("utf-8", errors="surrogateescape")
        if relative.startswith(IGNORED_DIRECTORIES):
            continue
        path = root / relative
        if path.is_file():
            files.append(path)
    return sorted(files)


def markdown_without_fences(path: Path) -> str:
    kept: list[str] = []
    fence_marker = ""
    for line in path.read_text(encoding="utf-8").splitlines():
        match = FENCE.match(line)
        if match:
            marker = match.group(1)
            if not fence_marker:
                fence_marker = marker[0]
            elif marker[0] == fence_marker:
                fence_marker = ""
            continue
        if not fence_marker:
            kept.append(line)
    return "\n".join(kept)


def link_destination(raw: str) -> str:
    raw = raw.strip()
    if raw.startswith("<") and ">" in raw:
        return raw[1 : raw.index(">")]  # angle brackets allow spaces
    return raw.split(maxsplit=1)[0]


def local_target(source: Path, destination: str) -> Path | None:
    destination = unquote(destination.strip())
    lowered = destination.lower()
    if lowered.startswith(IGNORED_PREFIXES):
        return None

    parsed = urlsplit(destination)
    if parsed.scheme or parsed.netloc or not parsed.path:
        return None
    if any(part in parsed.path for part in IGNORED_PARTS):
        return None

    target = source.parent / parsed.path
    if target.exists():
        return target

    # Developer-facing links sometimes use path/to/file.go:123.
    line_match = LINE_SUFFIX.match(parsed.path)
    if line_match:
        target_without_line = source.parent / line_match.group(1)
        if target_without_line.exists():
            return target_without_line
    return target


def main() -> int:
    root = repository_root()
    failures: list[tuple[Path, str, Path]] = []
    checked = 0

    for source in markdown_files(root):
        text = markdown_without_fences(source)
        destinations = [match.group(1) for match in INLINE_LINK.finditer(text)]
        destinations.extend(
            match.group(1)
            for line in text.splitlines()
            if (match := REFERENCE_LINK.match(line))
        )
        for raw in destinations:
            target = local_target(source, link_destination(raw))
            if target is None:
                continue
            checked += 1
            if not target.exists():
                failures.append((source.relative_to(root), raw, target))

    if failures:
        print(f"Broken local Markdown links: {len(failures)}", file=sys.stderr)
        for source, raw, target in failures:
            try:
                resolved = target.resolve().relative_to(root)
            except ValueError:
                resolved = target.resolve()
            print(f"  {source}: {raw} -> {resolved}", file=sys.stderr)
        return 1

    print(f"Checked {checked} local links across maintained Markdown files")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
