#!/usr/bin/env python3
"""
Burned-in subtitle probe — SPIKE.

Tier-1 (cheap, no ML): edge/contrast/bright-stroke density in the bottom-center
subtitle band of each sampled frame.
Tier-2 (robust): Tesseract OCR over the same band -> confirms real text + detects
script (Cyrillic vs Latin vs CJK) -> RU vs EN vs JP hardsub.

Operates on a directory of PNG frames. Source-agnostic.

Usage:
  analyze.py <frames_dir> <label>
"""
import sys, os, glob, subprocess, json, re, unicodedata
import numpy as np
from PIL import Image

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
    alnum = sum(ch.isalnum() for ch in joined)
    real = (len(good) >= 2 and alnum >= 6)
    scr, counts = script_of(joined)
    mean_conf = float(np.mean(confs)) if confs else 0.0
    return dict(real_text=bool(real), n_words=len(words), n_conf60=len(good),
                alnum=alnum, mean_conf=mean_conf, script=scr, counts=counts,
                text=joined[:120])

def main():
    frames_dir, label = sys.argv[1], sys.argv[2]
    paths = sorted(glob.glob(os.path.join(frames_dir, '*.png')) +
                   glob.glob(os.path.join(frames_dir, '*.jpg')))
    if not paths:
        print(f"no frames in {frames_dir}"); sys.exit(1)
    rows = []
    print(f"\n=== {label}  ({len(paths)} frames) ===")
    print(f"{'frame':22} {'edge':>6} {'strk':>6} {'rowpk':>6} | {'real':>4} {'scr':>8} {'conf':>5} {'txt'}")
    for p in paths:
        img = Image.open(p).convert('RGB')
        arr = np.asarray(img)
        t1 = tier1(arr)
        t2 = tier2(img)
        rows.append((t1, t2))
        print(f"{os.path.basename(p):22} {t1['edge_density']:6.3f} {t1['text_stroke']:6.3f} "
              f"{t1['row_peak']:6.3f} | {str(t2['real_text']):>4} {t2['script']:>8} "
              f"{t2['mean_conf']:5.1f} {t2['text'][:48]!r}")
    # aggregate
    def col(k): return np.array([r[0][k] for r in rows])
    real_frac = np.mean([r[1]['real_text'] for r in rows])
    scripts = {}
    for r in rows:
        if r[1]['real_text']:
            scripts[r[1]['script']] = scripts.get(r[1]['script'], 0) + 1
    agg = dict(
        label=label, n=len(rows),
        edge_density_med=float(np.median(col('edge_density'))),
        edge_density_p75=float(np.percentile(col('edge_density'), 75)),
        text_stroke_med=float(np.median(col('text_stroke'))),
        text_stroke_p75=float(np.percentile(col('text_stroke'), 75)),
        row_peak_med=float(np.median(col('row_peak'))),
        ocr_real_text_frac=float(real_frac),
        ocr_scripts=scripts,
    )
    print(f"\n--- AGGREGATE [{label}] ---")
    print(json.dumps(agg, indent=2))
    with open(os.path.join(frames_dir, '..', f'agg_{label}.json'), 'w') as f:
        json.dump({'agg': agg, 'frames': [{'t1': r[0], 't2': r[1]} for r in rows]}, f, indent=2)
    return agg

if __name__ == '__main__':
    main()
