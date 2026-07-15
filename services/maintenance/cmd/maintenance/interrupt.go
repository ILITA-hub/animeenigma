package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
)

// --- Emoji interrupt protocol (AUTO-456) ---
//
// A long-running Claude invocation is interrupted by the admin reacting 💔 to
// the source message. Detection happens in the Telegram poller (NOT the
// processor), because the processor goroutine is blocked inside the very
// analysis we want to cancel — so an update queued behind it could never reach
// a busy processor in time.
const (
	heartBreak = "\U0001F494" // 💔
	// interruptTTL bounds how long a cancel func lingers in the registry if a
	// computation neither completes nor is interrupted (safety net only —
	// runInterruptible always deregisters on return). Must exceed the claude
	// analysis timeout (1h) so the sweeper never kills a legitimately running
	// analysis before the admin can react 💔.
	interruptTTL = 90 * time.Minute
)

// errInterrupted is returned by runInterruptible when the computation's context
// was cancelled by an admin 💔 reply (as opposed to a timeout or shutdown).
// Callers skip their normal failure reply for it — the poller already sent the
// abort confirmation.
var errInterrupted = errors.New("computation interrupted by admin")

// interruptEntry pairs a computation's cancel func with its expiry for TTL sweep.
type interruptEntry struct {
	cancel  context.CancelFunc
	expires time.Time
}

// registerInterrupt records the cancel func for a 👁️ watch message.
func (s *service) registerInterrupt(watchMsgID int, cancel context.CancelFunc) {
	s.interrupts.Store(watchMsgID, &interruptEntry{
		cancel:  cancel,
		expires: time.Now().Add(interruptTTL),
	})
}

// clearInterrupt removes a registry entry (idempotent). Does NOT call cancel —
// the owning runInterruptible defers its own cancel().
func (s *service) clearInterrupt(watchMsgID int) {
	s.interrupts.Delete(watchMsgID)
}

// tryInterrupt cancels the computation registered under watchMsgID and removes
// the entry. Returns false if nothing was registered (already finished/unknown).
func (s *service) tryInterrupt(watchMsgID int) bool {
	v, ok := s.interrupts.LoadAndDelete(watchMsgID)
	if !ok {
		return false
	}
	if e, ok := v.(*interruptEntry); ok && e.cancel != nil {
		e.cancel()
	}
	return true
}

// sweepInterrupts cancels and drops registry entries past their TTL. A pure
// safety net against a leaked entry; the happy path deregisters on return.
func (s *service) sweepInterrupts(now time.Time) {
	s.interrupts.Range(func(k, v any) bool {
		if e, ok := v.(*interruptEntry); ok && now.After(e.expires) {
			if e.cancel != nil {
				e.cancel()
			}
			s.interrupts.Delete(k)
		}
		return true
	})
}

// runInterruptible runs fn under a cancellable context registered against the
// source message (srcMsgID — the message already wearing the 👀 reaction). An
// admin aborts by reacting 💔 to that message; the Telegram poller detects the
// reaction out-of-band (the processor goroutine is blocked inside fn) and
// cancels this context, SIGKILLing the Claude subprocess. If fn was cancelled
// by an admin interrupt (and not by service shutdown) it returns errInterrupted
// so the caller suppresses its normal failure reply. The "watching" status is
// shown entirely by the 👀→👍/💔 reaction lifecycle the callers drive on
// srcMsgID — runInterruptible sends no message of its own.
func (s *service) runInterruptible(ctx context.Context, srcMsgID int, label string, fn func(context.Context) (*domain.AnalysisResult, error)) (*domain.AnalysisResult, error) {
	aCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if srcMsgID <= 0 {
		// No source message to react to → no abort handle; run anyway.
		return fn(aCtx)
	}

	if log != nil {
		log.Infow("running interruptible analysis", "label", label, "src_msg_id", srcMsgID)
	}
	s.registerInterrupt(srcMsgID, cancel)
	defer s.clearInterrupt(srcMsgID)

	result, err := fn(aCtx)

	// Distinguish an admin interrupt (only aCtx cancelled) from a shutdown
	// (parent ctx cancelled too). Only the former gets the dedicated sentinel.
	if err != nil && aCtx.Err() != nil && ctx.Err() == nil {
		return nil, errInterrupted
	}
	return result, err
}

// reactionsContain reports whether rs has an emoji reaction containing emoji
// (bare-codepoint substring match, tolerant of VS16 variants).
func reactionsContain(rs []telegram.ReactionType, emoji string) bool {
	for _, r := range rs {
		if r.Type == "emoji" && strings.Contains(r.Emoji, emoji) {
			return true
		}
	}
	return false
}

// isReactionAbort reports whether update u is a 💔 reaction added by a human
// (not the bot itself), returning the reacted message ID. The bot runs in a
// single trusted admin chat and abort is fail-safe (it only stops work), so
// this is intentionally not gated beyond the structural match. Whether that
// message has a live analysis is decided by tryInterrupt at the call site.
func isReactionAbort(u telegram.Update, botID int64) (msgID int, ok bool) {
	r := u.MessageReaction
	if r == nil {
		return 0, false
	}
	if r.User != nil && r.User.ID == botID { // belt-and-suspenders: Telegram never delivers bot-set reactions back, so this guard is defense-in-depth
		return 0, false
	}
	if !reactionsContain(r.NewReaction, heartBreak) {
		return 0, false
	}
	return r.MessageID, true
}
