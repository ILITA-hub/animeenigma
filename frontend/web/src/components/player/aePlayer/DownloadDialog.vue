<template>
  <div class="dl-dialog" :class="{ 'dl-dialog--sheet': sheet }" role="dialog" :aria-label="$t('player.aePlayer.offline.quality')">
    <template v-if="showSource">
      <div class="dl-title text-sm font-semibold">{{ $t('player.aePlayer.offline.source') }}</div>
      <div class="dl-opts">
        <button type="button" class="dl-opt" :class="{ 'dl-opt-active': combo!.audio === 'sub' }"
          :disabled="!rawAvailable" data-test="dl-audio-sub" @click="setAudio('sub')">{{ $t('player.aePlayer.offline.audioRaw') }}</button>
        <button type="button" class="dl-opt" :class="{ 'dl-opt-active': combo!.audio === 'dub' }"
          :disabled="!dubAvailable" data-test="dl-audio-dub" @click="setAudio('dub')">{{ $t('player.aePlayer.offline.audioDub') }}</button>
      </div>
      <div v-if="combo!.audio === 'dub'" class="dl-opts">
        <button type="button" class="dl-opt" :class="{ 'dl-opt-active': combo!.lang === 'ru' }" data-test="dl-lang-ru" @click="applyFilter('dub', 'ru')">RU</button>
        <button type="button" class="dl-opt" :class="{ 'dl-opt-active': combo!.lang === 'en' }" data-test="dl-lang-en" @click="applyFilter('dub', 'en')">EN</button>
      </div>
      <select class="dl-select" :value="combo!.provider" data-test="dl-provider"
        @change="setProvider(($event.target as HTMLSelectElement).value)">
        <option v-for="r in rows" :key="r.id" :value="r.id" :disabled="!r.selectable">{{ r.label }}</option>
      </select>
      <select v-if="teams.length > 0" class="dl-select" :value="combo!.team ?? ''" data-test="dl-team"
        @change="setTeam(($event.target as HTMLSelectElement).value)">
        <option value="">{{ $t('player.aePlayer.offline.teamAuto') }}</option>
        <option v-for="tm in teams" :key="tm" :value="tm">{{ tm }}</option>
      </select>
    </template>

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
        <span class="dl-scope-est" data-test="episode-estimate">~{{ episodeEstimate }}</span>
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

    <template v-if="subOptions.length > 0">
      <div class="dl-title text-sm font-semibold">{{ $t('player.aePlayer.offline.subs') }}</div>
      <select v-model="subKey" class="dl-select" data-test="dl-subs">
        <option value="off">{{ $t('player.aePlayer.offline.subsOff') }}</option>
        <option v-for="o in subOptions" :key="o.key" :value="o.key">{{ o.label }}</option>
      </select>
    </template>

    <div v-if="lowSpace" class="dl-warn text-warning" data-test="low-space">
      {{ $t('player.aePlayer.offline.lowSpace', { free: freeLabel }) }}
    </div>

    <div v-if="cellularStep" class="dl-warn text-warning" data-test="cellular-warn">
      {{ $t('player.aePlayer.offline.cellularWarn') }}
    </div>
    <div class="dl-actions">
      <template v-if="cellularStep">
        <button type="button" class="dl-btn dl-btn-warn text-warning font-medium" data-test="dl-cellular-confirm" @click="confirmCellular">
          {{ $t('player.aePlayer.offline.cellularConfirm') }}
        </button>
        <button type="button" class="dl-btn font-medium" @click="cellularStep = false">
          {{ $t('player.aePlayer.offline.cancel') }}
        </button>
      </template>
      <template v-else>
        <button type="button" class="dl-btn dl-btn-primary font-medium" data-test="dl-start" @click="confirm">
          {{ $t('player.aePlayer.offline.start') }}
        </button>
        <button type="button" class="dl-btn font-medium" @click="emit('close')">
          {{ $t('player.aePlayer.offline.cancel') }}
        </button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { projectedBytesFor, storageEstimate } from '@/offline/downloadEngine'
import { rowsFromReport } from '@/composables/aePlayer/useProviderFeed'
import { pickSmartDefault, pickSelectableFallback } from '@/composables/aePlayer/smartDefault'
import { GROUP_PRIMARY_LANG } from '@/composables/aePlayer/providerGroups'
import type { Combo, AudioKind, TrackLang, ContentKind, ProviderRow } from '@/types/aePlayer'
import type { CapabilityReport } from '@/types/capabilities'
import type { SubPref, SubOption } from '@/offline/types'
import * as network from '@/offline/network'

const QUALITIES = ['480', '720', '1080'] as const
const LS_KEY = 'ae.downloadQuality'

const props = withDefaults(
  defineProps<{
    episodeNumber: number
    /** Episodes a season download would enqueue (pre-filtered). 0 = complete. */
    seasonCount: number
    /** Episode runtime in minutes — scales the size estimates (12-min shorts
     *  are half a 24-min episode, not a flat 900 MB at 1080p). */
    durationMin?: number
    /** Bottom-sheet presentation (mobile). */
    sheet?: boolean
    initialScope?: 'episode' | 'season'
    /** Capability report for the source combo picker. When absent the source
     *  section is hidden and `combo` emits as null (backward compatible). */
    report?: CapabilityReport | null
    /** Initial combo selection — if absent the source section is hidden. */
    initialCombo?: Combo | null
    /** Optional async loader for translation teams for a given provider+audio. */
    loadTeams?: (provider: string, audio: AudioKind) => Promise<string[]>
    /** Subtitle options to display in the picker. Empty = picker hidden. */
    subOptions?: SubOption[]
  }>(),
  { sheet: false, initialScope: 'episode', durationMin: undefined, report: null, initialCombo: null, loadTeams: undefined, subOptions: () => [] },
)

const emit = defineEmits<{
  (e: 'confirm', quality: string, scope: 'episode' | 'season', combo: Combo | null, subPref: SubPref | null): void
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

function fmtSize(bytes: number): string {
  return bytes >= 2 ** 30 ? fmtGb(bytes) : `${Math.round(bytes / 2 ** 20)} MB`
}

const perEpisode = computed(() => projectedBytesFor(quality.value, props.durationMin))
const projected = computed(() => perEpisode.value * (scope.value === 'season' ? props.seasonCount : 1))
const episodeEstimate = computed(() => fmtSize(perEpisode.value))
const seasonEstimate = computed(() => fmtSize(perEpisode.value * props.seasonCount))
const lowSpace = computed(() => free.value !== null && projected.value > free.value)
const freeLabel = computed(() => (free.value === null ? '' : fmtGb(free.value)))

// ─── Source combo picker ──────────────────────────────────────────────────────

const combo = ref<Combo | null>(props.initialCombo ? { ...props.initialCombo } : null)
const showSource = computed(() => !!props.report && combo.value !== null)

function rowsFor(audio: AudioKind, lang: TrackLang): ProviderRow[] {
  // Same content fallback as pickDefaultCombo: hentai rows only when common has none.
  for (const content of ['common', 'hentai'] as ContentKind[]) {
    const rows = rowsFromReport(props.report ?? null, { audio, lang, content })
    if (rows.length > 0) return rows
  }
  return []
}
const rows = computed(() => (combo.value ? rowsFor(combo.value.audio, combo.value.lang) : []))
const dubAvailable = computed(() => rowsFor('dub', 'ru').length > 0 || rowsFor('dub', 'en').length > 0)
const rawAvailable = computed(() => rowsFor('sub', 'en').length > 0)

const teams = ref<string[]>([])
async function refreshTeams(): Promise<void> {
  teams.value = []
  const c = combo.value
  if (!c?.provider || !props.loadTeams) return
  try { teams.value = await props.loadTeams(c.provider, c.audio) } catch { teams.value = [] }
}
onMounted(() => { void refreshTeams() })

/** Keep the picked provider when it survives the new filter, else re-default
 *  the same way the player's smart default does. */
function applyFilter(audio: AudioKind, lang: TrackLang): void {
  const c = combo.value
  if (!c) return
  const rs = rowsFor(audio, lang)
  const row = rs.find((r) => r.id === c.provider) ?? pickSmartDefault(rs) ?? pickSelectableFallback(rs)
  if (!row) return // nothing under this filter — leave the combo untouched
  const nextLang = audio === 'sub' ? GROUP_PRIMARY_LANG[row.group] : lang
  const providerChanged = row.id !== c.provider
  combo.value = { ...c, audio, lang: nextLang, provider: row.id, team: providerChanged ? null : c.team }
  if (providerChanged) void refreshTeams()
}
function setAudio(audio: AudioKind): void {
  const lang: TrackLang = audio === 'dub' ? (combo.value?.lang === 'ru' ? 'ru' : 'en') : (combo.value?.lang ?? 'en')
  applyFilter(audio, lang)
}
function setProvider(id: string): void {
  const c = combo.value
  const row = rows.value.find((r) => r.id === id)
  if (!c || !row) return
  combo.value = { ...c, provider: id, lang: c.audio === 'sub' ? GROUP_PRIMARY_LANG[row.group] : c.lang, team: null }
  void refreshTeams()
}
function setTeam(v: string): void {
  if (combo.value) combo.value = { ...combo.value, team: v === '' ? null : v }
}

// ─── Subtitle picker ─────────────────────────────────────────────────────────

const subKey = ref('off')
const pickedSubPref = computed<SubPref | null>(() =>
  props.subOptions.find((o) => o.key === subKey.value)?.pref ?? null)

// ─── Cellular guard ───────────────────────────────────────────────────────────

const cellularStep = ref(false)

function confirm() {
  if (network.isCellular() && !network.allowCellularThisSession()) {
    cellularStep.value = true
    return
  }
  doConfirm()
}

function confirmCellular() {
  network.setAllowCellularThisSession(true)
  doConfirm()
}

function doConfirm() {
  localStorage.setItem(LS_KEY, quality.value)
  emit('confirm', quality.value, scope.value, combo.value ? { ...combo.value } : null, pickedSubPref.value)
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
  max-height: calc(100% - 5rem);
  overflow-y: auto;
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
.dl-select {
  width: 100%;
  margin-bottom: 0.75rem;
  padding: 0.375rem 0.625rem;
  border-radius: 0.375rem;
  border: 1px solid var(--white-a8);
  background: transparent;
  color: var(--foreground);
  font-size: 0.8125rem;
}
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
.dl-btn-warn { border-color: currentColor; }
</style>
