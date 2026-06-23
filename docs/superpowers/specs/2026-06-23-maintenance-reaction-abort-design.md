# Maintenance bot: reaction-driven watch + 💔-reaction abort

**Date:** 2026-06-23
**Status:** Approved design
**Service:** `services/maintenance/` (systemd `animeenigma-maintenance.service`)
**Origin:** Feedback report `2026-06-22T04-43-17_tNeymik_telegram` — admin tNeymik: *«Разбитое сердечко не работает»* (the 💔 abort doesn't work).

## Problem

A long-running Claude analysis is fronted by a **«👁️ Analyzing… Reply 💔 to this message to abort.»** text message. The documented abort gesture is *reply to that message with text containing 💔*. The admin instead **reacted** with the 💔 emoji — the natural gesture — and nothing happened.

Two root causes, confirmed in code:

1. **The bot never receives reaction events.** `GetUpdates` requests only `allowed_updates=["message","callback_query"]` (`internal/telegram/client.go:249`); `message_reaction` is excluded, so Telegram never delivers emoji reactions.
2. **Abort is detected only as a text reply.** `isInterruptReply` (`cmd/maintenance/main.go:1714`) requires a reply *message* whose text contains 💔, sent to a 👁️ bot message. There is no reaction-handling path; the `Update` struct has no reaction field.

Secondary: the message in the report was already finished («✅ готово»), so even a correct text-reply 💔 would only have said «nothing to interrupt».

## Key insight

The bot **already** runs a reaction-based status lifecycle on the *source* message, in parallel with the redundant 👁️ text message:

| Reaction | Meaning | Set at |
|---|---|---|
| 👀 | analyzing / in progress | `main.go:622, 788, 1235` |
| 👍 | done / success | `main.go:685, 868, 1279` |
| 💔 | failed / aborted / dismissed | `main.go:658, 843, 1257, 1266, 1452` |

The 👁️ text message (`runInterruptible`, `main.go:1612-1686`) is a **second, redundant status display**. Removing it and moving abort onto the 💔 reaction satisfies both owner requests with one coherent change:

- *"the broken heart doesn't work"* → 💔 **reaction** now aborts.
- *"don't send 👁️ in a message — only use the eye in reactions"* → the 👁️ text message is deleted; status is the 👀 **reaction** the bot already sets.

## Feasibility (verified)

The bot is a **non-anonymous administrator** in the admin supergroup (`getChatMember` → `status: "administrator"`, `is_anonymous: false`). Telegram delivers `message_reaction` updates to bots in supergroups **only when the bot is an admin** — so once we add `message_reaction` to `allowed_updates`, 💔-reaction events will arrive.

**Hard constraint:** Telegram restricts reactions to a fixed emoji set. **👁️ (single eye, U+1F441) is NOT a valid reaction emoji; 👀 (eyes, U+1F440) IS** — and 👀 is exactly what the bot already reacts with. We therefore keep the existing 👀 reaction and remove only the 👁️ *text*. We cannot react with the literal 👁️.

## Design

### Behavior

| Phase | Today | After |
|---|---|---|
| Start | 👀 reaction on source msg **+** «👁️ Analyzing… Reply 💔 to abort» text msg | 👀 reaction only |
| Abort | reply *text* 💔 to the 👁️ msg | **react 💔** to the source msg |
| Done / Fail | flip 👀→👍/💔 reaction **+** edit text msg to ✅/❌/💔 | flip 👀→👍/💔 reaction only |

Abort confirmation is **silent**: the call site already flips the bot's 👀→💔 reaction when `fn` returns `errInterrupted`. That flip (plus the admin's own visible 💔) is the confirmation. No text reply.

### Components

**1. `internal/telegram/client.go`**

- `GetUpdates`: add `message_reaction` to `allowed_updates` →
  `allowed_updates=["message","callback_query","message_reaction"]`.
- New types parsed from the Bot API:
  ```go
  type ReactionType struct {
      Type  string `json:"type"`            // "emoji" | "custom_emoji" | "paid"
      Emoji string `json:"emoji,omitempty"` // present when Type == "emoji"
  }
  type MessageReactionUpdated struct {
      Chat        Chat           `json:"chat"`
      MessageID   int            `json:"message_id"`
      User        *UserInfo      `json:"user,omitempty"`       // absent for anonymous
      OldReaction []ReactionType `json:"old_reaction"`
      NewReaction []ReactionType `json:"new_reaction"`
  }
  ```
- `Update`: add `MessageReaction *MessageReactionUpdated `json:"message_reaction,omitempty"``.

**2. `cmd/maintenance/main.go` — `runInterruptible`**

Strip all messaging. The function becomes: register the interrupt **keyed by the `replyTo` source message ID** (the message already wearing 👀), run `fn` under a cancellable context, clear on return, and translate an admin-only context cancel into `errInterrupted`. No `SendMessage`/`SendReply`/`EditMessageText`.

- `replyTo == 0` (no source message): run `fn` without an abort handle, as the current `watchID == 0` path already degrades.
- `label` is retained for structured logs only.

Delete: `watchTerminalHTML`, `watchTerminalState` (+ its `iota` consts `watchDone/watchFailed/watchAborted`), and the `eyeBase` constant. Keep `heartBreak`, `interruptTTL`, `errInterrupted`, the registry helpers (`registerInterrupt`/`clearInterrupt`/`tryInterrupt`/`sweepInterrupts`), and `messageLabel`.

**3. `cmd/maintenance/main.go` — poller (`run`, goroutine 1)**

Replace the `isInterruptReply` branch in the per-update filter loop with reaction-abort detection:

```go
func isReactionAbort(u telegram.Update, botID int64) (msgID int, ok bool) {
    r := u.MessageReaction
    if r == nil {
        return 0, false
    }
    if r.User != nil && r.User.ID == botID { // ignore the bot's own flips
        return 0, false
    }
    if !reactionsContain(r.NewReaction, heartBreak) {
        return 0, false
    }
    return r.MessageID, true
}
```

- `reactionsContain` matches a `ReactionType{Type:"emoji"}` whose `Emoji` contains the 💔 codepoint (bare-codepoint substring, mirroring the existing VS16-tolerant match).
- On match: `if s.tryInterrupt(msgID) { log… }`. `tryInterrupt` is a no-op for an unregistered/finished message, so a late 💔 on a done analysis is harmless. No text reply (silent).
- **All `message_reaction` updates are handled in-poller and dropped from `kept`** — they are never queued to the processor (they have `Message == nil`).
- `botID` is fetched once at startup via `getMe` and stored on `service` (or the telegram client). Offset advancement is unchanged (already done for every update before the filter).

Delete `handleInterrupt` and call `s.tryInterrupt(msgID)` directly from the poller filter loop (with a log line) — there is no text confirmation to send, so the helper is no longer needed.

**4. Tests**

- Replace `cmd/maintenance/interrupt_test.go` and the 💔-reply cases in `autofix_test.go` with `isReactionAbort` table tests:
  - 💔 added (`new_reaction` contains 💔) to a registered msg → `(msgID, true)`.
  - bot's own reaction (`User.ID == botID`) → `(0, false)`.
  - non-💔 reaction (👍/👀) → `(0, false)`.
  - `MessageReaction == nil` (plain message update) → `(0, false)`.
  - VS16 / bare-codepoint 💔 forms both match.
- Keep / adjust any test asserting `runInterruptible` no longer sends or edits messages (assert the fake telegram client records zero `SendMessage`/`EditMessageText` for the watch path).

### Edge cases

- **Bot self-reaction** (the 👀→💔 flip emits a `message_reaction` with `User.ID == botID`) → ignored.
- **Late 💔** on a finished analysis → not in registry → `tryInterrupt` no-op.
- **Anonymous-admin reaction** → arrives as `message_reaction_count` (no per-user data), which we do **not** subscribe to; abort won't trigger. Acceptable: the reporting admin reacts non-anonymously.
- **`replyTo == 0`** call site → no 👀 message, no abort handle; `fn` still runs.

## Out of scope

- Anonymous-reaction abort (`message_reaction_count`).
- Reaction-driven control beyond abort (e.g., 👍 to approve a fix) — separate change.
- Any change to the Docker services or the report-mirroring pipeline.

## Deployment & verification

- All work in worktree `feat/maint-reaction-abort` off fresh `origin/main`.
- Build/deploy is **systemd**, not Docker: `cd services/maintenance && go build ./...`, then `sudo systemctl restart animeenigma-maintenance` (NOT `make redeploy-*`).
- Verify: `go test ./...` in `services/maintenance`; `go vet`.
- Live check: trigger an analysis, confirm only the 👀 reaction appears (no 👁️ text), react 💔, confirm the analysis is cancelled and the reaction flips to 💔.
- Finish with `/animeenigma-after-update` (changelog + push).
- Set feedback `2026-06-22T04-43-17_tNeymik_telegram` → `ai_done` after the live check.

## Metrics

- **UXΔ = +2 (Better)** — the natural 💔 gesture now works; one less noisy bot message per analysis.
- **CDI = 0.01 * 8** — single service, ~one file of logic + a client struct; low spread, low shift, Effort_Fib 8.
- **MVQ = Sprite 88%/82%** — small, self-contained, high-fit cleanup that removes a redundant surface.
