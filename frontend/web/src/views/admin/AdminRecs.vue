<template>
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-7xl">
      <!-- Header -->
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <div>
          <h1 class="text-3xl font-semibold text-white">{{ $t('admin.recs.title') }}</h1>
          <p class="text-white/60 text-sm mt-1">
            <span class="font-mono">{{ $t('admin.recs.breadcrumb') }}: {{ userId }}</span>
            <span v-if="computedAt" class="ml-3 text-white/40">
              {{ $t('admin.recs.lastComputed') }}: {{ computedAt }}
            </span>
          </p>
        </div>
        <div class="flex items-center gap-3">
          <span
            v-if="lastRecomputeLatencyMs !== null"
            class="text-xs text-cyan-300"
            :title="$t('admin.recs.lastComputed')"
          >{{ lastRecomputeLatencyMs }}ms</span>
          <Button
            variant="default"
            size="sm"
            :disabled="isRecomputing || !userId"
            :aria-label="$t('admin.recs.forceRecompute')"
            @click="recompute"
          >
            {{ isRecomputing ? $t('admin.recs.recomputing') : $t('admin.recs.forceRecompute') }}
          </Button>
        </div>
      </div>

      <!-- Error states -->
      <div v-if="error === '403'" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ $t('admin.recs.error403') }}</p>
      </div>
      <div v-else-if="error" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ $t('admin.recs.errorGeneric') }}: {{ error }}</p>
      </div>

      <!-- Loading -->
      <div v-if="isLoading" class="flex justify-center py-12">
        <Spinner size="lg" />
      </div>

      <!-- Main table -->
      <template v-else>
        <!-- Phase 12 (UA-098): wrapper is `relative` so the mobile scroll-fade
             affordance can pin to the right edge. -->
        <div class="glass-card overflow-x-auto mb-6 relative">
          <table class="w-full text-sm text-white" :aria-label="$t('admin.recs.tableCaption')">
            <caption class="sr-only">{{ $t('admin.recs.tableCaption') }}</caption>
            <thead class="sticky top-0 bg-black/40 backdrop-blur z-10">
              <tr class="text-white/70 text-xs uppercase">
                <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.recs.columnRank') }}</th>
                <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.recs.columnAnime') }}</th>
                <th scope="col" class="px-3 py-2 text-right">{{ $t('admin.recs.columnFinal') }}</th>
                <th scope="col" class="px-3 py-2 text-right" :title="$t('admin.recs.s1Title')">S1</th>
                <th scope="col" class="px-3 py-2 text-right" :title="$t('admin.recs.s2Title')">S2</th>
                <th scope="col" class="px-3 py-2 text-right" :title="$t('admin.recs.s3Title')">S3</th>
                <th scope="col" class="px-3 py-2 text-right" :title="$t('admin.recs.s4Title')">S4</th>
                <th scope="col" class="px-3 py-2 text-right" :title="$t('admin.recs.s5Title')">S5</th>
                <th scope="col" class="px-3 py-2 text-right" :title="$t('admin.recs.s7Title')">S7</th>
                <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.recs.columnTopContributor') }}</th>
                <th scope="col" class="px-3 py-2 w-8"></th>
              </tr>
            </thead>
            <tbody>
              <template v-for="row in rows" :key="row.rank">
                <tr
                  class="border-t border-white/10 hover:bg-white/5 cursor-pointer transition focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:ring-inset"
                  :class="{ 'ring-2 ring-cyan-400/40': row.pinned }"
                  tabindex="0"
                  role="button"
                  :aria-expanded="expandedRowIds.has(row.rank)"
                  :aria-controls="`detail-${row.rank}`"
                  @click="toggleRow(row.rank)"
                  @keydown.enter.prevent="toggleRow(row.rank)"
                  @keydown.space.prevent="toggleRow(row.rank)"
                >
                  <td class="px-3 py-2 font-mono whitespace-nowrap">
                    {{ row.rank }}
                    <span
                      v-if="s12Delta(row) !== 0"
                      class="ml-1 text-xs"
                      :class="s12Delta(row) > 0 ? 'text-success' : 'text-destructive'"
                      :title="$t('admin.recs.s12Delta', { from: row.pre_s12_rank, to: row.rank })"
                    >{{ s12Delta(row) > 0 ? '↑' : '↓' }}{{ Math.abs(s12Delta(row)) }}</span>
                  </td>
                  <td class="px-3 py-2">
                    <div class="flex items-center gap-3 min-w-0">
                      <PosterImage
                        v-if="row.anime.poster_url"
                        :src="row.anime.poster_url"
                        :alt="row.anime.name || row.anime.id"
                        ratio="2/3"
                        rounded="sm"
                        :proxy-width="128"
                        class="w-10 flex-shrink-0"
                      />
                      <router-link
                        :to="`/anime/${row.anime.id}`"
                        class="hover:text-cyan-300 truncate"
                        @click.stop
                      >
                        {{ row.anime.name || row.anime.name_ru || row.anime.id }}
                      </router-link>
                    </div>
                  </td>
                  <td class="px-3 py-2 text-right font-mono">{{ formatNum(row.final) }}</td>
                  <td class="px-3 py-2 text-right font-mono text-white/70">{{ formatBd(row.breakdown.s1) }}</td>
                  <td class="px-3 py-2 text-right font-mono text-white/70">{{ formatBd(row.breakdown.s2) }}</td>
                  <td class="px-3 py-2 text-right font-mono text-white/70">{{ formatBd(row.breakdown.s3) }}</td>
                  <td class="px-3 py-2 text-right font-mono text-white/70">{{ formatBd(row.breakdown.s4) }}</td>
                  <td class="px-3 py-2 text-right font-mono text-white/70">{{ formatBd(row.breakdown.s5) }}</td>
                  <td class="px-3 py-2 text-right font-mono text-white/70">{{ formatBd(row.breakdown.s7) }}</td>
                  <td class="px-3 py-2">
                    <span
                      class="px-2 py-0.5 rounded text-xs font-mono"
                      :class="topContributorClass(row.top_contributor)"
                    >{{ row.top_contributor }}</span>
                  </td>
                  <td class="px-3 py-2 text-white/40 text-xs">
                    {{ expandedRowIds.has(row.rank) ? '▾' : '▸' }}
                  </td>
                </tr>
                <!-- Expanded contributor detail -->
                <tr
                  v-if="expandedRowIds.has(row.rank)"
                  :id="`detail-${row.rank}`"
                  class="border-t border-white/5 bg-white/5"
                >
                  <td colspan="11" class="px-6 py-3">
                    <div v-if="row.pinned" class="space-y-1 text-sm">
                      <p class="text-cyan-300 font-medium">
                        {{ $t('admin.recs.contributorDetailS6Title') }}
                      </p>
                      <p class="text-white/70">
                        <span class="font-mono">pin_source:</span>
                        {{ pinSourceLabel(row.pin_source) }}
                      </p>
                      <p v-if="row.pin_seed_anime_id" class="text-white/70">
                        <span class="font-mono">pin_seed_anime_id:</span>
                        <router-link
                          :to="`/anime/${row.pin_seed_anime_id}`"
                          class="text-cyan-300 hover:underline ml-1"
                        >
                          {{ row.pin_seed_anime_id }}
                        </router-link>
                      </p>
                      <p v-if="row.pin_reason" class="text-white/50 italic">{{ row.pin_reason }}</p>
                    </div>
                    <div v-else-if="row.top_contributor === 's5' && s5TermsFor(row).length > 0" class="space-y-1 text-sm">
                      <p class="text-brand-violet font-medium">
                        {{ $t('admin.recs.contributorDetailS5Title') }}
                      </p>
                      <ul class="space-y-0.5">
                        <li
                          v-for="(term, i) in s5TermsFor(row)"
                          :key="i"
                          class="text-white/70 font-mono text-xs"
                        >
                          {{ term.attribute }}:{{ term.value }} → {{ formatNum(term.affinity) }}
                        </li>
                      </ul>
                    </div>
                    <div v-else class="text-white/40 italic text-sm">
                      <span class="font-mono">final:</span> {{ formatNum(row.final) }}
                      &middot;
                      <span class="font-mono">top:</span> {{ row.top_contributor }}
                    </div>
                  </td>
                </tr>
              </template>
              <tr v-if="rows.length === 0">
                <td colspan="11" class="px-3 py-6 text-center text-white/50 italic">
                  {{ $t('admin.recs.filterAuditEmpty') }}
                </td>
              </tr>
            </tbody>
          </table>
          <!-- Phase 12 / UA-098: mobile horizontal-scroll affordance.
               Fixed gradient at right edge hints to mobile users that the
               table extends beyond the viewport. Pointer-events-none so it
               never blocks taps; hidden at md+ where the table fits. -->
          <div
            aria-hidden="true"
            class="md:hidden absolute right-0 top-0 bottom-0 w-8 bg-gradient-to-l from-black/40 to-transparent pointer-events-none"
          ></div>
        </div>

        <!-- Signal legend: every signal in the pipeline + its production weight
             and a one-line description. Weights come from the response rows
             (mirrors adminEnsembleWeights on the backend) with a static
             fallback for the empty-table case. -->
        <div class="glass-card p-4 mb-6">
          <h2 class="text-lg font-semibold text-white mb-3">
            {{ $t('admin.recs.signalLegendTitle') }}
          </h2>
          <dl class="space-y-2 text-sm">
            <div
              v-for="sig in signalLegend"
              :key="sig.id"
              class="flex flex-wrap items-baseline gap-x-3 gap-y-0.5"
            >
              <dt class="flex items-center gap-2 w-56 flex-shrink-0">
                <span
                  class="px-2 py-0.5 rounded text-xs font-mono"
                  :class="topContributorClass(sig.badge)"
                >{{ sig.id.toUpperCase() }}</span>
                <span class="text-white/90 font-medium">{{ $t(`admin.recs.${sig.id}Title`) }}</span>
              </dt>
              <dd class="flex-1 min-w-[16rem] text-white/60">
                <span class="font-mono text-xs mr-2" :class="sig.weightClass">{{ sig.weightLabel }}</span>
                {{ $t(`admin.recs.${sig.id}Desc`) }}
              </dd>
            </div>
          </dl>
        </div>

        <!-- Filter audit panel -->
        <div class="glass-card p-4">
          <h2 class="text-lg font-semibold text-white mb-3">
            {{ $t('admin.recs.filterAuditTitle') }}
            <span class="text-white/40 text-sm font-normal ml-2">({{ filteredOut.length }})</span>
          </h2>
          <p v-if="filteredOut.length === 0" class="text-white/50 italic">
            {{ $t('admin.recs.filterAuditEmpty') }}
          </p>
          <ul v-else class="space-y-1 text-sm">
            <li
              v-for="entry in filteredOut"
              :key="entry.anime_id + entry.reason"
              class="flex items-center gap-3"
            >
              <span
                class="px-2 py-0.5 rounded text-xs font-mono"
                :class="reasonBadgeClass(entry.reason)"
              >{{ $t(reasonKey(entry.reason)) }}</span>
              <router-link
                :to="`/anime/${entry.anime_id}`"
                class="text-cyan-300 hover:underline font-mono text-xs"
              >{{ entry.anime_id }}</router-link>
            </li>
          </ul>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import PosterImage from '@/components/anime/PosterImage.vue'
import { useAdminRecs, type AdminRecRow } from '@/composables/useAdminRecs'
import Button from '@/components/ui/Button.vue'
import { Spinner } from '@/components/ui'

// Phase 14 (REC-ADMIN-01 / REC-ADMIN-02): admin debug page.
const route = useRoute()
const userId = computed(() => (route.params.user_id as string) || '')

const {
  rows,
  filteredOut,
  computedAt,
  isLoading,
  isRecomputing,
  error,
  lastRecomputeLatencyMs,
  refresh,
  recompute,
} = useAdminRecs(userId)

void refresh()

// S12 rank movement: positive = the MMR re-rank promoted the row. The S6 pin
// row carries pre_s12_rank=0 (it never went through the re-rank) — treated as
// "no movement".
function s12Delta(row: AdminRecRow): number {
  if (row.pinned || !row.pre_s12_rank) return 0
  return row.pre_s12_rank - row.rank
}

// Legend rows for every signal in the pipeline. Ensemble weights are read
// from the first response row (the backend mirrors its production weight
// registry into each row) with a static fallback for the empty-table case.
const fallbackWeights: Record<string, number> = {
  s1: 0.3,
  s2: 0.2,
  s3: 0.2,
  s4: 0.1,
  s5: 0.2,
  s7: -0.15,
}

interface SignalLegendEntry {
  id: string
  badge: string // key fed to topContributorClass for the colored chip
  weightLabel: string
  weightClass: string
}

const { t } = useI18n()

const signalLegend = computed<SignalLegendEntry[]>(() => {
  const weights = rows.value[0]?.weights ?? fallbackWeights
  const weighted = (id: string): SignalLegendEntry => {
    const w = weights[id] ?? fallbackWeights[id] ?? 0
    return {
      id,
      badge: id,
      weightLabel: `×${w > 0 ? '+' : ''}${w}`,
      weightClass: w < 0 ? 'text-destructive' : 'text-white/40',
    }
  }
  return [
    weighted('s1'),
    weighted('s2'),
    weighted('s3'),
    weighted('s4'),
    weighted('s5'),
    {
      id: 's6',
      badge: 's6_pin',
      weightLabel: t('admin.recs.legendPostRank'),
      weightClass: 'text-cyan-300',
    },
    weighted('s7'),
    {
      id: 's11',
      badge: 's11',
      weightLabel: t('admin.recs.legendFilter'),
      weightClass: 'text-white/40',
    },
    {
      id: 's12',
      badge: 's12',
      weightLabel: `${t('admin.recs.legendPostRank')} · λ=0.3`,
      weightClass: 'text-cyan-300',
    },
  ]
})

const expandedRowIds = ref<Set<number>>(new Set())
function toggleRow(rank: number) {
  if (expandedRowIds.value.has(rank)) expandedRowIds.value.delete(rank)
  else expandedRowIds.value.add(rank)
}

function formatNum(n: number | undefined | null): string {
  if (n === undefined || n === null) return '—'
  return Math.abs(n) < 0.001 && n !== 0 ? n.toExponential(2) : n.toFixed(3)
}

function formatBd(n: number | undefined | null): string {
  if (n === undefined || n === null) return '—'
  return n.toFixed(3)
}

function topContributorClass(sig: string): string {
  switch (sig) {
    case 's1':
      return 'bg-info/20 text-info'
    case 's2':
      return 'bg-success/20 text-success'
    case 's3':
      return 'bg-warning/20 text-warning'
    case 's4':
      return 'bg-orange-500/20 text-orange-300'
    case 's5':
      return 'bg-brand-violet/20 text-brand-violet'
    case 's7':
      return 'bg-destructive/20 text-destructive'
    case 's6_pin':
      return 'bg-cyan-500/20 text-cyan-300'
    default:
      return 'bg-white/10 text-white/70'
  }
}

function reasonKey(reason: string): string {
  if (reason === 'status=completed') return 'admin.recs.reasonCompleted'
  if (reason === 'status=dropped') return 'admin.recs.reasonDropped'
  if (reason === 'hidden=true') return 'admin.recs.reasonHidden'
  return reason
}

function reasonBadgeClass(reason: string): string {
  switch (reason) {
    case 'status=completed':
      return 'bg-success/20 text-success'
    case 'status=dropped':
      return 'bg-destructive/20 text-destructive'
    case 'hidden=true':
      return 'bg-warning/20 text-warning'
    default:
      return 'bg-white/10 text-white/70'
  }
}

function pinSourceLabel(src: string | undefined): string {
  switch (src) {
    case 'local':
      return 'admin.recs.pinSourceLocal'
    case 'shikimori_similar':
      return 'admin.recs.pinSourceShikimoriSimilar'
    case 'score_5_fallback':
      return 'admin.recs.pinSourceScore5Fallback'
    default:
      return src || ''
  }
}

interface S5Term {
  attribute: string
  value: string
  affinity: number
}

function s5TermsFor(row: AdminRecRow): S5Term[] {
  const detail = row.contributor_detail as Record<string, unknown> | undefined
  const raw = detail?.tf_idf_terms
  if (!Array.isArray(raw)) return []
  return raw as S5Term[]
}
</script>
