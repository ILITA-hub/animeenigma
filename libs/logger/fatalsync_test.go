package logger

import (
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// newObservedLogger builds a *Logger backed by an in-memory observer sink so
// tests can inspect emitted entries. It mirrors the construction in New(): the
// underlying *zap.Logger is built with AddCallerSkip(1) so the sugared logger's
// caller field points at the caller of the sugared method (not the wrapper).
func newObservedLogger(t *testing.T) (*Logger, *observer.ObservedLogs) {
	t.Helper()
	core, recorded := observer.New(zapcore.DebugLevel)
	base := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	return &Logger{
		SugaredLogger: base.Sugar(),
		base:          base,
		exiter:        defaultExiter,
	}, recorded
}

func TestFatalSync_CallsSyncBeforeExiter(t *testing.T) {
	l, recorded := newObservedLogger(t)

	var (
		syncCount    int32
		exitCount    int32
		exitCode     int32
		syncBeforeEx atomic.Bool
	)

	// Observe Sync() by wrapping the base logger's core indirectly — we cannot
	// intercept Sync() on the existing *zap.Logger directly, so use a sentinel:
	// the only entries reachable in this test go through Sync() before exit
	// because FatalSync calls l.Sync() first.
	//
	// To assert ordering deterministically we replace Sync via a sync observer
	// hook installed on the logger.
	setSyncObserver(l, func() error {
		atomic.AddInt32(&syncCount, 1)
		if atomic.LoadInt32(&exitCount) == 0 {
			syncBeforeEx.Store(true)
		}
		return nil
	})

	setExiter(l, func(code int) {
		atomic.AddInt32(&exitCount, 1)
		atomic.StoreInt32(&exitCode, int32(code))
	})

	l.FatalSync("boom", "key", "value")

	if got := atomic.LoadInt32(&syncCount); got != 1 {
		t.Fatalf("expected Sync to be called exactly once, got %d", got)
	}
	if got := atomic.LoadInt32(&exitCount); got != 1 {
		t.Fatalf("expected exiter to be called exactly once, got %d", got)
	}
	if got := atomic.LoadInt32(&exitCode); got != 1 {
		t.Fatalf("expected exit code 1, got %d", got)
	}
	if !syncBeforeEx.Load() {
		t.Fatal("expected Sync() to be called BEFORE exiter")
	}

	logs := recorded.All()
	if len(logs) != 1 {
		t.Fatalf("expected exactly 1 log entry, got %d", len(logs))
	}
	entry := logs[0]
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("expected ERROR level, got %s", entry.Level)
	}
	if entry.Message != "boom" {
		t.Fatalf("expected message 'boom', got %q", entry.Message)
	}

	// Verify structured field made it through
	foundField := false
	for _, f := range entry.Context {
		if f.Key == "key" && f.String == "value" {
			foundField = true
			break
		}
	}
	if !foundField {
		t.Fatalf("expected field key=value in entry context, got %+v", entry.Context)
	}

	// Verify caller skip: the caller field should point at THIS test file
	// (the line that calls FatalSync), not at fatalsync.go.
	caller := entry.Caller.File
	if !strings.HasSuffix(caller, "fatalsync_test.go") {
		t.Fatalf("expected caller to be fatalsync_test.go (caller-skip math), got %s:%d", caller, entry.Caller.Line)
	}
	t.Logf("caller correctly resolved to %s:%d", caller, entry.Caller.Line)
}

func TestFatalSync_DefaultExiterIsOSExit(t *testing.T) {
	// Sanity check: a freshly-built logger via New() has a non-nil exiter.
	l, err := New(Config{Level: "info", Development: false, Encoding: "json"})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if l.exiter == nil {
		t.Fatal("expected New() to install a default exiter, got nil")
	}
}

func TestFatalSync_ProductionStyleNew_ExiterReplaceable(t *testing.T) {
	// Build a logger the production way (via New()), then swap in a test
	// exiter to verify FatalSync uses the per-instance exiter rather than
	// the package-level defaultExiter directly.
	l, err := New(Config{Level: "info", Development: false, Encoding: "json"})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	var exitCode int32 = -1
	setExiter(l, func(code int) {
		atomic.StoreInt32(&exitCode, int32(code))
	})

	l.FatalSync("production-path", "context", "smoke")

	if got := atomic.LoadInt32(&exitCode); got != 1 {
		t.Fatalf("expected exiter to be called with code 1, got %d", got)
	}
}
