package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// verifyHintMsg is the work item queued on the producer's channel.
type verifyHintMsg struct {
	AnimeID string `json:"anime_id"`
	Visitor string `json:"visitor"`
	Source  string `json:"source"`
}

// VerifyHintProducer is a fire-and-forget producer that POSTs watching
// hints to the content-verify service's /internal/verify/hint endpoint.
// Catalog already fires visit-hints on browse/detail views; this is the
// player-side counterpart, fired once per watched episode.
//
// Contract (mirrors RecsHintProducer):
//   - Buffered channel (cap 256) + single worker goroutine.
//   - Channel full or content-verify outage => hint DROPPED with WARN
//     (drop-on-full).
//   - 3-second HTTP timeout; no retries.
//   - Nil-receiver safe; all methods no-op when p == nil or !p.enabled.
//   - Call Start() once after construction; defer Stop() at process shutdown.
type VerifyHintProducer struct {
	url     string
	ch      chan verifyHintMsg
	client  *http.Client
	log     *logger.Logger
	wg      sync.WaitGroup
	enabled bool
}

// NewVerifyHintProducer constructs a producer. Call Start() before sending
// any hints.
func NewVerifyHintProducer(url string, enabled bool, log *logger.Logger) *VerifyHintProducer {
	return &VerifyHintProducer{
		url:     url,
		ch:      make(chan verifyHintMsg, 256),
		client:  &http.Client{Timeout: 3 * time.Second},
		log:     log,
		enabled: enabled,
	}
}

// Start launches the background worker goroutine. Must be called once before
// any Hint calls.
func (p *VerifyHintProducer) Start() {
	if p == nil || !p.enabled {
		return
	}
	p.wg.Add(1)
	go p.worker()
}

// Stop drains the channel and waits for the worker to finish. Call once at
// process shutdown (e.g. via defer after Start).
func (p *VerifyHintProducer) Stop() {
	if p == nil || !p.enabled {
		return
	}
	close(p.ch)
	p.wg.Wait()
}

// Hint enqueues a watching hint for the given user/anime. Non-blocking: drops
// with a WARN log if the channel is full.
// NOTE (shutdown ordering): the select-send below does NOT protect against a
// closed channel — a send racing Stop() would panic. This is safe today only
// because main.go calls srv.Shutdown() (draining all in-flight HTTP handlers,
// the only callers) BEFORE the deferred Stop() closes the channel. If you add
// a non-HTTP caller or reorder shutdown, guard sends with an atomic closed flag.
func (p *VerifyHintProducer) Hint(userID, animeID string) {
	if p == nil || !p.enabled || userID == "" || animeID == "" {
		return
	}
	msg := verifyHintMsg{AnimeID: animeID, Visitor: "u:" + userID, Source: "watching"}
	select {
	case p.ch <- msg:
	default:
		p.log.Warnw("verify hint channel full; dropping",
			"user_id", userID, "anime_id", animeID)
	}
}

// worker drains the channel and POSTs each hint to the content-verify service.
func (p *VerifyHintProducer) worker() {
	defer p.wg.Done()
	for msg := range p.ch {
		p.send(msg)
	}
}

// send posts one hint message. Non-fatal: errors are logged at WARN level.
func (p *VerifyHintProducer) send(msg verifyHintMsg) {
	body, err := json.Marshal(msg)
	if err != nil {
		p.log.Warnw("verify hint: failed to marshal payload", "error", err)
		return
	}
	endpoint := p.url + "/internal/verify/hint"
	resp, err := p.client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		p.log.Warnw("verify hint: POST failed", "endpoint", endpoint, "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		p.log.Warnw("verify hint: non-2xx response",
			"endpoint", endpoint, "status", resp.StatusCode)
	}
}
