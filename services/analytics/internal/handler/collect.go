// Package handler holds the analytics HTTP handlers.
package handler

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/service"
)

// Sink is the subset of the batcher the handler needs (enables a fake in
// tests).
type Sink interface {
	Enqueue(domain.Event) bool
}

// CollectHandler parses beacon batches and enqueues validated events.
type CollectHandler struct {
	sink Sink
	salt string
}

func NewCollectHandler(sink Sink, ipSalt string) *CollectHandler {
	return &CollectHandler{sink: sink, salt: ipSalt}
}

// wire types mirror the snippet contract (spec §5).
type wireCtx struct {
	UserAgent string `json:"user_agent"`
	ScreenW   int    `json:"screen_w"`
	ScreenH   int    `json:"screen_h"`
}

type wireEvent struct {
	EventType  string            `json:"event_type"`
	EventName  string            `json:"event_name"`
	Timestamp  time.Time         `json:"timestamp"`
	URL        string            `json:"url"`
	Path       string            `json:"path"`
	Referrer   string            `json:"referrer"`
	Title      string            `json:"title"`
	ElSelector string            `json:"el_selector"`
	ElText     string            `json:"el_text"`
	ElTag      string            `json:"el_tag"`
	ElAttrs    map[string]string `json:"el_attrs"`
	ActiveMS   int               `json:"active_ms"`
	TraceID    string            `json:"trace_id"`
	Properties json.RawMessage   `json:"properties"`
}

type wireEnvelope struct {
	AnonymousID string      `json:"anonymous_id"`
	UserID      string      `json:"user_id"`
	SessionID   string      `json:"session_id"`
	Events      []wireEvent `json:"events"`
	Ctx         wireCtx     `json:"ctx"`
}

func (h *CollectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024)) // 256 KB cap
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var env wireEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	ipHash := service.HashIP(clientIP(r), h.salt, now)

	for _, we := range env.Events {
		ts := we.Timestamp
		if ts.IsZero() {
			ts = now
		}
		// Drop events with absurd clock skew (>1 day either way).
		if ts.After(now.Add(24*time.Hour)) || ts.Before(now.Add(-24*time.Hour)) {
			continue
		}
		attrs := "{}"
		if len(we.ElAttrs) > 0 {
			if b, err := json.Marshal(we.ElAttrs); err == nil {
				attrs = string(b)
			}
		}
		props := "{}"
		if len(we.Properties) > 0 {
			props = string(we.Properties)
		}
		ev := domain.Event{
			EventID:     uuid.NewString(),
			EventType:   domain.EventType(we.EventType),
			EventName:   we.EventName,
			AnonymousID: env.AnonymousID,
			UserID:      env.UserID,
			SessionID:   env.SessionID,
			Timestamp:   ts.UTC(),
			ReceivedAt:  now,
			URL:         we.URL, Path: we.Path, Referrer: we.Referrer, Title: we.Title,
			ElSelector: we.ElSelector, ElText: we.ElText, ElTag: we.ElTag, ElAttrs: attrs,
			ActiveMS:   we.ActiveMS,
			UserAgent:  env.Ctx.UserAgent, ScreenW: env.Ctx.ScreenW, ScreenH: env.Ctx.ScreenH,
			IPHash:  ipHash,
			TraceID: we.TraceID, Properties: props,
		}
		if err := ev.Validate(); err != nil {
			continue // skip the bad event, keep the rest
		}
		h.sink.Enqueue(ev)
	}

	w.WriteHeader(http.StatusNoContent)
}

func clientIP(r *http.Request) string {
	// The gateway sets X-Forwarded-For / X-Real-IP (chi middleware.RealIP
	// is applied upstream); fall back to RemoteAddr.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := strings.IndexByte(xff, ','); comma > 0 {
			return strings.TrimSpace(xff[:comma])
		}
		return strings.TrimSpace(xff)
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return rip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
