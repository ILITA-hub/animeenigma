package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/service"
)

type dailyService interface {
	GetOrCreateToday(ctx context.Context) (*domain.DailyPuzzle, error)
	Guess(ctx context.Context, userID, animeID string) (*service.GuessOutcome, error)
	GiveUp(ctx context.Context, userID string) (*service.VisibleAnime, error)
	Resume(ctx context.Context, userID string) (*service.DailyState, error)
}
type endlessService interface {
	NewRound(ctx context.Context) (*service.EndlessRound, error)
	Guess(ctx context.Context, token, animeID string) (*service.GuessOutcome, error)
}
type statsService interface {
	Get(ctx context.Context, userID string) (*domain.UserStats, error)
}
type leaderboardService interface {
	Top(ctx context.Context, date string, n int) ([]service.LeaderEntry, error)
	RecordSolve(ctx context.Context, date, username string, attempts int, solveUnix int64) error
}
type searchService interface {
	Search(ctx context.Context, q string, limit int) []domain.PoolAnime
}

type AnidleHandler struct {
	daily   dailyService
	endless endlessService
	stats   statsService
	lb      leaderboardService
	search  searchService
	log     *logger.Logger
}

func NewAnidleHandler(d dailyService, e endlessService, st statsService, lb leaderboardService, s searchService) *AnidleHandler {
	return &AnidleHandler{daily: d, endless: e, stats: st, lb: lb, search: s}
}

func userID(r *http.Request) (id, username string) {
	if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil {
		return claims.UserID, claims.Username
	}
	return "", ""
}

type guessReq struct {
	AnimeID    string `json:"anime_id"`
	RoundToken string `json:"round_token"`
}

func (h *AnidleHandler) DailyMeta(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	state, err := h.daily.Resume(r.Context(), uid)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, state)
}

func (h *AnidleHandler) DailyGuess(w http.ResponseWriter, r *http.Request) {
	var req guessReq
	if err := httputil.Bind(r, &req); err != nil || req.AnimeID == "" {
		httputil.BadRequest(w, "anime_id is required")
		return
	}
	uid, username := userID(r)
	out, err := h.daily.Guess(r.Context(), uid, req.AnimeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	// leaderboard hook on a fresh solve for a logged-in user
	if out.Solved && uid != "" && username != "" && h.lb != nil {
		date := time.Now().UTC().Format("2006-01-02")
		_ = h.lb.RecordSolve(r.Context(), date, username, out.Attempt, time.Now().UTC().Unix())
	}
	httputil.OK(w, out)
}

func (h *AnidleHandler) DailyGiveUp(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	ans, err := h.daily.GiveUp(r.Context(), uid)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]any{"answer": ans})
}

func (h *AnidleHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	res := h.search.Search(r.Context(), q, 10)
	if res == nil {
		res = []domain.PoolAnime{}
	}
	httputil.OK(w, res)
}

func (h *AnidleHandler) EndlessNew(w http.ResponseWriter, r *http.Request) {
	round, err := h.endless.NewRound(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, round)
}

func (h *AnidleHandler) EndlessGuess(w http.ResponseWriter, r *http.Request) {
	var req guessReq
	if err := httputil.Bind(r, &req); err != nil || req.RoundToken == "" || req.AnimeID == "" {
		httputil.BadRequest(w, "round_token and anime_id are required")
		return
	}
	out, err := h.endless.Guess(r.Context(), req.RoundToken, req.AnimeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, out)
}

func (h *AnidleHandler) Stats(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	if uid == "" {
		httputil.NoContent(w)
		return
	}
	st, err := h.stats.Get(r.Context(), uid)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, st)
}

func (h *AnidleHandler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	top, err := h.lb.Top(r.Context(), date, 50)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, top)
}
