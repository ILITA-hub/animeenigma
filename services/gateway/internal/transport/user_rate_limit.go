package transport

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/go-redis/redis_rate/v10"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
)

// userRateLimitBlockedTotal counts requests blocked by the per-user limiter.
// Deliberately label-less to bound cardinality under an abuse wave — forensic
// detail (user_id, path) lives in the structured log emitted alongside the
// 429 below. See WV3-T3 plan: "Even hashed user_id would create one
// label-value per unique 429'd user — open to cardinality blowup under an
// abuse wave."
var userRateLimitBlockedTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "gateway_rate_limit_user_blocked_total",
	Help: "Total authenticated requests blocked by the per-user rate limiter.",
})

// newUserRateLimitChainFn returns a middleware factory suitable for r.Use()
// inside every protected route group. When the Redis client is nil (caller
// declined to wire Redis, or a test passes nil), the factory yields a
// pass-through so existing routes keep working — the actual UserRateLimit
// gate is the responsibility of the caller plumbing in a live client.
//
// This indirection keeps router.go free of `if redisClient != nil { … }`
// branches at every Use-site and gives callers a single switch
// (cmd/gateway-api/main.go) for the whole feature.
func newUserRateLimitChainFn(rdb *redis.Client, perMinute, burst int, log *logger.Logger) func(http.Handler) http.Handler {
	if rdb == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return UserRateLimitMiddleware(rdb, perMinute, burst, log)
}

// UserRateLimitMiddleware returns a chi-compatible middleware that throttles
// authenticated requests on a per-user-id basis using a GCRA token bucket
// backed by Redis (redis_rate/v10).
//
// Layering (WV3-T3):
//   - The existing per-IP IPRateLimiter (RateLimitMiddleware) runs FIRST at
//     the top of NewRouter — it shields the auth service from anonymous
//     flooders before we ever hit JWT validation.
//   - JWTValidationMiddleware then attaches *authz.Claims to the request
//     context.
//   - This middleware runs AFTER auth on protected route groups. If no
//     claims are present (unexpected on a JWT-gated route — but possible on
//     optional-auth routes that share this middleware factory) we pass
//     through unchanged. Anonymous traffic stays per-IP-limited by the
//     stage above; this middleware never touches Redis for anonymous
//     callers.
//
// Failure mode: if Redis is unreachable or returns an error, we fail OPEN
// (log a WARN, let the request through). The alternative — 500ing every
// authenticated request because Redis blipped — is strictly worse for
// availability. The per-IP limiter still applies, so a true outage of
// both Redis AND the rate-limit stack is needed before all guards fall.
//
// 429 response shape matches existing errors.RateLimited():
//
//	HTTP/1.1 429 Too Many Requests
//	Content-Type: application/json
//	Retry-After: <integer seconds>
//	{"success":false,"error":{"code":"RATE_LIMITED","message":"rate limit exceeded"}}
func UserRateLimitMiddleware(rdb *redis.Client, perMinute, burst int, log *logger.Logger) func(http.Handler) http.Handler {
	limiter := redis_rate.NewLimiter(rdb)
	limit := redis_rate.Limit{
		Rate:   perMinute,
		Period: time.Minute,
		Burst:  burst,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authz.ClaimsFromContext(r.Context())
			if !ok || claims == nil || claims.UserID == "" {
				// Anonymous / no-claims traffic — skip entirely. The
				// per-IP limiter higher in the chain already covers
				// these requests; touching Redis here would just add
				// load with no upside.
				next.ServeHTTP(w, r)
				return
			}

			key := "ratelimit:user:" + claims.UserID
			res, err := limiter.Allow(r.Context(), key, limit)
			if err != nil {
				// Fail-open. Do NOT 500 the request — log a warning
				// and let it through so an unreachable Redis does not
				// take down every authenticated path. The per-IP
				// limiter is still in front of us as a backstop.
				log.Warnw("user rate limiter redis call failed; failing open",
					"user_id", claims.UserID,
					"path", r.URL.Path,
					"error", err,
				)
				next.ServeHTTP(w, r)
				return
			}

			if res.Allowed == 0 {
				userRateLimitBlockedTotal.Inc()

				retryAfter := int(res.RetryAfter.Seconds())
				if retryAfter < 1 {
					// RFC 7231: Retry-After must be a non-negative
					// integer. Round sub-second values up to 1 so the
					// client doesn't retry instantly and trip the
					// same bucket.
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))

				// Structured log carries the forensic detail kept off
				// the (deliberately label-less) Prometheus counter.
				log.Warnw("per-user rate limit exceeded",
					"user_id", claims.UserID,
					"path", r.URL.Path,
					"retry_after_seconds", retryAfter,
					"remaining", res.Remaining,
				)

				// httputil.TooManyRequests writes the existing
				// errors.RateLimited() JSON envelope — same shape the
				// per-IP limiter uses today, so clients don't need to
				// special-case a second 429 contract.
				httputil.TooManyRequests(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
