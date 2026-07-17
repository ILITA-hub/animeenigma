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

# A common run this long at this similarity is the SAME content served for
# both episodes (provider episode-mapping bug), not an OP/ED: no legit pair
# of different episodes shares 300+ contiguous seconds at near-identical
# audio. See cmd_pair's "duplicate" output.
DUP_SPAN_S = 300.0
DUP_SIM = 0.95

POP = np.array([bin(i).count("1") for i in range(65536)], dtype=np.uint8)


def popcount32(x: np.ndarray) -> np.ndarray:
    x = x.astype(np.uint32)
    return POP[x & 0xFFFF].astype(np.uint16) + POP[(x >> 16) & 0xFFFF]


def fpcalc(path: str) -> tuple[np.ndarray, float]:
    """Raw chromaprint of a wav → (int32 frames, frames-per-second rate).

    -length 0 lifts fpcalc's default 120-second processing cap; without it
    only the first two minutes of a 480s window get fingerprinted while the
    reported duration stays 480 — every boundary time downstream then scales
    by a ~4x-wrong rate and most of the window is never compared at all.
    fpcalc 1.5.x with an uncapped length hits EOF in the decoder and exits
    non-zero AFTER printing the complete JSON, so success is judged by
    parseable output with frames, not by the exit code.
    """
    proc = subprocess.run(
        ["fpcalc", "-raw", "-json", "-length", "0", path],
        capture_output=True, text=True,
    )
    try:
        data = json.loads(proc.stdout)
    except json.JSONDecodeError:
        raise ValueError(
            f"fpcalc produced no fingerprint for {path} "
            f"(exit {proc.returncode}): {proc.stderr.strip()[:200]}"
        )
    fp = np.array(data.get("fingerprint", []), dtype=np.uint32)
    dur = float(data.get("duration", 0))
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
    longest common run within [min,max] length, the string "duplicate" when
    the two inputs are near-identical content (see DUP_SPAN_S), or None.

    Length ties break on higher mean similarity — with the run capped at
    max frames, MANY lags can tie at the cap, and first-lag-wins would pick
    an arbitrary (often musically self-similar but wrong) alignment."""
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
        if run[1] - run[0] + 1 >= DUP_SPAN_S * rate and \
                float(np.mean(s[run[0]:run[1] + 1])) >= DUP_SIM:
            return "duplicate"
        a_off = 0 if lag >= 0 else -lag
        r0, r1 = run[0] + a_off, run[1] + a_off
        if r1 - r0 + 1 > max_f:
            r1 = r0 + max_f - 1
        seg = s[run[0]:run[0] + (r1 - r0) + 1]  # mean over the CAPPED slice
        cand = (r1 - r0, float(np.mean(seg)), r0, r1, lag)
        if best is None or cand[:2] > best[:2]:
            best = cand
    if best is None:
        return None
    _, ms, r0, r1, lag = best
    return r0, r1, lag, ms


def cmd_pair(args):
    a, rate_a = fpcalc(args.files[0])
    b, rate_b = fpcalc(args.files[1])
    rate = (rate_a + rate_b) / 2
    seg = best_common_segment(a, b, rate, args)
    if seg == "duplicate":
        # Both "episodes" are the same content — a provider episode-mapping
        # bug, not an OP. The caller must NOT fingerprint this.
        print(json.dumps({"found": False, "duplicate": True}))
        return
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

    # identical content (provider serving the same file for both episodes)
    # → "duplicate", never a fingerprint-worthy match
    dup = episode(20.0)
    assert best_common_segment(dup, dup.copy(), rate, A) == "duplicate", \
        "identical inputs must report duplicate"

    # real-fpcalc guard: fingerprint a synthesized wav and require the FULL
    # duration to be covered at chromaprint's ~8fps. Catches the fpcalc
    # default -length 120 cap (which silently truncated 480s windows to
    # 120s and skewed every rate-derived boundary by ~4x).
    import shutil
    if shutil.which("fpcalc"):
        import tempfile
        import wave
        with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp:
            with wave.open(tmp, "wb") as w:
                w.setnchannels(1)
                w.setsampwidth(2)
                w.setframerate(16000)
                w.writeframes(
                    (rng.normal(0, 3000, size=16000 * 200).astype(np.int16)).tobytes())
            wav_path = tmp.name
        try:
            fp, fp_rate = fpcalc(wav_path)
            assert 6 <= fp_rate <= 10, f"fpcalc rate {fp_rate:.2f} (window truncated? -length regression)"
            assert len(fp) >= 190 * 6, f"fpcalc frames {len(fp)} do not cover 200s"
        finally:
            import os
            os.unlink(wav_path)
    else:
        print("selftest: fpcalc missing — rate guard skipped", file=sys.stderr)

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
