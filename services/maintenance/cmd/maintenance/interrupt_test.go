package main

import (
	"context"
	"testing"
	"time"

	dom "github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
)

type domainResult = dom.AnalysisResult

// reactionUpdate builds a message_reaction update: userID changed their
// reaction on msgID, with newEmojis now present.
func reactionUpdate(msgID int, userID int64, newEmojis ...string) telegram.Update {
	rs := make([]telegram.ReactionType, 0, len(newEmojis))
	for _, e := range newEmojis {
		rs = append(rs, telegram.ReactionType{Type: "emoji", Emoji: e})
	}
	return telegram.Update{MessageReaction: &telegram.MessageReactionUpdated{
		MessageID:   msgID,
		User:        &telegram.UserInfo{ID: userID},
		NewReaction: rs,
	}}
}

func TestIsReactionAbort(t *testing.T) {
	const botID = int64(1)
	const heart = "\U0001F494" // 💔

	tests := []struct {
		name   string
		update telegram.Update
		wantID int
		wantOK bool
	}{
		{
			name:   "💔 reaction by admin → abort",
			update: reactionUpdate(4242, 7, heart),
			wantID: 4242,
			wantOK: true,
		},
		{
			name:   "💔 among several reactions → abort",
			update: reactionUpdate(4242, 7, "\U0001F44D", heart),
			wantID: 4242,
			wantOK: true,
		},
		{
			name:   "bot's own 💔 flip → ignored",
			update: reactionUpdate(4242, botID, heart),
			wantOK: false,
		},
		{
			name:   "non-💔 reaction (👍) → ignored",
			update: reactionUpdate(4242, 7, "\U0001F44D"),
			wantOK: false,
		},
		{
			name:   "reaction cleared (empty new_reaction) → ignored",
			update: reactionUpdate(4242, 7),
			wantOK: false,
		},
		{
			name:   "plain message update (no reaction) → ignored",
			update: telegram.Update{Message: &telegram.Message{Text: "\U0001F494"}},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotID, gotOK := isReactionAbort(tc.update, botID)
			if gotOK != tc.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tc.wantOK)
			}
			if tc.wantOK && gotID != tc.wantID {
				t.Fatalf("id = %d, want %d", gotID, tc.wantID)
			}
		})
	}
}

func TestInterruptRegistryLifecycle(t *testing.T) {
	s := &service{}

	canceled := false
	cancel := func() { canceled = true }
	s.registerInterrupt(100, cancel)

	if _, ok := s.interrupts.Load(100); !ok {
		t.Fatal("entry not registered")
	}

	// tryInterrupt fires the cancel func and removes the entry.
	if !s.tryInterrupt(100) {
		t.Fatal("tryInterrupt returned false for registered entry")
	}
	if !canceled {
		t.Fatal("cancel func not called by tryInterrupt")
	}
	if _, ok := s.interrupts.Load(100); ok {
		t.Fatal("entry not removed after tryInterrupt")
	}

	// Second call is a no-op (already finished/unknown).
	if s.tryInterrupt(100) {
		t.Fatal("tryInterrupt returned true for unknown entry")
	}
}

func TestClearInterruptDoesNotCancel(t *testing.T) {
	s := &service{}
	canceled := false
	s.registerInterrupt(7, func() { canceled = true })
	s.clearInterrupt(7)
	if canceled {
		t.Fatal("clearInterrupt must not call cancel (owner defers its own)")
	}
	if _, ok := s.interrupts.Load(7); ok {
		t.Fatal("clearInterrupt did not remove entry")
	}
}

func TestSweepInterruptsTTL(t *testing.T) {
	s := &service{}
	now := time.Now()

	freshCanceled, staleCanceled := false, false
	// Fresh entry (expires in the future).
	s.interrupts.Store(1, &interruptEntry{cancel: func() { freshCanceled = true }, expires: now.Add(5 * time.Minute)})
	// Stale entry (already expired).
	s.interrupts.Store(2, &interruptEntry{cancel: func() { staleCanceled = true }, expires: now.Add(-time.Minute)})

	s.sweepInterrupts(now)

	if _, ok := s.interrupts.Load(1); !ok {
		t.Fatal("fresh entry was swept")
	}
	if freshCanceled {
		t.Fatal("fresh entry cancel was called")
	}
	if _, ok := s.interrupts.Load(2); ok {
		t.Fatal("stale entry not swept")
	}
	if !staleCanceled {
		t.Fatal("stale entry cancel not called by sweep")
	}
}

// TestTryInterruptCancelsContext proves a registered cancel func — fired by
// tryInterrupt out-of-band — actually cancels the computation's context. This
// is the mechanism that propagates to the dispatcher's ctx.Done() select and
// SIGKILLs the Claude subprocess.
func TestTryInterruptCancelsContext(t *testing.T) {
	s := &service{}

	aCtx, cancel := context.WithCancel(context.Background())
	s.registerInterrupt(555, cancel)

	doneErr := make(chan error, 1)
	go func() {
		<-aCtx.Done()
		doneErr <- aCtx.Err()
	}()

	if !s.tryInterrupt(555) {
		t.Fatal("tryInterrupt returned false for registered entry")
	}

	select {
	case err := <-doneErr:
		if err != context.Canceled {
			t.Fatalf("ctx err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("context was not cancelled within 2s after tryInterrupt")
	}
}

// TestRunInterruptibleKeysBySourceMessage proves runInterruptible registers the
// interrupt under the source message id (the one wearing 👀) and sends NO bot
// message (tg is nil and must never be dereferenced), and that cancelling that
// registered context surfaces as errInterrupted.
func TestRunInterruptibleKeysBySourceMessage(t *testing.T) {
	s := &service{} // tg is nil — a watch message would panic

	const srcMsgID = 4242
	started := make(chan struct{})
	got := make(chan error, 1)
	go func() {
		_, err := s.runInterruptible(context.Background(), srcMsgID, "Analyzing alert X", func(c context.Context) (*domainResult, error) {
			close(started)
			<-c.Done() // block until interrupted
			return nil, c.Err()
		})
		got <- err
	}()

	<-started
	if _, ok := s.interrupts.Load(srcMsgID); !ok {
		t.Fatalf("interrupt not registered under source message id %d", srcMsgID)
	}
	if !s.tryInterrupt(srcMsgID) {
		t.Fatal("tryInterrupt returned false for the registered source message")
	}
	select {
	case err := <-got:
		if err != errInterrupted {
			t.Fatalf("err = %v, want errInterrupted", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runInterruptible did not return within 2s after interrupt")
	}
}
