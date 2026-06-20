// Package streamprobe — playability gate for the scraper service and
// scheduler canary. Walks master m3u8 → first variant → first segment
// HEAD, classifying the outcome into a typed Reason enum.
//
// SCRAPER-HEAL-01 / SCRAPER-HEAL-02. Consumed by:
//   - services/scraper/internal/providers/gogoanime (Plan 21-03)
//   - services/scheduler/internal/jobs/scraper_playability_canary (Phase 23)
package streamprobe

// Reason classifies the outcome of Probe. The string values are stable
// Prometheus label tokens — DO NOT rename without coordinating with
// .claude/maintenance-prompt.md Pattern 6/7 reason-enum dispatch table.
type Reason string

const (
	ReasonPlayable         Reason = "playable"
	ReasonAdDecoy          Reason = "ad_decoy"
	ReasonZeroMatch        Reason = "zero_match"
	ReasonStatus403        Reason = "status_403"
	ReasonSignedURLExpired Reason = "signed_url_expired"
	ReasonCDNUnreachable   Reason = "cdn_unreachable"
	ReasonEmptyResponse    Reason = "empty_response"
	ReasonDecodeFailed     Reason = "decode_failed"
	ReasonInvalidVideo     Reason = "invalid_video"
)

// AllReasons returns every defined Reason value in declaration order.
// Used by tests to verify exhaustive maintenance-prompt coverage.
func AllReasons() []Reason {
	return []Reason{
		ReasonPlayable,
		ReasonAdDecoy,
		ReasonZeroMatch,
		ReasonStatus403,
		ReasonSignedURLExpired,
		ReasonCDNUnreachable,
		ReasonEmptyResponse,
		ReasonDecodeFailed,
		ReasonInvalidVideo,
	}
}
