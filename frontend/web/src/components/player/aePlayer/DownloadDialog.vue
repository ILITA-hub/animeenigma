<template>
  <div class="dl-dialog" :class="{ 'dl-dialog--sheet': sheet }" role="dialog" :aria-label="$t('player.aePlayer.offline.quality')">
    <div class="dl-title text-sm font-semibold">{{ $t('player.aePlayer.offline.quality') }}</div>
    <div class="dl-opts">
      <button
        v-for="q in QUALITIES"
        :key="q"
        type="button"
        class="dl-opt"
        :class="{ 'dl-opt-active': q === quality }"
        @click="quality = q"
      >{{ q }}p</button>
    </div>

    <div class="dl-scopes" role="radiogroup" :aria-label="$t('player.aePlayer.offline.scope')">
      <button
        type="button"
        class="dl-scope"
        :class="{ 'dl-scope-active': scope === 'episode' }"
        role="radio"
        :aria-checked="scope === 'episode' ? 'true' : 'false'"
        data-test="scope-episode"
        @click="scope = 'episode'"
      >
        <span>{{ $t('player.aePlayer.offline.scopeEpisode', { n: episodeNumber }) }}</span>
        <span class="dl-scope-est">~{{ SIZE_HINT[quality] }}</span>
      </button>
      <button
        type="button"
        class="dl-scope"
        :class="{ 'dl-scope-active': scope === 'season' }"
        :disabled="seasonCount === 0"
        role="radio"
        :aria-checked="scope === 'season' ? 'true' : 'false'"
        data-test="scope-season"
        @click="seasonCount > 0 && (scope = 'season')"
      >
        <span>{{ seasonCount > 0 ? $t('player.aePlayer.offline.scopeSeason', { n: seasonCount }) : $t('player.aePlayer.offline.seasonDone') }}</span>
        <span v-if="seasonCount > 0" class="dl-scope-est">~{{ seasonEstimate }}</span>
      </button>
    </div>

    <div v-if="lowSpace" class="dl-warn text-warning" data-test="low-space">
      {{ $t('player.aePlayer.offline.lowSpace', { free: freeLabel }) }}
    </div>

    <div class="dl-actions">
      <button type="button" class="dl-btn dl-btn-primary font-medium" data-test="dl-start" @click="confirm">
        {{ $t('player.aePlayer.offline.start') }}
      </button>
      <button type="button" class="dl-btn font-medium" @click="emit('close')">
        {{ $t('player.aePlayer.offline.cancel') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { PROJECTED_BYTES, storageEstimate } from '@/offline/downloadEngine'

const QUALITIES = ['480', '720', '1080'] as const
const SIZE_HINT: Record<string, string> = { '480': '250 MB', '720': '450 MB', '1080': '900 MB' }
const LS_KEY = 'ae.downloadQuality'

const props = withDefaults(
  defineProps<{
    episodeNumber: number
    /** Episodes a season download would enqueue (pre-filtered). 0 = complete. */
    seasonCount: number
    /** Bottom-sheet presentation (mobile). */
    sheet?: boolean
    initialScope?: 'episode' | 'season'
  }>(),
  { sheet: false, initialScope: 'episode' },
)

const emit = defineEmits<{
  (e: 'confirm', quality: string, scope: 'episode' | 'season'): void
  (e: 'close'): void
}>()

const saved = localStorage.getItem(LS_KEY)
const quality = ref<string>(saved && (QUALITIES as readonly string[]).includes(saved) ? saved : '720')
const scope = ref<'episode' | 'season'>(props.initialScope === 'season' && props.seasonCount > 0 ? 'season' : 'episode')

const free = ref<number | null>(null)
onMounted(() => {
  void storageEstimate().then((est) => {
    if (est) free.value = est.quota - est.usage
  })
})

function fmtGb(bytes: number): string {
  return `${(bytes / 2 ** 30).toFixed(1)} GB`
}

const perEpisode = computed(() => PROJECTED_BYTES[quality.value] ?? PROJECTED_BYTES['720'])
const projected = computed(() => perEpisode.value * (scope.value === 'season' ? props.seasonCount : 1))
const seasonEstimate = computed(() => fmtGb(perEpisode.value * props.seasonCount))
const lowSpace = computed(() => free.value !== null && projected.value > free.value)
const freeLabel = computed(() => (free.value === null ? '' : fmtGb(free.value)))

function confirm() {
  localStorage.setItem(LS_KEY, quality.value)
  emit('confirm', quality.value, scope.value)
}
</script>

<style scoped>
.dl-dialog {
  position: absolute;
  inset-inline: 0;
  bottom: 4rem;
  margin-inline: auto;
  width: 18rem;
  padding: 0.75rem;
  border-radius: 0.5rem;
  background: var(--scrim-bg-strong);
  border: 1px solid var(--white-a8);
  z-index: 30;
}
.dl-dialog--sheet {
  position: fixed;
  inset-inline: 0;
  top: auto;
  bottom: 0;
  width: auto;
  margin-inline: 0;
  border-radius: 16px 16px 0 0;
  padding: 1rem 1rem calc(1rem + env(safe-area-inset-bottom));
  z-index: 110;
}
.dl-title { margin-bottom: 0.5rem; }
.dl-opts { display: flex; gap: 0.5rem; margin-bottom: 0.75rem; }
.dl-opt {
  flex: 1;
  padding: 0.375rem 0;
  border-radius: 0.375rem;
  border: 1px solid var(--white-a8);
  color: var(--color-muted-foreground, currentColor);
}
.dl-opt-active { border-color: var(--brand-cyan); color: var(--brand-cyan); }
.dl-scopes { display: flex; flex-direction: column; gap: 0.375rem; margin-bottom: 0.75rem; }
.dl-scope {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.625rem;
  border-radius: 0.375rem;
  border: 1px solid var(--white-a8);
  color: var(--muted-foreground);
  background: transparent;
  cursor: pointer;
  font-size: 0.8125rem;
  text-align: left;
}
.dl-scope:disabled { opacity: 0.5; cursor: default; }
.dl-scope-active { border-color: var(--brand-cyan); color: var(--foreground); }
.dl-scope-est { color: var(--muted-foreground); font-size: 0.75rem; flex-shrink: 0; }
.dl-warn { font-size: 0.75rem; margin-bottom: 0.5rem; }
.dl-actions { display: flex; gap: 0.5rem; }
.dl-btn { flex: 1; padding: 0.375rem 0; border-radius: 0.375rem; border: 1px solid var(--white-a8); }
.dl-btn-primary { border-color: var(--brand-cyan); color: var(--brand-cyan); }
</style>
