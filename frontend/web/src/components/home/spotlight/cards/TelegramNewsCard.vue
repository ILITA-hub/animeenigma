<template>
  <SpotlightCardShell
    accent="violet"
    icon="telegram"
    :kicker="t('spotlight.telegramNews.title')"
  >
  <!--
    Workstream hero-spotlight — v4 D-4 lock (2026-06-11, spec:
    2026-06-11-spotlight-v3-controls-and-cards.md). Hero post + chat tail:
    the latest post renders big on the left (photo with an overlay date
    badge — or a ✈ vignette when the post has no photo), the next two
    posts render as Telegram-style chat bubbles (SpotlightChatBubble) on
    the right, capped with a ghost «Подписаться на канал» CTA. Mobile
    stacks hero-then-bubbles.
  -->
    <template #kicker-extra>
      <span class="opacity-60 normal-case tracking-normal">· @anime_enigma</span>
    </template>

    <div class="flex-1 min-h-0 grid md:grid-cols-[11fr_9fr] gap-4 md:gap-6">
      <!-- ── Hero post (latest) ─────────────────────────────────────── -->
      <SpotlightTile
        v-if="hero"
        as="article"
        class="flex flex-col overflow-hidden min-h-0"
        data-testid="tg-hero-post"
      >
        <div class="relative flex-1 min-h-[88px] overflow-hidden bg-white/5">
          <!-- DS shimmer until the post photo decodes (2026-06-11). -->
          <div
            v-if="hero.image_url && !heroImgLoaded"
            class="absolute inset-0 skeleton-shimmer"
            aria-hidden="true"
          />
          <img
            v-if="hero.image_url"
            :src="hero.image_url"
            :alt="hero.title ?? ''"
            class="absolute inset-0 w-full h-full object-cover transition-opacity duration-300"
            :class="heroImgLoaded ? 'opacity-100' : 'opacity-0'"
            decoding="async"
            @load="onHeroImgLoad"
            @error="onHeroImgLoad"
          />
          <div
            v-else
            class="absolute inset-0 grid place-items-center"
            aria-hidden="true"
          >
            <Send class="w-9 h-9 text-white/20" />
          </div>
          <Badge v-if="heroDate" size="sm" overlay class="absolute left-2.5 bottom-2.5">
            {{ heroDate }}
          </Badge>
        </div>
        <div class="p-3.5">
          <h4 v-if="hero.title" class="text-sm font-semibold text-white line-clamp-1">
            {{ hero.title }}
          </h4>
          <p class="text-xs font-medium text-white/70 line-clamp-2 mt-1">
            {{ hero.excerpt }}
          </p>
          <a
            v-if="hero.link"
            :href="hero.link"
            target="_blank"
            rel="noopener noreferrer"
            :class="[buttonVariants({ variant: 'link', size: 'xs' }), 'mt-2 text-xs']"
          >
            {{ t('spotlight.telegramNews.openCta') }}
            <ExternalLink class="w-3 h-3" aria-hidden="true" />
          </a>
        </div>
      </SpotlightTile>

      <!-- ── Older posts as chat bubbles + subscribe ─────────────────── -->
      <div class="flex flex-col gap-3 justify-center min-w-0">
        <SpotlightChatBubble
          v-for="(post, i) in tail"
          :key="post.link ?? `tg-tail-${i}`"
          :time="formatPostDate(post.date)"
          data-testid="tg-post-tile"
        >
          <component
            :is="post.link ? 'a' : 'p'"
            v-bind="post.link
              ? { href: post.link, target: '_blank', rel: 'noopener noreferrer' }
              : {}"
            class="text-[13px] text-white line-clamp-2 hover:text-white/80 transition-colors"
          >
            {{ post.title || post.excerpt }}
          </component>
        </SpotlightChatBubble>

        <a
          href="https://t.me/anime_enigma"
          target="_blank"
          rel="noopener noreferrer"
          :class="[buttonVariants({ variant: 'ghost', size: 'sm' }), 'self-start text-[13px]']"
        >
          <Send class="w-3.5 h-3.5" aria-hidden="true" />
          {{ t('spotlight.telegramNews.subscribeCta') }}
        </a>
      </div>
    </div>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ExternalLink, Send } from 'lucide-vue-next'
import type { TelegramNewsData } from '@/types/spotlight'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import { isImageWarm, markImageWarm } from '@/utils/preload-image'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import SpotlightTile from '../ui/SpotlightTile.vue'
import SpotlightChatBubble from '../ui/SpotlightChatBubble.vue'

const props = defineProps<{ data: TelegramNewsData }>()
const { t, locale: i18nLocale } = useI18n()

const localeStr = computed<string>(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

// Hero = newest post; tail = the next two as chat bubbles.
const hero = computed(() => props.data.posts[0])
// Warm-skip (session registry): re-mounting the slide must not replay the
// shimmer over an HTTP-cache hit.
const heroImgLoaded = ref(isImageWarm(props.data.posts[0]?.image_url ?? ''))
function onHeroImgLoad(): void {
  heroImgLoaded.value = true
  markImageWarm(hero.value?.image_url ?? '')
}
const tail = computed(() => props.data.posts.slice(1, 3))

const heroDate = computed(() => (hero.value ? formatPostDate(hero.value.date) : ''))

// Telegram scraper dates arrive as loose strings ("2026-06-11T09:48:00Z"
// or pre-formatted text). Parseable → locale-aware relative/short form;
// unparseable → raw string passthrough (defensive).
function formatPostDate(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const diffDays = Math.round((d.getTime() - Date.now()) / 86_400_000)
  try {
    if (Math.abs(diffDays) < 7) {
      return new Intl.RelativeTimeFormat(localeStr.value, { numeric: 'auto' }).format(
        diffDays,
        'day',
      )
    }
    return new Intl.DateTimeFormat(localeStr.value, { day: 'numeric', month: 'short' }).format(d)
  } catch {
    return iso
  }
}
</script>
