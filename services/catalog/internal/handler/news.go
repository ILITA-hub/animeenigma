package handler

import (
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/telegram"
)

const (
	newsRedisKey = "news:telegram"
	newsTTL      = 30 * time.Minute
)

// NewsHandler handles news-related HTTP requests
type NewsHandler struct {
	telegramClient *telegram.Client
	cache          cache.Cache
	log            *logger.Logger
}

// NewNewsHandler creates a new NewsHandler
func NewNewsHandler(telegramClient *telegram.Client, cache cache.Cache, log *logger.Logger) *NewsHandler {
	return &NewsHandler{
		telegramClient: telegramClient,
		cache:          cache,
		log:            log,
	}
}

// GetNews returns cached Telegram channel news items
func (h *NewsHandler) GetNews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var items []telegram.NewsItem

	err := h.cache.GetOrSet(ctx, newsRedisKey, &items, newsTTL, func() (interface{}, error) {
		h.log.Infow("fetching news from telegram channel")

		fetched, err := h.telegramClient.FetchNews(ctx)
		if err != nil {
			h.log.Errorw("failed to fetch telegram news", "error", err)
			return nil, errors.ExternalAPI("telegram", err)
		}

		h.log.Infow("fetched telegram news", "count", len(fetched))
		return fetched, nil
	})
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, items)
}
