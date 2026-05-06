package recs

import (
	"context"
	"errors"
	"testing"
)

// trackingSignal records every Precompute call for assertion.
type trackingSignal struct {
	id    SignalID
	calls []UserID
	err   error
}

func (t *trackingSignal) ID() SignalID { return t.id }
func (t *trackingSignal) Precompute(_ context.Context, userID UserID) error {
	t.calls = append(t.calls, userID)
	return t.err
}
func (t *trackingSignal) Score(_ context.Context, _ UserID, _ []AnimeID) (map[AnimeID]RawScore, error) {
	return nil, nil
}

func TestOrchestrator_RunForUserCallsAllModules(t *testing.T) {
	a := &trackingSignal{id: "s1"}
	b := &trackingSignal{id: "s2"}
	o := NewOrchestrator([]SignalModule{a, b})

	if err := o.RunForUser(context.Background(), "user-1"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(a.calls) != 1 || a.calls[0] != "user-1" {
		t.Errorf("a.calls=%v want [user-1]", a.calls)
	}
	if len(b.calls) != 1 || b.calls[0] != "user-1" {
		t.Errorf("b.calls=%v want [user-1]", b.calls)
	}
}

func TestOrchestrator_RunForUserPropagatesError(t *testing.T) {
	want := errors.New("boom")
	bad := &trackingSignal{id: "s1", err: want}
	good := &trackingSignal{id: "s2"}
	o := NewOrchestrator([]SignalModule{bad, good})

	err := o.RunForUser(context.Background(), "user-1")
	if !errors.Is(err, want) {
		t.Errorf("err=%v want wraps %v", err, want)
	}
	// Even on error, the second module must still have been called —
	// orchestrator collects errors rather than short-circuiting, so a slow
	// or failing signal can't block fresh data for others.
	if len(good.calls) != 1 {
		t.Errorf("good.calls=%v want 1 call (orchestrator must not short-circuit)", good.calls)
	}
}

func TestOrchestrator_RunForUserNoModules(t *testing.T) {
	o := NewOrchestrator(nil)
	if err := o.RunForUser(context.Background(), "user-1"); err != nil {
		t.Errorf("empty registry must not error: %v", err)
	}
}
