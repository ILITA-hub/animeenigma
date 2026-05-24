# Phase 06: TelegramNewsCard refactor - Context

**Gathered:** 2026-05-22
**Mode:** Auto-generated from approved REFACTOR-PROPOSAL.md

<domain>
Give TelegramNewsCard a clear Telegram identity:
- `<SpotlightBackdrop variant="gradient-mesh" accent="sky">` (Telegram blue).
- Telegram SVG logo + channel attribution in header.
- Post thumbnails when available (backend pass-through).
- "Открыть пост →" link uses sky accent + external-link icon.

In scope: `cards/TelegramNewsCard.vue` + spec, `services/catalog/internal/service/spotlight/cards/telegram_news.go` (pass-through image_url).
</domain>

<decisions>
- Channel attribution: `@anime_enigma` shown in header right side (hardcoded literal — channel name is stable).
- Thumbnail aspect: `aspect-square`; fallback gracefully to text-only when `image_url` absent.
- 5-min pre-implementation spike: `redis-cli GET news:telegram | jq '.[0] | keys'` to confirm `image_url` field exists in the cached payload before extending the Go struct. If absent, ship without thumbnails (backend touch becomes no-op).
</decisions>

<code_context>
- `SpotlightBackdrop variant="gradient-mesh" accent="sky"` exists from Phase 01.
- `SpotlightIcon name="telegram"` exists from Phase 01.
- TelegramNewsData.posts: array of `{title?, excerpt, date?, link?}` — need to add `image_url?` to TS type + Go struct.
- Existing T-03-18 pin: external `<a>` MUST have `target="_blank" rel="noopener noreferrer"`. Keep.
</code_context>

<specifics>
- Header: `<SpotlightIcon name="telegram" class="w-6 h-6 text-sky-300" aria-label="Telegram">` + h3 title + right-aligned `<span>@anime_enigma</span>`.
- Post layout: thumbnail (if present) + title (if present) + excerpt + date + open-cta.
- Backend struct extension: `ImageURL *string json:"image_url,omitempty"`.
</specifics>

<deferred>
- Live subscriber count in attribution (would need new Telegram API call — not worth it for v1.1).
</deferred>
