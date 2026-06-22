package probe

import (
	"context"
	"errors"
	"math/rand"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

// ProbeTarget binds a provider to its anime-set selection rule and its stream
// resolver. The EN scraper providers share one spotlight AnimeSet + one HTTP
// scraper Resolver; ae and kodik-noads get custom ones. The Validator is shared
// across all targets (the HLS proxy handles signed-scraper, allowlisted-kodik
// CDN, and signed+presigned-MinIO uniformly).
type ProbeTarget struct {
	Provider string
	AnimeSet AnimeSetResolver
	Resolver Resolver
}

type Engine struct {
	targets []ProbeTarget
	val     Validator
	rep     Reporter
	pool    PopularPool
	rng     *rand.Rand
	now     func() int64
	log     *logger.Logger
}

func NewEngine(targets []ProbeTarget, val Validator, rep Reporter, pool PopularPool, rng *rand.Rand, now func() int64, log *logger.Logger) *Engine {
	return &Engine{targets: targets, val: val, rep: rep, pool: pool, rng: rng, now: now, log: log}
}

// probeProvider runs all anime refs for one target, recovering from any panic
// so a single provider can never abort the whole run. Always returns ≥1 verdict
// so the provider is never absent from the dashboard (a target whose anime-set
// is empty yields one synthetic empty_response → Rollup → Down).
func (e *Engine) probeProvider(ctx context.Context, t ProbeTarget, refs []AnimeRef) (verdicts []Verdict) {
	defer func() {
		if r := recover(); r != nil {
			if e.log != nil {
				e.log.Errorw("probe provider panicked", "provider", t.Provider, "panic", r)
			}
			// ensure the provider still produces a verdict so Rollup -> Down, not absent
			verdicts = append(verdicts, Verdict{Provider: t.Provider, Stage: StageStream, Reason: streamprobe.ReasonCDNUnreachable})
		}
	}()
	for _, ref := range refs {
		streams, stage, rerr := t.Resolver.Resolve(ctx, ref.UUID, ref.Name, ref.Episode, ref.Slot, t.Provider)
		if rerr != nil {
			if errors.Is(rerr, ErrProbeNotFound) {
				// Provider has no catalogue entry for this anime: record a zero_match
				// verdict, then attempt one re-roll with a different popular anime.
				verdicts = append(verdicts, Verdict{
					Provider: t.Provider, AnimeUUID: ref.UUID, AnimeName: ref.Name, Slot: ref.Slot,
					Stage: StageSearch, Reason: streamprobe.ReasonZeroMatch,
				})
				verdicts = append(verdicts, e.reroll(ctx, t, ref)...)
			} else {
				verdicts = append(verdicts, Verdict{
					Provider: t.Provider, AnimeUUID: ref.UUID, AnimeName: ref.Name, Slot: ref.Slot, Stage: stage,
					Reason: streamprobe.ReasonCDNUnreachable,
				})
			}
			continue
		}
		for _, s := range streams {
			verdicts = append(verdicts, e.val.Validate(ctx, s))
		}
	}
	// Guarantee at least one verdict: a target whose anime-set resolved nothing
	// (e.g. the library is empty or catalog is down) would otherwise vanish from
	// the dashboard instead of reading "down".
	if len(verdicts) == 0 {
		verdicts = append(verdicts, Verdict{Provider: t.Provider, Stage: StageEpisodes, Reason: streamprobe.ReasonEmptyResponse})
	}
	return verdicts
}

// resolveAndValidate resolves+validates one anime for a target, tagging the slot.
func (e *Engine) resolveAndValidate(ctx context.Context, t ProbeTarget, uuid, name string, slot AnimeSlot) []Verdict {
	streams, stage, err := t.Resolver.Resolve(ctx, uuid, name, 0, slot, t.Provider)
	if err != nil {
		return []Verdict{{Provider: t.Provider, AnimeUUID: uuid, AnimeName: name, Slot: slot, Stage: stage, Reason: streamprobe.ReasonCDNUnreachable}}
	}
	out := make([]Verdict, 0, len(streams))
	for _, s := range streams {
		out = append(out, e.val.Validate(ctx, s))
	}
	return out
}

// reroll picks one random pool anime (≠ exclude) and resolves+validates it under the SAME slot.
func (e *Engine) reroll(ctx context.Context, t ProbeTarget, ref AnimeRef) []Verdict {
	cands, err := e.pool.Pool(ctx)
	if err != nil || len(cands) == 0 {
		if e.log != nil {
			e.log.Warnw("probe re-roll pool unavailable", "provider", t.Provider, "error", err)
		}
		return nil
	}
	start := e.rng.Intn(len(cands))
	for i := 0; i < len(cands); i++ {
		c := cands[(start+i)%len(cands)]
		if c.UUID != ref.UUID {
			return e.resolveAndValidate(ctx, t, c.UUID, c.Name, ref.Slot)
		}
	}
	return nil
}

func (e *Engine) RunOnce(ctx context.Context) error {
	var allVerdicts []Verdict
	var provVerdicts []ProviderVerdict

	for _, t := range e.targets {
		refs, _ := t.AnimeSet.Resolve(ctx) // empty refs → synthetic verdict in probeProvider
		verdicts := e.probeProvider(ctx, t, refs)
		allVerdicts = append(allVerdicts, verdicts...)
		provVerdicts = append(provVerdicts, Rollup(t.Provider, verdicts))
	}

	return e.rep.Report(ctx, RunResult{ProviderVerdicts: provVerdicts, Verdicts: allVerdicts, At: e.now()})
}
