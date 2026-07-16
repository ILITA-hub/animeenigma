package queue

import (
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

// Owner-specified priority weights (spec §1).
const (
	weightVisitor = 15
	weightOngoing = 10
	weightTop     = 5

	backoffBase = 6 * time.Hour
	backoffCap  = 168 * time.Hour
)

type Candidate struct {
	AnimeID       string
	Name          string
	Ongoing       bool
	Top           bool
	Visitors      int
	EpisodesAired int
}

func (c Candidate) Score() int {
	s := weightVisitor * c.Visitors
	if c.Ongoing {
		s += weightOngoing
	}
	if c.Top {
		s += weightTop
	}
	return s
}

// BuildCandidates merges membership (ongoing ∪ top ∪ visited) and attaches
// the unique-visitor count to every candidate.
func BuildCandidates(m *catalogclient.Membership, visited []string, visitors func(string) int) []Candidate {
	byID := map[string]*Candidate{}
	add := func(id, name string, aired int) *Candidate {
		if c, ok := byID[id]; ok {
			if c.Name == "" {
				c.Name = name
			}
			if aired > c.EpisodesAired {
				c.EpisodesAired = aired
			}
			return c
		}
		c := &Candidate{AnimeID: id, Name: name, EpisodesAired: aired}
		byID[id] = c
		return c
	}
	if m != nil {
		for _, r := range m.Ongoing {
			add(r.ID, r.Name, r.EpisodesAired).Ongoing = true
		}
		for _, r := range m.Top {
			add(r.ID, r.Name, r.EpisodesAired).Top = true
		}
	}
	for _, id := range visited {
		add(id, "", 0)
	}
	out := make([]Candidate, 0, len(byID))
	for _, c := range byID {
		c.Visitors = visitors(c.AnimeID)
		out = append(out, *c)
	}
	return out
}

func Rank(cs []Candidate) []Candidate {
	sort.SliceStable(cs, func(i, j int) bool {
		si, sj := cs[i].Score(), cs[j].Score()
		if si != sj {
			return si > sj
		}
		if cs[i].Ongoing != cs[j].Ongoing {
			return cs[i].Ongoing
		}
		return cs[i].AnimeID < cs[j].AnimeID
	})
	return cs
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

func CooldownTTL(ongoing bool) time.Duration {
	if ongoing {
		return 6 * time.Hour // new episodes must surface same-day
	}
	return 24 * time.Hour
}
