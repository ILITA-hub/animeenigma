// Package-level score→cap curve for graduated degradation (spec 2026-07-21):
// piecewise-linear between breakpoints, floor()-rounded, endpoint-clamped.
package service

import (
	"math"
	"strconv"
	"strings"
)

type CurvePoint struct {
	Score float64
	Cap   int
}

// Curve is an ascending-score list of breakpoints. Below the first point the
// first cap applies; at/after the last point the last cap applies.
type Curve []CurvePoint

// ParseCurve parses "0.40:6,0.60:2,0.80:0". Any malformed piece, a negative
// cap, or non-ascending scores falls back to def (operator env, not user input).
func ParseCurve(s string, def Curve) Curve {
	parts := strings.Split(s, ",")
	out := make(Curve, 0, len(parts))
	prev := -1.0
	for _, p := range parts {
		scoreStr, capStr, ok := strings.Cut(strings.TrimSpace(p), ":")
		if !ok {
			return def
		}
		sc, err1 := strconv.ParseFloat(scoreStr, 64)
		cp, err2 := strconv.Atoi(capStr)
		if err1 != nil || err2 != nil || cp < 0 || sc <= prev {
			return def
		}
		prev = sc
		out = append(out, CurvePoint{Score: sc, Cap: cp})
	}
	if len(out) == 0 {
		return def
	}
	return out
}

// Cap maps a score to the allowed worker count.
func (c Curve) Cap(score float64) int {
	if len(c) == 0 {
		return 0
	}
	if score <= c[0].Score {
		return c[0].Cap
	}
	last := c[len(c)-1]
	if score >= last.Score {
		return last.Cap
	}
	for i := 1; i < len(c); i++ {
		if score <= c[i].Score {
			a, b := c[i-1], c[i]
			frac := (score - a.Score) / (b.Score - a.Score)
			return int(math.Floor(float64(a.Cap) + frac*float64(b.Cap-a.Cap)))
		}
	}
	return last.Cap
}
