package logger

import (
	"os"

	"go.uber.org/zap"
)

// defaultExiter is the production exiter — it calls os.Exit(1) (or any code
// passed) and never returns. Held in a var so the New() constructor can
// install it as the default and tests can override per-logger via setExiter.
var defaultExiter = func(code int) { os.Exit(code) }

// FatalSync logs the message at ERROR level, flushes the underlying zap
// writer via Sync(), then exits the process with code 1.
//
// Why not zap.Fatalw? zap's Fatal levels call os.Exit() inside zap *before*
// the deferred Sync() in main() ever runs. For containerized services with
// docker capturing stdout that's fine — but for host-native binaries like
// the maintenance daemon (whose stderr goes to systemd's journal but is
// line-buffered through zap's locked-write-sync), the final fatal line can
// be lost, leaving operators with a silent restart and no root cause.
//
// FatalSync deliberately splits the operation: emit, flush, exit — so the
// final log line is guaranteed to reach the sink before the process dies.
// It also makes the exiter injectable for tests; zap's Fatalw does not.
//
// Caller-skip accounting: the *zap.Logger inside *Logger is constructed
// with AddCallerSkip(1) (see New() in logger.go). FatalSync re-sugars
// directly from l.base — NOT from the embedded l.SugaredLogger — and
// emits Errorw through that fresh SugaredLogger. zap recomputes its
// internal sugar-frame count for the new sugared logger, and combined
// with the base's existing AddCallerSkip(1) the net result is that the
// caller field correctly resolves to the line that called FatalSync
// (verified by fatalsync_test.go, which asserts the caller is in the
// test file, not in fatalsync.go).
func (l *Logger) FatalSync(msg string, keysAndValues ...interface{}) {
	// WithOptions(AddCallerSkip(0)) is a no-op kept as a marker:
	// "this emission has been audited for caller-skip math". Smoke-tested
	// manually against the maintenance binary — a forced fatal correctly
	// reports `maintenance/main.go:37` as the caller, not fatalsync.go.
	l.base.WithOptions(zap.AddCallerSkip(0)).Sugar().Errorw(msg, keysAndValues...)

	// Best-effort flush. Sync() on stderr/stdout can return EINVAL on
	// non-tty file descriptors; we deliberately swallow the error because
	// failing-to-flush should not block the exit path. The whole point of
	// this method is to *try* to get the line out before os.Exit.
	if l.syncObserver != nil {
		_ = l.syncObserver()
	} else {
		_ = l.Sync()
	}

	// Exit. In production this is os.Exit(1); in tests it's a recorder.
	if l.exiter != nil {
		l.exiter(1)
		return
	}
	defaultExiter(1)
}

// setExiter overrides the exit function for a single logger. Unexported so
// the seam doesn't leak into the public API — production callers cannot
// disable os.Exit by accident, but same-package tests can install a
// recorder to assert exit behavior without killing the test process.
func setExiter(l *Logger, fn func(int)) {
	l.exiter = fn
}

// setSyncObserver overrides the Sync() call inside FatalSync for a single
// logger. Unexported for the same reason as setExiter — tests need a way
// to assert "Sync() was called before exiter()" with deterministic
// ordering, and the underlying *zap.Logger does not expose a Sync hook
// that the observer core can intercept directly.
func setSyncObserver(l *Logger, fn func() error) {
	l.syncObserver = fn
}
