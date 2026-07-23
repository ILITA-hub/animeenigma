#!/usr/bin/env python3
"""Burned-in subtitle detection for content-verify.

Usage: hardsub.py <frames_dir>
Prints JSON: {"frames","tier1_hits","ocr_real","script","text_stroke_p75"}

Port of tools/subprobe (tier1 pixel band heuristic + tesseract OCR script
ID). Aggregation uses hit counts / p75, not means — subs are intermittent.
"""
import glob
import json
import os
import subprocess
import sys

import numpy as np
from PIL import Image

# --- BEGIN verbatim copies from tools/subprobe/analyze.py ---
# BAND_* constants, luma(), grad_mag(), band_box(), tier1(), script_of(), tier2()

# Subtitle band: bottom-center. Relative to frame size so it's aspect-agnostic.
BAND_Y0, BAND_Y1 = 0.72, 0.985   # bottom ~26%
BAND_X0, BAND_X1 = 0.06, 0.94    # drop far edges (channel logos live in corners)

def luma(rgb):
    return 0.299*rgb[...,0] + 0.587*rgb[...,1] + 0.114*rgb[...,2]

def grad_mag(g):
    gy, gx = np.gradient(g)
    return np.hypot(gx, gy)

def band_box(w, h):
    return (int(BAND_X0*w), int(BAND_Y0*h), int(BAND_X1*w), int(BAND_Y1*h))

def tier1(arr):
    """arr: HxWx3 uint8. Returns dict of cheap scores for the subtitle band."""
    h, w, _ = arr.shape
    x0, y0, x1, y1 = band_box(w, h)
    g = luma(arr.astype(np.float32))
    band = g[y0:y1, x0:x1]
    gm = grad_mag(band)
    edge = gm > 40                       # strong local contrast
    bright = band > 195                  # near-white (typical hardsub fill)
    dark = band < 70
    text_stroke = bright & (gm > 50)     # bright pixel sitting on a strong edge = glyph stroke
    # row profile: subs concentrate edges into a few horizontal rows
    row_edge = edge.mean(axis=1)
    return dict(
        edge_density=float(edge.mean()),
        bright_frac=float(bright.mean()),
        dark_frac=float(dark.mean()),
        text_stroke=float(text_stroke.mean()),
        row_peak=float(row_edge.max()),          # busiest row
        row_active=float((row_edge > 0.12).mean()) # fraction of rows that look "texty"
    )

def script_of(text):
    cyr = lat = cjk = 0
    for ch in text:
        if not ch.strip():
            continue
        o = ord(ch)
        if 0x0400 <= o <= 0x04FF: cyr += 1
        elif (0x3040 <= o <= 0x30FF) or (0x4E00 <= o <= 0x9FFF): cjk += 1
        elif ('A' <= ch <= 'Z') or ('a' <= ch <= 'z'): lat += 1
    counts = {'cyrillic': cyr, 'latin': lat, 'cjk': cjk}
    top = max(counts, key=counts.get)
    return (top if counts[top] > 0 else 'none'), counts

def tier2(img, langs='eng+rus+jpn', tmp='/tmp/_subband.png'):
    """img: PIL RGB. OCR the band, upscaled 3x for legibility."""
    w, h = img.size
    x0, y0, x1, y1 = band_box(w, h)
    crop = img.crop((x0, y0, x1, y1)).resize(((x1-x0)*3, (y1-y0)*3))
    crop.save(tmp)
    r = subprocess.run(['tesseract', tmp, 'stdout', '-l', langs, '--psm', '6', 'tsv'],
                       capture_output=True, text=True)
    words, confs = [], []
    for line in r.stdout.splitlines()[1:]:
        f = line.split('\t')
        if len(f) < 12: continue
        try: conf = float(f[10])
        except ValueError: continue
        txt = f[11].strip()
        if conf >= 0 and txt:
            words.append(txt); confs.append(conf)
    joined = ' '.join(words)
    # "real text" = enough alnum chars recognised with decent confidence
    good = [c for c in confs if c >= 60]
    good_words = [w for w, c in zip(words, confs) if c >= 60]
    alnum = sum(ch.isalnum() for ch in joined)
    real = (len(good) >= 2 and alnum >= 6)
    # Script is classified from HIGH-CONFIDENCE words only: low-conf OCR noise
    # routinely misreads Cyrillic glyphs as Latin homoglyphs (e.g. digit '3'
    # for 'З', 'H' for 'Н') and, left in the unfiltered joined text, can tip
    # the majority-script vote to the wrong language (live RU hardsub
    # misclassified "en", 2026-07-23).
    scr, counts = script_of(' '.join(good_words))
    mean_conf = float(np.mean(confs)) if confs else 0.0
    return dict(real_text=bool(real), n_words=len(words), n_conf60=len(good),
                alnum=alnum, mean_conf=mean_conf, script=scr, counts=counts,
                text=joined[:120])

# --- END verbatim copies ---

STROKE_T = 0.005  # tools/subprobe/verify_verdict.py: hardsub >= ~0.006, clean < ~0.003


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: hardsub.py <frames_dir>", file=sys.stderr)
        return 2
    frames_dir = sys.argv[1]
    paths = sorted(glob.glob(os.path.join(frames_dir, "*.png")) + glob.glob(os.path.join(frames_dir, "*.jpg")))
    if not paths:
        print(json.dumps({"frames": 0, "tier1_hits": 0, "ocr_real": 0, "script": "none", "text_stroke_p75": 0.0}))
        return 0
    strokes, hits = [], []
    n_ok = 0
    for p in paths:
        try:
            img = Image.open(p).convert("RGB")
            arr = np.asarray(img)
            t1 = tier1(arr)
        except Exception as exc:  # one corrupt/truncated frame must not kill the batch
            print(f"hardsub: {p}: {exc}", file=sys.stderr)
            continue
        n_ok += 1
        strokes.append(t1["text_stroke"])
        if t1["text_stroke"] >= STROKE_T:
            hits.append((p, img))
    ocr_real = 0
    scripts = {}
    for p, img in hits:
        try:
            t2 = tier2(img)
        except Exception as exc:  # OCR failure on a hit frame must not kill the batch
            print(f"hardsub: {p}: {exc}", file=sys.stderr)
            continue
        if t2["real_text"]:
            ocr_real += 1
            scripts[t2["script"]] = scripts.get(t2["script"], 0) + 1
    top_script = max(scripts, key=scripts.get) if scripts else "none"
    print(json.dumps({
        "frames": n_ok,
        "tier1_hits": len(hits),
        "ocr_real": ocr_real,
        "script": top_script,
        "text_stroke_p75": float(np.percentile(np.array(strokes), 75)) if strokes else 0.0,
    }))
    return 0


if __name__ == "__main__":
    sys.exit(main())
