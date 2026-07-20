package queue

import (
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

const (
	backoffBase = 6 * time.Hour
	backoffCap  = 168 * time.Hour
)

// Band is a content-verify priority class (spec §1). Lower value = higher
// priority; pinned titles always lead.
type Band int

const (
	BandPinned     Band = iota // operator CV_PIN_ANIME
	BandOngoing                // hot ongoing
	BandWatchedTop             // watched (visitors>0) or browse-order top-100
	BandIdle                   // idle backfill (planned + tail windows)
)

type Candidate struct {
	AnimeID       string
	Name          string
	Ongoing       bool
	Top           bool
	Pinned        bool // CV_PIN_ANIME operator pin
	Visitors      int
	EpisodesAired int
	MalScore      float64    // animes.score — intra-band tiebreak
	TopRank       int        // 1-based browse rank; 0 = outside top/idle window
	NextEpisodeAt *time.Time // Band-1 freshBoost
	Planners      int        // Band-3 planned ordering
	Idle          bool       // sourced from planned/idle-window
}

// BandOf classifies a candidate.
func BandOf(c Candidate) Band {
	switch {
	case c.Pinned:
		return BandPinned
	case c.Ongoing:
		return BandOngoing
	case c.Visitors > 0 || c.Top:
		return BandWatchedTop
	default:
		return BandIdle
	}
}

// freshBoost reports whether an ongoing has an episode within ±window of now
// (a just-aired or imminent episode), floating it to the front of Band 1.
func freshBoost(c Candidate, now time.Time, window time.Duration) bool {
	if c.NextEpisodeAt == nil {
		return false
	}
	d := c.NextEpisodeAt.Sub(now)
	if d < 0 {
		d = -d
	}
	return d <= window
}

// IntraLess reports whether a should sort BEFORE b within their shared band.
func IntraLess(a, b Candidate, now time.Time, freshWindow time.Duration) bool {
	switch BandOf(a) {
	case BandOngoing:
		if af, bf := freshBoost(a, now, freshWindow), freshBoost(b, now, freshWindow); af != bf {
			return af
		}
		if a.Visitors != b.Visitors {
			return a.Visitors > b.Visitors
		}
		return a.MalScore > b.MalScore
	case BandWatchedTop:
		if a.Visitors != b.Visitors {
			return a.Visitors > b.Visitors
		}
		aRanked, bRanked := a.TopRank > 0, b.TopRank > 0
		if aRanked != bRanked {
			return aRanked // ranked before unranked
		}
		if aRanked && a.TopRank != b.TopRank {
			return a.TopRank < b.TopRank
		}
		return a.MalScore > b.MalScore
	default: // BandIdle
		if a.Planners != b.Planners {
			return a.Planners > b.Planners
		}
		return a.MalScore > b.MalScore
	}
}

// weightedPick chooses a primary band [Band1..Band3] from the lottery weights.
func weightedPick(w [3]int, r float64) Band {
	total := w[0] + w[1] + w[2]
	if total <= 0 {
		return BandOngoing
	}
	x := r * float64(total)
	switch {
	case x < float64(w[0]):
		return BandOngoing
	case x < float64(w[0]+w[1]):
		return BandWatchedTop
	default:
		return BandIdle
	}
}

// bandOrder returns the per-claim band try-order: pins first, then the
// lottery-chosen primary, then the remaining organic bands in fixed priority.
func bandOrder(w [3]int, r float64) []Band {
	primary := weightedPick(w, r)
	order := []Band{BandPinned, primary}
	for _, b := range []Band{BandOngoing, BandWatchedTop, BandIdle} {
		if b != primary {
			order = append(order, b)
		}
	}
	return order
}

// CooldownTTL is the settled-title cooldown by band: ongoing must resurface
// same-day for new episodes; watched/top daily; idle-backfill on the long
// CV_IDLE_COOLDOWN so the tail doesn't re-spin before the cursor sweeps past.
func CooldownTTL(band Band, idle time.Duration) time.Duration {
	switch band {
	case BandOngoing, BandPinned:
		return 6 * time.Hour
	case BandWatchedTop:
		return 24 * time.Hour
	default:
		return idle
	}
}

// BuildCandidates merges the interest snapshot (ongoing ∪ top ∪ planned ∪
// idle-window ∪ visited ∪ pinned) and attaches the unique-visitor count. Rows
// carry their band-relevant signals (top_rank, next_episode_at, planners,
// score). Pinned titles are injected even when in no bucket.
func BuildCandidates(it *catalogclient.Interest, visited []string, pins map[string]string, visitors func(string) int) []Candidate {
	byID := map[string]*Candidate{}
	get := func(id string) *Candidate {
		c, ok := byID[id]
		if !ok {
			c = &Candidate{AnimeID: id}
			byID[id] = c
		}
		return c
	}
	setName := func(c *Candidate, n string) {
		if c.Name == "" {
			c.Name = n
		}
	}
	if it != nil {
		for _, r := range it.Ongoing {
			c := get(r.ID)
			setName(c, r.Name)
			c.Ongoing = true
			c.EpisodesAired = maxInt(c.EpisodesAired, r.EpisodesAired)
			c.MalScore = r.Score
			c.NextEpisodeAt = r.NextEpisodeAt
		}
		for _, r := range it.Top {
			c := get(r.ID)
			setName(c, r.Name)
			c.Top = true
			c.TopRank = r.TopRank
			c.EpisodesAired = maxInt(c.EpisodesAired, r.EpisodesAired)
			if r.Score > 0 {
				c.MalScore = r.Score
			}
		}
		for _, r := range it.Planned {
			c := get(r.ID)
			setName(c, r.Name)
			c.Idle = true
			c.Planners = r.Planners
			c.EpisodesAired = maxInt(c.EpisodesAired, r.EpisodesAired)
			if r.Score > 0 {
				c.MalScore = r.Score
			}
		}
		for _, r := range it.IdleWindow {
			c := get(r.ID)
			setName(c, r.Name)
			c.Idle = true
			if c.TopRank == 0 {
				c.TopRank = r.TopRank
			}
			c.EpisodesAired = maxInt(c.EpisodesAired, r.EpisodesAired)
			if r.Score > 0 {
				c.MalScore = r.Score
			}
		}
	}
	for _, id := range visited {
		get(id)
	}
	for id := range pins {
		get(id).Pinned = true
	}
	out := make([]Candidate, 0, len(byID))
	for _, c := range byID {
		c.Visitors = visitors(c.AnimeID)
		out = append(out, *c)
	}
	return out
}

func Backoff(fails int) time.Duration {
	if fails < 1 {
		fails = 1
	}
	d := backoffBase
	for i := 1; i < fails; i++ {
		d *= 2
		if d >= backoffCap {
			return backoffCap
		}
	}
	return d
}

// UnitDue decides whether a unit needs (re-)probing.
func UnitDue(u Unit, prev *domain.UnitVerdict, now time.Time, reprobeTTL time.Duration) bool {
	if prev == nil {
		return true
	}
	if u.Episode > prev.Episode {
		return true // a newer episode aired — the old sample is stale
	}
	if prev.Status == domain.StatusUnreachable {
		return now.After(prev.ProbedAt.Add(Backoff(prev.Fails)))
	}
	return now.After(prev.ProbedAt.Add(reprobeTTL))
}

// PendingUnits diffs live structure against stored verdicts, keeping probe
// order (StateRank from enumeration).
func PendingUnits(units []Unit, rows []domain.ContentVerification, now time.Time, ttl time.Duration) []Unit {
	prev := map[string]*domain.UnitVerdict{}
	for i := range rows {
		for j := range rows[i].Units {
			u := &rows[i].Units[j]
			prev[rows[i].Provider+"|"+u.Key.String()] = u
		}
	}
	var out []Unit
	for _, u := range units {
		if UnitDue(u, prev[u.Provider+"|"+u.Key.String()], now, ttl) {
			out = append(out, u)
		}
	}
	return out
}
