<template>
  <!-- Resolving indicator — bottom-center pill while capabilities/episodes load -->
  <Teleport to="body">
    <div v-if="seasonFlow.phase === 'resolving'" class="sdh-pill" role="status">
      <Loader2 class="size-4 animate-spin" aria-hidden="true" />
      {{ $t('downloads.seasonPreparing') }}
    </div>
  </Teleport>

  <!-- Quality/scope chooser — reuses the player's DownloadDialog, season-first -->
  <Teleport to="body">
    <div v-if="seasonFlow.phase === 'choose'">
      <div class="sdh-scrim" data-test="sdh-scrim" @click="cancelSeasonDownload()" />
      <div class="sdh-anchor">
        <DownloadDialog
          :episode-number="seasonFlow.targets[0]?.number ?? 1"
          :season-count="seasonFlow.targets.length"
          :sheet="isMobile"
          initial-scope="season"
          @confirm="onConfirm"
          @close="cancelSeasonDownload()"
        />
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
// Global host for the card-launched season download flow (mounted once in
// App.vue, like <Toaster /> / <ConfirmDialogHost />). Renders the flow's
// dialog and converts its one-shot notices into toasts — all i18n lives here
// so the flow module stays translation-free.
import { watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Loader2 } from 'lucide-vue-next'
import DownloadDialog from '@/components/player/aePlayer/DownloadDialog.vue'
import { seasonFlow, confirmSeasonDownload, cancelSeasonDownload, consumeSeasonNotice } from '@/offline/seasonDownloadFlow'
import { useMobilePlayer } from '@/composables/aePlayer/useMobilePlayer'
import { useToast } from '@/composables/useToast'

const { t } = useI18n()
const toast = useToast()
const { isMobile } = useMobilePlayer()

function onConfirm(quality: string, scope: 'episode' | 'season') {
  void confirmSeasonDownload(quality, scope)
}

const NOTICE_KEY: Record<string, string> = {
  'no-sw': 'downloads.noSw',
  'no-source': 'downloads.seasonFailed',
  'nothing-left': 'downloads.seasonNothing',
  failed: 'downloads.seasonFailed',
}

watch(
  () => seasonFlow.notice,
  (n) => {
    if (!n) return
    if (n.kind === 'queued') {
      toast.push(t('downloads.seasonQueued', { n: n.n }), 'success')
    } else if (n.kind === 'failed' && n.message) {
      // Carry the raw error — a bare "couldn't prepare" is undebuggable from
      // a user screenshot (learned the hard way: DataCloneError on proxies).
      toast.push(`${t('downloads.seasonFailed')} (${n.message})`, 'error', 6000)
    } else {
      toast.push(t(NOTICE_KEY[n.kind] ?? 'downloads.seasonFailed'), n.kind === 'nothing-left' ? 'info' : 'error')
    }
    consumeSeasonNotice()
  },
)
</script>

<style scoped>
.sdh-pill {
  position: fixed;
  left: 50%;
  bottom: 24px;
  transform: translateX(-50%);
  z-index: 110;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  border-radius: 999px;
  background: var(--scrim-bg-strong);
  border: 1px solid var(--white-a20);
  color: var(--foreground);
  font-size: 13px;
  font-weight: 600;
}

.sdh-scrim {
  position: fixed;
  inset: 0;
  z-index: 105;
  background: var(--black-a60);
}

/* The dialog positions itself (absolute bottom-center; fixed sheet on mobile).
   The anchor only provides a viewport-sized positioned ancestor and must not
   swallow clicks outside the dialog (the scrim handles those). */
.sdh-anchor {
  position: fixed;
  inset: 0;
  z-index: 110;
  pointer-events: none;
}

.sdh-anchor :deep(.dl-dialog) {
  pointer-events: auto;
}
</style>
