package service

import "github.com/ILITA-hub/animeenigma/services/governor/internal/domain"

// Machine is the enter-fast / exit-slow hysteresis state machine over the
// instantaneous target level. It is pure (no clock, no IO): the governor loop
// feeds it one target per tick.
//
// Semantics:
//   - Raising: after enterTicks CONSECUTIVE ticks with target > level, the
//     level rises to the MINIMUM target seen during the streak (the level that
//     was sustained the whole time — a [2,2,2,1] streak proves only L1).
//   - Lowering: after exitTicks consecutive ticks with target < level, the
//     level drops to the MAXIMUM target seen during the streak (multi-level
//     descent happens one sustained step at a time).
//   - Any tick where target equals the current level — or flips direction —
//     resets the streak, so flapping input pins the level in place rather
//     than oscillating the fleet.
type Machine struct {
	enterTicks int
	exitTicks  int

	level  domain.Level
	streak int
	dir    int // +1 raising, -1 lowering, 0 idle
	// pending is the promotion floor (min target, raising) or demotion
	// ceiling (max target, lowering) observed during the current streak.
	pending domain.Level
}

// NewMachine builds a Machine starting at LevelNormal.
func NewMachine(enterTicks, exitTicks int) *Machine {
	if enterTicks < 1 {
		enterTicks = 1
	}
	if exitTicks < 1 {
		exitTicks = 1
	}
	return &Machine{enterTicks: enterTicks, exitTicks: exitTicks}
}

// Level returns the current smoothed level.
func (m *Machine) Level() domain.Level { return m.level }

// Tick feeds one instantaneous target; returns the (possibly unchanged)
// smoothed level and whether it changed on this tick.
func (m *Machine) Tick(target domain.Level) (domain.Level, bool) {
	if target == m.level {
		m.reset()
		return m.level, false
	}

	dir := 1
	if target < m.level {
		dir = -1
	}
	if dir != m.dir {
		m.dir = dir
		m.streak = 0
		m.pending = target
	}
	m.streak++
	if dir > 0 && target < m.pending {
		m.pending = target // promotion floor: weakest sustained target
	}
	if dir < 0 && target > m.pending {
		m.pending = target // demotion ceiling: strongest target still seen
	}

	need := m.enterTicks
	if dir < 0 {
		need = m.exitTicks
	}
	if m.streak >= need {
		m.level = m.pending
		m.reset()
		return m.level, true
	}
	return m.level, false
}

func (m *Machine) reset() {
	m.streak = 0
	m.dir = 0
	m.pending = m.level
}
