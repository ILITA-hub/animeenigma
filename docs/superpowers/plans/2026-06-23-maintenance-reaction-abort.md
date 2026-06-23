# Maintenance Reaction-Abort Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the maintenance bot abort a running analysis when an admin reacts 💔 to the source message, and remove the redundant "👁️ Analyzing… Reply 💔 to abort" text message — status is shown only by the existing 👀→👍/💔 reaction.

**Architecture:** The bot already drives a reaction lifecycle (👀 analyzing → 👍 done / 💔 failed) on the source message. We delete the parallel 👁️ text-message status display, re-key the interrupt registry by the source message ID, subscribe to Telegram `message_reaction` updates, and detect a 💔 reaction in the poller (out-of-band from the blocked processor goroutine) to cancel the analysis. The bot flips its own 👀→💔 reaction as the silent confirmation.

**Tech Stack:** Go 1.x, Telegram Bot API (`getUpdates` long-poll, `setMessageReaction`), systemd service `animeenigma-maintenance`.

## Global Constraints

- All work in worktree `feat/maint-reaction-abort` off `origin/main`. NEVER edit the base tree at `/data/animeenigma` (except `.env`).
- Deployment is **systemd**, not Docker: `go build` then `sudo systemctl restart animeenigma-maintenance`. Do NOT use `make redeploy-*`.
- Telegram reactions are restricted to a fixed emoji set: **👁️ (U+1F441) is NOT a valid reaction; 👀 (U+1F440) IS** — keep the existing 👀 reaction, never react with 👁️.
- The bot is a non-anonymous supergroup admin → it receives `message_reaction` updates once they are in `allowed_updates`. Anonymous-admin reactions (`message_reaction_count`) are out of scope.
- Commits use the project co-author trailer:
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- Each commit must compile (`go build ./...`) and pass `go test ./...` from `services/maintenance`.

---

### Task 1: Telegram client — receive `message_reaction` updates

**Files:**
- Modify: `services/maintenance/internal/telegram/client.go` (`Update` struct ~68-72; `GetUpdates` ~247-259)
- Test: `services/maintenance/internal/telegram/reaction_test.go` (create)

**Interfaces:**
- Produces:
  - `type ReactionType struct { Type string; Emoji string }`
  - `type MessageReactionUpdated struct { Chat Chat; MessageID int; User *UserInfo; OldReaction []ReactionType; NewReaction []ReactionType }`
  - `Update.MessageReaction *MessageReactionUpdated`
  - package const `allowedUpdates string` (contains `message_reaction`)

- [ ] **Step 1: Write the failing test**

Create `services/maintenance/internal/telegram/reaction_test.go`:

```go
package telegram

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAllowedUpdatesIncludesReactions(t *testing.T) {
	if !strings.Contains(allowedUpdates, "message_reaction") {
		t.Fatalf("allowedUpdates must request message_reaction, got %q", allowedUpdates)
	}
}

func TestUnmarshalMessageReactionUpdate(t *testing.T) {
	raw := `[{
		"update_id": 100,
		"message_reaction": {
			"chat": {"id": -1003753190340, "type": "supergroup"},
			"message_id": 4242,
			"user": {"id": 898912046, "is_bot": false, "username": "tNeymik"},
			"old_reaction": [],
			"new_reaction": [{"type": "emoji", "emoji": "\U0001F494"}]
		}
	}]`

	var updates []Update
	if err := json.Unmarshal([]byte(raw), &updates); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("want 1 update, got %d", len(updates))
	}
	r := updates[0].MessageReaction
	if r == nil {
		t.Fatal("MessageReaction is nil — field not parsed")
	}
	if r.MessageID != 4242 {
		t.Errorf("MessageID = %d, want 4242", r.MessageID)
	}
	if r.User == nil || r.User.ID != 898912046 {
		t.Errorf("User.ID not parsed: %+v", r.User)
	}
	if len(r.NewReaction) != 1 || r.NewReaction[0].Type != "emoji" || r.NewReaction[0].Emoji != "\U0001F494" {
		t.Errorf("NewReaction not parsed: %+v", r.NewReaction)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/maintenance && go test ./internal/telegram/ -run 'AllowedUpdates|MessageReaction' -v`
Expected: FAIL — `undefined: allowedUpdates` and `r.MessageReaction` undefined (build error).

- [ ] **Step 3: Add the reaction types and `MessageReaction` field**

In `services/maintenance/internal/telegram/client.go`, change the `Update` struct (currently lines ~68-72) to:

```go
type Update struct {
	UpdateID        int64                   `json:"update_id"`
	Message         *Message                `json:"message,omitempty"`
	CallbackQuery   *CallbackQuery          `json:"callback_query,omitempty"`
	MessageReaction *MessageReactionUpdated `json:"message_reaction,omitempty"`
}

// ReactionType is one element of a message's reaction list. Type is
// "emoji" | "custom_emoji" | "paid"; Emoji is set only when Type == "emoji".
type ReactionType struct {
	Type  string `json:"type"`
	Emoji string `json:"emoji,omitempty"`
}

// MessageReactionUpdated is a Bot API message_reaction update: a single user
// changed their reaction on a message. User is absent for anonymous reactions.
type MessageReactionUpdated struct {
	Chat        Chat           `json:"chat"`
	MessageID   int            `json:"message_id"`
	User        *UserInfo      `json:"user,omitempty"`
	OldReaction []ReactionType `json:"old_reaction"`
	NewReaction []ReactionType `json:"new_reaction"`
}
```

- [ ] **Step 4: Request reaction updates in `GetUpdates`**

In `client.go`, just above `GetUpdates` (line ~247), add the const, then use it. Replace:

```go
// GetUpdates long-polls for new messages. Blocks up to timeoutSec.
func (c *Client) GetUpdates(offset int64, timeoutSec int) ([]Update, error) {
	url := fmt.Sprintf("getUpdates?offset=%d&timeout=%d&allowed_updates=[\"message\",\"callback_query\"]", offset, timeoutSec)
```

with:

```go
// allowedUpdates is the getUpdates allowed_updates filter. message_reaction is
// required so the bot receives 💔-reaction aborts (delivered only because the
// bot is a supergroup admin).
const allowedUpdates = `["message","callback_query","message_reaction"]`

// GetUpdates long-polls for new messages. Blocks up to timeoutSec.
func (c *Client) GetUpdates(offset int64, timeoutSec int) ([]Update, error) {
	url := fmt.Sprintf("getUpdates?offset=%d&timeout=%d&allowed_updates=%s", offset, timeoutSec, allowedUpdates)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd services/maintenance && go test ./internal/telegram/ -run 'AllowedUpdates|MessageReaction' -v`
Expected: PASS (both tests).

- [ ] **Step 6: Build + commit**

```bash
cd services/maintenance && go build ./... && go vet ./internal/telegram/
git -C /data/animeenigma-wt/maint-reaction-abort add services/maintenance/internal/telegram/client.go services/maintenance/internal/telegram/reaction_test.go
git -C /data/animeenigma-wt/maint-reaction-abort commit services/maintenance/internal/telegram/client.go services/maintenance/internal/telegram/reaction_test.go -m "feat(maintenance): receive Telegram message_reaction updates

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: `runInterruptible` — drop the 👁️ watch message, key by source message

**Files:**
- Modify: `services/maintenance/cmd/maintenance/main.go` (`runInterruptible` ~1612-1659; `watchTerminalState`/`watchTerminalHTML` ~1661-1686; `eyeBase` is kept this task; `interrupts` field comment ~201-206)
- Modify: `services/maintenance/cmd/maintenance/autofix_test.go` (delete `TestWatchTerminalHTML` ~161-196; drop `"strings"` import)

**Interfaces:**
- Consumes: `registerInterrupt`, `clearInterrupt`, `errInterrupted` (unchanged); `domain.AnalysisResult`.
- Produces: `runInterruptible(ctx context.Context, srcMsgID int, label string, fn func(context.Context) (*domain.AnalysisResult, error)) (*domain.AnalysisResult, error)` — same signature, but registers the interrupt under `srcMsgID` (the message wearing 👀) and sends/edits NO messages.

- [ ] **Step 1: Update the failing test (assert no watch message + interrupt keyed by source id)**

`runInterruptible` now makes zero telegram calls, so it is unit-testable with a nil-`tg` `&service{}`. Add this test to `services/maintenance/cmd/maintenance/interrupt_test.go` (append at end of file — it already imports `context`, `testing`, `time`):

```go
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
```

> NOTE: `domainResult` is an alias to keep the test readable. Add this once near the top of `interrupt_test.go`, after the imports:
> ```go
> import dom "github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
> type domainResult = dom.AnalysisResult
> ```
> If the import block syntax is awkward, instead just use `*dom.AnalysisResult` inline and skip the alias.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/maintenance && go test ./cmd/maintenance/ -run TestRunInterruptibleKeysBySourceMessage -v`
Expected: FAIL — current `runInterruptible` calls `s.tg.SendMessage` → nil-pointer panic (or registers under a fresh watch id, not `srcMsgID`).

- [ ] **Step 3: Rewrite `runInterruptible`**

Replace the whole `runInterruptible` function (lines ~1612-1659, from its doc comment through its closing brace) with:

```go
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

	log.Infow("running interruptible analysis", "label", label, "src_msg_id", srcMsgID)
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
```

- [ ] **Step 4: Delete the now-dead watch-message helpers**

Delete `watchTerminalState` (the type + its `iota` const block `watchDone`/`watchFailed`/`watchAborted`) and `watchTerminalHTML` — lines ~1661-1686 (from `// watchTerminalState is the resolved outcome…` through the closing brace of `watchTerminalHTML`). Leave `eyeBase`, `heartBreak`, `interruptTTL`, `errInterrupted`, `interruptEntry`, and the registry helpers intact. (`eyeBase` is still referenced by `isInterruptReply` — removed in Task 3.)

- [ ] **Step 5: Update the stale `interrupts` field comment**

In `main.go` ~201-206, replace the comment above the `interrupts sync.Map` field:

```go
	// interrupts maps a source message ID (the message wearing the 👀 reaction)
	// → *interruptEntry. Each long-running Claude invocation registers its
	// context.CancelFunc here; an admin aborts by reacting 💔 to that message
	// (detected in the Telegram poller). Entries are removed on completion, on
	// interrupt, or by the TTL sweeper.
	interrupts sync.Map // map[int]*interruptEntry
```

- [ ] **Step 6: Delete `TestWatchTerminalHTML` and its orphaned import**

In `services/maintenance/cmd/maintenance/autofix_test.go`, delete the entire `TestWatchTerminalHTML` function (~161-196). Then remove the now-unused `"strings"` line from its import block (all 5 `strings.` uses were inside that test).

- [ ] **Step 7: Run tests + build**

Run: `cd services/maintenance && go build ./... && go test ./cmd/maintenance/ -run 'Interrupt|RunInterruptible' -v`
Expected: PASS — `TestRunInterruptibleKeysBySourceMessage`, `TestInterruptRegistryLifecycle`, `TestClearInterruptDoesNotCancel`, `TestSweepInterruptsTTL`, `TestTryInterruptCancelsContext`. `TestIsInterruptReply` still passes (unchanged this task).

- [ ] **Step 8: Commit**

```bash
git -C /data/animeenigma-wt/maint-reaction-abort commit services/maintenance/cmd/maintenance/main.go services/maintenance/cmd/maintenance/interrupt_test.go services/maintenance/cmd/maintenance/autofix_test.go -m "refactor(maintenance): drop 👁️ watch message, key interrupt by source msg

Status is now shown only by the 👀→👍/💔 reaction lifecycle. runInterruptible
no longer sends/edits a bot message; the interrupt registry is keyed by the
source message id (the one wearing 👀).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: Poller — abort on 💔 reaction; remove the text-reply path

**Files:**
- Modify: `services/maintenance/cmd/maintenance/main.go` (poller filter ~245-258; delete `isInterruptReply` ~1709-1729 and `handleInterrupt` ~1731-1744; delete `eyeBase` const ~1546-1549; add `reactionsContain` + `isReactionAbort`)
- Modify: `services/maintenance/cmd/maintenance/interrupt_test.go` (replace `TestIsInterruptReply` + `botReply` with `TestIsReactionAbort` + `reactionUpdate`)

**Interfaces:**
- Consumes: `telegram.Update.MessageReaction`, `telegram.ReactionType`, `telegram.MessageReactionUpdated` (Task 1); `s.tg.BotUserID()`; `s.tryInterrupt`; `heartBreak`; `s.tg.SetReaction`.
- Produces:
  - `reactionsContain(rs []telegram.ReactionType, emoji string) bool`
  - `isReactionAbort(u telegram.Update, botID int64) (msgID int, ok bool)`

- [ ] **Step 1: Write the failing test**

In `services/maintenance/cmd/maintenance/interrupt_test.go`, delete `TestIsInterruptReply` (~28-95) and the `botReply` helper (~11-26), and add:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/maintenance && go test ./cmd/maintenance/ -run TestIsReactionAbort -v`
Expected: FAIL — `undefined: isReactionAbort` (build error).

- [ ] **Step 3: Add `reactionsContain` + `isReactionAbort`**

In `main.go`, replace `isInterruptReply` (lines ~1709-1729, the doc comment through closing brace) with:

```go
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
	if r.User != nil && r.User.ID == botID { // ignore the bot's own 👀→💔 flip
		return 0, false
	}
	if !reactionsContain(r.NewReaction, heartBreak) {
		return 0, false
	}
	return r.MessageID, true
}
```

- [ ] **Step 4: Delete `handleInterrupt`**

Delete the entire `handleInterrupt` function (lines ~1731-1744). The poller will call `tryInterrupt` directly (Step 5). It sent the now-removed text confirmation.

- [ ] **Step 5: Rewrite the poller filter loop**

In `main.go`, replace the abort-intercept block (lines ~245-258) with:

```go
				// Handle message_reaction updates HERE, in the poller, and never
				// queue them to the processor. A 💔 reaction on a message with a
				// live analysis aborts it: the processor goroutine is blocked
				// inside that very analysis, so the cancel must act out-of-band.
				// The bot flips its own 👀→💔 reaction as the silent confirmation.
				kept := updates[:0]
				for _, u := range updates {
					if u.MessageReaction != nil {
						if msgID, ok := isReactionAbort(u, s.tg.BotUserID()); ok && s.tryInterrupt(msgID) {
							s.tg.SetReaction(msgID, heartBreak)
							log.Infow("analysis aborted by admin 💔 reaction", "message_id", msgID)
						}
						continue
					}
					kept = append(kept, u)
				}
				updates = kept
```

- [ ] **Step 6: Delete the `eyeBase` const**

In the `const` block at ~1545-1557, delete the `eyeBase` declaration and its doc comment (the 3 comment lines + `eyeBase = "\U0001F441"`). Keep `heartBreak` and `interruptTTL`. Verify no remaining references: `grep -n eyeBase services/maintenance/cmd/maintenance/*.go` must print nothing.

- [ ] **Step 7: Run tests + build + vet**

Run:
```bash
cd services/maintenance && go build ./... && go vet ./... && go test ./... -count=1
```
Expected: PASS — all maintenance tests, including `TestIsReactionAbort` and the unchanged registry-lifecycle tests. No `eyeBase`/`isInterruptReply`/`handleInterrupt`/`watchTerminalHTML` references remain.

- [ ] **Step 8: Commit**

```bash
git -C /data/animeenigma-wt/maint-reaction-abort commit services/maintenance/cmd/maintenance/main.go services/maintenance/cmd/maintenance/interrupt_test.go -m "feat(maintenance): abort running analysis on 💔 reaction

Poller subscribes to message_reaction updates and cancels the analysis keyed by
the reacted source message, flipping 👀→💔 as silent confirmation. Removes the
dead 💔-text-reply abort path (isInterruptReply/handleInterrupt). Closes
feedback 2026-06-22T04-43-17_tNeymik_telegram.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Post-implementation (outside task loop)

1. **Deploy (systemd):**
   ```bash
   cd services/maintenance && go build -o /tmp/maintenance ./cmd/maintenance   # smoke build
   sudo systemctl restart animeenigma-maintenance
   journalctl -u animeenigma-maintenance -n 30 --no-pager   # confirm clean start + "reactions support determined" = true
   ```
   > NOTE: the binary is built/installed by the existing service mechanism; confirm how the unit builds (`cat /etc/systemd/system/animeenigma-maintenance.service`) and follow it — do not hand-copy a binary if the unit `go run`s or builds on start.
2. **Live verification:** trigger an analysis (e.g. submit a test report), confirm in Telegram that only the 👀 reaction appears (NO "👁️ Analyzing…" text message), react 💔 to that message, and confirm the analysis is cancelled (logs: "analysis aborted by admin 💔 reaction") and the bot's reaction flips to 💔.
3. **Feedback status:** `bin/feedback-status 2026-06-22T04-43-17_tNeymik_telegram ai_done` after the live check passes.
4. **Finish:** run `/animeenigma-after-update` (changelog in Russian Trump-mode + push), then integrate the branch to `main` per the git workflow and remove the worktree.

## Self-review notes

- **Spec coverage:** allowed_updates (T1) ✓; reaction parse types (T1) ✓; runInterruptible strip + re-key (T2) ✓; delete watch helpers/eyeBase (T2/T3) ✓; poller 💔-reaction abort + silent 👀→💔 flip (T3) ✓; remove text-reply path (T3) ✓; tests replaced (T2/T3) ✓; systemd deploy + live check + feedback status (post) ✓.
- **Edge cases:** bot self-reaction ignored (`isReactionAbort` botID guard); late 💔 on finished analysis → `tryInterrupt` false → no flip; `srcMsgID <= 0` → runs without abort handle; non-💔 / cleared reactions dropped, never queued.
- **Type consistency:** `isReactionAbort(u, botID)`/`reactionsContain` signatures match T3 usage; `runInterruptible` keeps its 4-arg signature so call sites at `main.go:637, 826, 1239` are untouched; `MessageReactionUpdated`/`ReactionType` field names match T1 ↔ T3.
