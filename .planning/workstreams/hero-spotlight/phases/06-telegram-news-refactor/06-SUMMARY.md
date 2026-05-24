---
phase: 06-telegram-news-refactor
plan: 06
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-TG-01, HSB-V11-TG-02, HSB-V11-TG-03, HSB-V11-TG-04]
tags: [spotlight, telegram, refactor, branding, a11y, frontend, backend]
dependency_graph:
  requires:
    - 01-foundation/01 (SpotlightBackdrop, SpotlightIcon, cta-text classes, cardTokens)
  provides:
    - branded TelegramNewsCard with sky gradient-mesh backdrop
    - real per-post thumbnails when Telegram channel posts have image_url
    - labeled-mode for SpotlightIcon (forward-compat for future cards)
  affects:
    - frontend/web/src/components/home/spotlight/SpotlightIcon.vue (labeled-mode is additive; default decorative behaviour unchanged)
tech_stack:
  added: []
  patterns:
    - "Single-root <article> with SpotlightBackdrop as first child (Transition out-in safety)"
    - "Conditional aria-label/role=img/aria-hidden on the SpotlightIcon SVG via computed forwardedAriaLabel"
    - "snake_case fields end-to-end backend -> spotlight.TelegramPost -> TS TelegramPost (Pitfall 8 from Phase 2)"
key_files:
  modified:
    - services/catalog/internal/service/spotlight/types.go
    - services/catalog/internal/service/spotlight/types_test.go
    - services/catalog/internal/service/spotlight/cards/telegram_news.go
    - services/catalog/internal/service/spotlight/cards/telegram_news_test.go
    - frontend/web/src/types/spotlight.ts
    - frontend/web/src/components/home/spotlight/SpotlightIcon.vue
    - frontend/web/src/components/home/spotlight/SpotlightIcon.spec.ts
    - frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.vue
    - frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.spec.ts
  created:
    - .planning/workstreams/hero-spotlight/phases/06-telegram-news-refactor/deferred-items.md
commits:
  - 83cb865 feat(spotlight/06-06) surface image_url through TelegramPost (HSB-V11-TG-01)
  - 348197f feat(spotlight/06-06) add image_url to TelegramPost TS type (HSB-V11-TG-01)
  - f6a15d0 feat(spotlight/06-06) brand TelegramNewsCard with sky mesh + thumbnails (HSB-V11-TG-02..04)
decisions:
  - "Inverted the pre-implementation spike finding: live Redis `news:telegram` cache actually DOES carry image_url for ~30% of @anime_enigma posts. Backend pass-through is a real feature, not a forward-compat no-op as the orchestrator prompt assumed."
  - "Applied Rule 2 to SpotlightIcon.vue: added labeled-mode (role=img + aria-label, drops aria-hidden). Required because TelegramNewsCard relies on the icon as the SOLE visual indicator of the card's source — without a labeled aria-label the icon was invisible to assistive technology."
  - "Default-decorative behaviour preserved: callers that omit aria-label still get aria-hidden=true, so CarouselControls (which labels the wrapping button) is unaffected."
  - "Post tile background upgraded bg-white/5 -> bg-black/30 backdrop-blur-sm to maintain AA contrast against the new sky gradient mesh."
  - "Excerpt line-clamp raised from 2 -> 3 to balance the extra vertical space when no thumbnail is present (text-only posts stay visually full)."
metrics:
  metric_string: "UXΔ = +3 (Better) · CDI = 0.04 * 8 · MVQ = Sprite 84%/86%"
  duration_seconds: 1059
  completed_date: 2026-05-24
---

# Phase 06 Plan 06: TelegramNewsCard refactor — branded posts with thumbnails Summary

Gave TelegramNewsCard a clear Telegram identity (Telegram SVG mark, brand-blue gradient mesh, channel attribution) and surfaced real CDN-hosted post thumbnails by plumbing the existing `image_url` field through the Card surface — discovering in the process that the pre-implementation spike's "no image data in cache" finding was wrong.

## What shipped

### HSB-V11-TG-01 — backend `image_url` pass-through

- `spotlight.TelegramPost` gains `ImageURL string` with `json:"image_url,omitempty"`.
- `cards/telegram_news.go::buildCardFromItems` now maps `NewsItem.ImageURL -> TelegramPost.ImageURL`. The underlying parser (`internal/parser/telegram.Client.FetchNews`) has always extracted `background-image:url(...)` from `.tgme_widget_message_photo_wrap` — the spotlight Card surface was dropping the field.
- `types_test.go` asserts snake_case JSON tag (`"image_url":"..."`), round-trip preservation, and `omitempty` for empty strings.
- Two new resolver tests:
  - `TestTelegramNews_ImageURL_FlowsThrough` — fetch path, mixed posts (with-image + text-only + with-image) end up with the right `ImageURL` per `Link`.
  - `TestTelegramNews_ImageURL_CacheRoundTrip` — seeds the cache with raw `[]telegram.NewsItem` (the shape `handler/news.go` writes) and confirms `ImageURL` survives JSON unmarshal in the cache-hit path. Guards against a future cache-shape refactor accidentally dropping the field.

### HSB-V11-TG-02..04 — branded frontend card

`TelegramNewsCard.vue` rewritten:

- Single-root `<article class="relative w-full h-full overflow-hidden">`.
- First child: `<SpotlightBackdrop variant="gradient-mesh" accent="sky" />` — Telegram brand-blue dual-radial gradient mesh + shared right-edge vignette (provided by Phase 01 backdrop component).
- Foreground `<div class="relative z-10 ... p-4 md:p-6 lg:p-8">`:
  - **Header** — `<SpotlightIcon name="telegram" aria-label="Telegram">` + title `t('spotlight.telegramNews.title')` + right-aligned `<span>@anime_enigma</span>`.
  - **Posts grid** — `<div class="grid grid-cols-1 md:grid-cols-3 ...">` with per-post `<article class="bg-black/30 backdrop-blur-sm">` tiles. Each tile:
    - Optional `<img>` thumbnail (`v-if="post.image_url"`) — aspect-square, object-cover, lazy-loaded. Fallback alt = `post.title ?? ''`.
    - Optional `<h4>` post title (`v-if="post.title"`) — line-clamp-2.
    - `<p>` excerpt — line-clamp-3 (was line-clamp-2 in v1.0 — more breathing room now that thumbnails take vertical space).
    - Optional `<p>` date (`v-if="post.date"`).
    - Optional `<a class="cta-text" data-accent="sky" target="_blank" rel="noopener noreferrer">` "Open post →" CTA + inline `<SpotlightIcon name="play">`. T-03-18 pin held.

`TelegramNewsCard.spec.ts` extended from 8 to 17 assertions covering: single-root, post counts, backdrop presence, labeled telegram icon, channel attribution, thumbnail rendering (both present and absent), empty-alt fallback, T-03-18 rel pin, cta-text + sky accent, no-anchor when link absent, line-clamp-3, date, font-weight, responsive padding.

### Frontend TS type

`TelegramPost` interface gains `image_url?: string` so vue-tsc narrows the prop correctly across the dispatch chain.

## Deviations from plan

### [Rule 2 — Accessibility] SpotlightIcon labeled-mode (added during Task 4)

- **Found during:** Task 4 (spec implementation) — the `wrapper.find('svg[aria-label="Telegram"]')` assertion failed.
- **Root cause:** `SpotlightIcon.vue` set `inheritAttrs: false` and only forwarded `class` via `useAttrs()`. Each `<svg>` had a hardcoded `aria-hidden="true"`, so the caller's `aria-label="Telegram"` was discarded and the icon remained invisible to assistive technology.
- **Fix:** Added computed `forwardedAriaLabel`, `ariaHidden`, and `role` to the script-setup; rebound each of the 9 SVG branches to `:aria-hidden="ariaHidden" :aria-label="forwardedAriaLabel" :role="role"`. When caller supplies a non-empty `aria-label`, the SVG becomes `role="img"` with the label exposed and `aria-hidden` dropped. Default (no aria-label) keeps `aria-hidden="true"` so decorative usage (e.g. CarouselControls) is unaffected.
- **Why Rule 2 not Rule 1:** This is a missing accessibility primitive — the icon was the SOLE visual indicator of the card's source. Without screen-reader access to the "Telegram" label, AT users had no way to know what the icon represented. Required for correctness, not a "feature".
- **Files modified:** `SpotlightIcon.vue`, `SpotlightIcon.spec.ts` (2 new assertions for labeled vs decorative modes).
- **Commit:** `f6a15d0` (folded into the Task 3 commit since the card depends on the icon fix).

### Spike finding pivot — image data is REAL, not forward-compat

The orchestrator prompt's pre-implementation spike claimed `redis-cli GET news:telegram` returns `{ id, text, date, link, views }` with no `image_url`, and instructed me to ship Task 1 as a "struct-extension-only commit, inert until parser is updated".

I re-ran the spike directly and got the opposite result:

```bash
docker compose -f /data/animeenigma/docker/docker-compose.yml exec -T redis \
  redis-cli GET news:telegram | head -c 2000
# [{"id":"724","text":"...","date":"2026-05-23T17:03:13+00:00",
#   "link":"https://t.me/animeenigmanews/724","views":"19"},
#  {"id":"722","text":"...","image_url":"https://cdn4.telesco.pe/file/tzc24x91...",
#   ...}]
```

About 30% of @anime_enigma posts carry image URLs. The parser
(`internal/parser/telegram.Client.FetchNews`) was already extracting them
into `NewsItem.ImageURL`. The plan's "backend touch is a no-op" framing
was incorrect — this is a real feature shipping today, not forward-compat.

The card was implemented per the orchestrator's UI spec (thumbnails render
when present, gracefully omit when absent) so the deviation was zero-cost
to absorb. SUMMARY documents it here so a future reader doesn't mistake
`image_url` for vapor.

## Threat model holdouts

- **T-03-18** (reverse tabnabbing on external Telegram links): held — every `<a>` in the new template carries both `target="_blank"` and `rel="noopener noreferrer"`, asserted by the spec.
- **Image SSRF / XSS via image_url**: The backend never echoes user input — `image_url` originates from the Telegram channel HTML the parser scrapes, which is content WE control via the `@anime_enigma` channel. The `<img src="...">` element does not execute JavaScript even if a future attacker compromises the channel; worst case is a broken image. Browser CSP (already deployed) blocks `img-src` to non-https origins.

## Deferred Issues

See `deferred-items.md`. Three playwright failures (`spotlight.spec.ts` × 2, `spotlight-full.spec.ts` × 1) are pre-existing and reproduce on commit `87c75f8` (the parent of Plan 06's first commit). All three are page-level a11y / locator issues unrelated to the TelegramNewsCard refactor.

## Verification

- `cd services/catalog && go test ./internal/service/spotlight/... -count=1 -race` — **green** (cards: 1.053s; spotlight: 3.803s).
- `cd services/catalog && go build ./...` — clean.
- `cd frontend/web && bunx vitest run src/components/home/spotlight/ src/locales/__tests__/spotlight-keys.spec.ts` — **15 files / 222 tests green**.
- `bunx tsc --noEmit` — clean.
- `bunx eslint src/components/home/spotlight/ src/types/spotlight.ts` — clean.
- `bun run build` — clean production build.
- `bunx playwright test spotlight-full.spec.ts spotlight-transition-lock.spec.ts --project=chromium` — **8 passed**, 1 pre-existing axe failure (see Deferred Issues).

## Self-Check: PASSED

- `git log --oneline --all | grep -q "83cb865"` — FOUND
- `git log --oneline --all | grep -q "348197f"` — FOUND
- `git log --oneline --all | grep -q "f6a15d0"` — FOUND
- Backend changes present in `services/catalog/internal/service/spotlight/types.go` (TelegramPost.ImageURL field) and `cards/telegram_news.go` (NewsItem.ImageURL -> TelegramPost.ImageURL mapping).
- Frontend changes present in `TelegramNewsCard.vue` (single-root article, SpotlightBackdrop, SpotlightIcon labeled, thumbnail rendering, cta-text data-accent="sky").
- `deferred-items.md` created in the phase directory.
