#!/usr/bin/env python3
"""Decide HARDSUB vs SOFTSUB/CLEAN from extracted frames + tracks count."""
import sys, glob, os
import numpy as np
from PIL import Image
from analyze import tier1, tier2

STROKE_T = 0.005   # real hardsub frames >= ~0.006; busy scenes stay < 0.003
def main():
    d, prov, tracks = sys.argv[1], sys.argv[2], int(sys.argv[3] or 0)
    paths = sorted(glob.glob(os.path.join(d, '*.png')))
    strokes, hit_texts, scripts = [], 0, {}
    n_hit = 0
    for p in paths:
        arr = np.asarray(Image.open(p).convert('RGB'))
        s = tier1(arr)['text_stroke']; strokes.append(s)
        if s >= STROKE_T:
            n_hit += 1
            o = tier2(Image.open(p).convert('RGB'))
            if o['real_text']:
                hit_texts += 1; scripts[o['script']] = scripts.get(o['script'],0)+1
    strokes = np.array(strokes)
    p90 = float(np.percentile(strokes, 90)); mx = float(strokes.max())
    burned = (n_hit >= max(2, len(paths)//5)) and (hit_texts >= 1)
    scr = max(scripts, key=scripts.get) if scripts else '-'
    if burned:
        verdict = f"HARDSUB ({scr})"
    elif tracks > 0:
        verdict = "SOFTSUB (clean video + soft tracks)"
    else:
        verdict = "CLEAN/RAW or DUB (no burned text, no tracks)"
    print(f"  stroke p90={p90:.3f} max={mx:.3f} | tier1-hits={n_hit}/{len(paths)} "
          f"text-confirmed={hit_texts} scripts={scripts} tracks={tracks}")
    print(f"VERDICT {prov}: {verdict}")

if __name__ == '__main__': main()
