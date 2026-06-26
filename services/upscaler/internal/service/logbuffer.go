package service

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// LogLine is one log entry from a worker stream.
type LogLine struct {
	Source  string    `json:"source"` // e.g. "worker", "orchestrator"
	Level   string    `json:"level"`  // "info", "warn", "error"
	Msg     string    `json:"msg"`
	Segment int       `json:"segment,omitempty"`
	TS      time.Time `json:"ts"`
}

// LogBufferConfig controls ring-buffer capacity.
type LogBufferConfig struct {
	Cap         int           // max lines per job (default 500)
	KeyTTL      time.Duration // Redis key TTL (default 24h)
	FlushBucket string        // MinIO bucket for Flush (default "upscaler-logs")
}

func (c LogBufferConfig) withDefaults() LogBufferConfig {
	if c.Cap <= 0 {
		c.Cap = 500
	}
	if c.KeyTTL <= 0 {
		c.KeyTTL = 24 * time.Hour
	}
	if c.FlushBucket == "" {
		c.FlushBucket = "upscaler-logs"
	}
	return c
}

// logRedis abstracts the Redis list operations LogBuffer needs.
// *redis.Client satisfies this interface via redisLogAdapter. Tests use memLogRedis.
type logRedis interface {
	appendLog(ctx context.Context, key, val string, cap int, ttl time.Duration) error
	rangeLogs(ctx context.Context, key string, n int) ([]string, error)
}

// logFlusher writes log dumps to object storage.
type logFlusher interface {
	PutObject(ctx context.Context, bucket, key string, data []byte, contentType string) error
}

// LogBuffer stores per-job log lines in a Redis ring-buffer.
// It also fans out in-process to Subscribe callers.
type LogBuffer struct {
	redis   logRedis
	cfg     LogBufferConfig
	flusher logFlusher // optional; Flush no-ops if nil
	log     *logger.Logger

	mu   sync.RWMutex
	subs map[string][]chan LogLine
}

// NewLogBuffer constructs a LogBuffer.
func NewLogBuffer(rc logRedis, cfg LogBufferConfig) *LogBuffer {
	return &LogBuffer{
		redis: rc,
		cfg:   cfg.withDefaults(),
		log:   logger.Default(),
		subs:  make(map[string][]chan LogLine),
	}
}

// WithFlusher wires an optional MinIO flusher.
func (b *LogBuffer) WithFlusher(f logFlusher) *LogBuffer {
	b.flusher = f
	return b
}

func (b *LogBuffer) listKey(jobID string) string { return "upscaler:logs:" + jobID }

// Append adds a log line to the Redis ring-buffer and fans out to subscribers.
func (b *LogBuffer) Append(ctx context.Context, jobID string, line LogLine) error {
	raw, err := json.Marshal(line)
	if err != nil {
		return err
	}
	if err := b.redis.appendLog(ctx, b.listKey(jobID), string(raw), b.cfg.Cap, b.cfg.KeyTTL); err != nil {
		return err
	}
	// Fan out to in-process subscribers (non-blocking). The read lock is held
	// across the sends so a concurrent Subscribe-cancel (which takes the write
	// lock to remove AND close the channel) cannot close a channel mid-send —
	// that would panic with "send on closed channel". The sends are non-blocking
	// (select/default), so holding the read lock here is cheap and never stalls.
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs[jobID] {
		select {
		case ch <- line:
		default:
			// Channel full — skip.
		}
	}
	return nil
}

// Tail returns the last n lines for a job from Redis.
func (b *LogBuffer) Tail(ctx context.Context, jobID string, n int) []LogLine {
	strs, err := b.redis.rangeLogs(ctx, b.listKey(jobID), n)
	if err != nil || len(strs) == 0 {
		return nil
	}
	out := make([]LogLine, 0, len(strs))
	for _, s := range strs {
		var l LogLine
		if err := json.Unmarshal([]byte(s), &l); err == nil {
			out = append(out, l)
		}
	}
	return out
}

// Subscribe returns a buffered channel and a cancel func for in-process subscription.
func (b *LogBuffer) Subscribe(jobID string) (<-chan LogLine, func()) {
	ch := make(chan LogLine, 64)
	b.mu.Lock()
	b.subs[jobID] = append(b.subs[jobID], ch)
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		chs := b.subs[jobID]
		for i, c := range chs {
			if c == ch {
				b.subs[jobID] = append(chs[:i], chs[i+1:]...)
				break
			}
		}
		close(ch)
	}
	return ch, cancel
}

// Flush dumps all buffered log lines to MinIO.
// If flusher is nil, logs a message and returns nil (no-op).
func (b *LogBuffer) Flush(ctx context.Context, jobID string) error {
	if b.flusher == nil {
		b.log.Infow("logbuffer: flush not wired — MinIO flusher not set", "job_id", jobID)
		return nil
	}
	lines := b.Tail(ctx, jobID, b.cfg.Cap)
	data, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	return b.flusher.PutObject(ctx, b.cfg.FlushBucket, "logs/"+jobID+".json", data, "application/json")
}
