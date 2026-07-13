package probe

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

// ---------------------------------------------------------------------------
// Fakes shared across tests
// ---------------------------------------------------------------------------

type fakePool struct{ items []PopularAnime }

func (f fakePool) Pool(_ context.Context) ([]PopularAnime, error) { return f.items, nil }

type fakeAS struct{}

func (fakeAS) Resolve(_ context.Context) ([]AnimeRef, error) {
	return []AnimeRef{{UUID: "u", Name: "Anchor Title", Slot: SlotAnchor}}, nil
}

// emptyAS returns no refs (e.g. an empty library or a down catalog).
type emptyAS struct{}

func (emptyAS) Resolve(_ context.Context) ([]AnimeRef, error) { return nil, nil }

type fakeRes struct{}

func (fakeRes) Resolve(_ context.Context, u, name string, _ int, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
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

func target(p string, as AnimeSetResolver, res Resolver) ProbeTarget {
	return ProbeTarget{Provider: p, AnimeSet: as, Resolver: res}
}

// ---------------------------------------------------------------------------
// Fake PlanClient
// ---------------------------------------------------------------------------

// fakePlan is a PlanClient that returns a configured plan and records verdicts
// posted back via PostVerdict. Used in tests that exercise plan-gated probing.
type fakePlan struct {
	entries []PlanEntry
	// posted captures (provider, pass, reason) tuples from PostVerdict calls.
	posted []postedVerdict
	// fetchErr, if non-nil, is returned by FetchPlan to simulate a catalog outage.
	fetchErr error
}

type postedVerdict struct {
	provider string
	pass     bool
	reason   string
	metrics  *TickMetrics
}

func (f *fakePlan) FetchPlan(_ context.Context) ([]PlanEntry, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.entries, nil
}

func (f *fakePlan) PostVerdict(_ context.Context, provider string, pass bool, reason string, metrics *TickMetrics) error {
	f.posted = append(f.posted, postedVerdict{provider: provider, pass: pass, reason: reason, metrics: metrics})
	return nil
}

// errFetchPlan always returns an error from FetchPlan (catalog unreachable).
type errFetchPlan struct{}

func (errFetchPlan) FetchPlan(_ context.Context) ([]PlanEntry, error) {
	return nil, context.DeadlineExceeded
}

func (errFetchPlan) PostVerdict(_ context.Context, _ string, _ bool, _ string, _ *TickMetrics) error {
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newEngine(targets []ProbeTarget, val Validator, rep Reporter, pool PopularPool, plan PlanClient) *Engine {
	return NewEngine(targets, val, rep, pool, rand.New(rand.NewSource(1)), func() int64 { return 42 }, nil, plan)
}

// planForAll returns a fakePlan that includes every listed provider with the
// given sampleSize and failFast settings — convenience for legacy-style tests
// that don't care about plan gating but still need the engine to probe.
func planForAll(providers []string, sampleSize int, failFast bool) *fakePlan {
	entries := make([]PlanEntry, len(providers))
	for i, p := range providers {
		entries[i] = PlanEntry{Provider: p, SampleSize: sampleSize, FailFast: failFast}
	}
	return &fakePlan{entries: entries}
}

// ---------------------------------------------------------------------------
// Existing tests — updated to pass a fake plan client
// ---------------------------------------------------------------------------

func TestEngine_RunOnce(t *testing.T) {
	rep := &capRep{}
	plan := planForAll([]string{"gogoanime"}, 0, false)
	e := NewEngine([]ProbeTarget{target("gogoanime", fakeAS{}, fakeRes{})}, fakeVal{}, rep, fakePool{}, rand.New(rand.NewSource(1)), func() int64 { return 42 }, nil, plan)
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

func (r errRes) Resolve(_ context.Context, u, name string, _ int, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
	return nil, r.stage, context.DeadlineExceeded
}

func TestEngine_ResolveError_SynthesizesCDNUnreachable(t *testing.T) {
	rep := &capRep{}
	plan := planForAll([]string{"badprov"}, 0, false)
	e := NewEngine([]ProbeTarget{target("badprov", fakeAS{}, errRes{stage: StageServers})}, fakeVal{}, rep, fakePool{}, rand.New(rand.NewSource(1)), func() int64 { return 1 }, nil, plan)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rep.n != 1 {
		t.Fatalf("Reporter called %d times, want 1", rep.n)
	}
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
	if len(rep.run.ProviderVerdicts) != 1 || rep.run.ProviderVerdicts[0].Status != StatusDown {
		t.Fatalf("ProviderVerdicts=%+v, want StatusDown", rep.run.ProviderVerdicts)
	}
}

// panicRes panics for the given provider, delegates to fakeRes for all others.
type panicRes struct{ bad string }

func (r panicRes) Resolve(ctx context.Context, u, name string, ep int, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
	if p == r.bad {
		panic("injected test panic for " + p)
	}
	return fakeRes{}.Resolve(ctx, u, name, ep, s, p)
}

func TestEngine_ProviderPanic_Isolated(t *testing.T) {
	rep := &capRep{}
	pr := panicRes{bad: "bad"}
	plan := planForAll([]string{"bad", "good"}, 0, false)
	e := NewEngine([]ProbeTarget{
		target("bad", fakeAS{}, pr),
		target("good", fakeAS{}, pr),
	}, fakeVal{}, rep, fakePool{}, rand.New(rand.NewSource(1)), func() int64 { return 7 }, nil, plan)

	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rep.n != 1 {
		t.Fatalf("Reporter called %d times, want 1", rep.n)
	}
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

// recordingRes records which provider names it was asked to resolve, so the
// per-target dispatch can be asserted.
type recordingRes struct{ seen *[]string }

func (r recordingRes) Resolve(_ context.Context, u, name string, _ int, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
	*r.seen = append(*r.seen, p)
	return []ResolvedStream{{Provider: p, AnimeUUID: u, AnimeName: name, Slot: s, Server: "srv", Stage: StageStream}}, StageStream, nil
}

func TestEngine_PerTargetDispatch(t *testing.T) {
	// Two targets, each with its OWN resolver instance; the engine must call each
	// target's resolver (not a single shared one) with that target's provider.
	var seenA, seenB []string
	rep := &capRep{}
	plan := planForAll([]string{"provA", "provB"}, 0, false)
	e := NewEngine([]ProbeTarget{
		target("provA", fakeAS{}, recordingRes{seen: &seenA}),
		target("provB", fakeAS{}, recordingRes{seen: &seenB}),
	}, fakeVal{}, rep, fakePool{}, rand.New(rand.NewSource(1)), func() int64 { return 1 }, nil, plan)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(seenA) != 1 || seenA[0] != "provA" {
		t.Fatalf("resolver A saw %v, want [provA]", seenA)
	}
	if len(seenB) != 1 || seenB[0] != "provB" {
		t.Fatalf("resolver B saw %v, want [provB]", seenB)
	}
	if len(rep.run.ProviderVerdicts) != 2 {
		t.Fatalf("want 2 provider verdicts, got %d", len(rep.run.ProviderVerdicts))
	}
}

func TestEngine_EmptyAnimeSet_SyntheticDown(t *testing.T) {
	// A target whose anime-set resolves nothing must still appear, as Down, with
	// exactly one synthetic empty_response verdict (so it lands a probe_runs row).
	rep := &capRep{}
	plan := planForAll([]string{"ae"}, 0, false)
	e := NewEngine([]ProbeTarget{target("ae", emptyAS{}, fakeRes{})}, fakeVal{}, rep, fakePool{}, rand.New(rand.NewSource(1)), func() int64 { return 1 }, nil, plan)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rep.run.Verdicts) != 1 {
		t.Fatalf("verdicts = %d, want 1 synthetic", len(rep.run.Verdicts))
	}
	if rep.run.Verdicts[0].Reason != streamprobe.ReasonEmptyResponse {
		t.Fatalf("synthetic reason = %v, want empty_response", rep.run.Verdicts[0].Reason)
	}
	if len(rep.run.ProviderVerdicts) != 1 || rep.run.ProviderVerdicts[0].Status != StatusDown {
		t.Fatalf("provider verdict = %+v, want Down", rep.run.ProviderVerdicts)
	}
}

// notFoundThenPlayRes returns ErrProbeNotFound for the anchor UUID, and a
// playable stream for any other UUID (the re-roll candidate).
type notFoundThenPlayRes struct{ anchorUUID string }

func (r notFoundThenPlayRes) Resolve(_ context.Context, u, name string, _ int, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
	if u == r.anchorUUID {
		return nil, StageSearch, ErrProbeNotFound
	}
	return []ResolvedStream{{Provider: p, AnimeUUID: u, AnimeName: name, Slot: s, Server: "srv", Stage: StageStream}}, StageStream, nil
}

func TestEngine_NotFound_RerollsOnce_Pass(t *testing.T) {
	const anchorUUID = "anchor-uuid"
	const poolUUID = "pool-uuid"

	pool := fakePool{items: []PopularAnime{{UUID: poolUUID, Name: "Pool Anime"}}}
	res := notFoundThenPlayRes{anchorUUID: anchorUUID}

	as := struct{ AnimeSetResolver }{}
	as.AnimeSetResolver = singleRefAS{ref: AnimeRef{UUID: anchorUUID, Name: "Anchor Title", Slot: SlotAnchor}}

	rep := &capRep{}
	plan := planForAll([]string{"gogoanime"}, 0, false)
	e := NewEngine(
		[]ProbeTarget{target("gogoanime", as.AnimeSetResolver, res)},
		fakeVal{}, rep, pool, rand.New(rand.NewSource(1)),
		func() int64 { return 1 }, nil, plan,
	)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Must have a zero_match verdict for the anchor UUID.
	var hasZeroMatch, hasPlayable bool
	var playableUUID string
	for _, v := range rep.run.Verdicts {
		if v.Reason == streamprobe.ReasonZeroMatch && v.Stage == StageSearch && v.AnimeUUID == anchorUUID {
			hasZeroMatch = true
		}
		if v.Playable() {
			hasPlayable = true
			playableUUID = v.AnimeUUID
		}
	}
	if !hasZeroMatch {
		t.Fatalf("expected a zero_match/StageSearch verdict for the anchor; got verdicts: %+v", rep.run.Verdicts)
	}
	if !hasPlayable {
		t.Fatalf("expected a playable re-roll verdict; got verdicts: %+v", rep.run.Verdicts)
	}
	if playableUUID != poolUUID {
		t.Fatalf("playable verdict uuid = %q, want %q", playableUUID, poolUUID)
	}
	if len(rep.run.ProviderVerdicts) != 1 || rep.run.ProviderVerdicts[0].Status != StatusUp {
		t.Fatalf("provider status = %+v, want StatusUp", rep.run.ProviderVerdicts)
	}
}

// TestEngine_NotFound_RerollSucceeds_PostsPassTrue verifies that when the
// anchor (index-0, the only ref under today's sample_size=1/fail_fast=false
// "down"-state plan config — see ProbeSample) misses with ErrProbeNotFound
// but the one-shot re-roll resolves+validates to a playable stream, the
// engine posts pass=true to the catalog state machine — not just a healthy
// Rollup status. Rollup and PostVerdict must agree: a niche/sparse-catalog
// provider (e.g. nineanime) whose anchor title happens to be missing, but
// which serves real content for other titles, must not be pinned "down"
// forever by a discarded re-roll success.
func TestEngine_NotFound_RerollSucceeds_PostsPassTrue(t *testing.T) {
	const anchorUUID = "anchor-uuid"
	const poolUUID = "pool-uuid"

	pool := fakePool{items: []PopularAnime{{UUID: poolUUID, Name: "Pool Anime"}}}
	res := notFoundThenPlayRes{anchorUUID: anchorUUID}
	as := singleRefAS{ref: AnimeRef{UUID: anchorUUID, Name: "Anchor Title", Slot: SlotAnchor}}

	rep := &capRep{}
	// Mirrors ProbeSample's real "down" config: sample_size=1, fail_fast=false.
	plan := &fakePlan{entries: []PlanEntry{{Provider: "nineanime", SampleSize: 1, FailFast: false}}}
	e := newEngine([]ProbeTarget{target("nineanime", as, res)}, fakeVal{}, rep, pool, plan)

	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(plan.posted) != 1 {
		t.Fatalf("posted verdicts = %+v, want exactly 1", plan.posted)
	}
	if !plan.posted[0].pass {
		t.Fatalf("posted pass = false, want true (re-roll produced a playable stream): %+v", plan.posted[0])
	}
}

// singleRefAS is an AnimeSetResolver returning exactly one ref.
type singleRefAS struct{ ref AnimeRef }

func (s singleRefAS) Resolve(_ context.Context) ([]AnimeRef, error) {
	return []AnimeRef{s.ref}, nil
}

// ---------------------------------------------------------------------------
// New tests — plan-gated fail-fast probing + verdict POST
// ---------------------------------------------------------------------------

// multiRefAS returns multiple refs for testing sampleSize and failFast.
type multiRefAS struct{ refs []AnimeRef }

func (m multiRefAS) Resolve(_ context.Context) ([]AnimeRef, error) { return m.refs, nil }

// firstFailRes returns an error for the first ref UUID, succeeds for all others.
type firstFailRes struct{ firstUUID string }

func (r firstFailRes) Resolve(_ context.Context, u, name string, _ int, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
	if u == r.firstUUID {
		return nil, StageServers, context.DeadlineExceeded
	}
	return []ResolvedStream{{Provider: p, AnimeUUID: u, AnimeName: name, Slot: s, Server: "srv", Stage: StageStream}}, StageStream, nil
}

// TestEngine_FailFast_TopRefFails_PostsFalse verifies that when failFast=true and
// the first (top) ref fails, the engine:
//   - posts pass=false to the plan client
//   - appends StageNotTried verdicts for all remaining refs in the sample window
//   - those not_tried verdicts are excluded from Rollup (so Rollup only scores the one failed ref)
func TestEngine_FailFast_TopRefFails_PostsFalse(t *testing.T) {
	refs := []AnimeRef{
		{UUID: "ref-0", Name: "Anime A", Slot: SlotAnchor},
		{UUID: "ref-1", Name: "Anime B", Slot: SlotFeatured},
		{UUID: "ref-2", Name: "Anime C", Slot: SlotRandom},
	}
	res := firstFailRes{firstUUID: "ref-0"}
	plan := &fakePlan{entries: []PlanEntry{{Provider: "nineanime", SampleSize: 3, FailFast: true}}}
	rep := &capRep{}

	e := NewEngine(
		[]ProbeTarget{target("nineanime", multiRefAS{refs: refs}, res)},
		fakeVal{}, rep, fakePool{}, rand.New(rand.NewSource(1)),
		func() int64 { return 1 }, nil, plan,
	)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// 1) Provider verdict should be posted with pass=false.
	if len(plan.posted) != 1 {
		t.Fatalf("expected 1 PostVerdict call, got %d", len(plan.posted))
	}
	pv := plan.posted[0]
	if pv.provider != "nineanime" {
		t.Errorf("PostVerdict provider=%q, want nineanime", pv.provider)
	}
	if pv.pass {
		t.Error("PostVerdict pass=true, want false")
	}

	// 2) Verify the full verdict set contains not_tried entries for refs 1 and 2.
	allVerdicts := rep.run.Verdicts
	var notTriedCount int
	for _, v := range allVerdicts {
		if v.Stage == StageNotTried {
			notTriedCount++
		}
	}
	if notTriedCount != 2 {
		t.Errorf("expected 2 not_tried verdicts (refs 1 and 2), got %d; all: %+v", notTriedCount, allVerdicts)
	}

	// 3) Rollup sees only the failed ref (not the not_tried ones) → Down.
	if len(rep.run.ProviderVerdicts) != 1 || rep.run.ProviderVerdicts[0].Status != StatusDown {
		t.Errorf("expected ProviderVerdict=Down, got %+v", rep.run.ProviderVerdicts)
	}
}

// TestEngine_FailFast_AllPlayable_PostsTrue verifies that when all refs play
// the engine posts pass=true and there are no not_tried verdicts.
func TestEngine_FailFast_AllPlayable_PostsTrue(t *testing.T) {
	refs := []AnimeRef{
		{UUID: "ref-0", Name: "Anime A", Slot: SlotAnchor},
		{UUID: "ref-1", Name: "Anime B", Slot: SlotFeatured},
	}
	plan := &fakePlan{entries: []PlanEntry{{Provider: "gogoanime", SampleSize: 2, FailFast: true}}}
	rep := &capRep{}

	e := NewEngine(
		[]ProbeTarget{target("gogoanime", multiRefAS{refs: refs}, fakeRes{})},
		fakeVal{}, rep, fakePool{}, rand.New(rand.NewSource(1)),
		func() int64 { return 1 }, nil, plan,
	)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// 1) pass=true was posted.
	if len(plan.posted) != 1 {
		t.Fatalf("expected 1 PostVerdict call, got %d", len(plan.posted))
	}
	if !plan.posted[0].pass {
		t.Errorf("PostVerdict pass=false, want true")
	}

	// 2) No not_tried verdicts — all refs were probed successfully.
	for _, v := range rep.run.Verdicts {
		if v.Stage == StageNotTried {
			t.Errorf("unexpected not_tried verdict: %+v", v)
		}
	}

	// 3) Provider should be Up.
	if len(rep.run.ProviderVerdicts) != 1 || rep.run.ProviderVerdicts[0].Status != StatusUp {
		t.Errorf("expected ProviderVerdict=Up, got %+v", rep.run.ProviderVerdicts)
	}
}

// TestEngine_TargetNotInPlan_NotProbed verifies that a target absent from the
// plan map is completely skipped — no verdicts and no PostVerdict call.
func TestEngine_TargetNotInPlan_NotProbed(t *testing.T) {
	// Plan only lists "gogoanime"; "allanime" is present as a target but absent from the plan.
	plan := &fakePlan{entries: []PlanEntry{{Provider: "gogoanime", SampleSize: 1, FailFast: false}}}
	rep := &capRep{}

	var seenAllanime []string
	e := NewEngine(
		[]ProbeTarget{
			target("gogoanime", fakeAS{}, fakeRes{}),
			target("allanime", fakeAS{}, recordingRes{seen: &seenAllanime}),
		},
		fakeVal{}, rep, fakePool{}, rand.New(rand.NewSource(1)),
		func() int64 { return 1 }, nil, plan,
	)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// allanime resolver should never have been called.
	if len(seenAllanime) != 0 {
		t.Errorf("allanime resolver was called %d times, want 0; seen: %v", len(seenAllanime), seenAllanime)
	}
	// Only one PostVerdict (for gogoanime).
	if len(plan.posted) != 1 || plan.posted[0].provider != "gogoanime" {
		t.Errorf("PostVerdict calls = %+v, want exactly 1 for gogoanime", plan.posted)
	}
	// Only one ProviderVerdict in the report (gogoanime).
	if len(rep.run.ProviderVerdicts) != 1 {
		t.Errorf("ProviderVerdicts count = %d, want 1", len(rep.run.ProviderVerdicts))
	}
	if rep.run.ProviderVerdicts[0].Provider != "gogoanime" {
		t.Errorf("ProviderVerdict provider = %q, want gogoanime", rep.run.ProviderVerdicts[0].Provider)
	}
}

// TestProbeProvider_EmptyRefs_NeverPass verifies that when the anime-set resolves
// to zero refs, probeProvider always returns pass=false regardless of failFast —
// an empty sample must never feed a recovering/promote signal to the state machine.
func TestProbeProvider_EmptyRefs_NeverPass(t *testing.T) {
	e := newEngine(nil, fakeVal{}, &capRep{}, fakePool{}, &fakePlan{})
	tgt := target("ae", emptyAS{}, fakeRes{})

	for _, ff := range []bool{true, false} {
		_, pass, _ := e.probeProvider(context.Background(), tgt, nil, 0, ff, false)
		if pass {
			t.Errorf("failFast=%v: empty refs returned pass=true, want false", ff)
		}
	}
}

// TestEngine_PlanFetchError_FallbackLegacy verifies that when FetchPlan errors,
// the engine falls back to probing ALL targets (no plan gating) and does NOT
// call PostVerdict.
func TestEngine_PlanFetchError_FallbackLegacy(t *testing.T) {
	var seenA, seenB []string
	rep := &capRep{}

	e := NewEngine(
		[]ProbeTarget{
			target("provA", fakeAS{}, recordingRes{seen: &seenA}),
			target("provB", fakeAS{}, recordingRes{seen: &seenB}),
		},
		fakeVal{}, rep, fakePool{}, rand.New(rand.NewSource(1)),
		func() int64 { return 1 }, nil, errFetchPlan{},
	)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Both targets probed despite plan error.
	if len(seenA) == 0 {
		t.Error("provA resolver not called during fallback")
	}
	if len(seenB) == 0 {
		t.Error("provB resolver not called during fallback")
	}
	// Two provider verdicts in the report.
	if len(rep.run.ProviderVerdicts) != 2 {
		t.Errorf("ProviderVerdicts count = %d, want 2", len(rep.run.ProviderVerdicts))
	}
}

// ---------------------------------------------------------------------------
// Warmup (browser-engine providers)
// ---------------------------------------------------------------------------

// resolverFunc adapts a plain function to the Resolver interface for one-off
// counting/behavior fakes.
type resolverFunc func(context.Context, string, string, int, AnimeSlot, string) ([]ResolvedStream, Stage, error)

func (f resolverFunc) Resolve(ctx context.Context, uuid, name string, ep int, slot AnimeSlot, prov string) ([]ResolvedStream, Stage, error) {
	return f(ctx, uuid, name, ep, slot, prov)
}

// validatorFunc adapts a plain function to the Validator interface for one-off
// fakes that need to return specific measurement fields on the Verdict.
type validatorFunc func(context.Context, ResolvedStream) Verdict

func (f validatorFunc) Validate(ctx context.Context, rs ResolvedStream) Verdict { return f(ctx, rs) }

func TestProbeProviderAssemblesMeasure(t *testing.T) {
	res := resolverFunc(func(_ context.Context, uuid, name string, ep int, slot AnimeSlot, prov string) ([]ResolvedStream, Stage, error) {
		return []ResolvedStream{{Provider: prov, AnimeUUID: uuid, AnimeName: name, Slot: slot, MasterURL: "https://cdn.test/m.m3u8"}}, StageStream, nil
	})
	val := validatorFunc(func(_ context.Context, rs ResolvedStream) Verdict {
		return Verdict{Provider: rs.Provider, AnimeName: rs.AnimeName, Slot: rs.Slot, Stage: StagePlayback,
			Reason: streamprobe.ReasonPlayable, ManifestMs: 40, SegmentMs: 20, SegmentBytes: 250000, CDNHost: "cdn.test", Quality: "720p"}
	})
	e := &Engine{val: val, now: func() int64 { return 0 }}
	tgt := ProbeTarget{Provider: "miruro", Resolver: res}
	refs := []AnimeRef{{UUID: "u1", Name: "Frieren", Slot: SlotAnchor}}

	_, pass, meas := e.probeProvider(context.Background(), tgt, refs, 1, true, false)
	if !pass {
		t.Fatal("expected pass")
	}
	if meas.CDNHost != "cdn.test" || meas.Quality != "720p" || meas.Anime != "Frieren" {
		t.Fatalf("bad meta: %+v", meas)
	}
	if meas.ValidateMs != 60 { // ManifestMs + SegmentMs
		t.Fatalf("ValidateMs = %d, want 60", meas.ValidateMs)
	}
	// throughput = bytes*8/segmentMs = 250000*8/20 = 100000 kbps
	if meas.ThroughputKbps != 100000 {
		t.Fatalf("ThroughputKbps = %d, want 100000", meas.ThroughputKbps)
	}
	if meas.SampleSize != 1 {
		t.Fatalf("SampleSize = %d, want 1", meas.SampleSize)
	}
}

func TestWarmupResolvesTopRefBestEffort(t *testing.T) {
	var calls int
	res := resolverFunc(func(_ context.Context, uuid, name string, ep int, slot AnimeSlot, prov string) ([]ResolvedStream, Stage, error) {
		calls++
		return nil, StageStream, errors.New("cold solve failed") // best-effort: error must be swallowed
	})
	e := &Engine{now: func() int64 { return 0 }}
	tgt := ProbeTarget{Provider: "miruro", Resolver: res}
	refs := []AnimeRef{{UUID: "u1", Name: "Frieren", Slot: SlotAnchor}}

	ms := e.warmup(context.Background(), tgt, refs)
	if calls != 1 {
		t.Fatalf("warmup resolve calls = %d, want 1", calls)
	}
	if ms < 0 {
		t.Fatalf("warmup ms = %d, want >= 0", ms)
	}
	// No refs → no resolve, zero ms.
	calls = 0
	if got := e.warmup(context.Background(), tgt, nil); got != 0 || calls != 0 {
		t.Fatalf("empty warmup: ms=%d calls=%d, want 0/0", got, calls)
	}
}

// ---------------------------------------------------------------------------
// Resolve retry (flaky browser cold-resolve resilience)
// ---------------------------------------------------------------------------

// TestProbeProvider_BrowserRetry_TransientResolveError_Recovers verifies that with
// retryOnResolveErr=true (browser engine), a resolve that fails transiently on the
// first attempt and succeeds on the second yields a PASS — one Turnstile hiccup no
// longer false-negatives a working provider.
func TestProbeProvider_BrowserRetry_TransientResolveError_Recovers(t *testing.T) {
	var calls int
	res := resolverFunc(func(_ context.Context, uuid, name string, _ int, slot AnimeSlot, prov string) ([]ResolvedStream, Stage, error) {
		calls++
		if calls == 1 {
			return nil, StageStream, context.DeadlineExceeded // transient cold-resolve hiccup
		}
		return []ResolvedStream{{Provider: prov, AnimeUUID: uuid, AnimeName: name, Slot: slot, Server: "srv", Stage: StageStream}}, StageStream, nil
	})
	e := &Engine{val: fakeVal{}, now: func() int64 { return 0 }}
	tgt := ProbeTarget{Provider: "miruro", Resolver: res}
	refs := []AnimeRef{{UUID: "u1", Name: "Frieren", Slot: SlotAnchor}}

	_, pass, _ := e.probeProvider(context.Background(), tgt, refs, 1, true, true) // retryOnResolveErr=true
	if !pass {
		t.Fatalf("expected pass after one retry recovered the transient error; calls=%d", calls)
	}
	if calls != 2 {
		t.Fatalf("resolve calls=%d, want 2 (1 transient fail + 1 successful retry)", calls)
	}
}

// TestProbeProvider_NoRetry_TransientResolveError_Fails verifies that with
// retryOnResolveErr=false (http engine — the default), the transient error is the
// verdict: no retry, provider reported not-played. Guards against regressing the
// retry into an always-on double-resolve.
func TestProbeProvider_NoRetry_TransientResolveError_Fails(t *testing.T) {
	var calls int
	res := resolverFunc(func(_ context.Context, uuid, name string, _ int, slot AnimeSlot, prov string) ([]ResolvedStream, Stage, error) {
		calls++
		if calls == 1 {
			return nil, StageStream, context.DeadlineExceeded
		}
		return []ResolvedStream{{Provider: prov, AnimeUUID: uuid, AnimeName: name, Slot: slot, Server: "srv", Stage: StageStream}}, StageStream, nil
	})
	e := &Engine{val: fakeVal{}, now: func() int64 { return 0 }}
	tgt := ProbeTarget{Provider: "gogoanime", Resolver: res}
	refs := []AnimeRef{{UUID: "u1", Name: "Frieren", Slot: SlotAnchor}}

	_, pass, _ := e.probeProvider(context.Background(), tgt, refs, 1, true, false) // retryOnResolveErr=false
	if pass {
		t.Fatal("expected fail without retry (transient error is the verdict)")
	}
	if calls != 1 {
		t.Fatalf("resolve calls=%d, want 1 (no retry)", calls)
	}
}

// TestProbeProvider_Retry_NotFoundNotRetried verifies that ErrProbeNotFound is
// definitive: even with retryOnResolveErr=true the anchor is resolved exactly once
// (then the reroll path handles it) — retrying the same missing title is pointless.
func TestProbeProvider_Retry_NotFoundNotRetried(t *testing.T) {
	var anchorCalls int
	res := resolverFunc(func(_ context.Context, uuid, name string, _ int, slot AnimeSlot, prov string) ([]ResolvedStream, Stage, error) {
		if uuid == "anchor" {
			anchorCalls++
			return nil, StageSearch, ErrProbeNotFound
		}
		return []ResolvedStream{{Provider: prov, AnimeUUID: uuid, AnimeName: name, Slot: slot, Server: "srv", Stage: StageStream}}, StageStream, nil
	})
	e := &Engine{val: fakeVal{}, pool: fakePool{items: []PopularAnime{{UUID: "pool", Name: "Pool"}}}, rng: rand.New(rand.NewSource(1)), now: func() int64 { return 0 }}
	tgt := ProbeTarget{Provider: "miruro", Resolver: res}
	refs := []AnimeRef{{UUID: "anchor", Name: "Anchor", Slot: SlotAnchor}}

	_, _, _ = e.probeProvider(context.Background(), tgt, refs, 1, true, true) // retryOnResolveErr=true
	if anchorCalls != 1 {
		t.Fatalf("anchor resolve calls=%d, want 1 (ErrProbeNotFound must not be retried)", anchorCalls)
	}
}
