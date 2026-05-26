<template>
  <article
    class="relative w-full h-full overflow-hidden"
  >
    <SpotlightBackdrop variant="gradient-mesh" accent="sky" />
    <div
      class="relative z-10 w-full h-full flex flex-col gap-3 p-4 md:p-6 lg:p-8"
    >
      <header class="flex items-center gap-3">
        <SpotlightIcon
          name="telegram"
          class="w-6 h-6 text-sky-300 flex-shrink-0"
          aria-label="Telegram"
        />
        <h3
          class="text-lg md:text-xl font-semibold text-white"
        >
          {{ t('spotlight.telegramNews.title') }}
        </h3>
        <span
          class="ml-auto text-xs font-medium text-sky-200 truncate"
        >@anime_enigma</span>
      </header>
      <div
        class="flex-1 grid grid-cols-1 md:grid-cols-3 gap-3 md:gap-4 min-h-0"
      >
        <article
          v-for="(post, i) in data.posts.slice(0, 3)"
          :key="post.link ?? `tg-${i}`"
          class="gap-2 p-3 rounded-xl bg-black/30 backdrop-blur-sm hover:bg-black/40 transition min-w-0"
          :class="i === 0 ? 'flex flex-col' : 'hidden md:flex md:flex-col'"
        >
          <!--
            Optional thumbnail. The Telegram scraper extracts background-
            image URLs from .tgme_widget_message_photo_wrap (see
            services/catalog/internal/parser/telegram.NewsItem.ImageURL).
            Roughly 30% of @anime_enigma posts carry a CDN URL. When
            absent, the rest of the card collapses cleanly (title +
            excerpt + date + CTA fill the space — no awkward empty box).
          -->
          <div
            v-if="post.image_url"
            class="relative h-24 md:h-28 overflow-hidden rounded-lg bg-white/5 flex-shrink-0"
          >
            <img
              :src="post.image_url"
              :alt="post.title ?? ''"
              class="w-full h-full object-cover"
              loading="lazy"
            />
          </div>
          <h4
            v-if="post.title"
            class="text-sm font-semibold text-white line-clamp-2"
          >
            {{ post.title }}
          </h4>
          <p
            class="text-xs font-medium text-gray-300 line-clamp-3 flex-1"
          >
            {{ post.excerpt }}
          </p>
          <p
            v-if="post.date"
            class="text-[10px] font-medium text-sky-300/70"
          >
            {{ post.date }}
          </p>
          <a
            v-if="post.link"
            :href="post.link"
            target="_blank"
            rel="noopener noreferrer"
            data-accent="sky"
            class="cta-text mt-auto"
          >
            {{ t('spotlight.telegramNews.openCta') }}
            <SpotlightIcon name="play" class="w-3 h-3" />
          </a>
        </article>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
/**
 * Workstream hero-spotlight — v1.1-polish Phase 06 (HSB-V11-TG-01..04).
 *
 * Branded TelegramNewsCard. Phase 03's prior implementation rendered a
 * bare heading + 3 plain text excerpts with no Telegram identity. The
 * v1.1-polish refactor:
 *
 *  - Wraps the card in <SpotlightBackdrop variant="gradient-mesh"
 *    accent="sky"> — Telegram brand-blue radial mesh + shared right-edge
 *    vignette (preserves AA contrast for foreground text).
 *  - Header carries the inline Telegram SVG (SpotlightIcon name="telegram"),
 *    the existing title key, and a right-aligned @anime_enigma channel
 *    attribution string. The literal handle is hardcoded — the project
 *    decision (06-CONTEXT §decisions) is that surfacing a live subscriber
 *    count is not worth a new Telegram API call for v1.1.
 *  - Each post now renders an optional thumbnail (`v-if="post.image_url"`)
 *    above the title. The image is `aspect-square object-cover` so the
 *    card stays balanced when 0/3 vs 3/3 posts have images.
 *  - The "Open post →" anchor uses `.cta-text data-accent="sky"` so it
 *    inherits the Phase 01 sky-accent hover state. T-03-18 pin
 *    (`rel="noopener noreferrer"` on every external anchor) held.
 *
 * SINGLE-ROOT <article> — Vue 3 <Transition mode="out-in"> in
 * HeroSpotlightBlock.vue silently wedges if a card has a top-level v-if,
 * multiple root elements, or leading template comments. The pattern
 * matches PersonalPickCard.vue / NowWatchingCard.vue.
 *
 * NOTE: this template intentionally keeps backdrop + foreground inside
 * the same <article> root rather than splitting into a wrapper <div>.
 * The block-level <article> is the semantic carrier of the slide; the
 * backdrop is decorative and aria-hidden (provided by SpotlightBackdrop).
 */
import { useI18n } from 'vue-i18n'
import type { TelegramNewsData } from '@/types/spotlight'
import SpotlightBackdrop from '../SpotlightBackdrop.vue'
import SpotlightIcon from '../SpotlightIcon.vue'

defineProps<{ data: TelegramNewsData }>()
const { t } = useI18n()
</script>
