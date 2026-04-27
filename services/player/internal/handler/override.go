package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
)

// OverrideRequest is the payload posted by the frontend useOverrideTracker composable
// when a user changes language, player, team, or episode within 30s of player load.
// See .planning/phases/01-instrumentation-baseline/01-CONTEXT.md D-04 / D-07.
type OverrideRequest struct {
	AnimeID       string                 `json:"anime_id"`
	LoadSessionID string                 `json:"load_session_id"`
	Dimension     string                 `json:"dimension"` // language|player|team|episode
	OriginalCombo map[string]interface{} `json:"original_combo"`
	NewCombo      map[string]interface{} `json:"new_combo"`
	MsSinceLoad   int64                  `json:"ms_since_load"`
	Tier          string                 `json:"tier"`
	TierNumber    int                    `json:"tier_number"`
	Player        string                 `json:"player"`
}

// validDimensions enforces the strict whitelist for the `dimension` Prometheus label.
// T-01-02 (cardinality protection): user-controlled label values MUST come from a closed set.
var validDimensions = map[string]bool{
	"language": true,
	"player":   true,
	"team":     true,
	"episode":  true,
}

// OverrideHandler emits a Prometheus counter increment + structured Loki log line per
// accepted request. No DB writes, no caching, no business logic — pure instrumentation.
type OverrideHandler struct {
	log *logger.Logger
}

func NewOverrideHandler(log *logger.Logger) *OverrideHandler {
	return &OverrideHandler{log: log}
}

// RecordOverride is intentionally permissive: best-effort instrumentation, fast path,
// no DB writes. The handler emits one Prometheus increment and one structured log line,
// then returns 204.
//
// Auth: OptionalAuth — accepts JWT claims (sets anon=false) OR X-Anon-ID header
// (sets anon=true). Rejects requests with neither (T-01-01: prevents headerless flood
// and ensures the `anon` label segmentation stays well-defined).
func (h *OverrideHandler) RecordOverride(w http.ResponseWriter, r *http.Request) {
	var req OverrideRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, errors.InvalidInput("invalid override payload"))
		return
	}
	if req.AnimeID == "" || req.LoadSessionID == "" {
		httputil.Error(w, errors.InvalidInput("anime_id and load_session_id required"))
		return
	}
	if !validDimensions[req.Dimension] {
		httputil.Error(w, errors.InvalidInput("dimension must be language|player|team|episode"))
		return
	}

	// Identify the user (optional auth).
	var userID, anonID string
	if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil {
		userID = claims.UserID
	} else {
		anonID = r.Header.Get("X-Anon-ID")
	}
	if userID == "" && anonID == "" {
		httputil.Error(w, errors.InvalidInput("X-Anon-ID required for unauthenticated requests"))
		return
	}

	anonLabel := "true"
	if userID != "" {
		anonLabel = "false"
	}

	language := stringFromMap(req.NewCombo, "language")

	metrics.ComboOverrideTotal.WithLabelValues(
		labelOrUnknown(req.Tier),
		req.Dimension, // already whitelisted above
		labelOrUnknown(language),
		anonLabel,
		labelOrUnknown(req.Player),
	).Inc()

	// Structured log line — fields, not labels (Loki indexes labels only). Searchable
	// via `{service="player"} |= "combo_override" | json` in Grafana Explore.
	// T-01-03: NO username, NO Authorization header, NO raw token. Only user_id (numeric
	// string) or anon_id (UUIDv4) — exactly one is set.
	h.log.Infow("combo_override",
		"anime_id", req.AnimeID,
		"load_session_id", req.LoadSessionID,
		"dimension", req.Dimension,
		"user_id", userID,
		"anon_id", anonID,
		"original_combo", req.OriginalCombo,
		"new_combo", req.NewCombo,
		"ms_since_load", req.MsSinceLoad,
		"tier", req.Tier,
		"tier_number", req.TierNumber,
		"player", req.Player,
	)

	w.WriteHeader(http.StatusNoContent)
}

// labelOrUnknown coerces empty strings to "unknown" so Prometheus labels never have
// an empty-string value (which renders awkwardly in Grafana legends and breaks some
// PromQL operators). T-01-02 cardinality protection: combined with whitelisting at
// the `dimension` boundary, this keeps the label cardinality bounded for free-form
// fields like `tier` and `player`.
func labelOrUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// stringFromMap extracts a string value from a map[string]interface{} or returns "" if
// the key is missing or the value is not a string.
func stringFromMap(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
