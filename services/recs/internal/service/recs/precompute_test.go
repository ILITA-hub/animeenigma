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

type sharedCtxKey struct{ id SignalID }

// sharedSignal is a SignalModule that also implements SharedPrecomputer. It
// counts PrecomputeShared invocations and seeds a value into ctx so callers
// can confirm the chained context is threaded through.
type sharedSignal struct {
	trackingSignal
	sharedCalls int
	sharedErr   error
}

func (s *sharedSignal) PrecomputeShared(ctx context.Context) (context.Context, error) {
	s.sharedCalls++
	if s.sharedErr != nil {
		return ctx, s.sharedErr // return parent ctx unchanged on error
	}
	return context.WithValue(ctx, sharedCtxKey{s.id}, "seeded"), nil
}

func TestOrchestrator_BuildSharedContext_RunsSharedPrecomputersOnce(t *testing.T) {
	shared := &sharedSignal{trackingSignal: trackingSignal{id: "s5"}}
	plain := &trackingSignal{id: "s2"} // not a SharedPrecomputer
	o := NewOrchestrator([]SignalModule{plain, shared})

	ctx, err := o.BuildSharedContext(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if shared.sharedCalls != 1 {
		t.Errorf("PrecomputeShared calls=%d want 1 (once per sweep)", shared.sharedCalls)
	}
	if ctx.Value(sharedCtxKey{"s5"}) != "seeded" {
		t.Errorf("BuildSharedContext must thread the enriched context from the shared precomputer")
	}
}

func TestOrchestrator_BuildSharedContext_ErrorIsNonFatalAndKeepsParentCtx(t *testing.T) {
	wantErr := errors.New("idf query failed")
	shared := &sharedSignal{trackingSignal: trackingSignal{id: "s5"}, sharedErr: wantErr}
	o := NewOrchestrator([]SignalModule{shared})

	ctx, err := o.BuildSharedContext(context.Background())
	if !errors.Is(err, wantErr) {
		t.Errorf("err=%v want wraps %v", err, wantErr)
	}
	// On error the parent ctx is returned unchanged (no seeded value) so the
	// per-user inline fallback path runs.
	if ctx.Value(sharedCtxKey{"s5"}) != nil {
		t.Errorf("a failed shared precompute must NOT seed a value (callers fall back to inline)")
	}
}

func TestOrchestrator_BuildSharedContext_NoSharedPrecomputers(t *testing.T) {
	plain := &trackingSignal{id: "s2"}
	o := NewOrchestrator([]SignalModule{plain})

	ctx, err := o.BuildSharedContext(context.Background())
	if err != nil {
		t.Errorf("registry with no SharedPrecomputers must not error: %v", err)
	}
	if ctx == nil {
		t.Errorf("BuildSharedContext must always return a usable context")
	}
}
