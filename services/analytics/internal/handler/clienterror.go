package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/observ"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/service"
)

// ClientErrorHandler ingests batched frontend error reports and emits them as
// structured log lines (which the OTel filelog receiver ships to ClickHouse
// otel_logs / Grafana). It deliberately writes NO database rows and enqueues
// nothing on the clickstream event store — this is a log-only sink for FE
// failure visibility (uncaught JS errors, unhandled rejections, Vue errors,
// failed HTTP responses, and explicit player-source failures).
//
// Public + anonymous, same trust model as /api/analytics/collect: every field
// is attacker-controlled, so sizes are bounded and the `kind` label is mapped
// through a fixed whitelist before it reaches a Prometheus label.
type ClientErrorHandler struct {
	sink ClientErrorSink
	salt string
}

// NewClientErrorHandler builds the handler with the given sink and IP salt.
func NewClientErrorHandler(sink ClientErrorSink, ipSalt string) *ClientErrorHandler {
	return &ClientErrorHandler{sink: sink, salt: ipSalt}
}

// ClientErrorSink receives each accepted FE error. The production impl logs +
// counts; tests inject a capturing fake (mirrors collect.go's Sink seam).
type ClientErrorSink interface {
	Record(e WireClientError, ua, ipHash string)
}

// WireClientError is one frontend error in the beacon batch. snake_case tags
// match the FE wire (utils/feErrorLog.ts).
type WireClientError struct {
	Kind      string    `json:"kind"`
	Message   string    `json:"message"`
	Stack     string    `json:"stack"`
	Source    string    `json:"source"`
	URL       string    `json:"url"`
	Path      string    `json:"path"`
	Method    string    `json:"method"`
	Status    int       `json:"status"`
	Provider  string    `json:"provider"`
	AnimeID   string    `json:"anime_id"`
	Count     int       `json:"count"` // >1 for a deduped/suppressed summary
	Timestamp time.Time `json:"ts"`
}

type wireClientErrorEnvelope struct {
	Errors []WireClientError `json:"errors"`
	Ctx    wireCtx           `json:"ctx"` // reused from collect.go (same package)
}

const (
	maxClientErrorsPerBatch = 50
	maxClientErrorBody      = 64 * 1024 // 64 KB — matches the FE sendBeacon cap
	maxMessageLen           = 500
	maxStackLen             = 2000
	maxURLLen               = 500
)

func (h *ClientErrorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxClientErrorBody))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var env wireClientErrorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	ipHash := service.HashIP(clientIP(r), h.salt, now)
	ua := capString(env.Ctx.UserAgent)

	for i, e := range env.Errors {
		if i >= maxClientErrorsPerBatch {
			break // clamp — never let a single beacon flood the log stream
		}
		e.Message = capLen(e.Message, maxMessageLen)
		e.Stack = capLen(e.Stack, maxStackLen)
		e.URL = capLen(e.URL, maxURLLen)
		e.Path = capLen(e.Path, maxURLLen)
		e.Source = capString(e.Source)
		e.Method = capString(e.Method)
		e.Provider = capString(e.Provider)
		e.AnimeID = capString(e.AnimeID)
		// Drop empty noise: an error with neither a message nor a URL carries
		// nothing actionable.
		if e.Message == "" && e.URL == "" {
			continue
		}
		h.sink.Record(e, ua, ipHash)
	}

	// 204 keeps the response body empty — beacon-friendly, like /collect.
	w.WriteHeader(http.StatusNoContent)
}

// loggingClientErrorSink is the production sink: one structured Warnw line per
// FE error plus a kind-labelled Prometheus counter.
type loggingClientErrorSink struct {
	log *logger.Logger
}

// NewLoggingClientErrorSink builds the production log-only sink.
func NewLoggingClientErrorSink(log *logger.Logger) ClientErrorSink {
	return loggingClientErrorSink{log: log}
}

func (s loggingClientErrorSink) Record(e WireClientError, ua, ipHash string) {
	observ.FEErrorsReceived.WithLabelValues(kindLabel(e.Kind)).Inc()
	s.log.Warnw("fe_error",
		"kind", e.Kind,
		"message", e.Message,
		"source", e.Source,
		"url", e.URL,
		"path", e.Path,
		"method", e.Method,
		"status", e.Status,
		"provider", e.Provider,
		"anime_id", e.AnimeID,
		"count", e.Count,
		"ua", ua,
		"remote_ip", ipHash,
		"stack", e.Stack,
	)
}

// kindLabel maps the attacker-controlled `kind` onto a fixed whitelist so a
// forged value can never explode Prometheus label cardinality.
func kindLabel(kind string) string {
	switch kind {
	case "js", "unhandledrejection", "vue", "http", "player", "suppressed", "cap":
		return kind
	default:
		return "other"
	}
}

// capLen truncates a public-beacon string to n runes (capString in collect.go
// is fixed at maxFieldLen=256; FE error message/stack/url need wider bounds).
func capLen(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
