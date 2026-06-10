<template>
  <!--
    Workstream watch-together — Phase 02 (frontend-shell) Plan 02.4 Task 1.

    Sidebar member roster. Pure presentational component:
      - Props-only data flow (members + hostUserId come from the parent's
        useWatchTogetherRoom composable).
      - The `(you)` badge target is read from useAuthStore() so the parent
        doesn't need to plumb the local user-id through.
      - Avatars render through the DS Avatar primitive (initials fallback,
        image error handling, brand-cyan tint).

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
        <Avatar
          :src="m.meta.avatar_url || undefined"
          :name="m.meta.username"
          size="sm"
        />

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

import Avatar from '@/components/ui/Avatar.vue'
import { useAuthStore } from '@/stores/auth'
import type { Member } from '@/api/watch-together'

defineProps<{
  members: Member[]
  hostUserId: string
}>()

const { t } = useI18n()

const authStore = useAuthStore()
const selfUserId = computed(() => authStore.user?.id ?? '')
</script>
