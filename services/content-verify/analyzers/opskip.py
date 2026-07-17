#!/usr/bin/env python3
"""opskip — OP/ED audio fingerprint matching (content-verify skip lane).

pair   : cross-correlate two episodes' window fingerprints, emit the longest
         common segment (the OP/ED) with per-file bounds + its fp slice.
locate : find a stored fingerprint inside one episode's window.

Fingerprints come from chromaprint's fpcalc (-raw): int32 frames at ~8 fps.
Similarity = 1 - popcount(a XOR b)/32 per frame; a "hit" frame has
similarity >= --sim. Runs tolerate gaps <= GAP_FRAMES (~1s) so a single
noisy frame doesn't split the OP in two.
"""
import argparse
import json
import subprocess
import sys

import numpy as np

GAP_FRAMES = 8  # ~1s at chromaprint's ~8fps

POP = np.array([bin(i).count("1") for i in range(65536)], dtype=np.uint8)


def popcount32(x: np.ndarray) -> np.ndarray:
    x = x.astype(np.uint32)
    return POP[x & 0xFFFF].astype(np.uint16) + POP[(x >> 16) & 0xFFFF]


def fpcalc(path: str) -> tuple[np.ndarray, float]:
    """Raw chromaprint of a wav → (int32 frames, frames-per-second rate)."""
    out = subprocess.run(
        ["fpcalc", "-raw", "-json", path],
        capture_output=True, text=True, check=True,
    ).stdout
    data = json.loads(out)
    fp = np.array(data["fingerprint"], dtype=np.uint32)
    dur = float(data["duration"])
    if len(fp) == 0 or dur <= 0:
        raise ValueError(f"empty fingerprint for {path}")
    return fp, len(fp) / dur


def sim_series(a: np.ndarray, b: np.ndarray, lag: int) -> np.ndarray:
    """Per-frame similarity of a vs b shifted by lag (b[i+lag] matched to a[i])."""
    if lag >= 0:
        n = min(len(a), len(b) - lag)
        if n <= 0:
            return np.empty(0)
        d = popcount32(a[:n] ^ b[lag:lag + n])
    else:
        n = min(len(a) + lag, len(b))
        if n <= 0:
            return np.empty(0)
        d = popcount32(a[-lag:-lag + n] ^ b[:n])
    return 1.0 - d / 32.0


def longest_run(hits: np.ndarray, min_frames: int) -> tuple[int, int] | None:
    """Longest run of True allowing gaps <= GAP_FRAMES; None if < min_frames."""
    best, cur_start, gap, start = None, None, 0, None
    for i, h in enumerate(hits):
        if h:
            if start is None:
                start = i
            gap = 0
        elif start is not None:
            gap += 1
            if gap > GAP_FRAMES:
                end = i - gap
                if best is None or end - start > best[1] - best[0]:
                    best = (start, end)
                start, gap = None, 0
    if start is not None:
        end = len(hits) - 1
        while end > start and not hits[end]:
            end -= 1
        if best is None or end - start > best[1] - best[0]:
            best = (start, end)
    if best and best[1] - best[0] + 1 >= min_frames:
        return best
    return None


def best_common_segment(a, b, rate, args):
    """Scan all lags; return (a0, a1, lag, mean_sim) frame bounds of the
    longest common run within [min,max] length, or None."""
    min_f = int(args.min * rate)
    max_f = int(args.max * rate)
    best = None
    for lag in range(-(len(b) - min_f), len(b) - min_f):
        s = sim_series(a, b, lag)
        if len(s) < min_f:
            continue
        run = longest_run(s >= args.sim, min_f)
        if run is None:
            continue
        a_off = 0 if lag >= 0 else -lag
        r0, r1 = run[0] + a_off, run[1] + a_off
        if r1 - r0 + 1 > max_f:
            r1 = r0 + max_f - 1
        score = r1 - r0
        if best is None or score > best[0]:
            seg = s[run[0]:run[1] + 1]
            best = (score, r0, r1, lag, float(np.mean(seg)))
    if best is None:
        return None
    _, r0, r1, lag, ms = best
    return r0, r1, lag, ms


def cmd_pair(args):
    a, rate_a = fpcalc(args.files[0])
    b, rate_b = fpcalc(args.files[1])
    rate = (rate_a + rate_b) / 2
    seg = best_common_segment(a, b, rate, args)
    if seg is None:
        print(json.dumps({"found": False}))
        return
    a0, a1, lag, ms = seg
    print(json.dumps({
        "found": True,
        "a_start": a0 / rate, "a_end": (a1 + 1) / rate,
        "b_start": (a0 + lag) / rate, "b_end": (a1 + 1 + lag) / rate,
        "similarity": ms,
        "fp": [int(x) for x in a[a0:a1 + 1]],
    }))


def cmd_locate(args):
    ep, rate = fpcalc(args.files[0])
    with open(args.files[1]) as f:
        stored = json.load(f)
    best = None
    for idx, item in enumerate(stored):
        q = np.array(item["fp"], dtype=np.uint32)
        if len(q) < 8 or len(q) > len(ep):
            continue
        need = int(len(q) * 0.85)  # run must cover >=85% of the query
        for lag in range(0, len(ep) - len(q) + 1):
            s = sim_series(q, ep, lag)
            run = longest_run(s >= args.sim, need)
            if run is None:
                continue
            ms = float(np.mean(s[run[0]:run[1] + 1]))
            if best is None or ms > best[0]:
                best = (ms, lag, len(q), idx)
    if best is None:
        print(json.dumps({"found": False}))
        return
    ms, lag, qlen, idx = best
    print(json.dumps({
        "found": True,
        "start": lag / rate, "end": (lag + qlen) / rate,
        "similarity": ms, "fp_index": idx,
    }))


def selftest():
    rng = np.random.default_rng(7)
    rate = 8.0
    op = rng.integers(0, 2**32, size=int(90 * rate), dtype=np.uint32)

    def episode(op_at: float, total: float = 480.0):
        ep = rng.integers(0, 2**32, size=int(total * rate), dtype=np.uint32)
        i = int(op_at * rate)
        ep[i:i + len(op)] = op
        return ep

    a, b = episode(20.0), episode(65.0)

    class A:  # argparse stand-in
        min, max, sim = 50, 150, 0.75

    seg = best_common_segment(a, b, rate, A)
    assert seg is not None, "pair: common segment not found"
    a0, a1, lag, ms = seg
    assert abs(a0 / rate - 20.0) < 2, f"pair a_start off: {a0 / rate}"
    assert abs((a0 + lag) / rate - 65.0) < 2, f"pair b_start off: {(a0 + lag) / rate}"
    assert ms > 0.95, f"pair similarity low: {ms}"

    # no shared segment → not found
    assert best_common_segment(episode(20.0), rng.integers(0, 2**32, size=int(480 * rate), dtype=np.uint32), rate, A) is None

    print("selftest OK", file=sys.stderr)


def main():
    if "--selftest" in sys.argv:
        selftest()
        return
    p = argparse.ArgumentParser()
    p.add_argument("mode", choices=["pair", "locate"])
    p.add_argument("files", nargs=2)
    p.add_argument("--min", type=float, default=50)
    p.add_argument("--max", type=float, default=150)
    p.add_argument("--sim", type=float, default=0.75)
    args = p.parse_args()
    (cmd_pair if args.mode == "pair" else cmd_locate)(args)


if __name__ == "__main__":
    main()
