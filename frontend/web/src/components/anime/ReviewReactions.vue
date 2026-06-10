<script setup lang="ts">
/**
 * ReviewReactions — Discord/Telegram-style reactions for a single review.
 *
 * - Only emojis that HAVE reactions render as pills (emoji + count); the full
 *   12-emoji palette lives behind a "＋" add button (popover picker).
 * - ONE reaction per person: picking a new emoji replaces your prior one;
 *   clicking your current emoji removes it. (Server-enforced; mirrored here.)
 *   ADMINS may stack multiple reactions — each emoji toggles independently.
 * - Hover / focus a pill to see WHO reacted (the System auto-👍 shows as
 *   «AnimeEnigma»). Admins additionally get an × per reactor to remove that
 *   user's reaction (moderation).
 * - You cannot react to your OWN review — the picker is hidden there.
 *
 * Both popovers are Teleported to <body> with fixed positioning: each review
 * card is a .glass-card (backdrop-filter ⇒ its own stacking context), so an
 * absolute z-50 popover inside card N paints UNDER the next sibling card. A
 * window scroll/resize closes them rather than tracking the anchor.
 *
 * State is reconciled from the authoritative `{ added, counts }` response.
 * AUTO-408 (admin @tNeymik).
 */
import { ref, computed, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { SmilePlus, X } from 'lucide-vue-next'
import { reviewApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useToast } from '@/composables/useToast'

interface Reactor {
  user_id: string
  username: string
}

interface ReactionCount {
  emoji: string
  count: number
  reacted_by_me: boolean
  users?: string[]
  reactors?: Reactor[]
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

// Approximate popover footprint used for viewport-edge clamping/flipping.
const PICKER_W = 240
const PICKER_H = 140

const { t } = useI18n()
const authStore = useAuthStore()
const toast = useToast()

const isAdmin = computed(() => authStore.isAdmin === true)

// Live reaction list — only emojis with count > 0, in server display order.
const reactions = ref<ReactionCount[]>([])
const pickerOpen = ref(false)
const pickerStyle = ref<Record<string, string>>({})
const whoFor = ref<string | null>(null)
const whoStyle = ref<Record<string, string>>({})
const bouncing = ref<string | null>(null)
const pending = ref(false)

let whoCloseTimer: number | null = null

function seed(): void {
  const list = (props.initialReactions ?? [])
    .filter((r) => r.count > 0)
    .map((r) => ({ ...r, users: r.users ?? [], reactors: r.reactors ?? [] }))
  // Honor an explicit viewerReacted hint (the viewer's reacted emoji subset).
  const mine = new Set(props.viewerReacted ?? [])
  for (const r of list) if (mine.has(r.emoji)) r.reacted_by_me = true
  reactions.value = list
}
seed()

// All emojis the viewer reacted with (admins can hold several at once).
const myEmojis = computed(() => new Set(reactions.value.filter((r) => r.reacted_by_me).map((r) => r.emoji)))

// The reaction whose who-reacted popover is currently shown.
const whoReaction = computed(() => reactions.value.find((r) => r.emoji === whoFor.value) ?? null)

// Reactor rows for the popover — prefer the id-carrying `reactors`, fall back
// to the legacy name-only `users` list (no admin × without an id).
const whoRows = computed<Reactor[]>(() => {
  const r = whoReaction.value
  if (!r) return []
  if (r.reactors && r.reactors.length > 0) return r.reactors
  return (r.users ?? []).map((u) => ({ user_id: '', username: u }))
})

const nameFor = (emoji: string): string => {
  const p = PALETTE.find((x) => x.emoji === emoji)
  return p ? t(`anime.reactions.names.${p.name}`, emoji) : emoji
}

// Native-tooltip fallback (mobile has no hover): "Like · alice, AnimeEnigma".
function whoTitle(r: ReactionCount): string {
  const who = (r.users ?? []).join(', ')
  return who ? `${nameFor(r.emoji)} · ${who}` : nameFor(r.emoji)
}

// --- Teleported-popover positioning (fixed, viewport coords) ---------------

function anchorRect(ev: Event): DOMRect | null {
  const el = ev.currentTarget as HTMLElement | null
  return el ? el.getBoundingClientRect() : null
}

function openWho(emoji: string, ev: Event): void {
  if (whoCloseTimer !== null) {
    window.clearTimeout(whoCloseTimer)
    whoCloseTimer = null
  }
  const rect = anchorRect(ev)
  if (rect) {
    const left = Math.max(8, Math.min(rect.left, window.innerWidth - 200))
    // Open above the pill; flip below when the pill sits near the viewport
    // top (e.g. scrolled right under the fixed navbar) so the popover never
    // lands off-screen.
    whoStyle.value =
      rect.top < 140
        ? { left: `${left}px`, top: `${rect.bottom + 6}px` }
        : { left: `${left}px`, top: `${rect.top - 6}px`, transform: 'translateY(-100%)' }
  }
  whoFor.value = emoji
}

// Grace delay so the cursor can travel from the pill into the popover (the
// popover is teleported — it is NOT a DOM child of the pill anymore).
function scheduleCloseWho(): void {
  if (whoCloseTimer !== null) window.clearTimeout(whoCloseTimer)
  whoCloseTimer = window.setTimeout(() => {
    whoFor.value = null
    whoCloseTimer = null
  }, 150)
}

function cancelCloseWho(): void {
  if (whoCloseTimer !== null) {
    window.clearTimeout(whoCloseTimer)
    whoCloseTimer = null
  }
}

function togglePicker(ev: MouseEvent): void {
  if (!authStore.isAuthenticated) {
    toast.push(t('anime.reactions.login_prompt'), 'info')
    return
  }
  if (!pickerOpen.value) {
    const rect = anchorRect(ev)
    if (rect) {
      const left = Math.max(8, Math.min(rect.left, window.innerWidth - PICKER_W - 8))
      const openUp = rect.bottom + PICKER_H + 8 > window.innerHeight
      pickerStyle.value = openUp
        ? { left: `${left}px`, top: `${rect.top - 4}px`, transform: 'translateY(-100%)' }
        : { left: `${left}px`, top: `${rect.bottom + 4}px` }
    }
  }
  pickerOpen.value = !pickerOpen.value
}

// Fixed-positioned popovers don't follow their anchor — close them on any
// scroll/resize instead of drifting.
function closeAllPopovers(): void {
  pickerOpen.value = false
  whoFor.value = null
}

watch([pickerOpen, whoFor], ([picker, who]) => {
  const anyOpen = picker || who !== null
  if (anyOpen) {
    window.addEventListener('scroll', closeAllPopovers, { capture: true, passive: true })
    window.addEventListener('resize', closeAllPopovers)
  } else {
    window.removeEventListener('scroll', closeAllPopovers, { capture: true })
    window.removeEventListener('resize', closeAllPopovers)
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', closeAllPopovers, { capture: true })
  window.removeEventListener('resize', closeAllPopovers)
  if (whoCloseTimer !== null) window.clearTimeout(whoCloseTimer)
})

// --- Actions ----------------------------------------------------------------

function onPillClick(emoji: string): void {
  if (props.isOwnReview) return
  void toggle(emoji)
}

function pick(emoji: string): void {
  pickerOpen.value = false
  void toggle(emoji)
}

function reconcile(counts: ReactionCount[] | undefined): void {
  reactions.value = (counts ?? [])
    .filter((r) => r.count > 0)
    .map((r) => ({ ...r, users: r.users ?? [], reactors: r.reactors ?? [] }))
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
    reconcile(payload.counts)
  } catch {
    toast.push(t('anime.reactions.error'), 'error')
  } finally {
    pending.value = false
  }
}

// Admin moderation: remove one user's reaction from the who-reacted popover.
async function adminRemove(emoji: string, reactor: Reactor): Promise<void> {
  if (!isAdmin.value || !reactor.user_id) return
  if (pending.value) return
  pending.value = true
  try {
    const { data } = await reviewApi.adminRemoveReaction(
      props.animeId,
      props.reviewId,
      emoji,
      reactor.user_id,
    )
    const payload = (data?.data ?? data) as { counts: ReactionCount[] }
    reconcile(payload.counts)
    if (!reactions.value.some((r) => r.emoji === whoFor.value)) whoFor.value = null
  } catch {
    toast.push(t('anime.reactions.error'), 'error')
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <div
    class="flex flex-wrap items-center gap-1.5"
    data-testid="review-reactions"
    @keydown.esc="closeAllPopovers"
  >
    <!-- Existing reactions (only emojis with count > 0) -->
    <button
      v-for="r in reactions"
      :key="r.emoji"
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
      @mouseenter="openWho(r.emoji, $event)"
      @mouseleave="scheduleCloseWho"
      @focus="openWho(r.emoji, $event)"
      @blur="scheduleCloseWho"
      @click="onPillClick(r.emoji)"
    >
      <span class="leading-none">{{ r.emoji }}</span>
      <span class="tabular-nums text-xs leading-none">{{ r.count }}</span>
    </button>

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
      <SmilePlus class="size-4" aria-hidden="true" />
    </button>

    <!-- Who-reacted popover — teleported: .glass-card review cards each create
         a stacking context (backdrop-filter), trapping any in-card z-index. -->
    <Teleport to="body">
      <div
        v-if="whoReaction && whoRows.length > 0"
        class="fixed z-[90] min-w-28 max-w-56 rounded-lg border border-white/10 bg-black/85 px-2.5 py-1.5 shadow-lg backdrop-blur-sm"
        :style="whoStyle"
        role="tooltip"
        @mouseenter="cancelCloseWho"
        @mouseleave="scheduleCloseWho"
      >
        <div class="mb-0.5 flex items-center gap-1 text-[10px] uppercase tracking-wide text-white/40">
          <span>{{ whoReaction.emoji }}</span><span>{{ nameFor(whoReaction.emoji) }}</span>
        </div>
        <div
          v-for="reactor in whoRows"
          :key="reactor.user_id || reactor.username"
          class="flex items-center justify-between gap-2"
        >
          <span
            class="truncate text-xs leading-snug"
            :class="reactor.username === SYSTEM_NAME ? 'font-semibold text-cyan-300' : 'text-white/80'"
          >
            {{ reactor.username }}
          </span>
          <!-- Admin moderation: remove this user's reaction -->
          <button
            v-if="isAdmin && reactor.user_id"
            type="button"
            class="flex h-4 w-4 shrink-0 items-center justify-center rounded text-white/40 transition-colors hover:bg-destructive/20 hover:text-destructive"
            :aria-label="t('anime.reactions.admin_remove', { user: reactor.username })"
            :title="t('anime.reactions.admin_remove', { user: reactor.username })"
            :disabled="pending"
            data-testid="reaction-admin-remove"
            @click="adminRemove(whoReaction.emoji, reactor)"
          >
            <X class="size-3" aria-hidden="true" />
          </button>
        </div>
      </div>
    </Teleport>

    <!-- Palette picker popover (teleported for the same stacking reason) -->
    <Teleport to="body">
      <template v-if="pickerOpen">
        <button
          type="button"
          class="fixed inset-0 z-[80] cursor-default"
          :aria-label="t('anime.reactions.add')"
          @click="pickerOpen = false"
        />
        <div
          class="fixed z-[90] flex max-w-[15rem] flex-wrap gap-1 rounded-xl border border-white/10 bg-black/85 p-2 shadow-xl backdrop-blur-sm"
          :style="pickerStyle"
          role="menu"
          @keydown.esc="pickerOpen = false"
        >
          <button
            v-for="p in PALETTE"
            :key="p.emoji"
            type="button"
            class="flex h-8 w-8 items-center justify-center rounded-lg text-lg transition-colors hover:bg-white/10"
            :class="myEmojis.has(p.emoji) ? 'bg-cyan-500/20 ring-1 ring-cyan-400/60' : ''"
            :title="nameFor(p.emoji)"
            :aria-label="nameFor(p.emoji)"
            :disabled="pending"
            @click="pick(p.emoji)"
          >
            {{ p.emoji }}
          </button>
        </div>
      </template>
    </Teleport>
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
