package service

import (
	"context"
	"encoding/json"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"gorm.io/datatypes"
)

// Emit sends one SSE event to the client. A non-nil return (client gone) is
// logged and ignored — server-side accumulation + persistence continue.
type Emit func(event string, data any) error

type streamer interface {
	Stream(ctx context.Context, system, user string, maxTokens int, temperature float64, onDelta func(string)) (string, int, error)
}

type fanficStore interface {
	Create(ctx context.Context, f *domain.Fanfic) error
	UpdateResult(ctx context.Context, id, title, content string, usage int) error
	MarkFailed(ctx context.Context, id, msg string) error
	Get(ctx context.Context, userID, id string) (*domain.Fanfic, error)
	AppendPart(ctx context.Context, userID, id, appended string, addedUsage, newPartCount int) error
}

type quota interface {
	Acquire(ctx context.Context, userID string) (func(), error)
}

// synopsisFetcher preloads an anime's real synopsis for canon mode. Nil-safe:
// a nil fetcher (or a non-canon request) skips the preload.
type synopsisFetcher interface {
	FetchSynopsis(ctx context.Context, animeID, shikimoriID string) (title, synopsis string, err error)
}

type Generator struct {
	groq         streamer
	store        fanficStore
	quota        quota
	catalog      synopsisFetcher
	model        string
	contextRunes int
	log          *logger.Logger
}

func NewGenerator(groq streamer, store fanficStore, quota quota, catalog synopsisFetcher, model string, contextRunes int, log *logger.Logger) *Generator {
	if contextRunes <= 0 {
		contextRunes = 24000
	}
	return &Generator{groq: groq, store: store, quota: quota, catalog: catalog, model: model, contextRunes: contextRunes, log: log}
}

func (g *Generator) Generate(ctx context.Context, userID string, req domain.GenerateRequest, emit Emit) error {
	release, err := g.quota.Acquire(ctx, userID)
	if err != nil {
		g.safeEmit(emit, "error", map[string]any{"message": err.Error()})
		return err
	}
	defer release()

	chars, _ := json.Marshal(req.Characters)
	tags, _ := json.Marshal(req.Tags)
	f := &domain.Fanfic{
		UserID:           userID,
		AnimeID:          req.Anime.ID,
		AnimeShikimoriID: req.Anime.ShikimoriID,
		AnimeTitle:       req.Anime.Title,
		AnimeJapanese:    req.Anime.Japanese,
		AnimePoster:      req.Anime.Poster,
		Characters:       datatypes.JSON(chars),
		Tags:             datatypes.JSON(tags),
		Length:           req.Length,
		POV:              req.POV,
		Rating:           req.Rating,
		Language:         req.Language,
		Prompt:           req.Prompt,
		Canon:            req.Canon,
		PartCount:        1,
		Model:            g.model,
		Status:           domain.StatusGenerating,
	}
	if err := g.store.Create(ctx, f); err != nil {
		return err
	}
	g.safeEmit(emit, "meta", map[string]any{"id": f.ID, "model": g.model})

	synopsis := ""
	if req.Canon && g.catalog != nil {
		if _, syn, err := g.catalog.FetchSynopsis(ctx, req.Anime.ID, req.Anime.ShikimoriID); err != nil {
			if g.log != nil {
				g.log.Warnw("canon synopsis preload failed; continuing without it", "anime_id", req.Anime.ID, "error", err)
			}
		} else {
			synopsis = syn
		}
	}

	system, user := BuildMessages(req, synopsis)
	text, usage, err := g.groq.Stream(ctx, system, user, MaxTokensFor(req.Length), 0.9, func(delta string) {
		g.safeEmit(emit, "delta", map[string]any{"text": delta})
	})
	if err != nil {
		_ = g.store.MarkFailed(ctx, f.ID, err.Error())
		g.safeEmit(emit, "error", map[string]any{"message": err.Error()})
		return err
	}

	title, body := SplitTitle(text)
	if err := g.store.UpdateResult(ctx, f.ID, title, body, usage); err != nil {
		if g.log != nil {
			g.log.Errorw("failed to persist fanfic result", "id", f.ID, "error", err)
		}
	}
	g.safeEmit(emit, "done", map[string]any{"id": f.ID, "title": title, "token_usage": usage})
	return nil
}

func (g *Generator) safeEmit(emit Emit, event string, data any) {
	if emit == nil {
		return
	}
	if err := emit(event, data); err != nil && g.log != nil {
		g.log.Debugw("sse emit failed (client likely disconnected)", "event", event, "error", err)
	}
}
