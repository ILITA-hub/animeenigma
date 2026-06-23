package main

import (
	"context"
	"testing"
	"time"

	dom "github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
)

type domainResult = dom.AnalysisResult

// botReply builds a Telegram update where a human replies (text) to a bot
// message whose text is replyToText.
func botReply(text, replyToText string, replyToID int) telegram.Update {
	return telegram.Update{
		Message: &telegram.Message{
			MessageID: 999,
			From:      &telegram.UserInfo{ID: 7, Username: "tNeymik", IsBot: false},
			Text:      text,
			ReplyTo: &telegram.Message{
				MessageID: replyToID,
				From:      &telegram.UserInfo{ID: 1, Username: "maint_bot", IsBot: true},
				Text:      replyToText,
			},
		},
	}
}

func TestIsInterruptReply(t *testing.T) {
	const watchID = 4242

	tests := []struct {
		name    string
		update  telegram.Update
		wantID  int
		wantOK  bool
	}{
		{
			name:   "valid abort: 💔 reply to 👁️ bot message (VS16 form)",
			update: botReply("💔", "👁️ Analyzing alert…\nReply 💔 to this message to abort.", watchID),
			wantID: watchID,
			wantOK: true,
		},
		{
			name:   "valid abort: bare 👁 codepoint in watch text",
			update: botReply("please stop 💔", "👁 working", watchID),
			wantID: watchID,
			wantOK: true,
		},
		{
			name:   "no heartbreak in new message",
			update: botReply("stop please", "👁️ Analyzing…", watchID),
			wantOK: false,
		},
		{
			name:   "reply target is not the eye watch message",
			update: botReply("💔", "🔧 Fix Applied", watchID),
			wantOK: false,
		},
		{
			name:   "reply target is not a bot",
			update: telegram.Update{Message: &telegram.Message{
				Text: "💔",
				From: &telegram.UserInfo{Username: "tNeymik"},
				ReplyTo: &telegram.Message{
					MessageID: watchID,
					From:      &telegram.UserInfo{Username: "tNeymik", IsBot: false},
					Text:      "👁️ Analyzing…",
				},
			}},
			wantOK: false,
		},
		{
			name:   "not a reply at all",
			update: telegram.Update{Message: &telegram.Message{Text: "💔"}},
			wantOK: false,
		},
		{
			name:   "callback query (no message)",
			update: telegram.Update{CallbackQuery: &telegram.CallbackQuery{Data: "fix:AUTO-1"}},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotID, gotOK := isInterruptReply(tc.update)
			if gotOK != tc.wantOK {
				t.Fatalf("isInterruptReply ok = %v, want %v", gotOK, tc.wantOK)
			}
			if tc.wantOK && gotID != tc.wantID {
				t.Fatalf("isInterruptReply id = %d, want %d", gotID, tc.wantID)
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
