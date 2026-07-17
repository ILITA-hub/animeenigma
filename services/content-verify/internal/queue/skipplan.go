package queue

import (
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

// skipPendingFPTTL is how long a pending_fp row waits before it's due again
// — the anime may accumulate a fingerprint from another unit's pair-bootstrap
// in the meantime, at which point a plain locate resolves it.
const skipPendingFPTTL = 6 * time.Hour

// SkipTask is what the worker executes: locate one episode, or pair two.
type SkipTask struct {
	Unit   SkipUnit
	Pair   *SkipUnit // non-nil => pair-bootstrap with this second episode
	RePair bool      // true when re-pairing two adjacent no_match rows
	// PairKinds is the set of kinds (domain.SkipKindOp / SkipKindEd) the
	// pair probe must BOOTSTRAP — meaningful only when Pair != nil. Kinds
	// NOT in this set already have a stored fingerprint anime-wide, so the
	// prober locates them independently per episode instead of re-running
	// (and re-fingerprinting) the pair for a kind that's already settled.
	// Locate tasks (Pair == nil) leave this nil.
	PairKinds []string
}

// NextSkipTask picks the next skip work item, or nil when the anime's skip
// lane is settled. Rules (spec §2.3, §3; per-kind bootstrap/re-pair fix):
//   - rows keyed by (provider|team|episode); units grouped by family
//     (provider|team) preserving first-seen order — the caller is
//     responsible for handing units in probe-priority order (StateRank,
//     then enumeration order); episodes ascending within a family.
//   - due(row): no row → due; OpStatus or EdStatus == pending_fp → due
//     after 6h from ProbedAt; either side unreachable → due after
//     Backoff(row.Fails); otherwise (both sides detected/no_match) →
//     terminal, not due.
//   - hasOp/hasEd := whether any stored fingerprint of that kind exists,
//     anime-level (derived from fps).
//   - Re-pair scan runs FIRST, across every family in first-seen order: two
//     ADJACENT units in a family (consecutive slice entries, not
//     necessarily consecutive episode numbers) whose rows both have that
//     KIND's status == no_match (checked independently per kind — a kind
//     with one row detected and the other no_match does NOT qualify, so a
//     settled kind is never re-run), with PairTried==false on the earlier
//     row, yield SkipTask{Unit: earlier, Pair: &later, RePair: true,
//     PairKinds: <qualifying kinds>}. This runs before the due scan so
//     self-heal wins.
//   - If either kind lacks a fingerprint (hasOp or hasEd is false): take
//     the first family (in first-seen order) with >=2 due episodes and
//     pair-bootstrap its first two due episodes, PairKinds set to the
//     missing kind(s) only — a kind that already has a fingerprint is
//     located, not re-bootstrapped (see SkipProber.Probe). If no family
//     has >=2 due episodes, fall back to a locate task for the very first
//     due unit overall (family order, then episode) — the prober records
//     pending_fp for it.
//   - Else (both kinds have a fingerprint): locate task for the first due
//     unit (family order, then episode).
func NextSkipTask(units []SkipUnit, rows []domain.SkipTiming, fps []domain.SkipFingerprint, now time.Time) *SkipTask {
	if len(units) == 0 {
		return nil
	}

	rowByKey := make(map[string]*domain.SkipTiming, len(rows))
	for i := range rows {
		r := &rows[i]
		rowByKey[skipRowKey(r.Provider, r.Team, r.Episode)] = r
	}

	familyOrder, families := groupSkipFamilies(units)

	if task := rePairScan(familyOrder, families, rowByKey); task != nil {
		return task
	}

	if missing := missingFPKinds(fps); len(missing) > 0 {
		if task := bootstrapPairScan(familyOrder, families, rowByKey, now); task != nil {
			task.PairKinds = missing
			return task
		}
	}

	if u := firstDueUnit(familyOrder, families, rowByKey, now); u != nil {
		return &SkipTask{Unit: *u}
	}
	return nil
}

// missingFPKinds returns the kinds (op/ed, in that order) that have NO
// stored fingerprint at all anime-wide — the kinds a pair-bootstrap task
// still needs to seed. A kind already has a fingerprint the moment ANY unit
// successfully bootstrapped it, regardless of which other episodes are
// still no_match/pending for that kind.
func missingFPKinds(fps []domain.SkipFingerprint) []string {
	var hasOp, hasEd bool
	for _, f := range fps {
		switch f.Kind {
		case domain.SkipKindOp:
			hasOp = true
		case domain.SkipKindEd:
			hasEd = true
		}
	}
	var missing []string
	if !hasOp {
		missing = append(missing, domain.SkipKindOp)
	}
	if !hasEd {
		missing = append(missing, domain.SkipKindEd)
	}
	return missing
}

// groupSkipFamilies buckets units by (provider|team), preserving the order
// families are first encountered in the input slice.
func groupSkipFamilies(units []SkipUnit) ([]string, map[string][]SkipUnit) {
	var order []string
	families := make(map[string][]SkipUnit)
	for _, u := range units {
		key := skipFamilyKey(u)
		if _, ok := families[key]; !ok {
			order = append(order, key)
		}
		families[key] = append(families[key], u)
	}
	return order, families
}

func skipFamilyKey(u SkipUnit) string { return u.Provider + "|" + u.Team }

func skipRowKey(provider, team string, episode int) string {
	return provider + "|" + team + "|" + strconv.Itoa(episode)
}

// rowDue implements the due(row) rule described on NextSkipTask.
func rowDue(row *domain.SkipTiming, now time.Time) bool {
	if row == nil {
		return true
	}
	if row.OpStatus == domain.SkipPendingFP || row.EdStatus == domain.SkipPendingFP {
		return now.After(row.ProbedAt.Add(skipPendingFPTTL))
	}
	if row.OpStatus == domain.SkipUnreachable || row.EdStatus == domain.SkipUnreachable {
		return now.After(row.ProbedAt.Add(Backoff(row.Fails)))
	}
	return false // both sides detected/no_match → terminal
}

// rowFor looks up the stored row for a unit, or nil when none exists.
func rowFor(rowByKey map[string]*domain.SkipTiming, u SkipUnit) *domain.SkipTiming {
	return rowByKey[skipRowKey(u.Provider, u.Team, u.Episode)]
}

// rePairScan finds the first pair of adjacent same-family units with
// PairTried==false on the earlier row where at least one KIND has no_match
// on BOTH rows. Each kind qualifies independently — a kind detected on one
// row and no_match on the other is NOT a re-pair candidate (that's a
// legitimate single-episode no-OP finale/recap, not two genuinely OP-less
// neighbors) and must not be re-run, which would both waste a probe and
// AddFingerprint a duplicate for a kind that already has one.
func rePairScan(familyOrder []string, families map[string][]SkipUnit, rowByKey map[string]*domain.SkipTiming) *SkipTask {
	for _, key := range familyOrder {
		fam := families[key]
		for i := 0; i+1 < len(fam); i++ {
			r1 := rowFor(rowByKey, fam[i])
			r2 := rowFor(rowByKey, fam[i+1])
			if r1 == nil || r2 == nil {
				continue
			}
			if r1.PairTried {
				continue
			}
			var kinds []string
			if r1.OpStatus == domain.SkipNoMatch && r2.OpStatus == domain.SkipNoMatch {
				kinds = append(kinds, domain.SkipKindOp)
			}
			if r1.EdStatus == domain.SkipNoMatch && r2.EdStatus == domain.SkipNoMatch {
				kinds = append(kinds, domain.SkipKindEd)
			}
			if len(kinds) == 0 {
				continue
			}
			earlier, later := fam[i], fam[i+1]
			return &SkipTask{Unit: earlier, Pair: &later, RePair: true, PairKinds: kinds}
		}
	}
	return nil
}

// bootstrapPairScan finds the first family (in first-seen order) with at
// least two due episodes and returns a pair-bootstrap task for its first
// two due episodes, in family-slice order.
func bootstrapPairScan(familyOrder []string, families map[string][]SkipUnit, rowByKey map[string]*domain.SkipTiming, now time.Time) *SkipTask {
	for _, key := range familyOrder {
		var due []SkipUnit
		for _, u := range families[key] {
			if rowDue(rowFor(rowByKey, u), now) {
				due = append(due, u)
				if len(due) == 2 {
					break
				}
			}
		}
		if len(due) == 2 {
			return &SkipTask{Unit: due[0], Pair: &due[1]}
		}
	}
	return nil
}

// firstDueUnit scans families in first-seen order, episodes ascending
// within each family, and returns the first due unit found.
func firstDueUnit(familyOrder []string, families map[string][]SkipUnit, rowByKey map[string]*domain.SkipTiming, now time.Time) *SkipUnit {
	for _, key := range familyOrder {
		for _, u := range families[key] {
			if rowDue(rowFor(rowByKey, u), now) {
				u := u
				return &u
			}
		}
	}
	return nil
}
