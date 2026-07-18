package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/alert"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/catalog"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/groq"
)

const (
	FanficBotUserID   = "00000000-0000-0000-0000-0000000000b0"
	FanficBotUsername = "AnimeEnigma"
)

type dailyRepo interface {
	ListEligibleSince(ctx context.Context, since time.Time) ([]domain.Fanfic, error)
	Create(ctx context.Context, f *domain.Fanfic) error
}

type animeMetaFetcher interface {
	FetchMeta(ctx context.Context, animeID, shikimoriID string) (catalog.AnimeMeta, error)
}

// EnsureResult reports what EnsureDaily did.
type EnsureResult struct {
	Generated bool
	Reason    string
	FanficID  string
}

// DailyService owns the "Фанфик дня" pick + idempotent bot-generation flow.
type DailyService struct {
	groq      streamer
	repo      dailyRepo
	meta      animeMetaFetcher
	alerter   alert.Alerter
	model     string
	animePool []string
	lang      string
	now       func() time.Time
	log       *logger.Logger
}

func NewDailyService(groq streamer, repo dailyRepo, meta animeMetaFetcher, alerter alert.Alerter, model string, animePool []string, lang string, now func() time.Time, log *logger.Logger) *DailyService {
	if now == nil {
		now = time.Now
	}
	if lang == "" {
		lang = "ru"
	}
	return &DailyService{groq: groq, repo: repo, meta: meta, alerter: alerter, model: model, animePool: animePool, lang: lang, now: now, log: log}
}

// DailyPick returns the day's fanfic (or nil). Shared by both daily handlers.
func (s *DailyService) DailyPick(ctx context.Context) (*domain.Fanfic, error) {
	now := s.now()
	eligible, err := s.repo.ListEligibleSince(ctx, EligibleWindowStart(now))
	if err != nil {
		return nil, err
	}
	return PickDaily(eligible, DailySeed(now)), nil
}

// EnsureDaily generates today's bot fanfic. It runs unconditionally of user
// fanfics: the bot is the guaranteed fallback for the day any user fanfic ages
// out of the window (PickDaily still prefers user fanfics, so an unneeded bot
// simply stays invisible), and the Groq call doubles as the daily key-health
// probe — a 401/403 fires a Telegram alert. Idempotent — skips only when a bot
// fanfic was already created on the CURRENT UTC day; a bot from yesterday
// still sitting in the eligibility window must NOT satisfy the check (it
// expires at the next midnight rollover, which would leave the day empty).
func (s *DailyService) EnsureDaily(ctx context.Context) (EnsureResult, error) {
	now := s.now()
	eligible, err := s.repo.ListEligibleSince(ctx, EligibleWindowStart(now))
	if err != nil {
		return EnsureResult{}, err
	}
	today := DailySeed(now)
	for _, f := range eligible {
		if f.AIGenerated && DailySeed(f.CreatedAt) == today {
			return EnsureResult{Generated: false, Reason: "bot_exists"}, nil
		}
	}

	req := s.randomRequest(ctx)
	system, user := BuildMessages(req, "")
	text, usage, err := s.groq.Stream(ctx, system, user, MaxTokensFor(req.Length), 0.9, func(string) {})
	if err != nil {
		var se *groq.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
			msg := fmt.Sprintf("🚨 Fanfic daily generation FAILED: Groq rejected the API key (status %d). Model=%s. Fix FANFIC_GROQ_API_KEY.", se.Code, s.model)
			_ = s.alerter.Send(ctx, msg)
			if s.log != nil {
				s.log.Errorw("fanfic.daily.groq_auth_failed", "status", se.Code)
			}
		} else if s.log != nil {
			s.log.Warnw("fanfic.daily.groq_failed", "error", err)
		}
		return EnsureResult{}, fmt.Errorf("ensure-daily: groq: %w", err)
	}

	title, body := SplitTitle(text)
	f := &domain.Fanfic{
		UserID:           FanficBotUserID,
		AuthorUsername:   FanficBotUsername,
		SpotlightCredit:  true,
		AIGenerated:      true,
		AnimeID:          req.Anime.ID,
		AnimeShikimoriID: req.Anime.ShikimoriID,
		AnimeTitle:       req.Anime.Title,
		AnimeJapanese:    req.Anime.Japanese,
		AnimePoster:      req.Anime.Poster,
		Length:           req.Length,
		POV:              req.POV,
		Rating:           req.Rating,
		Language:         req.Language,
		PartCount:        1,
		Title:            title,
		Content:          body,
		Model:            s.model,
		TokenUsage:       usage,
		Status:           domain.StatusComplete,
	}
	if err := s.repo.Create(ctx, f); err != nil {
		return EnsureResult{}, fmt.Errorf("ensure-daily: persist: %w", err)
	}
	return EnsureResult{Generated: true, Reason: "generated", FanficID: f.ID}, nil
}

// randomRequest builds deterministic-per-day random params (teen, RU) and fetches
// anime metadata (fail-soft — generation proceeds with whatever title is available).
func (s *DailyService) randomRequest(ctx context.Context) domain.GenerateRequest {
	seed := DailySeed(s.now())
	shiki := ""
	if len(s.animePool) > 0 {
		shiki = s.animePool[seed%len(s.animePool)]
	}
	anime := domain.AnimeRef{ShikimoriID: shiki, Title: "аниме"}
	if m, err := s.meta.FetchMeta(ctx, "", shiki); err == nil && m.Title != "" {
		anime.ID, anime.Title, anime.Japanese, anime.Poster = m.ID, m.Title, m.Japanese, m.Poster
	}
	tags := domain.CuratedTags
	t1 := tags[seed%len(tags)].Slug
	t2 := tags[(seed/7)%len(tags)].Slug
	povs := []string{"first", "third"}
	return domain.GenerateRequest{
		Anime:    anime,
		Tags:     []string{t1, t2},
		Length:   "oneshot",
		POV:      povs[seed%2],
		Rating:   "teen",
		Language: s.lang,
	}
}
