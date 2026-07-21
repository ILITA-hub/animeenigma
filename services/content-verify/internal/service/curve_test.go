package service

import "testing"

func TestCurveCapBands(t *testing.T) {
	c := ParseCurve("0.40:6,0.60:2,0.80:0", nil)
	cases := []struct {
		score float64
		want  int
	}{
		{0.0, 6}, {0.39, 6}, {0.40, 6},
		{0.41, 5},          // floor(6 - 4*(0.01/0.20*... )) = floor(5.8)
		{0.50, 4},          // midpoint of 6->2
		{0.55, 3},          // epsilon parity: raw IEEE value is 2.999999999999999,
		                    // not 3 — matches Python scaling.pool_target_for's
		                    // +1e-9 guard (review finding 2).
		{0.60, 2}, {0.70, 1}, {0.80, 0}, {0.95, 0}, {1.0, 0},
	}
	for _, tc := range cases {
		if got := c.Cap(tc.score); got != tc.want {
			t.Errorf("Cap(%v) = %d; want %d", tc.score, got, tc.want)
		}
	}
}

func TestParseCurveFallsBackOnGarbage(t *testing.T) {
	def := Curve{{0.5, 3}}
	for _, bad := range []string{"", "nonsense", "0.6:2,0.4:6" /* unsorted */, "0.4:-1"} {
		got := ParseCurve(bad, def)
		if len(got) != 1 || got[0].Cap != 3 {
			t.Errorf("ParseCurve(%q) did not fall back to default", bad)
		}
	}
}
