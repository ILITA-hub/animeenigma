package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/groq"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/service"
)

// dailyProvider is the subset of *service.DailyService this handler depends
// on — a local interface (project convention, see `generator`/`libraryStore`
// above) so tests can drive a fake instead of the real service.
type dailyProvider interface {
	DailyPick(ctx context.Context) (*domain.Fanfic, error)
	EnsureDaily(ctx context.Context) (service.EnsureResult, error)
}

// DailyHandler serves the "Фанфик дня" (fanfic of the day) reader + the
// scheduler's ensure-generated cron hook.
type DailyHandler struct {
	daily dailyProvider
}

func NewDailyHandler(daily dailyProvider) *DailyHandler {
	return &DailyHandler{daily: daily}
}

// Internal serves GET /internal/fanfic/daily — the compact spotlight DTO
// consumed by catalog's HeroSpotlightBlock resolver. Docker-network only, no
// JWT (see router.go). 404 when nothing is eligible yet.
func (h *DailyHandler) Internal(w http.ResponseWriter, r *http.Request) {
	pick, err := h.daily.DailyPick(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if pick == nil {
		httputil.NotFound(w, "no daily fanfic")
		return
	}
	httputil.OK(w, service.ToDTO(pick))
}

// publicDaily is the public-reader wire shape: the compact DTO fields plus
// full content and explicit-gating metadata. Embedding DailyDTO keeps the
// fields shared with the internal/spotlight DTO defined exactly once.
type publicDaily struct {
	service.DailyDTO
	Content    string `json:"content"`
	Gated      bool   `json:"gated"`
	GateReason string `json:"gate_reason,omitempty"`
}

// Public serves GET /api/fanfic/daily — the public reader. The route carries
// no mandatory auth (anon must get a 200, not a 401); router.go's
// optional-auth wrapper attaches claims to the context when a bearer token IS
// present. Non-explicit picks always return full content. Explicit picks
// NEVER leak content: content stays empty and gated=true, with gate_reason
// distinguishing an anonymous reader (must log in) from a logged-in one
// (must opt into adult content) — see authz.UserIDFromContext.
func (h *DailyHandler) Public(w http.ResponseWriter, r *http.Request) {
	pick, err := h.daily.DailyPick(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if pick == nil {
		httputil.NotFound(w, "no daily fanfic")
		return
	}

	resp := publicDaily{DailyDTO: service.ToDTO(pick)}
	if resp.Explicit {
		resp.Gated = true
		if authz.UserIDFromContext(r.Context()) == "" {
			resp.GateReason = "login"
		} else {
			resp.GateReason = "adult_setting"
		}
	} else {
		resp.Content = pick.Content
	}
	httputil.OK(w, resp)
}

// ensureResponse is the JSON shape for POST /internal/fanfic/ensure-daily.
type ensureResponse struct {
	Generated bool   `json:"generated"`
	Reason    string `json:"reason,omitempty"`
	FanficID  string `json:"fanfic_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Ensure serves POST /internal/fanfic/ensure-daily — the scheduler's cron
// hitting the idempotent generate-if-missing flow. A Groq auth failure
// (401/403, surfaced as a *groq.StatusError) has ALREADY fired a Telegram
// alert inside EnsureDaily itself — the alert IS the operator-facing signal
// — so this responds 200 with error:"groq_auth" instead of 500, letting the
// scheduler record a normal "ran successfully" tick rather than flag a
// retry-worthy scheduler fault. Any other error (DB write failure, etc.) is a
// genuine handler-level fault and gets a real 500.
func (h *DailyHandler) Ensure(w http.ResponseWriter, r *http.Request) {
	res, err := h.daily.EnsureDaily(r.Context())
	if err != nil {
		var se *groq.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
			httputil.OK(w, ensureResponse{Generated: false, Error: "groq_auth"})
			return
		}
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, ensureResponse{Generated: res.Generated, Reason: res.Reason, FanficID: res.FanficID})
}
