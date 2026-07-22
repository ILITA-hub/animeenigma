package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ILITA-hub/animeenigma/services/governor/internal/domain"
)

// defaultQuantizer uses the vetted production defaults.
func defaultQuantizer() *Quantizer { return NewQuantizer(0.45, 0.20, 0.90, 0.55) }

// feed pushes scores in order and returns the final level.
func feed(q *Quantizer, scores ...float64) domain.Level {
	lvl := q.Level()
	for _, s := range scores {
		lvl = q.Tick(s)
	}
	return lvl
}

func TestQuantizer_EntersElevatedAtThreshold(t *testing.T) {
	q := defaultQuantizer()
	assert.Equal(t, domain.LevelNormal, feed(q, 0.44), "just below enterElevated")
	assert.Equal(t, domain.LevelElevated, q.Tick(0.45), "at enterElevated")
}

func TestQuantizer_DirectJumpToCriticalOnHighScore(t *testing.T) {
	q := defaultQuantizer()
	// A single smoothed score already past enterCritical jumps straight to L2.
	assert.Equal(t, domain.LevelCritical, feed(q, 0.95))
}

func TestQuantizer_HysteresisPreventsFlapAtElevatedEdge(t *testing.T) {
	q := defaultQuantizer()
	feed(q, 0.5) // enter Elevated
	assert.Equal(t, domain.LevelElevated, q.Level())
	// Score dithers between 0.30 and 0.44 — above exitElevated(0.20), below
	// enterElevated(0.45): level must stay pinned, no oscillation.
	assert.Equal(t, domain.LevelElevated, feed(q, 0.30, 0.44, 0.30, 0.44, 0.30))
}

func TestQuantizer_ExitsElevatedOnlyBelowExitThreshold(t *testing.T) {
	q := defaultQuantizer()
	feed(q, 0.5)
	assert.Equal(t, domain.LevelElevated, feed(q, 0.21), "just above exitElevated still holds")
	assert.Equal(t, domain.LevelNormal, q.Tick(0.20), "at exitElevated drops to Normal")
}

func TestQuantizer_CriticalStepsDownToElevatedThenNormal(t *testing.T) {
	q := defaultQuantizer()
	feed(q, 0.95)
	assert.Equal(t, domain.LevelCritical, q.Level())
	assert.Equal(t, domain.LevelCritical, q.Tick(0.60), "above exitCritical(0.55) holds")
	assert.Equal(t, domain.LevelElevated, q.Tick(0.55), "at exitCritical steps to Elevated")
	assert.Equal(t, domain.LevelElevated, q.Tick(0.30), "above exitElevated holds")
	assert.Equal(t, domain.LevelNormal, q.Tick(0.10), "below exitElevated drops to Normal")
}

func TestQuantizer_CriticalCanCollapseStraightToNormal(t *testing.T) {
	q := defaultQuantizer()
	feed(q, 0.95)
	// Score cratered below exitElevated in one tick (e.g. after a Prometheus
	// blip resolved): don't get stuck an extra cycle at Elevated.
	assert.Equal(t, domain.LevelNormal, q.Tick(0.05))
}

func TestQuantizer_CriticalEntryFromElevatedNeedsEnterCritical(t *testing.T) {
	q := defaultQuantizer()
	feed(q, 0.5) // Elevated
	assert.Equal(t, domain.LevelElevated, q.Tick(0.89), "just below enterCritical")
	assert.Equal(t, domain.LevelCritical, q.Tick(0.90), "at enterCritical")
}

func TestQuantizer_Reset(t *testing.T) {
	q := defaultQuantizer()
	feed(q, 0.95)
	q.Reset()
	assert.Equal(t, domain.LevelNormal, q.Level())
}

func TestQuantizer_RejectsBadThresholdsAndUsesDefaults(t *testing.T) {
	// exit >= enter (would flap) and out-of-range values both fall back.
	for _, bad := range []*Quantizer{
		NewQuantizer(0.4, 0.5, 0.9, 0.55),   // exitElevated > enterElevated
		NewQuantizer(0.45, 0.20, 1.5, 0.55), // enterCritical out of range
		NewQuantizer(0.9, 0.2, 0.45, 0.55),  // elevated enter > critical enter
	} {
		// Default behavior: 0.45 enters Elevated, 0.90 enters Critical.
		assert.Equal(t, domain.LevelElevated, bad.Tick(0.45))
		assert.Equal(t, domain.LevelCritical, bad.Tick(0.90))
	}
}
