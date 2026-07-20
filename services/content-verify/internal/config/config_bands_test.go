package config

import (
	"testing"
	"time"
)

func TestParseBandWeights(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
	}{
		{"", [3]int{60, 30, 10}},
		{"50,40,10", [3]int{50, 40, 10}},
		{"bad", [3]int{60, 30, 10}},
		{"1,2", [3]int{60, 30, 10}},   // wrong arity → default
		{"0,0,0", [3]int{60, 30, 10}}, // all-zero is meaningless → default
	}
	for _, c := range cases {
		if got := parseBandWeights(c.in); got != c.want {
			t.Errorf("parseBandWeights(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestIdleCooldownDefault(t *testing.T) {
	if getEnvDuration("CV_IDLE_COOLDOWN_UNSET_XYZ", 168*time.Hour) != 168*time.Hour {
		t.Fatal("duration default helper regressed")
	}
}
