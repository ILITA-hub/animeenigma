package service

import "time"

// SetIDProviderForTest overrides the room-ID generator. INTERNAL TEST USE
// ONLY — exposed so the handler-package tests (which can't reach
// RoomService's unexported newID field) can pin room_id values for
// deterministic assertions. Production callers MUST NOT use this; the
// default `uuid.NewString` is the only valid value outside tests.
//
// Lives in a separate file (not in rooms.go) to keep the production
// surface area visually flat — anyone reading rooms.go sees only the
// real public API.
func (s *RoomService) SetIDProviderForTest(fn func() string) {
	s.newID = fn
}

// SetClockForTest overrides the time provider. INTERNAL TEST USE ONLY —
// pin CreatedAt / PlaybackTimeUpdatedAtMs to a deterministic instant so
// handler tests can assert exact response bodies.
func (s *RoomService) SetClockForTest(fn func() time.Time) {
	s.now = fn
}
