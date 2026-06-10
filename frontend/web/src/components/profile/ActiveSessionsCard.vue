<script setup lang="ts">
import { onMounted } from 'vue'
import { useSessions } from '@/composables/useSessions'
import { parseUserAgent } from '@/utils/userAgent'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui'
import { useConfirm } from '@/composables/useConfirm'

const { t } = useI18n()
const { confirm } = useConfirm()
const { sessions, loading, error, refresh, revoke, revokeOthers } = useSessions()

onMounted(refresh)

function relative(iso: string): string {
  const diffSec = (Date.now() - new Date(iso).getTime()) / 1000
  if (diffSec < 60) return t('profile.settings.sessions.justNow')
  if (diffSec < 3600) return t('profile.settings.sessions.minutesAgo', { n: Math.floor(diffSec / 60) })
  if (diffSec < 86400) return t('profile.settings.sessions.hoursAgo', { n: Math.floor(diffSec / 3600) })
  return t('profile.settings.sessions.daysAgo', { n: Math.floor(diffSec / 86400) })
}

async function onRevokeOthers() {
  if (!(await confirm({
    title: t('common.confirmTitle'),
    description: t('profile.settings.sessions.confirmRevokeOthers'),
    confirmText: t('common.confirm'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  }))) return
  await revokeOthers()
}
</script>

<template>
  <section class="rounded-xl bg-white/5 border border-white/10 p-5 space-y-4">
    <header class="flex items-center justify-between">
      <h3 class="text-base font-semibold text-white">
        {{ $t('profile.settings.sessions.title') }}
      </h3>
      <button
        class="text-xs text-white/60 hover:text-white"
        :disabled="loading"
        @click="refresh"
      >
        {{ $t('profile.settings.sessions.refresh') }}
      </button>
    </header>

    <p class="text-sm text-white/60">
      {{ $t('profile.settings.sessions.description') }}
    </p>

    <div v-if="loading && sessions.length === 0" class="text-sm text-white/40">
      {{ $t('profile.settings.sessions.loading') }}
    </div>

    <div v-else-if="error" class="text-sm text-destructive">
      {{ error }}
    </div>

    <ul v-else class="space-y-2">
      <li
        v-for="s in sessions"
        :key="s.id"
        class="flex items-start gap-3 rounded-lg bg-black/20 border border-white/5 p-3"
      >
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-white truncate">
              {{ parseUserAgent(s.user_agent) }}
            </span>
            <span
              v-if="s.is_current"
              class="text-[10px] uppercase tracking-wide text-success bg-success/10 border border-success/30 rounded px-1.5 py-0.5"
            >
              {{ $t('profile.settings.sessions.thisDevice') }}
            </span>
          </div>
          <div class="text-xs text-white/50 mt-1">
            {{ s.ip || $t('profile.settings.sessions.unknownIp') }} ·
            {{ $t('profile.settings.sessions.lastSeen') }} {{ relative(s.last_seen_at) }}
          </div>
        </div>
        <Button
          v-if="!s.is_current"
          variant="secondary"
          size="sm"
          @click="revoke(s.id)"
        >
          {{ $t('profile.settings.sessions.revoke') }}
        </Button>
      </li>
    </ul>

    <footer v-if="sessions.length > 1" class="pt-2 border-t border-white/5">
      <Button variant="secondary" size="sm" @click="onRevokeOthers">
        {{ $t('profile.settings.sessions.revokeAllOthers') }}
      </Button>
    </footer>
  </section>
</template>
