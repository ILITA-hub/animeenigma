package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ILITA-hub/animeenigma/services/governor/internal/domain"
)

// feed pushes targets in order and returns the final level.
func feed(m *Machine, targets ...domain.Level) domain.Level {
	lvl := m.Level()
	for _, t := range targets {
		lvl, _ = m.Tick(t)
	}
	return lvl
}

func TestMachine_EntersAfterEnterTicks(t *testing.T) {
	m := NewMachine(4, 20)
	assert.Equal(t, domain.LevelNormal, feed(m, 1, 1, 1))
	lvl, changed := m.Tick(domain.LevelElevated) // 4th consecutive
	assert.True(t, changed)
	assert.Equal(t, domain.LevelElevated, lvl)
}

func TestMachine_FlapToNormalResetsEnterStreak(t *testing.T) {
	m := NewMachine(4, 20)
	feed(m, 1, 1, 1, 0, 1, 1, 1) // never 4 consecutive
	assert.Equal(t, domain.LevelNormal, m.Level())
}

func TestMachine_DirectJumpToCriticalWhenSustained(t *testing.T) {
	m := NewMachine(4, 20)
	assert.Equal(t, domain.LevelCritical, feed(m, 2, 2, 2, 2))
}

func TestMachine_MixedStreakPromotesToSustainedFloor(t *testing.T) {
	m := NewMachine(4, 20)
	// Streak of raises [2,2,2,1] proves only L1 was sustained.
	assert.Equal(t, domain.LevelElevated, feed(m, 2, 2, 2, 1))
}

func TestMachine_ExitIsSlow(t *testing.T) {
	m := NewMachine(1, 3)
	feed(m, 2) // enter critical instantly (enterTicks=1)
	assert.Equal(t, domain.LevelCritical, m.Level())

	feed(m, 0, 0)
	assert.Equal(t, domain.LevelCritical, m.Level(), "still holding before exitTicks")
	lvl, changed := m.Tick(domain.LevelNormal)
	assert.True(t, changed)
	assert.Equal(t, domain.LevelNormal, lvl)
}

func TestMachine_ExitDemotesToCeilingSeenDuringStreak(t *testing.T) {
	m := NewMachine(1, 3)
	feed(m, 2)
	// Below critical for 3 ticks, but Elevated was still seen -> demote to 1.
	assert.Equal(t, domain.LevelElevated, feed(m, 0, 1, 0))
}

func TestMachine_ReturnToLevelResetsExitStreak(t *testing.T) {
	m := NewMachine(1, 3)
	feed(m, 2)
	feed(m, 0, 0, 2, 0, 0) // exit streak broken by the 2
	assert.Equal(t, domain.LevelCritical, m.Level())
}

func TestMachine_DirectionFlipResetsStreak(t *testing.T) {
	m := NewMachine(2, 2)
	feed(m, 1) // raising streak 1/2
	feed(m, 1) // 2/2 -> level 1
	assert.Equal(t, domain.LevelElevated, m.Level())
	// From L1: up, down, up, down — direction flips each tick, level pinned.
	assert.Equal(t, domain.LevelElevated, feed(m, 2, 0, 2, 0))
}

func TestMachine_MinimumTickFloors(t *testing.T) {
	m := NewMachine(0, 0) // clamped to 1/1
	lvl, changed := m.Tick(domain.LevelCritical)
	assert.True(t, changed)
	assert.Equal(t, domain.LevelCritical, lvl)
}
