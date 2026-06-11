package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// recsHintMsg is the work item queued on the producer's channel.
type recsHintMsg struct {
	UserID  string `json:"user_id"`
	AnimeID string `json:"anime_id"`
}

// RecsHintProducer is a fire-and-forget producer that POSTs watch-activity
// hints to the recs service's /internal/recs/recompute-hint endpoint.
// Extraction Phase 1 (spec 2026-06-11): replaces the in-process
// userOrchestrator.TriggerForUser + synchronous S6 seed update that lived in
// ListService.MarkEpisodeWatched before the recs engine moved out of player.
//
// Contract (mirrors GachaCreditProducer):
//   - Buffered channel (cap 256) + single worker goroutine.
//   - Channel full or recs outage => hint DROPPED with WARN (drop-on-full).
//     Worst case the recs 6h cron refreshes the user instead.
//   - 3-second HTTP timeout; no retries.
//   - Nil-receiver safe; all methods no-op when p == nil or !p.enabled.
//   - Call Start() once after construction; defer Stop() at process shutdown.
type RecsHintProducer struct {
	url     string
	ch      chan recsHintMsg
	client  *http.Client
	log     *logger.Logger
	wg      sync.WaitGroup
	enabled bool
}

// NewRecsHintProducer constructs a producer. Call Start() before sending any
// hints.
func NewRecsHintProducer(url string, enabled bool, log *logger.Logger) *RecsHintProducer {
	return &RecsHintProducer{
		url:     url,
		ch:      make(chan recsHintMsg, 256),
		client:  &http.Client{Timeout: 3 * time.Second},
		log:     log,
		enabled: enabled,
	}
}

// Start launches the background worker goroutine. Must be called once before
// any Hint calls.
func (p *RecsHintProducer) Start() {
	if p == nil || !p.enabled {
		return
	}
	p.wg.Add(1)
	go p.worker()
}

// Stop drains the channel and waits for the worker to finish. Call once at
// process shutdown (e.g. via defer after Start).
func (p *RecsHintProducer) Stop() {
	if p == nil || !p.enabled {
		return
	}
	close(p.ch)
	p.wg.Wait()
}

// Hint enqueues a recompute hint for the given user/anime. Non-blocking: drops
// with a WARN log if the channel is full.
// NOTE (shutdown ordering): the select-send below does NOT protect against a
// closed channel — a send racing Stop() would panic. This is safe today only
// because main.go calls srv.Shutdown() (draining all in-flight HTTP handlers,
// the only callers) BEFORE the deferred Stop() closes the channel. If you add
// a non-HTTP caller or reorder shutdown, guard sends with an atomic closed flag.
func (p *RecsHintProducer) Hint(userID, animeID string) {
	if p == nil || !p.enabled {
		return
	}
	msg := recsHintMsg{UserID: userID, AnimeID: animeID}
	select {
	case p.ch <- msg:
	default:
		p.log.Warnw("recs hint channel full; dropping recompute hint",
			"user_id", userID, "anime_id", animeID)
	}
}

// worker drains the channel and POSTs each hint to the recs service.
func (p *RecsHintProducer) worker() {
	defer p.wg.Done()
	for msg := range p.ch {
		p.send(msg)
	}
}

// send posts one hint message. Non-fatal: errors are logged at WARN level.
func (p *RecsHintProducer) send(msg recsHintMsg) {
	body, err := json.Marshal(msg)
	if err != nil {
		p.log.Warnw("recs hint: failed to marshal payload", "error", err)
		return
	}
	endpoint := p.url + "/internal/recs/recompute-hint"
	resp, err := p.client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		p.log.Warnw("recs hint: POST failed", "endpoint", endpoint, "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		p.log.Warnw("recs hint: non-2xx response",
			"endpoint", endpoint, "status", resp.StatusCode)
	}
}
