package queue

import (
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

// mkUnit is a small builder to keep the table below readable.
func mkUnit(provider, team string, episode int) SkipUnit {
	return SkipUnit{AnimeID: "a1", Provider: provider, Team: team, Episode: episode}
}

func mkRow(provider, team string, episode int, opStatus, edStatus string, probedAt time.Time) domain.SkipTiming {
	return domain.SkipTiming{AnimeID: "a1", Provider: provider, Team: team, Episode: episode,
		OpStatus: opStatus, EdStatus: edStatus, ProbedAt: probedAt}
}

func TestNextSkipTask(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name  string
		units []SkipUnit
		rows  []domain.SkipTiming
		fps   []domain.SkipFingerprint
		now   time.Time
		check func(t *testing.T, got *SkipTask)
	}{
		{
			// (a) empty rows + no fps → pair of episodes 1+2.
			name:  "a_bootstrap_pair_no_rows_no_fps",
			units: []SkipUnit{mkUnit("gogoanime", "", 1), mkUnit("gogoanime", "", 2), mkUnit("gogoanime", "", 3)},
			rows:  nil,
			fps:   nil,
			now:   now,
			check: func(t *testing.T, got *SkipTask) {
				if got == nil {
					t.Fatal("want a pair-bootstrap task, got nil")
				}
				if got.RePair {
					t.Fatal("bootstrap pair must not be RePair")
				}
				if got.Unit.Episode != 1 || got.Pair == nil || got.Pair.Episode != 2 {
					t.Fatalf("want Unit=ep1 Pair=ep2, got %+v pair=%+v", got.Unit, got.Pair)
				}
			},
		},
		{
			// (b) fps exist → locate of first missing (due) episode.
			name:  "b_locate_first_due_when_fps_exist",
			units: []SkipUnit{mkUnit("gogoanime", "", 1), mkUnit("gogoanime", "", 2), mkUnit("gogoanime", "", 3)},
			rows: []domain.SkipTiming{
				mkRow("gogoanime", "", 1, domain.SkipDetected, domain.SkipDetected, now.Add(-time.Hour)),
			},
			fps: []domain.SkipFingerprint{{AnimeID: "a1", Kind: domain.SkipKindOp}},
			now: now,
			check: func(t *testing.T, got *SkipTask) {
				if got == nil {
					t.Fatal("want a locate task, got nil")
				}
				if got.Pair != nil || got.RePair {
					t.Fatalf("locate task must not carry a Pair/RePair: %+v", got)
				}
				if got.Unit.Episode != 2 {
					t.Fatalf("want first missing episode (2), got %+v", got.Unit)
				}
			},
		},
		{
			// (c) all rows detected → nil (settled).
			name:  "c_all_detected_settled",
			units: []SkipUnit{mkUnit("gogoanime", "", 1), mkUnit("gogoanime", "", 2)},
			rows: []domain.SkipTiming{
				mkRow("gogoanime", "", 1, domain.SkipDetected, domain.SkipDetected, now.Add(-time.Hour)),
				mkRow("gogoanime", "", 2, domain.SkipDetected, domain.SkipNoMatch, now.Add(-time.Hour)),
			},
			fps: []domain.SkipFingerprint{{AnimeID: "a1", Kind: domain.SkipKindOp}},
			now: now,
			check: func(t *testing.T, got *SkipTask) {
				if got != nil {
					t.Fatalf("want nil (settled), got %+v", got)
				}
			},
		},
		{
			// (d1) adjacent no_match pair with PairTried=false → RePair task.
			name:  "d1_repair_adjacent_no_match",
			units: []SkipUnit{mkUnit("gogoanime", "", 1), mkUnit("gogoanime", "", 2)},
			rows: []domain.SkipTiming{
				{AnimeID: "a1", Provider: "gogoanime", Episode: 1, OpStatus: domain.SkipNoMatch, EdStatus: domain.SkipNoMatch, PairTried: false, ProbedAt: now.Add(-time.Hour)},
				{AnimeID: "a1", Provider: "gogoanime", Episode: 2, OpStatus: domain.SkipNoMatch, EdStatus: domain.SkipNoMatch, PairTried: false, ProbedAt: now.Add(-time.Hour)},
			},
			fps: []domain.SkipFingerprint{{AnimeID: "a1", Kind: domain.SkipKindOp}},
			now: now,
			check: func(t *testing.T, got *SkipTask) {
				if got == nil {
					t.Fatal("want a RePair task, got nil")
				}
				if !got.RePair {
					t.Fatalf("want RePair=true: %+v", got)
				}
				if got.Unit.Episode != 1 || got.Pair == nil || got.Pair.Episode != 2 {
					t.Fatalf("want Unit=ep1 Pair=ep2, got %+v pair=%+v", got.Unit, got.Pair)
				}
			},
		},
		{
			// (d2) same adjacency, but PairTried=true on the earlier row → settled, nil.
			name:  "d2_repair_already_tried_settles",
			units: []SkipUnit{mkUnit("gogoanime", "", 1), mkUnit("gogoanime", "", 2)},
			rows: []domain.SkipTiming{
				{AnimeID: "a1", Provider: "gogoanime", Episode: 1, OpStatus: domain.SkipNoMatch, EdStatus: domain.SkipNoMatch, PairTried: true, ProbedAt: now.Add(-time.Hour)},
				{AnimeID: "a1", Provider: "gogoanime", Episode: 2, OpStatus: domain.SkipNoMatch, EdStatus: domain.SkipNoMatch, PairTried: false, ProbedAt: now.Add(-time.Hour)},
			},
			fps: []domain.SkipFingerprint{{AnimeID: "a1", Kind: domain.SkipKindOp}},
			now: now,
			check: func(t *testing.T, got *SkipTask) {
				if got != nil {
					t.Fatalf("want nil (PairTried already on earlier row), got %+v", got)
				}
			},
		},
		{
			// (e) single due episode + no fps → locate task (not nil), not a pair.
			name:  "e_single_due_no_fps_locate",
			units: []SkipUnit{mkUnit("gogoanime", "", 1)},
			rows:  nil,
			fps:   nil,
			now:   now,
			check: func(t *testing.T, got *SkipTask) {
				if got == nil {
					t.Fatal("want a locate task, got nil")
				}
				if got.Pair != nil || got.RePair {
					t.Fatalf("single due episode must yield a locate task, not a pair: %+v", got)
				}
				if got.Unit.Episode != 1 {
					t.Fatalf("want ep1, got %+v", got.Unit)
				}
			},
		},
		{
			// (f1) pending_fp row older than 6h → due again.
			name:  "f1_pending_fp_older_than_6h_due",
			units: []SkipUnit{mkUnit("gogoanime", "", 5)},
			rows: []domain.SkipTiming{
				mkRow("gogoanime", "", 5, domain.SkipPendingFP, domain.SkipPendingFP, now.Add(-7*time.Hour)),
			},
			fps: []domain.SkipFingerprint{{AnimeID: "a1", Kind: domain.SkipKindOp}},
			now: now,
			check: func(t *testing.T, got *SkipTask) {
				if got == nil {
					t.Fatal("want due (>6h pending_fp), got nil")
				}
				if got.Unit.Episode != 5 {
					t.Fatalf("want ep5, got %+v", got.Unit)
				}
			},
		},
		{
			// (f2) pending_fp row younger than 6h → not due, nil.
			name:  "f2_pending_fp_younger_than_6h_not_due",
			units: []SkipUnit{mkUnit("gogoanime", "", 5)},
			rows: []domain.SkipTiming{
				mkRow("gogoanime", "", 5, domain.SkipPendingFP, domain.SkipPendingFP, now.Add(-time.Hour)),
			},
			fps: []domain.SkipFingerprint{{AnimeID: "a1", Kind: domain.SkipKindOp}},
			now: now,
			check: func(t *testing.T, got *SkipTask) {
				if got != nil {
					t.Fatalf("want nil (<6h pending_fp), got %+v", got)
				}
			},
		},
		{
			// (g1) unreachable row past Backoff(fails) → due again.
			name:  "g1_unreachable_past_backoff_due",
			units: []SkipUnit{mkUnit("gogoanime", "", 5)},
			rows: []domain.SkipTiming{
				{AnimeID: "a1", Provider: "gogoanime", Episode: 5, OpStatus: domain.SkipUnreachable, EdStatus: domain.SkipUnreachable, Fails: 2, ProbedAt: now.Add(-13 * time.Hour)}, // Backoff(2)=12h
			},
			fps: []domain.SkipFingerprint{{AnimeID: "a1", Kind: domain.SkipKindOp}},
			now: now,
			check: func(t *testing.T, got *SkipTask) {
				if got == nil {
					t.Fatal("want due (past 12h backoff), got nil")
				}
				if got.Unit.Episode != 5 {
					t.Fatalf("want ep5, got %+v", got.Unit)
				}
			},
		},
		{
			// (g2) unreachable row within Backoff(fails) → not due, nil.
			name:  "g2_unreachable_within_backoff_not_due",
			units: []SkipUnit{mkUnit("gogoanime", "", 5)},
			rows: []domain.SkipTiming{
				{AnimeID: "a1", Provider: "gogoanime", Episode: 5, OpStatus: domain.SkipUnreachable, EdStatus: domain.SkipUnreachable, Fails: 2, ProbedAt: now.Add(-time.Hour)}, // Backoff(2)=12h
			},
			fps: []domain.SkipFingerprint{{AnimeID: "a1", Kind: domain.SkipKindOp}},
			now: now,
			check: func(t *testing.T, got *SkipTask) {
				if got != nil {
					t.Fatalf("want nil (within 12h backoff), got %+v", got)
				}
			},
		},
		{
			// (h) family ordering: kodik pinned team's episodes appear before
			// the second team's — the units slice already carries that order
			// (enumeration order), and NextSkipTask must preserve first-seen
			// family order rather than re-sorting.
			name: "h_family_order_preserved_pinned_team_first",
			units: []SkipUnit{
				mkUnit("kodik", "PinnedTeam", 1), mkUnit("kodik", "PinnedTeam", 2),
				mkUnit("kodik", "SecondTeam", 1), mkUnit("kodik", "SecondTeam", 2),
			},
			rows: nil,
			fps:  nil,
			now:  now,
			check: func(t *testing.T, got *SkipTask) {
				if got == nil {
					t.Fatal("want a pair-bootstrap task, got nil")
				}
				if got.Unit.Team != "PinnedTeam" || got.Pair == nil || got.Pair.Team != "PinnedTeam" {
					t.Fatalf("want bootstrap pair from the first-seen family (PinnedTeam), got %+v pair=%+v", got.Unit, got.Pair)
				}
				if got.Unit.Episode != 1 || got.Pair.Episode != 2 {
					t.Fatalf("want episodes 1+2 of PinnedTeam, got %+v pair=%+v", got.Unit, got.Pair)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NextSkipTask(tc.units, tc.rows, tc.fps, tc.now)
			tc.check(t, got)
		})
	}
}

// TestNextSkipTaskNilOnEmptyUnits guards the degenerate case: no units at
// all means the skip lane trivially has nothing to do.
func TestNextSkipTaskNilOnEmptyUnits(t *testing.T) {
	if got := NextSkipTask(nil, nil, nil, time.Now()); got != nil {
		t.Fatalf("want nil for empty units, got %+v", got)
	}
}
