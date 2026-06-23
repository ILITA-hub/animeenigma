#!/usr/bin/env python3
"""generate-favicons.py — render the site favicon set from the canonical logo.

Source of truth: frontend/web/public/logo.png (the «APPROVED» cat seal).
Whenever the logo changes, run `make favicons` (or this script directly) so
every icon variant referenced by index.html / the web manifest is rebuilt
from the same master — no more hand-exporting each size.

Outputs (all under frontend/web/public/):
    favicon.ico                 (16 + 32 + 48 multi-resolution)
    favicon-16x16.png
    favicon-32x32.png
    apple-touch-icon.png        (180x180)
    android-chrome-192x192.png
    android-chrome-512x512.png

Only dependency is Pillow (already vendored for subset-noto-jp.py).
"""

from __future__ import annotations

import os
import sys

try:
    from PIL import Image
except ImportError:
    sys.exit("Pillow is required: pip install Pillow")

# Resolve paths relative to this script so it works from any CWD.
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PUBLIC_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, "..", "public"))
SOURCE = os.path.join(PUBLIC_DIR, "logo.png")

# (filename, edge_px). favicon.ico is handled separately as multi-res.
PNG_TARGETS = [
    ("favicon-16x16.png", 16),
    ("favicon-32x32.png", 32),
    ("apple-touch-icon.png", 180),
    ("android-chrome-192x192.png", 192),
    ("android-chrome-512x512.png", 512),
]
ICO_SIZES = [16, 32, 48]


def load_square_source() -> Image.Image:
    """Load the logo as a transparent, perfectly square RGBA image."""
    img = Image.open(SOURCE).convert("RGBA")
    side = max(img.size)
    if img.size == (side, side):
        return img
    # Pad (never crop) onto a transparent square so nothing is clipped.
    canvas = Image.new("RGBA", (side, side), (0, 0, 0, 0))
    canvas.paste(img, ((side - img.width) // 2, (side - img.height) // 2))
    return canvas


def resized(src: Image.Image, edge: int) -> Image.Image:
    return src.resize((edge, edge), Image.LANCZOS)


def main() -> int:
    if not os.path.exists(SOURCE):
        sys.exit(f"source logo not found: {SOURCE}")

    src = load_square_source()
    print(f"source: {SOURCE} ({src.width}x{src.height})")

    for name, edge in PNG_TARGETS:
        out = os.path.join(PUBLIC_DIR, name)
        resized(src, edge).save(out, "PNG", optimize=True)
        print(f"  wrote {name} ({edge}x{edge})")

    ico_out = os.path.join(PUBLIC_DIR, "favicon.ico")
    # Pillow builds a multi-resolution .ico from the largest frame + sizes list.
    resized(src, max(ICO_SIZES)).save(
        ico_out, format="ICO", sizes=[(s, s) for s in ICO_SIZES]
    )
    print(f"  wrote favicon.ico ({'+'.join(map(str, ICO_SIZES))})")

    print("favicons regenerated from logo.png")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
