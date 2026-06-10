#!/usr/bin/env python3
"""Slice local Noto Sans JP woff2 files along Google Fonts' unicode-range
boundaries and emit matching @font-face CSS with local URLs.

Usage (e.g. after replacing the source fonts with a newer Noto release):
  1. Put full noto-sans-jp-{400,500}.woff2 into public/fonts/
  2. curl -A "Mozilla/5.0 ... Chrome/124.0" \
       "https://fonts.googleapis.com/css2?family=Noto+Sans+JP:wght@400;500&display=swap" \
       -o /tmp/noto-jp-google.css   # woff2-capable UA required to get unicode-range CSS
  3. python3 -m venv venv && venv/bin/pip install fonttools brotli
  4. venv/bin/python scripts/subset-noto-jp.py
  5. Keep only the slices in git — the monolith woff2 files don't ship
"""
import re
import sys
from pathlib import Path
from io import BytesIO

from fontTools.subset import Subsetter, Options, load_font, save_font

WEB_ROOT = Path(__file__).resolve().parent.parent
GOOGLE_CSS = Path("/tmp/noto-jp-google.css")
FONT_DIR = WEB_ROOT / "public/fonts"
OUT_DIR = FONT_DIR / "noto-jp"
CSS_OUT = WEB_ROOT / "src/styles/noto-sans-jp.css"

BLOCK_RE = re.compile(
    r"(?:/\*\s*([a-z-]+)\s*\*/\s*)?"
    r"@font-face\s*\{[^}]*?font-weight:\s*(\d+);[^}]*?"
    r"url\((\S+?)\)[^}]*?"
    r"unicode-range:\s*([^;]+);",
    re.S,
)
IDX_RE = re.compile(r"\.(\d+)\.woff2$")


def parse_range(spec: str):
    """'U+25ee8, U+26ff6-26ff8' -> list of ints"""
    out = []
    for part in spec.replace("U+", "").split(","):
        part = part.strip()
        if "-" in part:
            lo, hi = part.split("-")
            out.extend(range(int(lo, 16), int(hi, 16) + 1))
        else:
            out.append(int(part, 16))
    return out


def main():
    css = GOOGLE_CSS.read_text()
    raw = BLOCK_RE.findall(css)
    expected = css.count("@font-face")
    if len(raw) != expected:
        sys.exit(f"parsed {len(raw)} blocks but CSS has {expected}")
    blocks = []
    for name, weight, url, urange in raw:
        m = IDX_RE.search(url)
        idx = m.group(1) if m else name
        if not idx:
            sys.exit(f"block without index or subset name: {url}")
        blocks.append((weight, idx, urange))
    print(f"parsed {len(blocks)} blocks")

    OUT_DIR.mkdir(exist_ok=True)
    # preload source font bytes per weight to avoid re-reading
    src_bytes = {}
    for weight in {w for w, _, _ in blocks}:
        src = FONT_DIR / f"noto-sans-jp-{weight}.woff2"
        src_bytes[weight] = src.read_bytes()

    css_parts = []
    written = skipped = 0
    total_size = 0
    for weight, idx, urange in blocks:
        codepoints = parse_range(urange)
        opts = Options()
        opts.flavor = "woff2"
        opts.layout_features = ["*"]
        font = load_font(BytesIO(src_bytes[weight]), opts)
        sub = Subsetter(opts)
        sub.populate(unicodes=codepoints)
        sub.subset(font)
        nglyphs = len(font.getGlyphOrder())
        if nglyphs <= 1:  # only .notdef — source has no glyphs in this range
            skipped += 1
            font.close()
            continue
        out = OUT_DIR / f"noto-sans-jp-{weight}.{idx}.woff2"
        save_font(font, str(out), opts)
        font.close()
        sz = out.stat().st_size
        total_size += sz
        written += 1
        normalized = ", ".join(p.strip() for p in urange.split(","))
        css_parts.append(
            "@font-face { font-family: 'Noto Sans JP'; font-style: normal; "
            f"font-weight: {weight}; font-display: swap; "
            f"src: url('/fonts/noto-jp/noto-sans-jp-{weight}.{idx}.woff2') format('woff2'); "
            f"unicode-range: {normalized}; }}"
        )

    header = (
        "/* Noto Sans JP — unicode-range slices (Google Fonts slicing, "
        "subset locally from the full woff2 via fontTools).\n"
        "   Browsers download only the slices whose codepoints are actually "
        "rendered, instead of the former ~1 MB-per-weight monolith.\n"
        "   GENERATED FILE — regenerate with scripts/subset-noto-jp.py "
        "(usage in its docstring). */\n"
    )
    CSS_OUT.write_text(header + "\n".join(css_parts) + "\n")
    print(f"written={written} skipped(empty)={skipped} total={total_size/1024:.0f}KB css={CSS_OUT}")


if __name__ == "__main__":
    main()
