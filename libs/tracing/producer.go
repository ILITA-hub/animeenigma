package tracing

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// effectsDropped counts effects dropped because the producer ring buffer was
// full. Exposed on the embedding BE service's /metrics. Registered once,
// process-wide; safe across multiple producers (they share the counter).
var effectsDropped = promauto.NewCounter(prometheus.CounterOpts{
	Name: "tracing_effects_dropped_total",
	Help: "Egress effects dropped because the in-process producer buffer was full.",
})

// ProducerConfig configures the async effect producer.
type ProducerConfig struct {
	AnalyticsURL  string        // base URL, e.g. "http://analytics:8092"; "/internal/effects" is appended
	BufferSize    int           // channel capacity; full channel drops (default 4096)
	MaxBatch      int           // flush when buffered count reaches this (default 256)
	FlushInterval time.Duration // flush at least this often (default 2s)
	Client        *http.Client  // optional; defaults to a 5s-timeout client
}

// Producer batches recorded Effects and POSTs them to analytics
// /internal/effects. It mirrors services/analytics ingest.Batcher: a buffered
// channel + non-blocking Record (drop-on-full) + a run goroutine that flushes
// on size or interval, with a graceful Stop() drain. It is the production
// EffectSink for the recording RoundTripper.
type Producer struct {
	url     string
	client  *http.Client
	cfg     ProducerConfig
	ch      chan Effect
	stop    chan struct{}
	done    chan struct{}
	dropped int64
}

// wireProducerEffect mirrors the analytics handler.wireEffect JSON contract.
type wireProducerEffect struct {
	Origin     string `json:"origin,omitempty"`
	Operation  string `json:"operation,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	EffectKind string `json:"effect_kind,omitempty"`
	TargetKind string `json:"target_kind,omitempty"`
	Target     string `json:"target,omitempty"`
	Status     int    `json:"status,omitempty"`
	Requests   int    `json:"requests,omitempty"`
	BytesIn    int    `json:"bytes_in,omitempty"`
	BytesOut   int    `json:"bytes_out,omitempty"`
	DurationMS int    `json:"duration_ms,omitempty"`
}

type wireProducerBatch struct {
	Effects []wireProducerEffect `json:"effects"`
}

// NewProducer builds a Producer with sane defaults.
func NewProducer(cfg ProducerConfig) *Producer {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 4096
	}
	if cfg.MaxBatch <= 0 {
		cfg.MaxBatch = 256
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 2 * time.Second
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Producer{
		url:    cfg.AnalyticsURL + "/internal/effects",
		client: client,
		cfg:    cfg,
		ch:     make(chan Effect, cfg.BufferSize),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Record enqueues an effect WITHOUT blocking. On a full buffer the effect is
// dropped and the dropped counter increments (D-10: the producer must never
// block the outbound request hot path).
func (p *Producer) Record(e Effect) {
	select {
	case p.ch <- e:
	default:
		atomic.AddInt64(&p.dropped, 1)
		effectsDropped.Inc()
	}
}

// Dropped returns the number of effects dropped due to a full buffer.
func (p *Producer) Dropped() int64 { return atomic.LoadInt64(&p.dropped) }

// Start launches the flush loop.
func (p *Producer) Start() { go p.run() }

func (p *Producer) run() {
	defer close(p.done)
	ticker := time.NewTicker(p.cfg.FlushInterval)
	defer ticker.Stop()
	buf := make([]Effect, 0, p.cfg.MaxBatch)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		p.post(buf)
		buf = buf[:0]
	}

	for {
		select {
		case e := <-p.ch:
			buf = append(buf, e)
			if len(buf) >= p.cfg.MaxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-p.stop:
			// Drain whatever is buffered, then exit.
			for {
				select {
				case e := <-p.ch:
					buf = append(buf, e)
					if len(buf) >= p.cfg.MaxBatch {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// post marshals and ships one batch. Failures are swallowed (best-effort —
// analytics is not billing data; a dropped batch must never break the caller).
func (p *Producer) post(effects []Effect) {
	batch := wireProducerBatch{Effects: make([]wireProducerEffect, 0, len(effects))}
	for _, e := range effects {
		target := e.Target
		if target == "" {
			target = e.Host
		}
		batch.Effects = append(batch.Effects, wireProducerEffect{
			Origin:     e.Origin,
			Operation:  e.Operation,
			UserID:     e.UserID,
			EffectKind: e.EffectKind,
			TargetKind: "host",
			Target:     target,
			Status:     e.Status,
			Requests:   e.Requests,
			BytesIn:    e.BytesIn,
			BytesOut:   e.BytesOut,
			DurationMS: e.DurationMS,
		})
	}
	body, err := json.Marshal(batch)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// Stop signals the flush loop to drain and exit, then waits for it. Idempotency
// is the caller's responsibility (call once at shutdown).
func (p *Producer) Stop() {
	close(p.stop)
	<-p.done
}
