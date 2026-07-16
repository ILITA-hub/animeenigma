// Package prober turns one queue.Unit into a domain.UnitVerdict: resolve the
// stream via catalog, pull fragments through the streaming proxy with ffmpeg,
// run the python analyzers, assemble confidences.
package prober

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
)

var ErrResolve = errors.New("prober: stream resolve failed")

const (
	fragmentSeconds = 30
	baseFragments   = 3
	maxFragments    = 6
)

type Prober struct {
	cat     *catalogclient.Client
	gateway string
	ffmpeg  string
	workDir string
	runner  AnalyzerRunner
	hc      *http.Client
	log     *logger.Logger
	now     func() time.Time
}

func New(cat *catalogclient.Client, gatewayURL, ffmpegPath, workDir string, runner AnalyzerRunner, log *logger.Logger) *Prober {
	return &Prober{cat: cat, gateway: gatewayURL, ffmpeg: ffmpegPath, workDir: workDir,
		runner: runner, hc: &http.Client{Timeout: 15 * time.Second}, log: log, now: time.Now}
}

// resolveStream fetches the unit's stream, falling back to episode 1 when
// the latest episode is missing on this unit ("ближайший доступный").
func (p *Prober) resolveStream(ctx context.Context, u queue.Unit) (*catalogclient.Stream, int, error) {
	try := func(ep int) (*catalogclient.Stream, error) {
		switch {
		case u.Key.Team != "": // kodik translation
			tid := atoiSafe(u.Key.Team)
			return p.cat.KodikStream(ctx, u.AnimeID, ep, tid)
		case u.EpisodeID != "": // scraper server (episode id fixed at enumeration)
			return p.cat.ScraperStream(ctx, u.AnimeID, u.EpisodeID, u.Key.Server, u.Key.Category, u.Provider)
		default: // animejoy leg
			return p.cat.AnimejoyStream(ctx, u.AnimeID, u.Provider, ep)
		}
	}
	st, err := try(u.Episode)
	if err == nil {
		return st, u.Episode, nil
	}
	if u.Episode > 1 && u.EpisodeID == "" { // ep-numbered providers only
		if st, err2 := try(1); err2 == nil {
			return st, 1, nil
		}
	}
	return nil, 0, fmt.Errorf("%w: %v", ErrResolve, err)
}

// Probe never returns an error — failures become StatusUnreachable verdicts
// with an incremented Fails counter (queue backoff input).
func (p *Prober) Probe(ctx context.Context, u queue.Unit, prevFails int) domain.UnitVerdict {
	v := domain.UnitVerdict{Key: u.Key, Episode: u.Episode, ProbedAt: p.now().UTC()}
	dir, err := os.MkdirTemp(p.workDir, "unit-*")
	if err != nil {
		return p.unreachable(v, prevFails, err)
	}
	defer os.RemoveAll(dir)
	_ = os.MkdirAll(filepath.Join(dir, "frames"), 0o755)

	st, ep, err := p.resolveStream(ctx, u)
	if err != nil {
		return p.unreachable(v, prevFails, err)
	}
	v.Episode = ep
	for _, t := range st.Tracks {
		v.Softsubs = append(v.Softsubs, domain.SoftTrack{Lang: t.Label, Kind: t.Kind})
	}

	input := ProxiedURL(p.gateway, st.URL, st.Exp, st.Sig, st.Referer)
	duration := 0.0
	if st.Type != "mp4" { // HLS: localize + duration from EXTINF sum
		local, dur, err := LocalizeHLS(ctx, p.hc, p.gateway, input, dir)
		if err != nil {
			return p.unreachable(v, prevFails, err)
		}
		input, duration = local, dur
	}
	offsets := sampleOffsets(duration, st.Intro, st.Outro)

	var wavs []string
	for i, seek := range offsets[:baseFragments] {
		wav, err := ExtractFragment(ctx, p.ffmpeg, input, seek, fragmentSeconds, i, dir)
		if err != nil {
			if i == 0 {
				return p.unreachable(v, prevFails, err) // first fragment dead = stream dead
			}
			continue // partial extraction: analyze what we have
		}
		wavs = append(wavs, wav)
	}
	if len(wavs) == 0 {
		return p.unreachable(v, prevFails, errors.New("no fragments extracted"))
	}

	lid, err := p.runner.LID(ctx, wavs)
	if err != nil {
		return p.inconclusive(v, err)
	}
	// Not enough speech? Pull extra fragments (up to maxFragments total).
	if speechCount(lid.Fragments) < baseFragments && len(offsets) > baseFragments {
		for i, seek := range offsets[baseFragments:] {
			idx := baseFragments + i
			if idx >= maxFragments {
				break
			}
			if wav, err := ExtractFragment(ctx, p.ffmpeg, input, seek, fragmentSeconds, idx, dir); err == nil {
				wavs = append(wavs, wav)
			}
		}
		if extra, err := p.runner.LID(ctx, wavs); err == nil {
			lid = extra
		}
	}
	v.Audio = AssembleAudio(lid.Fragments)
	v.Sample = domain.SampleInfo{Fragments: len(wavs), SpeechSeconds: totalSpeech(lid.Fragments)}

	if hs, err := p.runner.Hardsub(ctx, filepath.Join(dir, "frames")); err == nil {
		v.Hardsub = AssembleHardsub(hs)
	} else if p.log != nil {
		p.log.Warnw("hardsub analyzer failed", "provider", u.Provider, "error", err)
	}

	if v.Audio != nil && v.Audio.Verified {
		v.Status = domain.StatusVerified
	} else {
		v.Status = domain.StatusInconclusive
	}
	return v
}

func (p *Prober) unreachable(v domain.UnitVerdict, prevFails int, err error) domain.UnitVerdict {
	if p.log != nil {
		p.log.Warnw("unit unreachable", "key", v.Key.String(), "error", err)
	}
	v.Status = domain.StatusUnreachable
	v.Fails = prevFails + 1
	return v
}

func (p *Prober) inconclusive(v domain.UnitVerdict, err error) domain.UnitVerdict {
	if p.log != nil {
		p.log.Warnw("unit inconclusive", "key", v.Key.String(), "error", err)
	}
	v.Status = domain.StatusInconclusive
	return v
}

// sampleOffsets picks up to maxFragments seek points. Known duration →
// fractions of runtime (skipping intro/outro windows); unknown → fixed
// seeks suited to a ~24min episode.
func sampleOffsets(duration float64, intro, outro *catalogclient.TimeRange) []float64 {
	fracs := []float64{0.25, 0.50, 0.75, 0.35, 0.60, 0.85}
	var out []float64
	if duration < 120 {
		return []float64{60, 240, 480, 300, 600, 720} // duration unknown/tiny: fixed
	}
	for _, f := range fracs {
		s := duration * f
		if intro != nil && s >= float64(intro.Start) && s <= float64(intro.End) {
			s = float64(intro.End) + 10
		}
		if outro != nil && s >= float64(outro.Start) {
			s = float64(outro.Start) - float64(fragmentSeconds) - 10
		}
		if s < 30 {
			s = 30
		}
		if s > duration-float64(fragmentSeconds)-5 {
			s = duration - float64(fragmentSeconds) - 5
		}
		out = append(out, s)
	}
	return out
}

func speechCount(frs []LIDFragment) int {
	n := 0
	for _, f := range frs {
		if f.Speech {
			n++
		}
	}
	return n
}

func totalSpeech(frs []LIDFragment) float64 {
	t := 0.0
	for _, f := range frs {
		t += f.SpeechSeconds
	}
	return t
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
