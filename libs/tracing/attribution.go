package tracing

import (
	"context"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel/baggage"
)

// serviceFrameAnchor is the uniform substring every project service frame
// carries (all 7 services follow services/{name}/internal/service/...). It is
// the robust anchor for picking the PRIMARY operation frame (D-08).
const serviceFrameAnchor = "/internal/service/"

// Operation carries the raw program counters captured SYNCHRONOUSLY at the
// effect record point, plus the originating ctx. Symbol resolution (the
// expensive part) is deferred to Resolve(), which runs on the async Producer
// goroutine — never on the request hot path (D-11). The zero value resolves to
// a non-empty origin fallback, so an Operation is always safe to Resolve.
type Operation struct {
	pcs []uintptr
	ctx context.Context
}

// CaptureOperationPCs records the calling stack cheaply: it copies up to 32
// program counters with a single runtime.Callers call and stores them on an
// Operation alongside ctx. It performs NO symbol resolution (that is Resolve's
// job, run async — D-11). The skip count (3) drops runtime.Callers itself, this
// function, and the immediate hook caller (the recordingTransport / GORM
// callback / cache hook) so the first resolved frame is the real call site.
func CaptureOperationPCs(ctx context.Context) Operation {
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:])
	return Operation{pcs: pcs[:n], ctx: ctx}
}

// Resolve picks the PRIMARY operation label for the captured stack, applying the
// never-empty fallback chain (D-08/D-09):
//
//  1. nearest */internal/service/* stack frame, normalized to "<pkg>.<Func>";
//  2. else the baggage operation (ReadBaggage), if non-empty;
//  3. else an origin name shaped goroutine/<name> / scheduled_job/<name>,
//     defaulting to goroutine/unknown.
//
// It runs on the async Producer side because runtime.CallersFrames symbol
// resolution is materially more expensive than the PC capture (Pitfall 2).
// Resolve only READS ctx — it NEVER seeds user_id (or anything) into baggage
// (T-03-01).
func (o Operation) Resolve() string {
	if len(o.pcs) > 0 {
		frames := runtime.CallersFrames(o.pcs)
		for {
			f, more := frames.Next()
			if strings.Contains(f.Function, serviceFrameAnchor) {
				return normalizeServiceFrame(f.Function)
			}
			if !more {
				break
			}
		}
	}
	if o.ctx != nil {
		if _, op := ReadBaggage(o.ctx); op != "" {
			return op
		}
	}
	return originName(o.ctx)
}

// normalizeServiceFrame strips a fully-qualified Go function path down to a
// compact "<pkg>.<Func>" / "<pkg>.<Type>.<Method>" label, dropping the module
// path prefix and pointer-receiver parens. Examples:
//
//	.../services/catalog/internal/service.UpdateAnimeInfo
//	    -> "catalog.UpdateAnimeInfo"        (last path segment before service is the service name)
//	.../internal/service/spotlight.(*AnimeOfDayResolver).Resolve
//	    -> "spotlight.AnimeOfDayResolver.Resolve"
//	.../internal/service.SaveProgress
//	    -> "service.SaveProgress"
func normalizeServiceFrame(funcPath string) string {
	// Split the path prefix from the package-qualified symbol. A Go function
	// path is "<import/path>.<symbol>", where <import/path> may itself contain
	// dots only within the leading host segment (e.g. "github.com"). The symbol
	// begins at the first '.' AFTER the final '/'.
	pkgPath := funcPath
	symbol := ""
	if slash := strings.LastIndex(funcPath, "/"); slash >= 0 {
		head := funcPath[:slash+1]
		tail := funcPath[slash+1:]
		if dot := strings.Index(tail, "."); dot >= 0 {
			pkgPath = head + tail[:dot]
			symbol = tail[dot+1:]
		} else {
			pkgPath = funcPath
			symbol = ""
		}
	} else if dot := strings.Index(funcPath, "."); dot >= 0 {
		pkgPath = funcPath[:dot]
		symbol = funcPath[dot+1:]
	}

	// Package name: prefer the path segment ONE level above a trailing
	// "internal/service" anchor (that segment is the service/sub-package name,
	// e.g. "catalog", "spotlight"). When the final segment is itself a named
	// sub-package (e.g. ".../internal/service/spotlight"), use that segment.
	pkg := serviceLabelFromPath(pkgPath)

	// Symbol: strip pointer-receiver parens "(*T)." -> "T.".
	symbol = strings.ReplaceAll(symbol, "(*", "")
	symbol = strings.ReplaceAll(symbol, ")", "")

	if pkg == "" {
		return symbol
	}
	if symbol == "" {
		return pkg
	}
	return pkg + "." + symbol
}

// serviceLabelFromPath derives the compact package label from an import path.
// For ".../services/<name>/internal/service" it returns "<name>"; for
// ".../internal/service/<sub>" it returns "<sub>"; otherwise the final path
// segment.
func serviceLabelFromPath(pkgPath string) string {
	segs := strings.Split(pkgPath, "/")
	if len(segs) == 0 {
		return pkgPath
	}
	last := segs[len(segs)-1]
	// ".../internal/service" -> the segment before "internal" is the service
	// name (e.g. "catalog").
	if last == "service" && len(segs) >= 3 && segs[len(segs)-2] == "internal" {
		return segs[len(segs)-3]
	}
	return last
}

// originName builds the final never-empty fallback label from the origin baggage
// member: scheduled_job/<name> when the origin denotes a scheduled job,
// otherwise goroutine/<name>. Defaults to goroutine/unknown when no origin is
// present. The "<channel>/<purpose>" slash shape (vs the older parenthesised
// form) makes a frame-less background effect read like a path — the purpose is
// the thing after the slash — so a seeded origin renders e.g.
// "scheduled_job/recs-precompute" or "goroutine/spotlight-snapshot".
func originName(ctx context.Context) string {
	origin := "unknown"
	if ctx != nil {
		if v := baggage.FromContext(ctx).Member(baggageKeyOrigin).Value(); v != "" {
			origin = v
		}
	}
	if isScheduledJobOrigin(origin) {
		return "scheduled_job/" + trimJobPrefix(origin)
	}
	return "goroutine/" + strings.TrimPrefix(origin, "goroutine:")
}

// FallbackOperationName returns the never-empty origin-shaped operation label
// for a ctx that carries no resolvable service-frame and no baggage operation —
// e.g. "goroutine/spotlight-snapshot", "scheduled_job/recs-precompute", or the
// "goroutine/unknown" default. Effect resolution (Operation.Resolve) uses it as
// its last fallback; the cache aggregator calls it directly so a frame-less
// cache effect carries the SAME shape as a frame-less egress/db effect (one
// consistent register dimension instead of a bare raw origin).
func FallbackOperationName(ctx context.Context) string {
	return originName(ctx)
}

// trimJobPrefix strips the scheduled-job origin prefix ("scheduled_job:" or
// "job:") so only the bare purpose remains for the slash label.
func trimJobPrefix(origin string) string {
	origin = strings.TrimPrefix(origin, "scheduled_job:")
	origin = strings.TrimPrefix(origin, "job:")
	return origin
}

// isScheduledJobOrigin reports whether an origin label denotes a background
// scheduled job (vs an ad-hoc goroutine / request).
func isScheduledJobOrigin(origin string) bool {
	return strings.HasPrefix(origin, "scheduled_job:") ||
		strings.HasPrefix(origin, "job:") ||
		strings.Contains(origin, "scheduled_job")
}
