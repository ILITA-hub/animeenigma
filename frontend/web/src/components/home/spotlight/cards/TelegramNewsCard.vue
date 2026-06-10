<template>
  <SpotlightCardShell
    accent="violet"
    icon="telegram"
    :kicker="t('spotlight.telegramNews.title')"
  >
    <template #kicker-extra>
      <span class="opacity-60 normal-case tracking-normal">· @anime_enigma</span>
    </template>

    <div
      class="flex-1 grid grid-cols-1 md:grid-cols-3 gap-3 md:gap-4 min-h-0"
    >
      <article
        v-for="(post, i) in data.posts.slice(0, 3)"
        :key="post.link ?? `tg-${i}`"
        data-testid="tg-post-tile"
        class="gap-2 p-3 rounded-xl bg-white/5 border border-white/10 hover:bg-white/10 backdrop-blur-sm transition min-w-0"
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
            decoding="async"
          />
        </div>
        <h4
          v-if="post.title"
          class="text-sm font-semibold text-white line-clamp-2"
        >
          {{ post.title }}
        </h4>
        <p
          class="text-xs font-medium text-white/70 line-clamp-3 flex-1"
        >
          {{ post.excerpt }}
        </p>
        <p
          v-if="post.date"
          class="text-[10px] font-medium text-brand-violet/80"
        >
          {{ post.date }}
        </p>
        <a
          v-if="post.link"
          :href="post.link"
          target="_blank"
          rel="noopener noreferrer"
          :class="[buttonVariants({ variant: 'link', size: 'xs' }), 'mt-auto self-start text-xs']"
        >
          {{ t('spotlight.telegramNews.openCta') }}
          <ExternalLink class="w-3 h-3" aria-hidden="true" />
        </a>
      </article>
    </div>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
/**
 * Workstream hero-spotlight — v1.1-polish Phase 06 (HSB-V11-TG-01..04);
 * DS alignment 2026-06-10 (SpotlightCardShell + Button-variant link CTA).
 *
 * Branded TelegramNewsCard:
 *  - SpotlightCardShell kicker (violet, telegram icon) + the @anime_enigma
 *    channel attribution as kicker-extra. The literal handle is hardcoded —
 *    the project decision (06-CONTEXT §decisions) is that surfacing a live
 *    subscriber count is not worth a new Telegram API call.
 *  - Each post renders an optional thumbnail (`v-if="post.image_url"`)
 *    above the title; when absent the tile collapses cleanly.
 *  - The "Open post →" anchor is the Button `link` variant; T-03-18 pin
 *    (`rel="noopener noreferrer"` on every external anchor) held.
 *
 * SINGLE-ROOT — SpotlightCardShell's <article> is the only root node
 * (Transition mode="out-in" safety).
 */
import { useI18n } from 'vue-i18n'
import { ExternalLink } from 'lucide-vue-next'
import type { TelegramNewsData } from '@/types/spotlight'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'

defineProps<{ data: TelegramNewsData }>()
const { t } = useI18n()
</script>
