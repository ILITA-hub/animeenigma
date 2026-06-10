# Maintenance Bot ↔ Feedback DB Integration + Telegram Media Support

**Date:** 2026-06-10
**Status:** Implemented
**UXΔ** = +3 (Better) — every Telegram report lands in /admin/feedback automatically with live status + attachments
**CDI** = 0.04 * 13
**MVQ** = Griffin 88%/85%

## Problem

1. The maintenance bot handled Telegram messages and Grafana alerts but its work was
   invisible to the `/admin/feedback` triage board — only site-footer reports lived there.
2. The bot could not see media (screenshots, log files), captions, forwarded messages,
   or reply context — `Message` parsing covered `text` only.
3. There was no lifecycle link: an auto-fixed user complaint never flipped any status
   anywhere a human could verify it.

## Design

### Feedback store write API (player service)

The store is on-disk JSON in the player container (`player_reports` volume) with a
`_status.json` sidecar guarded by an in-process mutex — so the bot must write **through
player**, not to disk, to avoid two-writer races. New internal endpoints (mounted outside
`/api`, not gateway-proxied; player publishes `127.0.0.1:8083` so the host-side bot can
reach them):

- `POST  /internal/feedback` — create entry (`player_type=telegram`, `source=telegram`,
  `telegram_meta`, same `{ts}_{user}_{type}.json` naming → zero changes to listing).
- `POST  /internal/feedback/{id}/attachments` — multipart upload; stored under
  `reportsDir/_attachments/{id}/` (the `_` prefix hides it from `List()`); appends the
  stored name to the entry's `attachments` array (cap 20, 25MB each).
- `PATCH /internal/feedback/{id}/status` — sidecar status update, `updated_by`
  defaults to `maintenance-bot`.
- Admin-gated `GET /api/admin/reports/{id}/attachments/{name}` serves files to the UI
  (gateway already wildcard-proxies `/admin/reports/*` to player).

### Feedback loop (maintenance bot)

**First thing on every human Telegram message** (admin message / user issue) the bot
mirrors it into the store (status `new`) — before any Claude analysis — then drives:

| Event | Feedback status |
|---|---|
| Analysis starts | `in_progress` |
| Analysis fails/times out | back to `new` |
| Fix applied (auto or admin button) | `ai_done` (human promotes to `resolved`) |
| Fix failed | `in_progress` |
| Button proposed / escalated | `in_progress` |
| info_only / already resolved | `resolved` |
| Admin dismisses | `not_relevant` |

HTTP reports (site footer/player) already exist in the store (player saves them before
forwarding) — the bot derives the entry id from `report_file` and drives the same
lifecycle, never creating a duplicate. Grafana alerts stay out of the feedback store
(they live in `issues.json` as before). Issues now carry `feedback_id` + `attachments`;
Telegram replies include a deep link `https://animeenigma.ru/admin/feedback?id=<id>`.
All feedback-store calls are WARN-and-continue — a store outage never blocks handling.

### Telegram media / reply / forward

- `Message` parsing extended: `caption`, `photo` (largest size taken), `document`,
  `video`, `audio`, `voice`, `media_group_id`, `forward_origin`, plus `getFile` +
  file download (≤25MB defensive cap; Bot API itself caps at 20MB).
- Classifier folds context into the message text: `[Forwarded from X]`,
  `[In reply to @user: …]`; caption substitutes for empty text. A human message with
  an attachment or forward is relevant even without issue keywords. Albums
  (shared `media_group_id`) are grouped by the poller and merged by the classifier
  into ONE message carrying all attachments.
- Attachments are downloaded to `ATTACHMENTS_DIR` (default
  `.claude/maintenance-attachments/{feedback-id}/`, gitignored) so the Claude dispatcher
  can `Read` them (the analysis prompt lists the paths), and uploaded into the
  feedback entry for the admin UI.

### Admin UI

`/admin/feedback`: `telegram` type filter; 📎 count in table/kanban rows; detail view
shows Telegram context (forwarded-from, reply-to, from-admin) and an attachments grid —
blob-fetched through axios (Bearer auth; a plain `<img src>` would arrive
unauthenticated) and rendered via object URLs (revoked on close).

### Config (new env, all with working defaults)

| Var | Default | Meaning |
|---|---|---|
| `PLAYER_INTERNAL_URL` | `http://localhost:8083` | player internal feedback API |
| `ATTACHMENTS_DIR` | `.claude/maintenance-attachments` | host attachment dir (rel. to PROJECT_ROOT) |
| `FEEDBACK_BASE_URL` | `https://animeenigma.ru` | deep-link prefix in Telegram replies |

## Error handling

- Feedback store unreachable → WARN, message handled without mirror (no status updates).
- Attachment download/upload failures are per-file WARN-and-skip.
- Path traversal guarded at every id/name boundary (existing `safeReportPath` +
  new sanitizers); internal create restricted to `player_type=telegram`.

## Testing

- `services/player/internal/handler/internal_feedback_test.go` — create / validation /
  status sidecar / attachment upload+serve / traversal / `_attachments` invisible to List.
- `services/maintenance/internal/classifier/classifier_media_test.go` — caption fallback,
  attachment relevance, forward+reply composition, album merge, chatter still ignored.
- `services/maintenance/internal/feedback/client_test.go` — client against httptest server.
