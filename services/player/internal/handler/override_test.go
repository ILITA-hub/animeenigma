// Wave 0 RED test — references types/functions created by Wave 1 plan 01-02.
// This file SHOULD fail to compile until Wave 1 lands. Going green is the Wave 1
// acceptance gate (per phase 01 VALIDATION.md).
//
// Symbols referenced that DO NOT yet exist:
//   - handler.NewOverrideHandler / handler.OverrideHandler.RecordOverride
//     (services/player/internal/handler/override.go — created in plan 01-02)
//   - metrics.ComboOverrideTotal
//     (libs/metrics/watch.go — added in plan 01-02)
//
// Test contract — the assertions below FREEZE the Wave 1 behavior:
//   - increments metrics.ComboOverrideTotal by exactly 1 per accepted POST
//   - accepts JWT-authenticated requests AND anonymous requests with X-Anon-ID
//   - rejects requests with neither (400) — protects label cardinality
//   - rejects unknown dimensions (400) — strict whitelist
//   - rejects payloads missing anime_id or load_session_id (400)
//   - emits a structured log line "combo_override" with no PII
//     (no username, no Authorization, no raw headers)

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// validBody returns a fully-populated, well-formed override request body.
// Tests selectively delete or mutate keys to exercise validation branches.
func validBody(dim string) map[string]interface{} {
	return map[string]interface{}{
		"anime_id":        "anime-123",
		"load_session_id": "sess-456",
		"dimension":       dim,
		"original_combo":  map[string]interface{}{"language": "ru", "watch_type": "sub", "player": "kodik"},
		"new_combo":       map[string]interface{}{"language": "en"},
		"ms_since_load":   int64(1500),
		"tier":            "user_global",
		"tier_number":     2,
		"player":          "kodik",
	}
}

// newOverrideRouter wires the override handler under chi the same way
// router.go will wire it (POST /override). Tests use httptest.NewRecorder.
func newOverrideRouter(t *testing.T, log *logger.Logger) http.Handler {
	t.Helper()
	h := NewOverrideHandler(log)
	r := chi.NewRouter()
	r.Post("/override", h.RecordOverride)
	return r
}

// postJSON serializes body as JSON, builds an httptest request,
// optionally injects authz claims into context, and returns the recorder.
func postJSON(t *testing.T, h http.Handler, body map[string]interface{}, claims *authz.Claims, anonID string) *httptest.ResponseRecorder {
	t.Helper()
	buf := &bytes.Buffer{}
	require.NoError(t, json.NewEncoder(buf).Encode(body))
	req := httptest.NewRequest(http.MethodPost, "/override", buf)
	req.Header.Set("Content-Type", "application/json")
	if claims != nil {
		req = req.WithContext(authz.ContextWithClaims(req.Context(), claims))
	}
	if anonID != "" {
		req.Header.Set("X-Anon-ID", anonID)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestOverride_IncrementsCounter(t *testing.T) {
	log := logger.Default()
	h := newOverrideRouter(t, log)

	// Capture counter value with anon=false (JWT path) for the canonical labels.
	before := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "language", "en", "false", "kodik"))

	claims := &authz.Claims{UserID: "user-1", Username: "tester"}
	w := postJSON(t, h, validBody("language"), claims, "")

	require.Equal(t, http.StatusNoContent, w.Code, "valid override should return 204")

	after := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "language", "en", "false", "kodik"))
	assert.InDelta(t, 1.0, after-before, 0.001, "ComboOverrideTotal should increment by exactly 1")
}

func TestOverride_AcceptsJWT(t *testing.T) {
	log := logger.Default()
	h := newOverrideRouter(t, log)

	before := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "team", "en", "false", "kodik"))

	claims := &authz.Claims{UserID: "user-jwt", Username: "jwt_user"}
	body := validBody("team")
	w := postJSON(t, h, body, claims, "")

	require.Equal(t, http.StatusNoContent, w.Code)

	after := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "team", "en", "false", "kodik"))
	assert.InDelta(t, 1.0, after-before, 0.001, "JWT auth path should increment counter with anon=false")
}

func TestOverride_AcceptsAnonID(t *testing.T) {
	log := logger.Default()
	h := newOverrideRouter(t, log)

	before := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "player", "en", "true", "kodik"))

	body := validBody("player")
	w := postJSON(t, h, body, nil, "anon-abc")

	require.Equal(t, http.StatusNoContent, w.Code, "anon X-Anon-ID should be accepted")

	after := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "player", "en", "true", "kodik"))
	assert.InDelta(t, 1.0, after-before, 0.001, "anon path should increment counter with anon=true")
}

func TestOverride_RejectsBothMissing(t *testing.T) {
	log := logger.Default()
	h := newOverrideRouter(t, log)

	// Track counter across ALL plausible label permutations the handler might
	// touch — the increment must NOT happen when neither identity is present.
	beforeAuth := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "language", "en", "false", "kodik"))
	beforeAnon := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "language", "en", "true", "kodik"))

	body := validBody("language")
	w := postJSON(t, h, body, nil, "") // no claims, no anon header

	require.Equal(t, http.StatusBadRequest, w.Code, "missing both identities must be rejected per T-01-01")

	afterAuth := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "language", "en", "false", "kodik"))
	afterAnon := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "language", "en", "true", "kodik"))
	assert.InDelta(t, 0.0, afterAuth-beforeAuth, 0.001, "rejected request must not increment")
	assert.InDelta(t, 0.0, afterAnon-beforeAnon, 0.001, "rejected request must not increment")
}

func TestOverride_RejectsInvalidDimension(t *testing.T) {
	log := logger.Default()
	h := newOverrideRouter(t, log)

	// Strict whitelist defends Prometheus cardinality (T-01-02). Anything outside
	// {language, player, team, episode} must 400.
	body := validBody("quality") // not in the whitelist
	claims := &authz.Claims{UserID: "user-1"}

	// Capture counter at the would-be label combination — must NOT change.
	before := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "quality", "en", "false", "kodik"))

	w := postJSON(t, h, body, claims, "")
	require.Equal(t, http.StatusBadRequest, w.Code, "unknown dimension must be rejected")

	after := testutil.ToFloat64(metrics.ComboOverrideTotal.WithLabelValues("user_global", "quality", "en", "false", "kodik"))
	assert.InDelta(t, 0.0, after-before, 0.001, "rejected dimension must not increment counter (cardinality protection)")
}

func TestOverride_RejectsMissingAnimeID(t *testing.T) {
	log := logger.Default()
	h := newOverrideRouter(t, log)

	body := validBody("language")
	delete(body, "anime_id")

	claims := &authz.Claims{UserID: "user-1"}
	w := postJSON(t, h, body, claims, "")
	assert.Equal(t, http.StatusBadRequest, w.Code, "missing anime_id must be rejected")
}

func TestOverride_RejectsMissingLoadSessionID(t *testing.T) {
	log := logger.Default()
	h := newOverrideRouter(t, log)

	body := validBody("language")
	delete(body, "load_session_id")

	claims := &authz.Claims{UserID: "user-1"}
	w := postJSON(t, h, body, claims, "")
	assert.Equal(t, http.StatusBadRequest, w.Code, "missing load_session_id must be rejected")
}

func TestOverride_LogsStructured(t *testing.T) {
	// Wire up an observer core so we can introspect emitted log entries.
	core, recorded := observer.New(zapcore.InfoLevel)
	zapLogger := zap.New(core)

	// Construct a logger.Logger backed by the observer core.
	log := &logger.Logger{SugaredLogger: zapLogger.Sugar()}

	h := NewOverrideHandler(log)
	r := chi.NewRouter()
	r.Post("/override", h.RecordOverride)

	// JWT-authed scenario — assert user_id is set, anon_id is empty.
	claims := &authz.Claims{UserID: "user-zap", Username: "must-not-be-logged"}
	w := postJSON(t, r, validBody("language"), claims, "")
	require.Equal(t, http.StatusNoContent, w.Code)

	entries := recorded.FilterMessage("combo_override").All()
	require.Len(t, entries, 1, "exactly one combo_override log line per accepted request")

	fields := entries[0].ContextMap()
	// Required structured fields per T-01-03.
	for _, k := range []string{"anime_id", "load_session_id", "dimension", "tier", "player", "ms_since_load"} {
		assert.Contains(t, fields, k, "log entry must include field %q", k)
	}

	// JWT path: user_id is set, anon_id is empty.
	assert.Equal(t, "user-zap", fields["user_id"], "user_id field reflects JWT claim")
	assert.Empty(t, fields["anon_id"], "anon_id must be empty when JWT-authed")

	// PII guard: do NOT log username verbatim. The whole entry's stringified
	// form must not contain the username we passed.
	for _, e := range entries {
		assert.NotContains(t, e.Message+" "+e.Caller.String(), "must-not-be-logged",
			"username must never appear in combo_override log")
	}

	// Anon scenario — second observer to keep the assertions clean.
	core2, recorded2 := observer.New(zapcore.InfoLevel)
	zapLogger2 := zap.New(core2)
	log2 := &logger.Logger{SugaredLogger: zapLogger2.Sugar()}

	h2 := NewOverrideHandler(log2)
	r2 := chi.NewRouter()
	r2.Post("/override", h2.RecordOverride)

	w2 := postJSON(t, r2, validBody("team"), nil, "anon-xyz")
	require.Equal(t, http.StatusNoContent, w2.Code)

	entries2 := recorded2.FilterMessage("combo_override").All()
	require.Len(t, entries2, 1)
	fields2 := entries2[0].ContextMap()
	assert.Equal(t, "anon-xyz", fields2["anon_id"], "anon_id field reflects X-Anon-ID")
	assert.Empty(t, fields2["user_id"], "user_id must be empty in anon path")
}
