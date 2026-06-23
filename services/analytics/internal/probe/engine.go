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
	plan    PlanClient
}

func NewEngine(targets []ProbeTarget, val Validator, rep Reporter, pool PopularPool, rng *rand.Rand, now func() int64, log *logger.Logger, plan PlanClient) *Engine {
	return &Engine{targets: targets, val: val, rep: rep, pool: pool, rng: rng, now: now, log: log, plan: plan}
}

// filterProbed drops StageNotTried verdicts — they represent refs skipped under
// fail_fast and must not count as failed slots in the Rollup scorer.
func filterProbed(verdicts []Verdict) []Verdict {
	out := verdicts[:0:0] // nil-ish but avoids allocation if all are probed
	for _, v := range verdicts {
		if v.Stage != StageNotTried {
			out = append(out, v)
		}
	}
	return out
}

// probeProvider runs anime refs for one target, recovering from any panic
// so a single provider can never abort the whole run. Always returns ≥1 verdict
// so the provider is never absent from the dashboard (a target whose anime-set
// is empty yields one synthetic empty_response → Rollup → Down).
//
// When sampleSize > 0, only the first min(sampleSize, len(refs)) refs are probed.
// When failFast is true, the first ref that fails makes all remaining refs receive
// a StageNotTried verdict and probing stops early. pass is true when all probed
// refs played (failFast=true) or when the top (index-0) ref played (failFast=false).
func (e *Engine) probeProvider(ctx context.Context, t ProbeTarget, refs []AnimeRef, sampleSize int, failFast bool) (verdicts []Verdict, pass bool) {
	defer func() {
		if r := recover(); r != nil {
			if e.log != nil {
				e.log.Errorw("probe provider panicked", "provider", t.Provider, "panic", r)
			}
			// ensure the provider still produces a verdict so Rollup -> Down, not absent
			verdicts = append(verdicts, Verdict{Provider: t.Provider, Stage: StageStream, Reason: streamprobe.ReasonCDNUnreachable})
		}
	}()

	// Determine how many refs to probe.
	n := len(refs)
	if sampleSize > 0 && sampleSize < n {
		n = sampleSize
	}

	allPlayed := true  // AND of every probed ref's played; start optimistic
	topPlayed := false // whether ref[0] was playable

	for i := 0; i < n; i++ {
		ref := refs[i]
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
			// This ref did not play.
			allPlayed = false
			if failFast {
				// Mark all remaining probed-window refs as not_tried and stop.
				for _, remaining := range refs[i+1 : n] {
					verdicts = append(verdicts, Verdict{
						Provider: t.Provider, AnimeUUID: remaining.UUID, AnimeName: remaining.Name,
						Slot: remaining.Slot, Stage: StageNotTried,
					})
				}
				break
			}
			continue
		}

		// Resolve succeeded — collect validate verdicts and check playability.
		refVerdicts := make([]Verdict, 0, len(streams))
		for _, s := range streams {
			refVerdicts = append(refVerdicts, e.val.Validate(ctx, s))
		}
		verdicts = append(verdicts, refVerdicts...)

		// A ref "played" if resolve succeeded AND at least one validate verdict is playable.
		played := false
		for _, rv := range refVerdicts {
			if rv.Reason == streamprobe.ReasonPlayable {
				played = true
				break
			}
		}
		if i == 0 {
			topPlayed = played
		}
		if !played {
			allPlayed = false
			if failFast {
				for _, remaining := range refs[i+1 : n] {
					verdicts = append(verdicts, Verdict{
						Provider: t.Provider, AnimeUUID: remaining.UUID, AnimeName: remaining.Name,
						Slot: remaining.Slot, Stage: StageNotTried,
					})
				}
				break
			}
		}
	}

	// Guarantee at least one verdict: a target whose anime-set resolved nothing
	// (e.g. the library is empty or catalog is down) would otherwise vanish from
	// the dashboard instead of reading "down".
	if len(verdicts) == 0 {
		verdicts = append(verdicts, Verdict{Provider: t.Provider, Stage: StageEpisodes, Reason: streamprobe.ReasonEmptyResponse})
	}

	if failFast {
		pass = allPlayed
	} else {
		pass = topPlayed
	}
	return verdicts, pass
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

	// Try to fetch the catalog plan that governs which providers to probe and how.
	planEntries, err := e.plan.FetchPlan(ctx)
	if err != nil {
		if e.log != nil {
			e.log.Warnw("probe plan unavailable — falling back to legacy full probe", "error", err)
		}
		// Legacy fallback: probe ALL targets, full sample, no fail_fast, no verdict POST.
		for _, t := range e.targets {
			refs, _ := t.AnimeSet.Resolve(ctx)
			verdicts, _ := e.probeProvider(ctx, t, refs, 0, false)
			allVerdicts = append(allVerdicts, verdicts...)
			provVerdicts = append(provVerdicts, Rollup(t.Provider, filterProbed(verdicts)))
		}
		return e.rep.Report(ctx, RunResult{ProviderVerdicts: provVerdicts, Verdicts: allVerdicts, At: e.now()})
	}

	// Build lookup map: provider → PlanEntry.
	planMap := make(map[string]PlanEntry, len(planEntries))
	for _, pe := range planEntries {
		planMap[pe.Provider] = pe
	}

	// Probe only targets that appear in the plan.
	for _, t := range e.targets {
		entry, inPlan := planMap[t.Provider]
		if !inPlan {
			// Not scheduled this tick — skip entirely.
			continue
		}

		refs, _ := t.AnimeSet.Resolve(ctx)
		verdicts, pass := e.probeProvider(ctx, t, refs, entry.SampleSize, entry.FailFast)

		pv := Rollup(t.Provider, filterProbed(verdicts))
		allVerdicts = append(allVerdicts, verdicts...)
		provVerdicts = append(provVerdicts, pv)

		// Report pass/fail back to catalog's state machine.
		reason := ""
		if !pass {
			reason = pv.Reason
		}
		if postErr := e.plan.PostVerdict(ctx, t.Provider, pass, reason); postErr != nil {
			if e.log != nil {
				e.log.Warnw("probe PostVerdict failed", "provider", t.Provider, "error", postErr)
			}
			// Never fail the run on a POST error — it's best-effort.
		}
	}

	return e.rep.Report(ctx, RunResult{ProviderVerdicts: provVerdicts, Verdicts: allVerdicts, At: e.now()})
}
