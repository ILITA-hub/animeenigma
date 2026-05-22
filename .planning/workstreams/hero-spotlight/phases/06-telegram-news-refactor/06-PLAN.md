---
phase: 06-telegram-news-refactor
plan: 06
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-TG-01, HSB-V11-TG-02, HSB-V11-TG-03, HSB-V11-TG-04]
blocked_by: [01]
status: ready
---

# Plan 06: TelegramNewsCard refactor — branded posts with thumbnails

## Goal

Give TelegramNewsCard a clear Telegram identity (logo + brand-blue accent)
and surface post thumbnails when available. Backend touch: a small
pass-through change so the existing `news:telegram` cache delivers any
image URL it already holds.

## Tasks

### Task 1 — Backend pass-through for image URL

`services/catalog/internal/service/spotlight/cards/telegram_news.go`:

Inspect the existing `news:telegram` Redis payload (already 11520 bytes per
Phase 03 verification). If it contains a per-post `image_url` field, add it
to the `TelegramPost` struct and surface in `TelegramNewsData`.

```go
type TelegramPost struct {
  Title    *string `json:"title,omitempty"`
  Excerpt  string  `json:"excerpt"`
  Date     *string `json:"date,omitempty"`
  Link     *string `json:"link,omitempty"`
  ImageURL *string `json:"image_url,omitempty"`  // NEW
}
```

Add a parser test that decodes a fixture cache payload with images and
confirms ImageURL flows through.

> **Pre-implementation spike (5 min):** dump
> `redis-cli GET news:telegram | jq '.[0] | keys'` to confirm which fields
> exist before committing the struct shape. Pivot to "no thumbnails
> available, ship without" if the field isn't present.

### Task 2 — Frontend Telegram branding + thumbnails

```vue
<article class="relative w-full h-full overflow-hidden">
  <SpotlightBackdrop variant="gradient-mesh" accent="sky" />
  <div class="relative z-10 w-full h-full flex flex-col gap-3 p-4 md:p-6 lg:p-8">
    <header class="flex items-center gap-3">
      <SpotlightIcon name="telegram" class="w-6 h-6 text-sky-300" aria-label="Telegram" />
      <h3 class="text-lg md:text-xl font-semibold text-white">
        {{ t('spotlight.telegramNews.title') }}
      </h3>
      <span class="ml-auto text-xs font-medium text-sky-200">@anime_enigma</span>
    </header>
    <div class="flex-1 grid grid-cols-1 md:grid-cols-3 gap-3 md:gap-4 min-h-0">
      <article
        v-for="(post, i) in data.posts.slice(0, 3)"
        :key="post.link ?? `tg-${i}`"
        class="flex flex-col gap-2 p-3 rounded-xl bg-black/30 backdrop-blur-sm hover:bg-black/40 transition min-w-0"
      >
        <!-- Optional thumbnail -->
        <div
          v-if="post.image_url"
          class="relative aspect-square overflow-hidden rounded-lg bg-white/5"
        >
          <img
            :src="post.image_url"
            :alt="post.title ?? ''"
            class="w-full h-full object-cover"
            loading="lazy"
          />
        </div>
        <h4 v-if="post.title" class="text-sm font-semibold text-white line-clamp-2">{{ post.title }}</h4>
        <p class="text-xs font-medium text-gray-300 line-clamp-3 flex-1">{{ post.excerpt }}</p>
        <p v-if="post.date" class="text-[10px] font-medium text-sky-300/70">{{ post.date }}</p>
        <a
          v-if="post.link"
          :href="post.link"
          target="_blank"
          rel="noopener noreferrer"
          class="cta-text mt-auto"
        >
          {{ t('spotlight.telegramNews.openCta') }}
          <SpotlightIcon name="play" class="w-3 h-3" />
        </a>
      </article>
    </div>
  </div>
</article>
```

### Task 3 — Inline Telegram SVG in SpotlightIcon.vue

Already added in Plan 01 — this phase just consumes it.

### Task 4 — Spec updates

`TelegramNewsCard.spec.ts`:

- `<SpotlightIcon name="telegram">` rendered in header with `aria-label="Telegram"`.
- When `post.image_url` present → thumbnail `<img>` rendered.
- When absent → no thumbnail; layout collapses gracefully.
- Channel attribution string `@anime_enigma` present.
- External `<a>` has `target="_blank" rel="noopener noreferrer"` (T-03-18 pin held).
- Backdrop uses `variant="gradient-mesh"` `accent="sky"`.

Backend test:

`services/catalog/internal/service/spotlight/cards/telegram_news_test.go`
gains a fixture cache payload with an image URL and asserts the resolver
emits `ImageURL` in the response.

## Verification

- Backend: `cd services/catalog && go test ./internal/service/spotlight/cards/... -count=1 -race` — green.
- Frontend: `bunx vitest run src/components/home/spotlight/cards/TelegramNewsCard.spec.ts` — green.
- `bunx tsc --noEmit` — clean.
- `bunx playwright test spotlight-full` — green.
- Manual: load `/`, cycle to TelegramNews, confirm Telegram logo + (thumbnail
  if cache has image_url) renders.

## Metrics

`UXΔ = +3 (Better) · CDI = 0.04 * 8 · MVQ = Sprite 84%/86%`
