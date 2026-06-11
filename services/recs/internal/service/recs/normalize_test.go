package recs

import (
	"math"
	"testing"
)

func TestMinMaxNormalize(t *testing.T) {
	const eps = 1e-6

	tests := []struct {
		name string
		raw  map[AnimeID]RawScore
		pool []AnimeID
		// expect[id] is the expected NormalizedScore for that id.
		expect map[AnimeID]NormalizedScore
	}{
		{
			name:   "empty pool returns empty map",
			raw:    map[AnimeID]RawScore{},
			pool:   []AnimeID{},
			expect: map[AnimeID]NormalizedScore{},
		},
		{
			name: "single-element pool collapses to zero (degenerate)",
			raw:  map[AnimeID]RawScore{"a": 5},
			pool: []AnimeID{"a"},
			expect: map[AnimeID]NormalizedScore{
				"a": 0,
			},
		},
		{
			name: "all-equal pool collapses to zero (degenerate)",
			raw:  map[AnimeID]RawScore{"a": 7, "b": 7, "c": 7},
			pool: []AnimeID{"a", "b", "c"},
			expect: map[AnimeID]NormalizedScore{
				"a": 0, "b": 0, "c": 0,
			},
		},
		{
			name: "normal pool: min->0, max->1",
			raw:  map[AnimeID]RawScore{"a": 0, "b": 5, "c": 10},
			pool: []AnimeID{"a", "b", "c"},
			expect: map[AnimeID]NormalizedScore{
				"a": 0,
				"b": 0.5,
				"c": 1,
			},
		},
		{
			name: "missing candidate defaults to zero",
			raw:  map[AnimeID]RawScore{"a": 0, "c": 10},
			pool: []AnimeID{"a", "b", "c"},
			expect: map[AnimeID]NormalizedScore{
				"a": 0,
				"b": 0,
				"c": 1,
			},
		},
		{
			name: "scale-invariance: small absolute values normalize the same as large",
			raw:  map[AnimeID]RawScore{"a": 0.001, "b": 0.002, "c": 0.003},
			pool: []AnimeID{"a", "b", "c"},
			expect: map[AnimeID]NormalizedScore{
				"a": 0,
				"b": 0.5,
				"c": 1,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MinMaxNormalize(tc.raw, tc.pool)
			if len(got) != len(tc.expect) {
				t.Fatalf("len(got)=%d want %d; got=%v", len(got), len(tc.expect), got)
			}
			for id, want := range tc.expect {
				gotV := got[id]
				if math.Abs(float64(gotV-want)) > eps {
					t.Errorf("got[%q]=%v want %v", id, gotV, want)
				}
			}
		})
	}
}

// Property: every output MUST be in [0, 1] regardless of input scale,
// including with negative raws and large magnitudes. No NaN, no Inf.
func TestMinMaxNormalize_OutputInZeroOneRange(t *testing.T) {
	raw := map[AnimeID]RawScore{
		"a": -100, "b": -50, "c": 0, "d": 50, "e": 100,
	}
	pool := []AnimeID{"a", "b", "c", "d", "e"}
	got := MinMaxNormalize(raw, pool)
	for id, v := range got {
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			t.Errorf("got[%q]=%v: must not be NaN/Inf", id, v)
		}
		if f < 0 || f > 1+1e-9 {
			t.Errorf("got[%q]=%v: out of [0,1] range", id, v)
		}
	}
}

// Property: monotonicity. If raw(a) > raw(b), then normalized(a) >= normalized(b).
func TestMinMaxNormalize_Monotonicity(t *testing.T) {
	raw := map[AnimeID]RawScore{"low": 1, "mid": 5, "high": 9}
	pool := []AnimeID{"low", "mid", "high"}
	got := MinMaxNormalize(raw, pool)
	if !(got["low"] <= got["mid"] && got["mid"] <= got["high"]) {
		t.Errorf("monotonicity violated: low=%v mid=%v high=%v", got["low"], got["mid"], got["high"])
	}
}
