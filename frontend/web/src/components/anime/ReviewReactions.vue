<script setup lang="ts">
/**
 * ReviewReactions — horizontal pill strip of the 12-emoji reaction palette for
 * a single review. Anyone can SEE counts; toggling requires login. Clicking a
 * pill optimistically bounces, calls the toggle endpoint, and reconciles from
 * the authoritative `{ added, counts }` response. AUTO-408 (admin @tNeymik).
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
}

const props = defineProps<{
  reviewId: string
  animeId: string
  initialReactions?: ReactionCount[]
  viewerReacted?: string[]
}>()

// The fixed 12-emoji palette + a stable name slug per emoji for the tooltip.
// Order is the canonical display order. MUST stay in sync with the backend
// domain.AllowedReactionEmojis set.
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

const { t } = useI18n()
const authStore = useAuthStore()
const toast = useToast()

// Local mutable state seeded from props. Keyed by emoji for O(1) updates.
const counts = ref<Record<string, number>>({})
const reacted = ref<Record<string, boolean>>({})
const bouncing = ref<Record<string, boolean>>({})
const pending = ref<Record<string, boolean>>({})

function seed(): void {
  const c: Record<string, number> = {}
  const r: Record<string, boolean> = {}
  for (const { emoji } of PALETTE) {
    c[emoji] = 0
    r[emoji] = false
  }
  for (const rc of props.initialReactions ?? []) {
    if (c[rc.emoji] !== undefined) {
      c[rc.emoji] = rc.count
      r[rc.emoji] = rc.reacted_by_me
    }
  }
  for (const emoji of props.viewerReacted ?? []) {
    if (r[emoji] !== undefined) r[emoji] = true
  }
  counts.value = c
  reacted.value = r
}
seed()

// Total reactions across the strip — used to decide whether to dim zero pills.
const total = computed(() =>
  Object.values(counts.value).reduce((sum, n) => sum + n, 0),
)

function tooltipFor(emoji: string, name: string): string {
  return t(`anime.reactions.names.${name}`, emoji)
}

async function toggle(emoji: string): Promise<void> {
  if (!authStore.isAuthenticated) {
    toast.push(t('anime.reactions.login_prompt'), 'info')
    return
  }
  if (pending.value[emoji]) return
  pending.value[emoji] = true

  // Optimistic bounce — reconciled by the server response below.
  bouncing.value[emoji] = true
  window.setTimeout(() => {
    bouncing.value[emoji] = false
  }, 350)

  try {
    const { data } = await reviewApi.toggleReaction(props.animeId, props.reviewId, emoji)
    const payload = (data?.data ?? data) as { added: boolean; counts: ReactionCount[] }
    // Reconcile this emoji from the authoritative counts. Reset to a clean
    // baseline for emojis present in the response; absent ones keep 0.
    const next: Record<string, number> = { ...counts.value }
    const nextReacted: Record<string, boolean> = { ...reacted.value }
    // Zero-out then re-apply so a removed last reaction collapses to 0.
    next[emoji] = 0
    nextReacted[emoji] = false
    for (const rc of payload.counts ?? []) {
      if (next[rc.emoji] !== undefined) {
        next[rc.emoji] = rc.count
        nextReacted[rc.emoji] = rc.reacted_by_me
      }
    }
    counts.value = next
    reacted.value = nextReacted
  } catch {
    toast.push(t('anime.reactions.error'), 'error')
  } finally {
    pending.value[emoji] = false
  }
}
</script>

<template>
  <div class="flex flex-wrap items-center gap-1.5" data-testid="review-reactions">
    <button
      v-for="{ emoji, name } in PALETTE"
      :key="emoji"
      type="button"
      class="group inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-sm font-medium transition-colors"
      :class="[
        reacted[emoji]
          ? 'border-cyan-400/60 bg-cyan-500/20 text-cyan-300'
          : 'border-white/10 bg-white/5 text-white/70 hover:border-white/20 hover:bg-white/10',
        total > 0 && counts[emoji] === 0 ? 'opacity-60' : '',
        bouncing[emoji] ? 'reaction-bounce' : '',
      ]"
      :title="tooltipFor(emoji, name)"
      :aria-label="tooltipFor(emoji, name)"
      :aria-pressed="reacted[emoji]"
      :disabled="pending[emoji]"
      @click="toggle(emoji)"
    >
      <span class="leading-none">{{ emoji }}</span>
      <span v-if="counts[emoji] > 0" class="tabular-nums text-xs leading-none">{{ counts[emoji] }}</span>
    </button>
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
