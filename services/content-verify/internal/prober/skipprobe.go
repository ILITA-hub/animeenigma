// skipprobe.go: the OP/ED (intro/outro) skip prober. Bootstraps season
// fingerprints by cross-correlating two episodes' head/tail windows (pair
// mode) and locates those fingerprints inside further episodes (locate
// mode) — see queue.NextSkipTask for how pair vs. locate is chosen.
package prober

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
)

// SkipConfig holds the skip prober's tunables. Concrete defaults (e.g.
// 480s windows) live in the service's config package, not here.
type SkipConfig struct {
	HeadWindow, TailWindow time.Duration
	MinMatch, MaxMatch     time.Duration
	SimThreshold           float64
}

// FingerprintStore is the season-fingerprint persistence surface the skip
// prober needs — satisfied by repo.Store.
type FingerprintStore interface {
	Fingerprints(ctx context.Context, animeID string) ([]domain.SkipFingerprint, error)
	AddFingerprint(ctx context.Context, fp domain.SkipFingerprint) error
}

// SkipProber turns one queue.SkipTask into 1 (locate) or 2 (pair) rows to
// upsert. Structurally mirrors Prober: temp workdir, the same 3x25s resolve
// warm-up retry policy, and the same "budget-ctx expiry is not a failure"
// rule — see resolveSkipStream and unreachableRows.
type SkipProber struct {
	cat       *catalogclient.Client
	gateway   string
	ffmpeg    string
	workDir   string
	runner    AnalyzerRunner
	fps       FingerprintStore
	cfg       SkipConfig
	hc        *http.Client
	log       *logger.Logger
	now       func() time.Time
	retryWait time.Duration // between resolve attempts; tests zero it
}

func NewSkipProber(cat *catalogclient.Client, gatewayURL, ffmpegPath, workDir string, runner AnalyzerRunner, fps FingerprintStore, cfg SkipConfig, log *logger.Logger) *SkipProber {
	return &SkipProber{cat: cat, gateway: gatewayURL, ffmpeg: ffmpegPath, workDir: workDir,
		runner: runner, fps: fps, cfg: cfg, hc: &http.Client{Timeout: 60 * time.Second}, log: log, now: time.Now,
		retryWait: resolveRetryWait}
}

// resolvedSkipUnit is one unit's extracted audio windows, ready to feed the
// opskip analyzer.
type resolvedSkipUnit struct {
	headWav  string
	tailWav  string  // "" for mp4 units — tail extraction skipped in v1, see resolveUnit
	tailSeek float64 // seek used for tailWav's window; meaningless when tailWav == ""
	duration float64 // 0 for mp4 (unknown up front)
}

// Probe never errors; it returns the rows to upsert (1 for a locate task,
// 2 for a pair task).
func (p *SkipProber) Probe(ctx context.Context, t queue.SkipTask, prevFails int) []domain.SkipTiming {
	// AniSkip probe gate (locate tasks): a covered kind's window is neither
	// extracted nor analyzed — its side records the terminal "aniskip"
	// status instead. Fully-covered units are filtered before planning, so
	// the both-covered branch is defensive only.
	opCovered := t.Pair == nil && kindCovered(t.CoveredKinds, domain.SkipKindOp)
	edCovered := t.Pair == nil && kindCovered(t.CoveredKinds, domain.SkipKindEd)
	if opCovered && edCovered {
		row := skipRowFromUnit(t.Unit, p.now().UTC())
		row.OpStatus, row.EdStatus = domain.SkipAniskip, domain.SkipAniskip
		return []domain.SkipTiming{row}
	}

	dir, err := os.MkdirTemp(p.workDir, "skip-*")
	if err != nil {
		return p.unreachableRows(ctx, t, prevFails, err)
	}
	defer os.RemoveAll(dir)

	a, err := p.resolveUnit(ctx, dir, "a", t.Unit, !opCovered, !edCovered)
	if err != nil {
		return p.unreachableRows(ctx, t, prevFails, err)
	}

	if t.Pair != nil {
		b, err := p.resolveUnit(ctx, dir, "b", *t.Pair, true, true)
		if err != nil {
			return p.unreachableRows(ctx, t, prevFails, err)
		}
		return p.probePair(ctx, dir, t, a, b)
	}
	return p.probeLocate(ctx, dir, t, a, opCovered, edCovered)
}

// kindCovered reports whether kind is in the task's AniSkip-covered set.
func kindCovered(kinds []string, kind string) bool {
	return slices.Contains(kinds, kind)
}

// resolveUnit resolves u's stream and extracts its head window (when
// needHead), plus its tail window when the input is HLS (duration is known
// from the playlist's summed EXTINF) and needTail. needHead/needTail are
// false only for a locate task's AniSkip-covered side — the window would be
// extracted just to be thrown away. mp4 units (animejoy, or scraper legs
// served as mp4) are proxied straight through with an unknown total
// duration: the head window (seek 0) is still valid, but a tail pulled via
// -sseof would yield offsets relative to end-of-file that we have no way to
// turn into absolute episode time without knowing the duration — so mp4
// tail extraction is skipped entirely in v1 (tailWav stays ""; callers must
// treat that as EdStatus = SkipNoMatch — v1 cannot absolutize mp4 tail
// times, so this is an honest terminal no-serve rather than an endless
// pending_fp retry; AniSkip still covers ED there).
func (p *SkipProber) resolveUnit(ctx context.Context, dir, tag string, u queue.SkipUnit, needHead, needTail bool) (*resolvedSkipUnit, error) {
	st, err := p.resolveSkipStream(ctx, u)
	if err != nil {
		return nil, err
	}
	input := ProxiedURL(p.gateway, st.URL, st.Exp, st.Sig, st.Referer)
	out := &resolvedSkipUnit{}
	if st.Type != "mp4" {
		local, dur, err := LocalizeHLSVariant(ctx, p.hc, p.gateway, input, dir, true)
		if err != nil {
			return nil, err
		}
		input = local
		out.duration = dur
		out.tailSeek = dur - p.cfg.TailWindow.Seconds()
		if out.tailSeek < 0 {
			out.tailSeek = 0
		}
		if needTail {
			tailWav, err := ExtractWindow(ctx, p.ffmpeg, input, out.tailSeek, p.cfg.TailWindow.Seconds(), tag+"_tail", dir)
			if err != nil {
				return nil, err
			}
			out.tailWav = tailWav
		}
	}
	if needHead {
		headWav, err := ExtractWindow(ctx, p.ffmpeg, input, 0, p.cfg.HeadWindow.Seconds(), tag+"_head", dir)
		if err != nil {
			return nil, err
		}
		out.headWav = headWav
	}
	return out, nil
}

// resolveSkipStream resolves u's stream via the appropriate catalog route,
// with the same warm-up retry policy as Prober.resolveStream (see
// resolveAttempts/resolveRetryWait in prober.go): cold engine=browser
// resolves fail transiently and succeed once the sidecar session warms.
// Unlike Prober.resolveStream there is no "fall back to episode 1" —
// skip units are enumerated from the real episode list, so a resolve
// failure here is a genuine miss, not a placeholder episode number.
func (p *SkipProber) resolveSkipStream(ctx context.Context, u queue.SkipUnit) (*catalogclient.Stream, error) {
	try := func() (*catalogclient.Stream, error) {
		switch {
		case u.TeamID != 0: // kodik translation
			return p.cat.KodikStream(ctx, u.AnimeID, u.Episode, u.TeamID)
		case u.EpisodeID != "": // scraper server: pick first non-dub server, else first
			servers, err := p.cat.ScraperServers(ctx, u.AnimeID, u.EpisodeID, u.Provider)
			if err != nil {
				return nil, err
			}
			if len(servers) == 0 {
				return nil, catalogclient.ErrNotFound
			}
			server := servers[0]
			for _, s := range servers {
				if s.Type != "dub" {
					server = s
					break
				}
			}
			return p.cat.ScraperStream(ctx, u.AnimeID, u.EpisodeID, server.ID, "sub", u.Provider)
		default: // animejoy leg
			return p.cat.AnimejoyStream(ctx, u.AnimeID, u.Provider, u.Episode)
		}
	}
	var lastErr error
	for i := 0; i < resolveAttempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(p.retryWait):
			}
		}
		st, err := try()
		if err == nil {
			return st, nil
		}
		if errors.Is(err, catalogclient.ErrNotFound) || ctx.Err() != nil {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

// pairOutcome is one opskip "pair" call's result, normalized to the two
// possible row statuses.
type pairOutcome struct {
	status                     string
	aStart, aEnd, bStart, bEnd float64
	similarity                 float64
	fp                         []uint32
}

func (p *SkipProber) pairSide(ctx context.Context, kind, wavA, wavB string) pairOutcome {
	res, err := p.runner.OpskipPair(ctx, wavA, wavB, p.cfg.MinMatch.Seconds(), p.cfg.MaxMatch.Seconds(), p.cfg.SimThreshold)
	if err != nil {
		// A technical failure to run the analyzer is not the same claim as
		// "we compared the audio and found no shared segment" — no_match
		// would wrongly feed the re-pair self-heal scan (which specifically
		// hunts for no_match rows) and, in locate mode, would wrongly look
		// terminal (rowDue only re-dues pending_fp/unreachable). Surface it
		// as pending_fp so the normal TTL retries it.
		if p.log != nil {
			p.log.Warnw("opskip pair analyzer failed", "kind", kind, "error", err)
		}
		return pairOutcome{status: domain.SkipPendingFP}
	}
	if res.Duplicate {
		// The two "episodes" carried the same content (provider
		// episode-mapping bug — the gogoanime fake-content pattern). That is
		// neither a shared-OP detection nor evidence the episodes lack one:
		// no_match would be a wrong terminal verdict AND feed the re-pair
		// scan, while detected would store a poisoned season fingerprint.
		// pending_fp re-dues the unit after its TTL — the provider may serve
		// real per-episode content by then.
		if p.log != nil {
			p.log.Warnw("opskip pair duplicate content", "kind", kind)
		}
		return pairOutcome{status: domain.SkipPendingFP}
	}
	if !res.Found {
		return pairOutcome{status: domain.SkipNoMatch}
	}
	return pairOutcome{status: domain.SkipDetected,
		aStart: res.AStart, aEnd: res.AEnd, bStart: res.BStart, bEnd: res.BEnd,
		similarity: res.Similarity, fp: res.Fp}
}

// probePair runs pair mode for the two given units, per-kind: a kind in
// t.PairKinds is bootstrapped (OP on both units' head windows, ED on both
// units' tail windows — HLS only, mp4 legs have no tailWav); a kind NOT in
// t.PairKinds already has a stored season fingerprint (see
// queue.NextSkipTask), so it's located independently in each of the two
// windows instead — reusing the audio already extracted for this probe
// rather than wasting it, and critically never calling AddFingerprint for a
// kind that's already settled (that duplicate is what fed every later
// locate a slower multi-fingerprint scan — see Finding 2). A bootstrapped
// kind that's found persists a new season fingerprint and marks both rows
// detected with per-episode absolute times; not-found marks both rows
// no_match. RePair tasks set PairTried on both rows unconditionally —
// that's what lets the re-pair scan (queue.NextSkipTask) move on to a
// different adjacent pair instead of retrying this one forever.
func (p *SkipProber) probePair(ctx context.Context, dir string, t queue.SkipTask, a, b *resolvedSkipUnit) []domain.SkipTiming {
	now := p.now().UTC()
	rowA := skipRowFromUnit(t.Unit, now)
	rowB := skipRowFromUnit(*t.Pair, now)
	if t.RePair {
		rowA.PairTried, rowB.PairTried = true, true
	}
	note := fmt.Sprintf("%s ep%d+ep%d", t.Unit.Provider, t.Unit.Episode, t.Pair.Episode)
	mp4Leg := a.tailWav == "" || b.tailWav == ""

	// Fetch the stored fingerprints once, up front, only when at least one
	// side of this probe will need to locate (rather than bootstrap)
	// against them.
	var allFPs []domain.SkipFingerprint
	needFPs := !pairKindWanted(t.PairKinds, domain.SkipKindOp) || (!mp4Leg && !pairKindWanted(t.PairKinds, domain.SkipKindEd))
	if needFPs {
		fps, err := p.fps.Fingerprints(ctx, t.Unit.AnimeID)
		if err != nil {
			if p.log != nil {
				p.log.Warnw("skip fingerprints fetch failed", "anime_id", t.Unit.AnimeID, "error", err)
			}
		} else {
			allFPs = fps
		}
	}

	if pairKindWanted(t.PairKinds, domain.SkipKindOp) {
		opOut := p.pairSide(ctx, domain.SkipKindOp, a.headWav, b.headWav)
		rowA.OpStatus, rowB.OpStatus = opOut.status, opOut.status
		if opOut.status == domain.SkipDetected {
			rowA.OpStart, rowA.OpEnd = opOut.aStart, opOut.aEnd
			rowB.OpStart, rowB.OpEnd = opOut.bStart, opOut.bEnd
			rowA.Confidence = maxFloat(rowA.Confidence, opOut.similarity)
			rowB.Confidence = maxFloat(rowB.Confidence, opOut.similarity)
			if err := p.fps.AddFingerprint(ctx, domain.SkipFingerprint{
				AnimeID: t.Unit.AnimeID, Kind: domain.SkipKindOp, Fp: opOut.fp,
				Length: opOut.aEnd - opOut.aStart, SourceNote: note,
			}); err != nil && p.log != nil {
				p.log.Warnw("add op fingerprint failed", "anime_id", t.Unit.AnimeID, "error", err)
			}
		}
	} else {
		p.locatePairSide(ctx, dir, domain.SkipKindOp, allFPs, a.headWav, b.headWav, 0, 0, &rowA, &rowB)
	}

	switch {
	case mp4Leg:
		// v1 cannot absolutize mp4 tail times (duration unknown up front) —
		// honest terminal no-serve rather than an endless pending_fp retry;
		// AniSkip still covers ED there (see resolveUnit).
		rowA.EdStatus, rowB.EdStatus = domain.SkipNoMatch, domain.SkipNoMatch
	case pairKindWanted(t.PairKinds, domain.SkipKindEd):
		edOut := p.pairSide(ctx, domain.SkipKindEd, a.tailWav, b.tailWav)
		rowA.EdStatus, rowB.EdStatus = edOut.status, edOut.status
		if edOut.status == domain.SkipDetected {
			rowA.EdStart, rowA.EdEnd = a.tailSeek+edOut.aStart, a.tailSeek+edOut.aEnd
			rowB.EdStart, rowB.EdEnd = b.tailSeek+edOut.bStart, b.tailSeek+edOut.bEnd
			rowA.Confidence = maxFloat(rowA.Confidence, edOut.similarity)
			rowB.Confidence = maxFloat(rowB.Confidence, edOut.similarity)
			if err := p.fps.AddFingerprint(ctx, domain.SkipFingerprint{
				AnimeID: t.Unit.AnimeID, Kind: domain.SkipKindEd, Fp: edOut.fp,
				Length: edOut.aEnd - edOut.aStart, SourceNote: note,
			}); err != nil && p.log != nil {
				p.log.Warnw("add ed fingerprint failed", "anime_id", t.Unit.AnimeID, "error", err)
			}
		}
	default:
		p.locatePairSide(ctx, dir, domain.SkipKindEd, allFPs, a.tailWav, b.tailWav, a.tailSeek, b.tailSeek, &rowA, &rowB)
	}

	return []domain.SkipTiming{rowA, rowB}
}

// pairKindWanted reports whether kind is one of the kinds a pair task must
// bootstrap.
func pairKindWanted(kinds []string, kind string) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// locatePairSide runs LOCATE (not pair-bootstrap) for kind against both
// units' already-extracted windows, writing fps.json once and reusing it
// for both OpskipLocate calls — this is the "kind already has a
// fingerprint" branch of a pair task, so no AddFingerprint call is ever
// reachable from here.
func (p *SkipProber) locatePairSide(ctx context.Context, dir, kind string, allFPs []domain.SkipFingerprint, wavA, wavB string, baseA, baseB float64, rowA, rowB *domain.SkipTiming) {
	kindFPs := filterFPsByKind(allFPs, kind)
	if len(kindFPs) == 0 {
		applyLocateOutcome(rowA, kind, locateOutcome{status: domain.SkipPendingFP})
		applyLocateOutcome(rowB, kind, locateOutcome{status: domain.SkipPendingFP})
		return
	}
	fpsPath, err := writeFPsJSON(dir, kind+"_pair", kindFPs)
	if err != nil {
		if p.log != nil {
			p.log.Warnw("write fps.json failed", "kind", kind, "error", err)
		}
		applyLocateOutcome(rowA, kind, locateOutcome{status: domain.SkipPendingFP})
		applyLocateOutcome(rowB, kind, locateOutcome{status: domain.SkipPendingFP})
		return
	}
	applyLocateOutcome(rowA, kind, p.runLocate(ctx, kind, wavA, fpsPath, baseA))
	applyLocateOutcome(rowB, kind, p.runLocate(ctx, kind, wavB, fpsPath, baseB))
}

// applyLocateOutcome writes a locateOutcome into the appropriate kind's
// fields on row.
func applyLocateOutcome(row *domain.SkipTiming, kind string, out locateOutcome) {
	switch kind {
	case domain.SkipKindOp:
		row.OpStatus = out.status
		if out.status == domain.SkipDetected {
			row.OpStart, row.OpEnd = out.start, out.end
			row.Confidence = maxFloat(row.Confidence, out.similarity)
		}
	case domain.SkipKindEd:
		row.EdStatus = out.status
		if out.status == domain.SkipDetected {
			row.EdStart, row.EdEnd = out.start, out.end
			row.Confidence = maxFloat(row.Confidence, out.similarity)
		}
	}
}

// locateOutcome is one opskip "locate" call's result, normalized to the
// possible row statuses (relative to its window's start).
type locateOutcome struct {
	status     string
	start, end float64
	similarity float64
}

// locateSide runs OpskipLocate against every stored fingerprint of kind for
// wav (a head or tail window starting at base seconds into the episode),
// writing the [{"id","fp"}...] scratch file opskip.py's locate mode reads.
// No fingerprints of this kind at all => pending_fp (nothing to compare
// against yet, not a confirmed absence).
func (p *SkipProber) locateSide(ctx context.Context, dir, tag string, allFPs []domain.SkipFingerprint, kind, wav string, base float64) locateOutcome {
	kindFPs := filterFPsByKind(allFPs, kind)
	if len(kindFPs) == 0 {
		return locateOutcome{status: domain.SkipPendingFP}
	}
	fpsPath, err := writeFPsJSON(dir, tag, kindFPs)
	if err != nil {
		if p.log != nil {
			p.log.Warnw("write fps.json failed", "kind", kind, "error", err)
		}
		return locateOutcome{status: domain.SkipPendingFP}
	}
	return p.runLocate(ctx, kind, wav, fpsPath, base)
}

// runLocate runs OpskipLocate for one wav window against an already-written
// fps.json scratch file. Shared by locateSide (single-episode locate tasks)
// and locatePairSide (the non-bootstrapped kind of a pair task), which
// write fps.json once and call this twice — once per episode.
func (p *SkipProber) runLocate(ctx context.Context, kind, wav, fpsPath string, base float64) locateOutcome {
	res, err := p.runner.OpskipLocate(ctx, wav, fpsPath, p.cfg.MinMatch.Seconds(), p.cfg.MaxMatch.Seconds(), p.cfg.SimThreshold)
	if err != nil {
		// Same reasoning as pairSide: a runner failure is not a confirmed
		// no_match — retry via pending_fp instead of going (possibly
		// permanently) no_match/terminal.
		if p.log != nil {
			p.log.Warnw("opskip locate analyzer failed", "kind", kind, "error", err)
		}
		return locateOutcome{status: domain.SkipPendingFP}
	}
	if !res.Found {
		return locateOutcome{status: domain.SkipNoMatch}
	}
	return locateOutcome{status: domain.SkipDetected, start: base + res.Start, end: base + res.End, similarity: res.Similarity}
}

// probeLocate runs locate mode: OP against the head window, ED against the
// tail window (mp4 units skip ED entirely — see resolveUnit). An
// AniSkip-covered side records the terminal "aniskip" status without any
// analysis — its window was never extracted (see Probe).
func (p *SkipProber) probeLocate(ctx context.Context, dir string, t queue.SkipTask, a *resolvedSkipUnit, opCovered, edCovered bool) []domain.SkipTiming {
	now := p.now().UTC()
	row := skipRowFromUnit(t.Unit, now)

	allFPs, err := p.fps.Fingerprints(ctx, t.Unit.AnimeID)
	if err != nil {
		if p.log != nil {
			p.log.Warnw("skip fingerprints fetch failed", "anime_id", t.Unit.AnimeID, "error", err)
		}
		row.OpStatus, row.EdStatus = domain.SkipPendingFP, domain.SkipPendingFP
		if opCovered {
			row.OpStatus = domain.SkipAniskip
		}
		if edCovered {
			row.EdStatus = domain.SkipAniskip
		}
		return []domain.SkipTiming{row}
	}

	if opCovered {
		row.OpStatus = domain.SkipAniskip
	} else {
		opOut := p.locateSide(ctx, dir, "op", allFPs, domain.SkipKindOp, a.headWav, 0)
		row.OpStatus = opOut.status
		if opOut.status == domain.SkipDetected {
			row.OpStart, row.OpEnd = opOut.start, opOut.end
			row.Confidence = maxFloat(row.Confidence, opOut.similarity)
		}
	}

	switch {
	case edCovered:
		row.EdStatus = domain.SkipAniskip
	case a.tailWav == "":
		// v1 cannot absolutize mp4 tail times (duration unknown up front) —
		// honest terminal no-serve rather than an endless pending_fp retry;
		// AniSkip still covers ED there.
		row.EdStatus = domain.SkipNoMatch
	default:
		edOut := p.locateSide(ctx, dir, "ed", allFPs, domain.SkipKindEd, a.tailWav, a.tailSeek)
		row.EdStatus = edOut.status
		if edOut.status == domain.SkipDetected {
			row.EdStart, row.EdEnd = edOut.start, edOut.end
			row.Confidence = maxFloat(row.Confidence, edOut.similarity)
		}
	}

	return []domain.SkipTiming{row}
}

// unreachableRows builds the failure row(s) for t (1 for a locate task, 2
// for a pair task) — mirrors Prober.unreachable's budget-ctx rule: a ctx
// deadline/cancellation encountered while resolving is a scheduling fact
// ("too slow for this budget round"), not a dead-stream claim, so it must
// not feed the unreachable backoff. Fails is left at its zero value in that
// case (mirroring Prober.unreachable, which never sets it on the
// budget-expired path either) rather than carrying prevFails forward.
func (p *SkipProber) unreachableRows(ctx context.Context, t queue.SkipTask, prevFails int, err error) []domain.SkipTiming {
	now := p.now().UTC()
	status := domain.SkipUnreachable
	var fails int
	if ctx.Err() != nil {
		status = domain.SkipPendingFP
		if p.log != nil {
			p.log.Warnw("skip probe budget exceeded", "anime_id", t.Unit.AnimeID, "provider", t.Unit.Provider,
				"episode", t.Unit.Episode, "ctx_err", ctx.Err(), "error", err)
		}
	} else {
		fails = prevFails + 1
		if p.log != nil {
			p.log.Warnw("skip unit unreachable", "anime_id", t.Unit.AnimeID, "provider", t.Unit.Provider,
				"episode", t.Unit.Episode, "error", err)
		}
	}

	a := skipRowFromUnit(t.Unit, now)
	a.OpStatus, a.EdStatus, a.Fails = status, status, fails
	rows := []domain.SkipTiming{a}
	if t.Pair != nil {
		b := skipRowFromUnit(*t.Pair, now)
		b.OpStatus, b.EdStatus, b.Fails = status, status, fails
		rows = append(rows, b)
	}
	return rows
}

func skipRowFromUnit(u queue.SkipUnit, now time.Time) domain.SkipTiming {
	return domain.SkipTiming{AnimeID: u.AnimeID, Provider: u.Provider, Team: u.Team, Episode: u.Episode, ProbedAt: now}
}

func filterFPsByKind(fps []domain.SkipFingerprint, kind string) []domain.SkipFingerprint {
	var out []domain.SkipFingerprint
	for _, f := range fps {
		if f.Kind == kind {
			out = append(out, f)
		}
	}
	return out
}

// fpsJSONEntry is one element of the fps.json array opskip.py's locate mode
// reads: [{"id":"...","fp":[...]}, ...]. "id" isn't read by opskip.py
// itself (only fp_index, an array position, comes back) but is carried
// through for any future correlation/debugging need.
type fpsJSONEntry struct {
	ID string   `json:"id"`
	Fp []uint32 `json:"fp"`
}

func writeFPsJSON(dir, name string, fps []domain.SkipFingerprint) (string, error) {
	entries := make([]fpsJSONEntry, len(fps))
	for i, f := range fps {
		entries[i] = fpsJSONEntry{ID: f.ID, Fp: f.Fp}
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+"_fps.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func maxFloat(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}
