"""Score -> pool-target curve for graduated degradation (spec 2026-07-21).

Pure functions: piecewise-linear between "score:cap" breakpoints,
floor()-rounded, clamped to [1, max_pool] — Camoufox always keeps one
instance (user-stream floor); the sustained-L2 DegradedShed backstop lives
in engine._shed_new_work, not here.
"""
from __future__ import annotations

import math

DEFAULT_CURVE = [(0.40, 6), (0.60, 2), (0.80, 1)]


def parse_curve(s: str) -> list[tuple[float, int]]:
    """Parse "0.40:6,0.60:2,0.80:1". Malformed input, negative caps, or
    non-ascending scores fall back to DEFAULT_CURVE (operator env)."""
    out: list[tuple[float, int]] = []
    prev = -1.0
    for part in (s or "").split(","):
        part = part.strip()
        if not part:
            return list(DEFAULT_CURVE)
        score_s, _, cap_s = part.partition(":")
        try:
            score, cap = float(score_s), int(cap_s)
        except ValueError:
            return list(DEFAULT_CURVE)
        if cap < 0 or score <= prev:
            return list(DEFAULT_CURVE)
        prev = score
        out.append((score, cap))
    return out or list(DEFAULT_CURVE)


def pool_target_for(score: float, curve: list[tuple[float, int]], max_pool: int) -> int:
    """Map a pressure score to the warm-browser target, clamped [1, max_pool]."""
    if not curve:
        return max(1, max_pool)
    if score <= curve[0][0]:
        raw = curve[0][1]
    elif score >= curve[-1][0]:
        raw = curve[-1][1]
    else:
        raw = curve[-1][1]
        for (s0, c0), (s1, c1) in zip(curve, curve[1:]):
            if score <= s1:
                frac = (score - s0) / (s1 - s0)
                # +epsilon: guards against float repr error landing just under
                # an integer boundary (e.g. score=0.55 on this exact curve
                # computes 2.9999999999999987, not 3 — verified against the
                # brief's own worked example) without affecting any real band.
                raw = math.floor(c0 + frac * (c1 - c0) + 1e-9)
                break
    return max(1, min(int(raw), max_pool))
