<template>
  <!--
    Workstream watch-together — Phase 02 (frontend-shell) Plan 02.4 Task 1.

    Sidebar member roster. Pure presentational component:
      - Props-only data flow (members + hostUserId come from the parent's
        useWatchTogetherRoom composable).
      - The `(you)` badge target is read from useAuthStore() so the parent
        doesn't need to plumb the local user-id through.
      - Avatar fallback is a CSS-only first-letter chip (no external util
        import — avatars in chat panels are rendered tiny enough that the
        existing imageFallback util is overkill).

    UI-SPEC contract (CLAUDE.md):
      - Tailwind utility classes only
      - Font weights: font-medium / font-semibold only
      - Padding: p-4 md:p-6 on the outer <section>
  -->
  <section
    class="w-full p-4 md:p-6"
    :aria-label="t('watch_together.members_heading')"
  >
    <h3 class="text-sm uppercase tracking-wider text-foreground/60 font-semibold mb-3">
      {{ t('watch_together.members_heading') }} ({{ members.length }})
    </h3>

    <ul
      v-if="members.length > 0"
      class="flex flex-col gap-1"
      data-testid="wt-member-list"
    >
      <li
        v-for="m in members"
        :key="m.user_id"
        data-testid="wt-member-entry"
        class="flex items-center gap-2 p-2 rounded-md hover:bg-foreground/5"
      >
        <img
          v-if="m.meta.avatar_url"
          :src="m.meta.avatar_url"
          :alt="m.meta.username"
          class="w-8 h-8 rounded-full object-cover flex-shrink-0"
          loading="lazy"
        />
        <span
          v-else
          aria-hidden="true"
          class="w-8 h-8 rounded-full bg-cyan-500/80 text-white flex items-center justify-center text-sm font-semibold flex-shrink-0"
        >
          {{ initialFor(m.meta.username) }}
        </span>

        <span class="font-medium truncate min-w-0">{{ m.meta.username }}</span>

        <span
          v-if="m.user_id === hostUserId"
          class="text-xs text-foreground/60 font-medium px-1.5 py-0.5 rounded bg-foreground/10"
        >
          {{ t('watch_together.host_badge') }}
        </span>
        <span
          v-if="m.user_id === selfUserId"
          class="text-xs text-foreground/60 font-medium px-1.5 py-0.5 rounded bg-foreground/10"
        >
          {{ t('watch_together.you_badge') }}
        </span>
      </li>
    </ul>

    <p v-else class="text-sm text-foreground/50 font-medium">—</p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { useAuthStore } from '@/stores/auth'
import type { Member } from '@/api/watch-together'

defineProps<{
  members: Member[]
  hostUserId: string
}>()

const { t } = useI18n()

const authStore = useAuthStore()
const selfUserId = computed(() => authStore.user?.id ?? '')

function initialFor(username: string): string {
  // Empty string falls back to '?' so the fallback chip never collapses to 0×0.
  const first = (username ?? '').trim().charAt(0)
  return first ? first.toUpperCase() : '?'
}
</script>
