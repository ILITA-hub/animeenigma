#!/usr/bin/env python3
"""generate-favicons.py — render the site icon set(s).

Two icon families, both rebuilt from canonical sources so the icons never
drift from the live brand:

  DEFAULT (main site)  — the Neon-Tokyo BRAND-MARK, faithfully reproduced
      from frontend/web/src/components/layout/BrandMark.vue: a rounded
      square with a 135deg cyan->pink gradient, a dark inset cutout, and
      "AE" centered in cyan (Manrope / font-display, weight 800).
        favicon.ico (16/32/48), favicon-16x16.png, favicon-32x32.png,
        apple-touch-icon.png (180), android-chrome-192x192/512x512.png

  ADMIN (/admin/*)     — the legacy «APPROVED» cat seal, rendered from
      frontend/web/public/logo.png. Retired from the main site; the Vue
      router swaps the tab favicon to these on admin routes.
        favicon-admin.ico (16/32/48),
        favicon-admin-16x16.png, favicon-admin-32x32.png

Run `make favicons` after changing the logo, the brand tokens, or BrandMark.

Deps: Pillow + numpy (gradient) + fontTools/brotli (woff2->ttf for "AE";
falls back to DejaVu Sans Bold if the Manrope woff2 can't be decoded).
"""

from __future__ import annotations

import os
import sys
import tempfile

try:
    import numpy as np
    from PIL import Image, ImageDraw, ImageFont
except ImportError as exc:  # pragma: no cover
    sys.exit(f"missing dependency ({exc}); need Pillow + numpy")

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PUBLIC_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, "..", "public"))
LOGO = os.path.join(PUBLIC_DIR, "logo.png")
MANROPE_WOFF2 = os.path.join(PUBLIC_DIR, "fonts", "manrope-800.woff2")
DEJAVU_BOLD = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"

# Brand tokens — keep in sync with frontend/web/src/styles/main.css.
BRAND_CYAN = (0x00, 0xD4, 0xFF)   # --color-cyan-400
BRAND_PINK = (0xFF, 0x2D, 0x7C)   # --color-pink-500
COLOR_BASE = (0x08, 0x08, 0x0F)   # --color-base

# BrandMark.vue proportions (relative to its 28px box).
R_OUTER = 8 / 28      # border-radius
R_INSET = 4 / 28      # ::after inset
R_INNER = 5 / 28      # ::after border-radius
MASTER = 1024         # supersampled master; downscaled per target

ICO_SIZES = [16, 32, 48]
DEFAULT_PNGS = [
    ("favicon-16x16.png", 16),
    ("favicon-32x32.png", 32),
    ("apple-touch-icon.png", 180),
    ("android-chrome-192x192.png", 192),
    ("android-chrome-512x512.png", 512),
]
ADMIN_PNGS = [
    ("favicon-admin-16x16.png", 16),
    ("favicon-admin-32x32.png", 32),
]


# ---------------------------------------------------------------------------
# Brand-mark renderer
# ---------------------------------------------------------------------------
def _manrope_ttf(tmpdir: str) -> str:
    """Decode the Manrope-800 woff2 to a ttf PIL can load; fall back to DejaVu."""
    try:
        from fontTools.ttLib import TTFont

        font = TTFont(MANROPE_WOFF2)
        font.flavor = None
        out = os.path.join(tmpdir, "manrope-800.ttf")
        font.save(out)
        return out
    except Exception as exc:  # brotli missing / file moved
        print(f"  (woff2 decode failed: {exc}; using DejaVu Sans Bold)")
        return DEJAVU_BOLD


def _gradient_square(size: int) -> Image.Image:
    """135deg cyan(top-left)->pink(bottom-right) RGB square."""
    ramp = np.linspace(0.0, 1.0, size, dtype=np.float32)
    # t = (x + y) / 2, normalized to [0,1] across the diagonal.
    t = (ramp[None, :] + ramp[:, None]) / 2.0
    cyan = np.array(BRAND_CYAN, dtype=np.float32)
    pink = np.array(BRAND_PINK, dtype=np.float32)
    rgb = cyan[None, None, :] * (1 - t[..., None]) + pink[None, None, :] * t[..., None]
    return Image.fromarray(rgb.round().astype("uint8"), "RGB")


def _ae_layer(size: int, ttf: str) -> Image.Image:
    """Cyan "AE" cropped tight to its ink, for true visual centering."""
    # Oversize canvas; fit font so inked height ~= 0.34 * size.
    target_h = 0.34 * size
    probe = ImageFont.truetype(ttf, size)
    bbox = probe.getbbox("AE")
    measured_h = bbox[3] - bbox[1]
    font_px = max(8, round(size * target_h / measured_h))
    font = ImageFont.truetype(ttf, font_px)

    pad = size
    layer = Image.new("RGBA", (size + 2 * pad, size + 2 * pad), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)
    d.text((pad + size // 2, pad + size // 2), "AE", font=font,
           fill=BRAND_CYAN + (255,), anchor="mm")
    return layer.crop(layer.getbbox())


def render_brand_mark(size: int, ttf: str) -> Image.Image:
    """Render the brand-mark at `size`x`size` (supersampled then downscaled)."""
    m = MASTER
    img = _gradient_square(m).convert("RGBA")

    # Clip the gradient to the outer rounded square (alpha mask).
    mask = Image.new("L", (m, m), 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        [0, 0, m - 1, m - 1], radius=round(R_OUTER * m), fill=255)
    img.putalpha(mask)

    # Dark inset cutout.
    inset = round(R_INSET * m)
    ImageDraw.Draw(img).rounded_rectangle(
        [inset, inset, m - 1 - inset, m - 1 - inset],
        radius=round(R_INNER * m), fill=COLOR_BASE + (255,))

    # Centered "AE".
    ae = _ae_layer(m, ttf)
    img.alpha_composite(ae, ((m - ae.width) // 2, (m - ae.height) // 2))

    return img if size == m else img.resize((size, size), Image.LANCZOS)


# ---------------------------------------------------------------------------
# Cat (legacy logo) renderer
# ---------------------------------------------------------------------------
def load_cat_source() -> Image.Image:
    img = Image.open(LOGO).convert("RGBA")
    side = max(img.size)
    if img.size == (side, side):
        return img
    canvas = Image.new("RGBA", (side, side), (0, 0, 0, 0))
    canvas.paste(img, ((side - img.width) // 2, (side - img.height) // 2))
    return canvas


# ---------------------------------------------------------------------------
def _save_png(img: Image.Image, edge: int, name: str) -> None:
    out = os.path.join(PUBLIC_DIR, name)
    img.resize((edge, edge), Image.LANCZOS).save(out, "PNG", optimize=True)
    print(f"  wrote {name} ({edge}x{edge})")


def _save_ico(master: Image.Image, name: str) -> None:
    out = os.path.join(PUBLIC_DIR, name)
    master.resize((max(ICO_SIZES), max(ICO_SIZES)), Image.LANCZOS).save(
        out, format="ICO", sizes=[(s, s) for s in ICO_SIZES])
    print(f"  wrote {name} ({'+'.join(map(str, ICO_SIZES))})")


def main() -> int:
    if not os.path.exists(LOGO):
        sys.exit(f"source logo not found: {LOGO}")

    with tempfile.TemporaryDirectory() as tmpdir:
        ttf = _manrope_ttf(tmpdir)

        print("DEFAULT family — brand-mark (main site):")
        brand = render_brand_mark(MASTER, ttf)
        for name, edge in DEFAULT_PNGS:
            _save_png(brand, edge, name)
        _save_ico(brand, "favicon.ico")

    print("ADMIN family — «APPROVED» cat (logo.png):")
    cat = load_cat_source()
    print(f"  source: {LOGO} ({cat.width}x{cat.height})")
    for name, edge in ADMIN_PNGS:
        _save_png(cat, edge, name)
    _save_ico(cat, "favicon-admin.ico")

    print("favicons regenerated")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
