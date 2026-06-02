package domain

import "testing"

func TestEvent_Validate(t *testing.T) {
	base := func() Event {
		return Event{
			EventType:   EventTypePageview,
			AnonymousID: "anon-1",
			SessionID:   "sess-1",
		}
	}

	t.Run("valid pageview passes", func(t *testing.T) {
		e := base()
		if err := e.Validate(); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})

	t.Run("missing anonymous_id fails", func(t *testing.T) {
		e := base()
		e.AnonymousID = ""
		if err := e.Validate(); err == nil {
			t.Fatal("expected error for empty anonymous_id")
		}
	})

	t.Run("missing session_id fails", func(t *testing.T) {
		e := base()
		e.SessionID = ""
		if err := e.Validate(); err == nil {
			t.Fatal("expected error for empty session_id")
		}
	})

	t.Run("unknown event_type fails", func(t *testing.T) {
		e := base()
		e.EventType = "bogus"
		if err := e.Validate(); err == nil {
			t.Fatal("expected error for unknown event_type")
		}
	})
}
