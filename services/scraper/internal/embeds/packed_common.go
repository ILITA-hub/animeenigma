// packed_common.go — shared Dean-Edwards-packer extractor base used by
// StreamHGExtractor (otakuhg.site) and EarnvidsExtractor (otakuvid.online).
//
// Both providers' wrapper HTML matches the Kwik unpacker shape (an
// `eval(function(p,a,c,k,e,d){...}(args))` IIFE) but emit the playable HLS URL
// under the field name `"hls2"` instead of Kwik's `const source=`. This file
// extracts the common pipeline:
//
//   1. HTTP GET the wrapper with caller-supplied headers + forced Referer
//   2. Cap the body at 2 MiB (io.LimitReader, T-18-16)
//   3. Locate the packer IIFE via balanced-paren extraction (reuses kwik.go's
//      extractPacker / balanceUntil helpers — same package)
//   4. Run the IIFE through goja with a watchdog goroutine (T-18-17, T-18-20)
//   5. Regex the unpacked output for `"hls2":"...m3u8..."`
//   6. Return *domain.Stream with one HLS Source + Referer for segment fetches
//
// `runGoja` is a package-level helper lifted from `(*KwikExtractor).runGoja`
// (Phase 16). The KwikExtractor method is now a thin one-line wrapper that
// dispatches to this shared helper — both consumers route through a single
// goja-runtime path. See kwik.go for the wrapper.
//
// Pitfalls (carried from Phase 16 RESEARCH):
//
//   - Pitfall 2: fresh goja.Runtime() per call, never pooled (T-18-20)
//   - Pitfall 3: vm.Interrupt() MUST come from a goroutine separate from the
//     one running RunString — see runGoja's watchdog block (T-18-17)
//   - SSRF: Matches() uses host-equality + strict-subdomain (HasSuffix on
//     "."+known); substring matches are forbidden by contract (T-18-14, T-18-15)
package embeds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// defaultPackedHTTPTimeout bounds the HTTP fetch of the wrapper page. Real
// wrapper pages return in <2s; the 15s budget exists to tolerate slow networks
// without breaking the orchestrator's per-stage SLO.
const defaultPackedHTTPTimeout = 15 * time.Second

// defaultPackedGojaTimeout bounds the goja-runtime JS execution budget per
// Extract call. Mirrors KwikExtractor's defaultKwikTimeout (5s) so hostile
// packed-JS payloads cannot pin a goroutine for the full 15s HTTP budget.
// WR-01 in the Phase 18 review — pre-fix, both StreamHG and Earnvids reused
// defaultPackedHTTPTimeout (15s) as the goja budget despite the comment
// claiming parity with Kwik.
const defaultPackedGojaTimeout = 5 * time.Second

// maxPackedBody caps the response body at 2 MiB. Real packed-JS wrapper pages
// are <200 KiB; this cap is purely a DoS guard against a hostile upstream
// streaming gigabytes (T-18-16).
const maxPackedBody = 2 << 20

// packedExtractor is the shared base type for Dean-Edwards-packer-style embed
// extractors (StreamHG, Earnvids). Composition over inheritance: concrete
// extractors embed *packedExtractor and override only Name() if they need a
// stable identifier different from `name`.
//
// Fields are unexported because this type is consumed only inside the embeds
// package; outside callers construct concrete extractors via NewStreamHGExtractor
// / NewEarnvidsExtractor.
type packedExtractor struct {
	// name is the stable observability identifier ("streamhg", "earnvids").
	name string
	// hosts is the case-insensitive allowlist. Match policy: host equality OR
	// strict subdomain (HasSuffix on "."+known).
	hosts []string
	// referer is forced as the upstream Referer header when the caller does not
	// supply one. Required by both upstreams (they 403 otherwise).
	referer string
	// selectorPackerFail / selectorGojaFail / selectorRegexFail /
	// selectorBodyFail are stable short identifiers emitted via
	// parser_zero_match_total on each failure path. They MUST be
	// low-cardinality (no raw HTML/regex strings).
	//
	// WR-02 — selectorGojaFail was added so a runtime trip inside goja
	// (upstream JS shape change / infinite loop / timeout) is distinct
	// from extractPacker's balance-paren miss (upstream HTML shape change).
	// Conflating them masked one inside the other on the Phase 17 dashboard.
	selectorPackerFail string
	selectorGojaFail   string
	selectorRegexFail  string
	selectorBodyFail   string
	// http is the HTTP client used for the wrapper fetch. Tests inject a custom
	// Transport (rewriteToSrv) to route the socket to a local httptest server
	// while preserving the request URL host (so Matches() still validates).
	http *http.Client
	// timeout bounds the goja-runtime execution per Extract call.
	timeout time.Duration
}

// hls2Regex matches `"hls2":"<m3u8 url>"` inside the unpacked packer body.
// Distinct from kwik.go's source regex (Kwik emits `const source=` directly).
// The leading double-quote anchor enforces JSON-property shape — a stray match
// inside a comment would be rare and would still fail downstream validation.
var hls2Regex = regexp.MustCompile(`"hls2"\s*:\s*"(https?://[^"]+\.m3u8[^"]*)"`)

// Name implements domain.EmbedExtractor.
func (e *packedExtractor) Name() string { return e.name }

// Matches reports whether embedURL is in this extractor's host allowlist.
// Match policy: host equality OR strict subdomain. Substring matches in path
// or query are NOT matched. T-18-14 / T-18-15.
func (e *packedExtractor) Matches(embedURL string) bool {
	u, err := url.Parse(embedURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	for _, known := range e.hosts {
		if host == known || strings.HasSuffix(host, "."+known) {
			return true
		}
	}
	return false
}

// Extract fetches the wrapper page, locates the Dean-Edwards-packed IIFE,
// evaluates it inside a fresh goja runtime, and parses the resulting unpacked
// string for the `"hls2"` URL. Returns *domain.Stream on success or a wrapped
// ErrExtractFailed / ErrProviderDown on failure.
//
// Error mapping:
//
//   - Matches() returns false        → WrapExtractFailed ("...: Matches gate")
//   - http.NewRequest failure        → WrapProviderDown ("...: build request")
//   - http.Do failure                → WrapProviderDown ("...: fetch")
//   - non-2xx status                 → WrapProviderDown ("...: HTTP status")
//   - body read failure              → WrapProviderDown ("...: read")
//   - no packer IIFE                 → WrapExtractFailed ("...: packer locate")
//   - goja runtime error / interrupt → WrapExtractFailed ("...: goja unpack")
//   - unpacked has no `"hls2"` URL   → WrapExtractFailed ("...: hls2 regex")
func (e *packedExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	if !e.Matches(embedURL) {
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("host not in allowlist"),
			e.name+": Matches gate",
		)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, domain.WrapProviderDown(err, e.name+": build request")
	}
	// Caller-supplied headers first so the caller can override UA / Accept etc.
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	// Force Referer to the wrapper origin if the caller didn't set one.
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", e.referer)
	}

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, domain.WrapProviderDown(err, e.name+": fetch")
	}
	defer func() {
		// Drain unread bytes so keep-alive can reuse the connection.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, domain.WrapProviderDown(
			fmt.Errorf("upstream %d", resp.StatusCode),
			e.name+": HTTP status",
		)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPackedBody))
	if err != nil {
		metrics.ParserZeroMatchTotal.WithLabelValues(e.name, e.selectorBodyFail).Inc()
		return nil, domain.WrapProviderDown(err, e.name+": read")
	}

	// Strip HTML comments first so a documented example packer block cannot
	// shadow the real `<script>eval(...)` block. Mirrors kwik.go's pattern.
	stripped := htmlCommentRegex.ReplaceAll(body, []byte(""))

	// Reuse kwik.go's extractPacker (same package, package-private helper).
	iife, ok := extractPacker(string(stripped))
	if !ok {
		metrics.ParserZeroMatchTotal.WithLabelValues(e.name, e.selectorPackerFail).Inc()
		return nil, domain.WrapExtractFailed(
			errors.New("no packer IIFE found in body"),
			e.name+": packer locate",
		)
	}

	// Wrap with leading + trailing parens so RunString returns the IIFE's
	// return value instead of executing it as a statement and discarding it.
	wrapper := "(" + iife + ")"
	unpacked, err := runGoja(ctx, wrapper, e.timeout)
	if err != nil {
		// WR-02 — emit the goja-specific selector so the Phase 17 dashboard
		// can split runtime trips from extractPacker balance-paren misses.
		metrics.ParserZeroMatchTotal.WithLabelValues(e.name, e.selectorGojaFail).Inc()
		return nil, domain.WrapExtractFailed(err, e.name+": goja unpack")
	}

	m := hls2Regex.FindStringSubmatch(unpacked)
	if m == nil {
		metrics.ParserZeroMatchTotal.WithLabelValues(e.name, e.selectorRegexFail).Inc()
		return nil, domain.WrapExtractFailed(
			errors.New("no hls2 URL in unpacked body"),
			e.name+": hls2 regex",
		)
	}

	return &domain.Stream{
		Sources: []domain.Source{{URL: m[1], Type: "hls"}},
		Headers: map[string]string{"Referer": e.referer},
	}, nil
}

// runGoja evaluates `expr` (a parenthesized IIFE) inside a fresh goja runtime
// with a watchdog goroutine that fires vm.Interrupt() on either ctx
// cancellation or the per-call timeout, whichever comes first.
//
// LIFTED from `(*KwikExtractor).runGoja` (Phase 16). The KwikExtractor method
// is now a thin one-line wrapper that calls this helper with k.timeout. Both
// consumers (kwik.go + packed_common.go) route through this single function so
// the goja-runtime + watchdog pattern is defined exactly once in the package.
//
// Pitfalls preserved verbatim from the Phase 16 reference impl:
//
//   - Pitfall 2: every call constructs a fresh goja.Runtime — never pooled,
//     never cached across goroutines (Runtime is NOT thread-safe).
//   - Pitfall 3: vm.Interrupt() comes from a goroutine OTHER than the one
//     running RunString. The `done` channel guarantees the watchdog exits
//     promptly after a normal completion so we don't leak goroutines.
func runGoja(ctx context.Context, expr string, timeout time.Duration) (string, error) {
	vm := goja.New()

	done := make(chan struct{})
	defer close(done)

	go func() {
		// Pitfall 3: Interrupt MUST come from a goroutine separate from the
		// RunString goroutine — otherwise an infinite loop in the script
		// blocks the only goroutine that could cancel it.
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-timer.C:
			vm.Interrupt("packed: unpack timeout")
		case <-ctx.Done():
			vm.Interrupt("packed: ctx cancel")
		case <-done:
			// Normal completion — exit cleanly.
		}
	}()

	val, err := vm.RunString(expr)
	if err != nil {
		return "", err
	}
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return "", errors.New("packed: goja returned nil/undefined")
	}
	return val.String(), nil
}
