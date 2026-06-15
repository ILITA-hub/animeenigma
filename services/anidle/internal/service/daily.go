package service

import (
	"context"
	"errors"
	"hash/fnv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/repo"
)

const recentExclusionDays = 30

type dailyRepo interface {
	GetDailyPuzzle(ctx context.Context, date string) (*domain.DailyPuzzle, error)
	CreateDailyPuzzle(ctx context.Context, p *domain.DailyPuzzle) error
	RecentAnswerIDs(ctx context.Context, days int) ([]string, error)
}

type poolReader interface {
	All(ctx context.Context) ([]domain.PoolAnime, error)
	Lookup(id string) (domain.PoolAnime, bool)
}

type resultStore interface {
	GetUserResult(ctx context.Context, userID, date, mode string) (*domain.UserGameResult, error)
	SaveUserResult(ctx context.Context, r *domain.UserGameResult) error
}

type statsUpdater interface {
	RecordDailyResult(ctx context.Context, userID, date string, won bool, attempts int) error
}

// DailyService is used by Guess/Resume (Task 3); declared here, used there.
type DailyService struct {
	repo  dailyRepo
	pool  poolReader
	clock Clock
	rs    resultStore // nil-safe: guest-only deployments still work
	stats statsUpdater
	log   *logger.Logger
}

func NewDailyService(r dailyRepo, p poolReader, clock Clock, rs resultStore, stats statsUpdater) *DailyService {
	if clock == nil {
		clock = realClock{}
	}
	return &DailyService{repo: r, pool: p, clock: clock, rs: rs, stats: stats}
}

// GetOrCreateToday returns today's puzzle, creating it deterministically on first call.
func (s *DailyService) GetOrCreateToday(ctx context.Context) (*domain.DailyPuzzle, error) {
	date := s.clock.Today()
	if p, err := s.repo.GetDailyPuzzle(ctx, date); err == nil {
		return p, nil
	} else if !errors.Is(err, repo.ErrNotFound) {
		return nil, err
	}

	pool, err := s.pool.All(ctx)
	if err != nil {
		return nil, err
	}
	if len(pool) == 0 {
		return nil, errors.New("anidle: empty pool")
	}

	recent, err := s.repo.RecentAnswerIDs(ctx, recentExclusionDays)
	if err != nil {
		return nil, err
	}
	recentSet := make(map[string]struct{}, len(recent))
	for _, id := range recent {
		recentSet[id] = struct{}{}
	}
	eligible := make([]domain.PoolAnime, 0, len(pool))
	for _, a := range pool {
		if _, bad := recentSet[a.ID]; !bad {
			eligible = append(eligible, a)
		}
	}
	if len(eligible) == 0 {
		eligible = pool // everything used recently — fall back to full pool
	}

	idx := int(hashDate(date) % uint32(len(eligible)))
	secret := eligible[idx]

	p := &domain.DailyPuzzle{Date: date, AnimeID: secret.ID, AnswerSnapshot: secret}
	if err := s.repo.CreateDailyPuzzle(ctx, p); err != nil {
		// lost a create race — re-read the winner
		if existing, gerr := s.repo.GetDailyPuzzle(ctx, date); gerr == nil {
			return existing, nil
		}
		return nil, err
	}
	return p, nil
}

func hashDate(date string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(date))
	return h.Sum32()
}

const modeDaily = "daily"

// VisibleAnime is the guessed anime's public fields echoed back to the client.
type VisibleAnime struct {
	ID        string `json:"id"`
	NameRU    string `json:"name_ru"`
	NameEN    string `json:"name_en"`
	PosterURL string `json:"poster_url"`
}

// GuessOutcome is the per-guess response (no secret unless solved).
type GuessOutcome struct {
	Anime   VisibleAnime           `json:"anime"`
	Result  domain.GuessComparison `json:"result"`
	Solved  bool                   `json:"solved"`
	Attempt int                    `json:"attempt"`
	Answer  *VisibleAnime          `json:"answer,omitempty"`
}

// DailyState is the resume payload (GET /daily for logged-in).
type DailyState struct {
	Date    string         `json:"date"`
	Solved  bool           `json:"solved"`
	Guesses []GuessOutcome `json:"guesses"`
	Answer  *VisibleAnime  `json:"answer,omitempty"`
}

func visible(a domain.PoolAnime) VisibleAnime {
	return VisibleAnime{ID: a.ID, NameRU: a.NameRU, NameEN: a.NameEN, PosterURL: a.PosterURL}
}

// Guess scores one guess. userID == "" means an anonymous guest (no persistence).
func (s *DailyService) Guess(ctx context.Context, userID, animeID string) (*GuessOutcome, error) {
	puzzle, err := s.GetOrCreateToday(ctx)
	if err != nil {
		return nil, err
	}
	guess, ok := s.pool.Lookup(animeID)
	if !ok {
		return nil, errors.New("anidle: unknown anime")
	}
	secret := puzzle.AnswerSnapshot
	solved := animeID == puzzle.AnimeID

	out := &GuessOutcome{
		Anime:  visible(guess),
		Result: Compare(secret, guess),
		Solved: solved,
	}

	if userID == "" || s.rs == nil { // guest path: compare only
		out.Attempt = 0
		if solved {
			a := visible(secret)
			out.Answer = &a
		}
		return out, nil
	}

	res, err := s.rs.GetUserResult(ctx, userID, puzzle.Date, modeDaily)
	if err != nil {
		return nil, err
	}
	if res == nil {
		res = &domain.UserGameResult{UserID: userID, PuzzleDate: puzzle.Date, Mode: modeDaily}
	}
	if !res.Solved { // ignore extra guesses after a solve
		res.Guesses = append(res.Guesses, animeID)
		res.Attempts = len(res.Guesses)
		if solved {
			res.Solved = true
			now := timeNow()
			res.SolvedAt = &now
		}
		if err := s.rs.SaveUserResult(ctx, res); err != nil {
			return nil, err
		}
		if solved && s.stats != nil {
			if serr := s.stats.RecordDailyResult(ctx, userID, puzzle.Date, true, res.Attempts); serr != nil && s.log != nil {
				s.log.Warnw("record daily stats failed", "user", userID, "error", serr)
			}
		}
	}
	out.Attempt = res.Attempts
	if res.Solved {
		a := visible(secret)
		out.Answer = &a
	}
	return out, nil
}

// GiveUp marks the day lost for a logged-in user and reveals the answer.
func (s *DailyService) GiveUp(ctx context.Context, userID string) (*VisibleAnime, error) {
	puzzle, err := s.GetOrCreateToday(ctx)
	if err != nil {
		return nil, err
	}
	if userID != "" && s.rs != nil {
		res, err := s.rs.GetUserResult(ctx, userID, puzzle.Date, modeDaily)
		if err != nil {
			return nil, err
		}
		if res == nil {
			res = &domain.UserGameResult{UserID: userID, PuzzleDate: puzzle.Date, Mode: modeDaily}
		}
		if !res.Solved {
			res.Attempts = len(res.Guesses)
			if err := s.rs.SaveUserResult(ctx, res); err != nil {
				return nil, err
			}
			if s.stats != nil {
				_ = s.stats.RecordDailyResult(ctx, userID, puzzle.Date, false, res.Attempts)
			}
		}
	}
	a := visible(puzzle.AnswerSnapshot)
	return &a, nil
}

// Resume rebuilds a logged-in user's progress for today (no secret unless solved).
func (s *DailyService) Resume(ctx context.Context, userID string) (*DailyState, error) {
	puzzle, err := s.GetOrCreateToday(ctx)
	if err != nil {
		return nil, err
	}
	state := &DailyState{Date: puzzle.Date, Guesses: []GuessOutcome{}}
	if userID == "" || s.rs == nil {
		return state, nil
	}
	res, err := s.rs.GetUserResult(ctx, userID, puzzle.Date, modeDaily)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return state, nil
	}
	state.Solved = res.Solved
	for _, gid := range res.Guesses {
		g, ok := s.pool.Lookup(gid)
		if !ok {
			continue
		}
		state.Guesses = append(state.Guesses, GuessOutcome{
			Anime:  visible(g),
			Result: Compare(puzzle.AnswerSnapshot, g),
			Solved: gid == puzzle.AnimeID,
		})
	}
	if res.Solved {
		a := visible(puzzle.AnswerSnapshot)
		state.Answer = &a
	}
	return state, nil
}

var timeNow = func() time.Time { return time.Now().UTC() }
