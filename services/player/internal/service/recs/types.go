// Package recs is the player service's recommendations engine.
// See docs/superpowers/specs/2026-05-03-rec-engine-design.md for the
// full design spec. Phase 1 wires up the interface and aggregator only;
// concrete signals land in Phase 2 and beyond.
package recs

// AnimeID identifies an anime by UUID string. Type alias matches existing
// domain conventions (see services/player/internal/domain/watch.go).
type AnimeID = string

// UserID identifies a user by UUID string.
type UserID = string

// SignalID is a stable identifier for a signal module ("s1", "s5", "s11").
// Used in admin debug breakdowns and the weight registry.
type SignalID string

// RawScore is the unnormalized output of a signal's Score method.
// Pre-normalization values can sit at any scale (e.g. S1 ~[0,10], S5 ~[0,0.05]).
// MinMaxNormalize collapses these to [0, 1] over a candidate pool.
type RawScore float64

// NormalizedScore is the per-pool min-max normalized score in [0, 1].
type NormalizedScore float64
