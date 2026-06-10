package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// creditMsg is the internal work item queued on the producer's channel.
type creditMsg struct {
	UserID string `json:"user_id"`
	Amount int64  `json:"amount"`
	Reason string `json:"reason"`
	Ref    string `json:"ref"`
}

// GachaCreditProducer is a fire-and-forget internal credit producer that posts
// earned «Энигмы» to the gacha service's /internal/gacha/credit endpoint.
//
// Design contract (spec §3.3):
//   - Buffered channel (cap 256) + single worker goroutine.
//   - If the channel is full, the event is DROPPED with a WARN log (drop-on-full).
//     A gacha outage or backpressure must never block MarkEpisodeWatched.
//   - 3-second HTTP client timeout; no retries (gacha dedups via ledger unique index).
//   - Nil-receiver safe: all public methods are no-ops when p == nil or !p.enabled.
//   - Call Start() once after construction; defer Stop() at process shutdown.
type GachaCreditProducer struct {
	url        string
	episodeAmt int64
	titleAmt   int64
	ch         chan creditMsg
	client     *http.Client
	log        *logger.Logger
	wg         sync.WaitGroup
	enabled    bool
}

// NewGachaCreditProducer constructs a producer. Call Start() before sending
// any events.
func NewGachaCreditProducer(url string, episodeAmt, titleAmt int64, enabled bool, log *logger.Logger) *GachaCreditProducer {
	return &GachaCreditProducer{
		url:        url,
		episodeAmt: episodeAmt,
		titleAmt:   titleAmt,
		ch:         make(chan creditMsg, 256),
		client:     &http.Client{Timeout: 3 * time.Second},
		log:        log,
		enabled:    enabled,
	}
}

// Start launches the background worker goroutine. Must be called once before
// any EpisodeWatched / TitleCompleted calls.
func (p *GachaCreditProducer) Start() {
	if p == nil || !p.enabled {
		return
	}
	p.wg.Add(1)
	go p.worker()
}

// Stop drains the channel and waits for the worker to finish. Call once at
// process shutdown (e.g. via defer after Start).
func (p *GachaCreditProducer) Stop() {
	if p == nil || !p.enabled {
		return
	}
	close(p.ch)
	p.wg.Wait()
}

// EpisodeWatched enqueues a credit event for a watched episode. Non-blocking:
// drops with a WARN log if the channel is full.
// ref format: "<animeID>:<episode>" — the gacha unique index deduplicates.
// NOTE (shutdown ordering): the select-send below does NOT protect against a
// closed channel — a send racing Stop() would panic. This is safe today only
// because main.go calls srv.Shutdown() (draining all in-flight HTTP handlers,
// the only callers) BEFORE the deferred Stop() closes the channel. If you add
// a non-HTTP caller or reorder shutdown, guard sends with an atomic closed flag.
func (p *GachaCreditProducer) EpisodeWatched(userID, animeID string, episode int) {
	if p == nil || !p.enabled {
		return
	}
	msg := creditMsg{
		UserID: userID,
		Amount: p.episodeAmt,
		Reason: "episode_watched",
		Ref:    fmt.Sprintf("%s:%d", animeID, episode),
	}
	select {
	case p.ch <- msg:
	default:
		p.log.Warnw("gacha credit channel full; dropping episode_watched event",
			"user_id", userID, "anime_id", animeID, "episode", episode)
	}
}

// TitleCompleted enqueues a credit event for completing a title. Non-blocking:
// drops with a WARN log if the channel is full.
// ref = animeID — the gacha unique index deduplicates per (user, reason, anime).
func (p *GachaCreditProducer) TitleCompleted(userID, animeID string) {
	if p == nil || !p.enabled {
		return
	}
	msg := creditMsg{
		UserID: userID,
		Amount: p.titleAmt,
		Reason: "title_completed",
		Ref:    animeID,
	}
	select {
	case p.ch <- msg:
	default:
		p.log.Warnw("gacha credit channel full; dropping title_completed event",
			"user_id", userID, "anime_id", animeID)
	}
}

// worker drains the channel and POSTs each message to the gacha service.
func (p *GachaCreditProducer) worker() {
	defer p.wg.Done()
	for msg := range p.ch {
		p.post(msg)
	}
}

// post sends one credit message. Non-fatal: errors are logged at WARN level.
func (p *GachaCreditProducer) post(msg creditMsg) {
	body, err := json.Marshal(msg)
	if err != nil {
		p.log.Warnw("gacha credit: failed to marshal payload", "error", err)
		return
	}
	endpoint := p.url + "/internal/gacha/credit"
	resp, err := p.client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		p.log.Warnw("gacha credit: POST failed", "endpoint", endpoint, "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		p.log.Warnw("gacha credit: non-200 response",
			"endpoint", endpoint, "status", resp.StatusCode)
	}
}
