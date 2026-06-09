<script setup lang="ts">
/**
 * ReviewReactions — Discord/Telegram-style reactions for a single review.
 *
 * - Only emojis that HAVE reactions render as pills (emoji + count); the full
 *   12-emoji palette lives behind a "＋" add button (popover picker).
 * - ONE reaction per person: picking a new emoji replaces your prior one;
 *   clicking your current emoji removes it. (Server-enforced; mirrored here.)
 * - Hover / focus a pill to see WHO reacted (the System auto-👍 shows as
 *   «AnimeEnigma»).
 * - You cannot react to your OWN review — the picker is hidden there.
 *
 * State is reconciled from the authoritative `{ added, counts }` response.
 * AUTO-408 (admin @tNeymik).
 */
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { reviewApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useToast } from '@/composables/useToast'

interface ReactionCount {
  emoji: string
  count: number
  reacted_by_me: boolean
  users?: string[]
}

const props = defineProps<{
  reviewId: string
  animeId: string
  initialReactions?: ReactionCount[]
  viewerReacted?: string[]
  isOwnReview?: boolean
}>()

// Canonical left-to-right palette + a name slug per emoji (tooltip i18n key).
// MUST stay in sync with backend domain.AllowedReactionEmojis.
const PALETTE = [
  { emoji: '👍', name: 'like' },
  { emoji: '❤️', name: 'love' },
  { emoji: '🫠', name: 'melting' },
  { emoji: '🤮', name: 'sick' },
  { emoji: '🤧', name: 'sneeze' },
  { emoji: '🤯', name: 'mindblown' },
  { emoji: '🥴', name: 'woozy' },
  { emoji: '😈', name: 'devil' },
  { emoji: '🤡', name: 'clown' },
  { emoji: '🤩', name: 'starstruck' },
  { emoji: '😏', name: 'smirk' },
  { emoji: '🥰', name: 'adore' },
] as const

// Display name of the System reactor (auto-👍 on admin reviews). Matches
// backend domain.SystemReactionUsername.
const SYSTEM_NAME = 'AnimeEnigma'

const { t } = useI18n()
const authStore = useAuthStore()
const toast = useToast()

// Live reaction list — only emojis with count > 0, in server display order.
const reactions = ref<ReactionCount[]>([])
const pickerOpen = ref(false)
const whoFor = ref<string | null>(null)
const bouncing = ref<string | null>(null)
const pending = ref(false)

function seed(): void {
  const list = (props.initialReactions ?? [])
    .filter((r) => r.count > 0)
    .map((r) => ({ ...r, users: r.users ?? [] }))
  // Honor an explicit viewerReacted hint (the viewer's reacted emoji subset).
  const mine = new Set(props.viewerReacted ?? [])
  for (const r of list) if (mine.has(r.emoji)) r.reacted_by_me = true
  reactions.value = list
}
seed()

// The single emoji the viewer reacted with (one-per-person → at most one).
const myEmoji = computed(() => reactions.value.find((r) => r.reacted_by_me)?.emoji ?? null)

const nameFor = (emoji: string): string => {
  const p = PALETTE.find((x) => x.emoji === emoji)
  return p ? t(`anime.reactions.names.${p.name}`, emoji) : emoji
}

// Native-tooltip fallback (mobile has no hover): "Like · alice, AnimeEnigma".
function whoTitle(r: ReactionCount): string {
  const who = (r.users ?? []).join(', ')
  return who ? `${nameFor(r.emoji)} · ${who}` : nameFor(r.emoji)
}

function togglePicker(): void {
  if (!authStore.isAuthenticated) {
    toast.push(t('anime.reactions.login_prompt'), 'info')
    return
  }
  pickerOpen.value = !pickerOpen.value
}

function onPillClick(emoji: string): void {
  if (props.isOwnReview) return
  void toggle(emoji)
}

function pick(emoji: string): void {
  pickerOpen.value = false
  void toggle(emoji)
}

async function toggle(emoji: string): Promise<void> {
  if (props.isOwnReview) return
  if (!authStore.isAuthenticated) {
    toast.push(t('anime.reactions.login_prompt'), 'info')
    return
  }
  if (pending.value) return
  pending.value = true

  bouncing.value = emoji
  window.setTimeout(() => {
    if (bouncing.value === emoji) bouncing.value = null
  }, 350)

  try {
    const { data } = await reviewApi.toggleReaction(props.animeId, props.reviewId, emoji)
    const payload = (data?.data ?? data) as { added: boolean; counts: ReactionCount[] }
    // Authoritative reconcile: keep only emojis that still have reactions.
    reactions.value = (payload.counts ?? [])
      .filter((r) => r.count > 0)
      .map((r) => ({ ...r, users: r.users ?? [] }))
  } catch {
    toast.push(t('anime.reactions.error'), 'error')
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <div
    class="relative flex flex-wrap items-center gap-1.5"
    data-testid="review-reactions"
    @keydown.esc="pickerOpen = false"
  >
    <!-- Existing reactions (only emojis with count > 0) -->
    <div
      v-for="r in reactions"
      :key="r.emoji"
      class="relative"
      @mouseenter="whoFor = r.emoji"
      @mouseleave="whoFor = null"
    >
      <button
        type="button"
        class="group inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-sm font-medium transition-colors"
        :class="[
          r.reacted_by_me
            ? 'border-cyan-400/60 bg-cyan-500/20 text-cyan-300'
            : 'border-white/10 bg-white/5 text-white/70 hover:border-white/20 hover:bg-white/10',
          bouncing === r.emoji ? 'reaction-bounce' : '',
          isOwnReview ? 'cursor-default' : 'cursor-pointer',
        ]"
        :title="whoTitle(r)"
        :aria-label="whoTitle(r)"
        :aria-pressed="r.reacted_by_me"
        :disabled="pending || isOwnReview"
        @focus="whoFor = r.emoji"
        @blur="whoFor = null"
        @click="onPillClick(r.emoji)"
      >
        <span class="leading-none">{{ r.emoji }}</span>
        <span class="tabular-nums text-xs leading-none">{{ r.count }}</span>
      </button>

      <!-- Who-reacted popover -->
      <div
        v-if="whoFor === r.emoji && (r.users?.length ?? 0) > 0"
        class="absolute bottom-full left-0 z-50 mb-1 min-w-28 max-w-48 rounded-lg border border-white/10 bg-black/85 px-2.5 py-1.5 shadow-lg backdrop-blur-sm"
        role="tooltip"
      >
        <div class="mb-0.5 flex items-center gap-1 text-[10px] uppercase tracking-wide text-white/40">
          <span>{{ r.emoji }}</span><span>{{ nameFor(r.emoji) }}</span>
        </div>
        <div
          v-for="u in r.users"
          :key="u"
          class="truncate text-xs leading-snug"
          :class="u === SYSTEM_NAME ? 'font-semibold text-cyan-300' : 'text-white/80'"
        >
          {{ u }}
        </div>
      </div>
    </div>

    <!-- Add-reaction button (hidden on your own review) -->
    <button
      v-if="!isOwnReview"
      type="button"
      class="inline-flex h-6 w-6 items-center justify-center rounded-full border border-white/10 bg-white/5 text-white/60 transition-colors hover:border-white/20 hover:bg-white/10 hover:text-white/90"
      :class="pickerOpen ? 'border-cyan-400/60 text-cyan-300' : ''"
      :aria-label="t('anime.reactions.add')"
      :title="t('anime.reactions.add')"
      :aria-expanded="pickerOpen"
      @click="togglePicker"
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-4 w-4">
        <path d="M22 11v1a10 10 0 1 1-9-10" />
        <path d="M8 14s1.5 2 4 2 4-2 4-2" />
        <line x1="9" x2="9.01" y1="9" y2="9" />
        <line x1="15" x2="15.01" y1="9" y2="9" />
        <path d="M16 5h6" />
        <path d="M19 2v6" />
      </svg>
    </button>

    <!-- Palette picker popover -->
    <template v-if="pickerOpen">
      <button
        type="button"
        class="fixed inset-0 z-40 cursor-default"
        :aria-label="t('anime.reactions.add')"
        @click="pickerOpen = false"
      />
      <div
        class="absolute left-0 top-full z-50 mt-1 flex max-w-[15rem] flex-wrap gap-1 rounded-xl border border-white/10 bg-black/85 p-2 shadow-xl backdrop-blur-sm"
        role="menu"
      >
        <button
          v-for="p in PALETTE"
          :key="p.emoji"
          type="button"
          class="flex h-8 w-8 items-center justify-center rounded-lg text-lg transition-colors hover:bg-white/10"
          :class="myEmoji === p.emoji ? 'bg-cyan-500/20 ring-1 ring-cyan-400/60' : ''"
          :title="nameFor(p.emoji)"
          :aria-label="nameFor(p.emoji)"
          :disabled="pending"
          @click="pick(p.emoji)"
        >
          {{ p.emoji }}
        </button>
      </div>
    </template>
  </div>
</template>

<style scoped>
@keyframes reaction-bounce {
  0% { transform: scale(1); }
  40% { transform: scale(1.3); }
  70% { transform: scale(0.92); }
  100% { transform: scale(1); }
}
.reaction-bounce {
  animation: reaction-bounce 0.35s ease-in-out;
}
</style>
