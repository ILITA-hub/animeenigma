package probe

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

type Engine struct {
	providers []string
	as        AnimeSetResolver
	res       Resolver
	val       Validator
	rep       Reporter
	now       func() int64
	log       *logger.Logger
}

func NewEngine(providers []string, as AnimeSetResolver, res Resolver, val Validator, rep Reporter, now func() int64, log *logger.Logger) *Engine {
	return &Engine{providers: providers, as: as, res: res, val: val, rep: rep, now: now, log: log}
}

func (e *Engine) RunOnce(ctx context.Context) error {
	refs, err := e.as.Resolve(ctx)
	if err != nil && len(refs) == 0 {
		return err
	}
	var allVerdicts []Verdict
	var provVerdicts []ProviderVerdict

	for _, p := range e.providers {
		var verdicts []Verdict
		for _, ref := range refs {
			streams, stage, rerr := e.res.Resolve(ctx, ref.UUID, ref.Slot, p)
			if rerr != nil {
				verdicts = append(verdicts, Verdict{
					Provider: p, AnimeUUID: ref.UUID, Slot: ref.Slot, Stage: stage,
					Reason: streamprobe.ReasonCDNUnreachable,
				})
				continue
			}
			for _, s := range streams {
				verdicts = append(verdicts, e.val.Validate(ctx, s))
			}
		}
		allVerdicts = append(allVerdicts, verdicts...)
		provVerdicts = append(provVerdicts, Rollup(p, verdicts))
	}

	return e.rep.Report(ctx, RunResult{ProviderVerdicts: provVerdicts, Verdicts: allVerdicts, At: e.now()})
}
