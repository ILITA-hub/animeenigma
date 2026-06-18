package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// demandChanCap is the buffered-channel capacity for the demand producer.
// Matches the RecsHintProducer cap (256). When full, demands are dropped with
// a WARN — the autocache Planner's periodic drain + the per-heartbeat re-fire
// make a dropped next_ep demand self-healing.
const demandChanCap = 256

// demandMsg is the work item queued on the producer's channel. JSON tags match
// the library /internal/library/autocache/demand wire contract {mal_id, episode,
// reason, titles}; the handler (Plan 09-01 Task 4) validates-and-honors the
// reason. Titles is the ordered fallback title list (name_jp → romaji → name_en)
// the library Planner searches trackers with.
type demandMsg struct {
	MalID   string          `json:"mal_id"`
	Episode int             `json:"episode"`
	Reason  string          `json:"reason"`
	Titles  []string        `json:"titles,omitempty"`
	Trigger *DemandTrigger  `json:"trigger,omitempty"`
}

// DemandTrigger is the cause→effect watcher context the player attaches to a
// Logic-B next_ep demand: who watched, the combo, and the episode they were
// actually watching (the cause). The demand's Episode is the TARGET (N+1) — what
// the autocache fetches. The library appends this to autocache_trigger_log so the
// dashboard can show the watch that caused each download.
type DemandTrigger struct {
	UserID         string `json:"user_id,omitempty"`
	Username       string `json:"username,omitempty"`
	Player         string `json:"player,omitempty"`
	Language       string `json:"language,omitempty"`
	WatchType      string `json:"watch_type,omitempty"`
	WatchedEpisode int    `json:"watched_episode,omitempty"`
}

// DemandProducer is a fire-and-forget producer that POSTs autocache demands to
// the library service's /internal/library/autocache/demand endpoint (Phase 9
// Logic B / TRIG-02). It is a verbatim clone of the proven RecsHintProducer
// pattern so that a slow or down library can NEVER block or fail the player
// heartbeat (UpdateProgress) that fires it.
//
// Contract (mirrors RecsHintProducer):
//   - Buffered channel (cap 256) + single worker goroutine.
//   - Channel full or library outage => demand DROPPED with WARN (drop-on-full).
//     The library Planner's drain loop + the per-heartbeat re-fire recover it.
//   - 3-second HTTP timeout; no retries.
//   - Nil-receiver safe; all methods no-op when p == nil or !p.enabled.
//   - Call Start() once after construction; defer Stop() at process shutdown.
type DemandProducer struct {
	url     string
	ch      chan demandMsg
	client  *http.Client
	log     *logger.Logger
	wg      sync.WaitGroup
	enabled bool
}

// NewDemandProducer constructs a producer. libraryURL is the base URL of the
// library service inside the Docker network (only the path
// /internal/library/autocache/demand is called). Call Start() before any Want
// calls.
func NewDemandProducer(libraryURL string, enabled bool, log *logger.Logger) *DemandProducer {
	return &DemandProducer{
		url:     libraryURL,
		ch:      make(chan demandMsg, demandChanCap),
		client:  &http.Client{Timeout: 3 * time.Second},
		log:     log,
		enabled: enabled,
	}
}

// Start launches the background worker goroutine. Must be called once before
// any Want calls.
func (p *DemandProducer) Start() {
	if p == nil || !p.enabled {
		return
	}
	p.wg.Add(1)
	go p.worker()
}

// Stop drains the channel and waits for the worker to finish. Call once at
// process shutdown (e.g. via defer after Start).
func (p *DemandProducer) Stop() {
	if p == nil || !p.enabled {
		return
	}
	close(p.ch)
	p.wg.Wait()
}

// Want enqueues an autocache demand for (malID, episode, reason). Non-blocking:
// drops with a WARN log if the channel is full.
// NOTE (shutdown ordering): the select-send below does NOT protect against a
// closed channel — a send racing Stop() would panic. This is safe today only
// because main.go calls srv.Shutdown() (draining all in-flight HTTP handlers,
// the only callers) BEFORE the deferred Stop() closes the channel. If you add
// a non-HTTP caller or reorder shutdown, guard sends with an atomic closed flag.
func (p *DemandProducer) Want(malID string, episode int, reason string, titles []string, trigger *DemandTrigger) {
	if p == nil || !p.enabled {
		return
	}
	msg := demandMsg{MalID: malID, Episode: episode, Reason: reason, Titles: titles, Trigger: trigger}
	select {
	case p.ch <- msg:
	default:
		p.log.Warnw("autocache demand channel full; dropping demand",
			"mal_id", malID, "episode", episode, "reason", reason)
	}
}

// worker drains the channel and POSTs each demand to the library service.
func (p *DemandProducer) worker() {
	defer p.wg.Done()
	for msg := range p.ch {
		p.send(msg)
	}
}

// send posts one demand message. Non-fatal: errors are logged at WARN level and
// never returned to the caller (fire-and-forget).
func (p *DemandProducer) send(msg demandMsg) {
	body, err := json.Marshal(msg)
	if err != nil {
		p.log.Warnw("autocache demand: failed to marshal payload", "error", err)
		return
	}
	endpoint := p.url + "/internal/library/autocache/demand"
	resp, err := p.client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		p.log.Warnw("autocache demand: POST failed", "endpoint", endpoint, "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		p.log.Warnw("autocache demand: non-2xx response",
			"endpoint", endpoint, "status", resp.StatusCode,
			"mal_id", msg.MalID, "episode", msg.Episode, "reason", msg.Reason)
	}
}
