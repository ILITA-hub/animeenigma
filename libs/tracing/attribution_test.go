package tracing

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/baggage"
)

// captureFromHelper synthesizes a PC slice whose top user frame is THIS
// function — used by tests to assert normalizeServiceFrame against a known
// function name. It returns the captured Operation. Because attribution_test.go
// lives in package tracing, runtime sees the function path
// ".../libs/tracing.captureFromHelper", which does NOT contain
// "/internal/service/" — so these helpers exercise the fallback chain unless we
// feed a synthetic PC slice. For the service-frame test we synthesize PCs by
// pointing at a function whose recorded path we control via a fake; instead we
// resolve real frames and assert normalization shape on whatever real service
// frame we can produce. Since the test package itself is not a service frame,
// the service-frame normalization is asserted directly against
// normalizeServiceFrame (a pure string function).

func TestNormalizeServiceFrame(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain service func",
			in:   "github.com/ILITA-hub/animeenigma/services/catalog/internal/service.UpdateAnimeInfo",
			want: "catalog.UpdateAnimeInfo",
		},
		{
			name: "sub-package method with pointer receiver",
			in:   "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight.(*AnimeOfDayResolver).Resolve",
			want: "spotlight.AnimeOfDayResolver.Resolve",
		},
		{
			name: "sub-package value receiver",
			in:   "github.com/ILITA-hub/animeenigma/services/player/internal/service/watch.Tracker.Save",
			want: "watch.Tracker.Save",
		},
		{
			name: "bare service package func resolves to service name",
			in:   "github.com/ILITA-hub/animeenigma/services/player/internal/service.SaveProgress",
			want: "player.SaveProgress",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeServiceFrame(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeServiceFrame(%q) = %q, want %q", tc.in, tc.want, got)
			}
			if strings.HasPrefix(got, "/") {
				t.Fatalf("result has a leading slash: %q", got)
			}
			if strings.Contains(got, "github.com/") {
				t.Fatalf("module path prefix not stripped: %q", got)
			}
		})
	}
}

// fakeServiceFrameOp builds an Operation backed by a single synthetic PC whose
// resolved function path contains "/internal/service/". We can't easily forge a
// real runtime PC, so we drive Resolve through a frame slice the test owns. The
// resolver iterates runtime.CallersFrames over real PCs; to test the
// service-frame branch deterministically we capture PCs from a helper defined in
// a fake "/internal/service/" path is impossible in-package, so the
// service-frame end-to-end path is covered by TestNormalizeServiceFrame (pure
// string) plus the live walk in TestOperationResolve which exercises the
// fallback chain over the real (non-service) test frames.

func TestOperationResolve(t *testing.T) {
	t.Run("no service frame falls back to baggage operation", func(t *testing.T) {
		// The test-package frames never contain "/internal/service/", so the
		// walk finds no service frame and falls back to the baggage operation.
		ctx := SeedBaggage(context.Background(), "api", "catalog GET /api/anime/{id}")
		op := CaptureOperationPCs(ctx)
		got := op.Resolve()
		if got != "catalog GET /api/anime/{id}" {
			t.Fatalf("expected baggage operation fallback, got %q", got)
		}
	})

	t.Run("no service frame and no baggage op falls back to origin name", func(t *testing.T) {
		ctx := SeedBaggage(context.Background(), "scheduler.refresh", "")
		op := CaptureOperationPCs(ctx)
		got := op.Resolve()
		// origin "scheduler.refresh" with no service/job hint becomes a
		// goroutine(<origin>) shaped label.
		if got == "" {
			t.Fatalf("resolve returned empty string; must never be empty")
		}
		if !strings.Contains(got, "scheduler.refresh") {
			t.Fatalf("expected origin name to carry %q, got %q", "scheduler.refresh", got)
		}
		if !strings.HasPrefix(got, "goroutine(") && !strings.HasPrefix(got, "scheduled_job(") {
			t.Fatalf("expected goroutine(...)/scheduled_job(...) shape, got %q", got)
		}
	})

	t.Run("scheduled_job origin shape", func(t *testing.T) {
		ctx := SeedBaggage(context.Background(), "scheduled_job:nightly-p95", "")
		op := CaptureOperationPCs(ctx)
		got := op.Resolve()
		if !strings.HasPrefix(got, "scheduled_job(") {
			t.Fatalf("expected scheduled_job(...) shape for a job origin, got %q", got)
		}
		if !strings.Contains(got, "nightly-p95") {
			t.Fatalf("expected job name in origin label, got %q", got)
		}
	})

	t.Run("empty ctx never returns empty string", func(t *testing.T) {
		op := CaptureOperationPCs(context.Background())
		got := op.Resolve()
		if got == "" {
			t.Fatalf("resolve over an empty ctx returned empty string; must be non-empty")
		}
		if !strings.HasPrefix(got, "goroutine(") && !strings.HasPrefix(got, "scheduled_job(") {
			t.Fatalf("expected goroutine(unknown)-shaped default, got %q", got)
		}
	})

	t.Run("resolver never seeds user_id into baggage", func(t *testing.T) {
		// Mirror TestNoUserIDOnOutboundWire: user_id rides the private ctx
		// value only. After a full capture+resolve the baggage must carry no
		// user_id member.
		ctx := SeedBaggage(context.Background(), "api", "catalog GET /api/anime/{id}")
		ctx = WithUserID(ctx, "user-42")
		op := CaptureOperationPCs(ctx)
		_ = op.Resolve()

		bg := baggage.FromContext(op.ctx)
		if bg.Member("user_id").Key() != "" {
			t.Fatalf("user_id leaked into baggage: members=%q", bg.String())
		}
		// The private ctx value is still readable (not on baggage).
		if UserIDFromContext(op.ctx) != "user-42" {
			t.Fatalf("private user_id ctx value lost: %q", UserIDFromContext(op.ctx))
		}
	})
}

// sanity check: confirm the test package frames really do not look like service
// frames, so the fallback-chain tests above are meaningful.
func TestTestPackageIsNotAServiceFrame(t *testing.T) {
	var pcs [8]uintptr
	n := runtime.Callers(1, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if strings.Contains(f.Function, "/internal/service/") {
			t.Fatalf("unexpected: test frame %q looks like a service frame", f.Function)
		}
		if !more {
			break
		}
	}
}
