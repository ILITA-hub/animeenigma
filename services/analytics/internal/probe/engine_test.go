package probe

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

type fakeAS struct{}

func (fakeAS) Resolve(_ context.Context) ([]AnimeRef, error) {
	return []AnimeRef{{UUID: "u", Slot: SlotAnchor}}, nil
}

type fakeRes struct{}

func (fakeRes) Resolve(_ context.Context, u string, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
	return []ResolvedStream{{Provider: p, AnimeUUID: u, Slot: s, Server: "srv", Stage: StageStream}}, StageStream, nil
}

type fakeVal struct{}

func (fakeVal) Validate(_ context.Context, rs ResolvedStream) Verdict {
	return Verdict{Provider: rs.Provider, AnimeUUID: rs.AnimeUUID, Slot: rs.Slot, Server: rs.Server, Stage: StagePlayback, Reason: streamprobe.ReasonPlayable}
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
}
