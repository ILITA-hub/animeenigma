package probe

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

type fakeAS struct{}

func (fakeAS) Resolve(_ context.Context) ([]AnimeRef, error) {
	return []AnimeRef{{UUID: "u", Name: "Anchor Title", Slot: SlotAnchor}}, nil
}

type fakeRes struct{}

func (fakeRes) Resolve(_ context.Context, u, name string, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
	return []ResolvedStream{{Provider: p, AnimeUUID: u, AnimeName: name, Slot: s, Server: "srv", Stage: StageStream}}, StageStream, nil
}

type fakeVal struct{}

func (fakeVal) Validate(_ context.Context, rs ResolvedStream) Verdict {
	return Verdict{Provider: rs.Provider, AnimeUUID: rs.AnimeUUID, AnimeName: rs.AnimeName, Slot: rs.Slot, Server: rs.Server, Stage: StagePlayback, Reason: streamprobe.ReasonPlayable}
}

type capRep struct {
	run RunResult
	n   int
}

func (c *capRep) Report(_ context.Context, run RunResult) error {
	c.run = run
	c.n++
	return nil
}

func TestEngine_RunOnce(t *testing.T) {
	rep := &capRep{}
	e := NewEngine([]string{"gogoanime"}, fakeAS{}, fakeRes{}, fakeVal{}, rep, func() int64 { return 42 }, nil)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rep.n != 1 || len(rep.run.ProviderVerdicts) != 1 || rep.run.ProviderVerdicts[0].Status != StatusUp {
		t.Fatalf("got %+v (n=%d)", rep.run, rep.n)
	}
	if rep.run.At != 42 {
		t.Fatalf("At=%d", rep.run.At)
	}
	// AnimeName must flow AnimeRef -> Resolve -> ResolvedStream -> Verdict.
	if len(rep.run.Verdicts) == 0 || rep.run.Verdicts[0].AnimeName != "Anchor Title" {
		t.Fatalf("AnimeName not propagated to verdict: %+v", rep.run.Verdicts)
	}
}

// errRes is a Resolver that always returns an error at the given stage.
type errRes struct{ stage Stage }

func (r errRes) Resolve(_ context.Context, u, name string, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
	return nil, r.stage, context.DeadlineExceeded
}

func TestEngine_ResolveError_SynthesizesCDNUnreachable(t *testing.T) {
	rep := &capRep{}
	e := NewEngine([]string{"badprov"}, fakeAS{}, errRes{stage: StageServers}, fakeVal{}, rep, func() int64 { return 1 }, nil)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Reporter must be called exactly once.
	if rep.n != 1 {
		t.Fatalf("Reporter called %d times, want 1", rep.n)
	}
	// Must produce at least one verdict for the provider.
	if len(rep.run.Verdicts) == 0 {
		t.Fatal("expected at least one verdict, got none")
	}
	v := rep.run.Verdicts[0]
	if v.Reason != streamprobe.ReasonCDNUnreachable {
		t.Fatalf("Reason=%v, want ReasonCDNUnreachable", v.Reason)
	}
	if v.Stage != StageServers {
		t.Fatalf("Stage=%v, want StageServers", v.Stage)
	}
	// Provider must roll up to StatusDown.
	if len(rep.run.ProviderVerdicts) != 1 || rep.run.ProviderVerdicts[0].Status != StatusDown {
		t.Fatalf("ProviderVerdicts=%+v, want StatusDown", rep.run.ProviderVerdicts)
	}
}

// panicRes panics for the given provider, delegates to fakeRes for all others.
type panicRes struct{ bad string }

func (r panicRes) Resolve(ctx context.Context, u, name string, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
	if p == r.bad {
		panic("injected test panic for " + p)
	}
	return fakeRes{}.Resolve(ctx, u, name, s, p)
}

func TestEngine_ProviderPanic_Isolated(t *testing.T) {
	rep := &capRep{}
	e := NewEngine([]string{"bad", "good"}, fakeAS{}, panicRes{bad: "bad"}, fakeVal{}, rep, func() int64 { return 7 }, nil)

	// RunOnce must not propagate the panic.
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Reporter called exactly once.
	if rep.n != 1 {
		t.Fatalf("Reporter called %d times, want 1", rep.n)
	}
	// Must have verdicts for both providers.
	if len(rep.run.ProviderVerdicts) != 2 {
		t.Fatalf("ProviderVerdicts count=%d, want 2", len(rep.run.ProviderVerdicts))
	}
	pvByName := make(map[string]ProviderVerdict, 2)
	for _, pv := range rep.run.ProviderVerdicts {
		pvByName[pv.Provider] = pv
	}
	if pvByName["bad"].Status != StatusDown {
		t.Fatalf("provider 'bad' status=%v, want StatusDown", pvByName["bad"].Status)
	}
	if pvByName["good"].Status != StatusUp {
		t.Fatalf("provider 'good' status=%v, want StatusUp", pvByName["good"].Status)
	}
}
