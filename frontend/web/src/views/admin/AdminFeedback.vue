<template>
  <!-- Admin feedback browser: read + triage user feedback / error reports. -->
  <div class="min-h-screen bg-base pt-20">
    <div class="container mx-auto px-4 py-8 max-w-7xl">
      <!-- Header -->
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <div>
          <h1 class="text-3xl font-bold text-white">{{ $t('admin.feedback.title') }}</h1>
          <p class="text-white/60 text-sm mt-1">{{ $t('admin.feedback.subtitle') }}</p>
        </div>
        <div class="flex items-center gap-3">
          <span class="text-white/50 text-sm">{{ $t('admin.feedback.total', { n: total }) }}</span>
          <button
            type="button"
            class="px-4 py-2 rounded-md bg-cyan-500/80 hover:bg-cyan-500 text-white font-medium text-sm transition disabled:opacity-50"
            :disabled="isLoading"
            @click="refresh"
          >
            {{ $t('admin.feedback.refresh') }}
          </button>
        </div>
      </div>

      <!-- Filters -->
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-3 mb-6">
        <Select
          v-model="filterType"
          size="sm"
          :options="typeOptions"
          :label="$t('admin.feedback.filters.type')"
          @change="applyFilters"
        />
        <Select
          v-model="filterCategory"
          size="sm"
          :options="categoryOptions"
          :label="$t('admin.feedback.filters.category')"
          @change="applyFilters"
        />
        <Select
          v-model="filterStatus"
          size="sm"
          :options="statusOptions"
          :label="$t('admin.feedback.filters.status')"
          @change="applyFilters"
        />
      </div>

      <!-- Error states -->
      <div v-if="error === '403'" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ $t('admin.recs.error403') }}</p>
      </div>
      <div v-else-if="error" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ $te(error) ? $t(error) : error }}</p>
      </div>

      <!-- Loading -->
      <div v-if="isLoading" class="flex justify-center py-12">
        <div class="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin"></div>
      </div>

      <!-- Empty state -->
      <div v-else-if="items.length === 0" class="glass-card p-8 text-center text-white/60">
        <p>{{ $t('admin.feedback.empty') }}</p>
      </div>

      <!-- Table -->
      <div v-else class="glass-card overflow-x-auto">
        <table class="w-full text-sm text-white">
          <thead class="bg-black/40 backdrop-blur">
            <tr class="text-white/70 text-xs uppercase">
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.feedback.columns.category') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.feedback.columns.user') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.feedback.columns.description') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.feedback.columns.date') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.feedback.columns.status') }}</th>
              <th scope="col" class="px-3 py-2 text-right">{{ $t('admin.feedback.columns.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="r in items"
              :key="r.id"
              class="border-t border-white/10 hover:bg-white/5 cursor-pointer transition"
              @click="openDetail(r.id)"
            >
              <td class="px-3 py-2 whitespace-nowrap">
                <span class="px-2 py-0.5 rounded text-[10px] font-mono uppercase" :class="categoryClass(r.category)">
                  {{ categoryLabel(r.category) }}
                </span>
                <span v-if="r.player_type && r.player_type !== 'feedback'" class="ml-2 text-white/40 text-xs">
                  {{ r.player_type }}
                </span>
              </td>
              <td class="px-3 py-2 whitespace-nowrap text-white/80">{{ r.username || r.user_id }}</td>
              <td class="px-3 py-2 text-white/70 max-w-md truncate">{{ r.description || '—' }}</td>
              <td class="px-3 py-2 whitespace-nowrap text-white/50 text-xs">{{ formatDate(r.timestamp, r.id) }}</td>
              <td class="px-3 py-2 whitespace-nowrap" @click.stop>
                <Select
                  size="xs"
                  :model-value="r.status"
                  :options="statusChangeOptions"
                  @change="(v) => setStatus(r.id, v as FeedbackStatus)"
                />
              </td>
              <td class="px-3 py-2 text-right">
                <button
                  type="button"
                  class="px-3 py-1 rounded bg-white/10 hover:bg-white/20 text-xs"
                  @click.stop="openDetail(r.id)"
                >
                  {{ $t('admin.feedback.view') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Pagination -->
      <div v-if="!isLoading && total > pageSize" class="flex items-center justify-center gap-4 mt-6 text-sm text-white/70">
        <button
          type="button"
          class="px-3 py-1 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
          :disabled="page <= 1"
          @click="setPage(page - 1)"
        >
          ‹ {{ $t('admin.feedback.prev') }}
        </button>
        <span>{{ page }} / {{ totalPages }}</span>
        <button
          type="button"
          class="px-3 py-1 rounded bg-white/10 hover:bg-white/20 disabled:opacity-40"
          :disabled="page >= totalPages"
          @click="setPage(page + 1)"
        >
          {{ $t('admin.feedback.next') }} ›
        </button>
      </div>
    </div>

    <!-- Detail overlay -->
    <div
      v-if="detail || isDetailLoading || detailError"
      class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/70 backdrop-blur-sm p-4 pt-20"
      @click.self="closeDetail"
    >
      <div class="glass-card w-full max-w-3xl p-4 md:p-6 lg:p-8 my-8">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-xl font-semibold text-white">{{ $t('admin.feedback.detail.title') }}</h2>
          <button type="button" class="text-white/60 hover:text-white text-2xl leading-none" @click="closeDetail">×</button>
        </div>

        <div v-if="isDetailLoading" class="flex justify-center py-12">
          <div class="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin"></div>
        </div>
        <p v-else-if="detailError" class="text-destructive py-4">
          {{ $te(detailError) ? $t(detailError) : detailError }}
        </p>

        <div v-else-if="detail" class="space-y-4 text-sm">
          <div class="flex flex-wrap items-center gap-2">
            <span class="px-2 py-0.5 rounded text-[10px] font-mono uppercase" :class="categoryClass(detail.category)">
              {{ categoryLabel(detail.category) }}
            </span>
            <span class="px-2 py-0.5 rounded text-[10px] font-mono uppercase" :class="statusClass(detail.status)">
              {{ statusLabel(detail.status) }}
            </span>
            <span class="text-white/50 text-xs">{{ formatDate(detail.timestamp, detail.id) }}</span>
          </div>

          <!-- Description -->
          <div>
            <div class="text-white/50 text-xs uppercase mb-1">{{ $t('admin.feedback.columns.description') }}</div>
            <p class="text-white whitespace-pre-wrap break-words">{{ detail.description || '—' }}</p>
          </div>

          <!-- Meta grid -->
          <dl class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-2 text-white/80">
            <div><dt class="text-white/40 text-xs inline">{{ $t('admin.feedback.detail.user') }}: </dt><dd class="inline">{{ detail.username || detail.user_id }}</dd></div>
            <div v-if="detail.anime_name"><dt class="text-white/40 text-xs inline">{{ $t('admin.feedback.detail.anime') }}: </dt><dd class="inline">{{ detail.anime_name }}</dd></div>
            <div v-if="detail.episode_number != null"><dt class="text-white/40 text-xs inline">{{ $t('admin.feedback.detail.episode') }}: </dt><dd class="inline">{{ detail.episode_number }}</dd></div>
            <div v-if="detail.server_name"><dt class="text-white/40 text-xs inline">{{ $t('admin.feedback.detail.server') }}: </dt><dd class="inline">{{ detail.server_name }}</dd></div>
            <div v-if="detail.status_updated_by"><dt class="text-white/40 text-xs inline">{{ $t('admin.feedback.detail.updatedBy') }}: </dt><dd class="inline">{{ detail.status_updated_by }}</dd></div>
            <div v-if="detail.screen_size || detail.language"><dt class="text-white/40 text-xs inline">{{ $t('admin.feedback.detail.browser') }}: </dt><dd class="inline">{{ [detail.screen_size, detail.language].filter(Boolean).join(' · ') }}</dd></div>
          </dl>

          <div v-if="detail.error_message">
            <div class="text-white/50 text-xs uppercase mb-1">{{ $t('admin.feedback.detail.error') }}</div>
            <pre class="text-destructive text-xs whitespace-pre-wrap break-words bg-black/30 rounded p-2">{{ detail.error_message }}</pre>
          </div>

          <div v-if="detail.url">
            <div class="text-white/50 text-xs uppercase mb-1">{{ $t('admin.feedback.detail.url') }}</div>
            <a :href="detail.url" target="_blank" rel="noopener noreferrer" class="text-cyan-300 hover:underline break-all">{{ detail.url }}</a>
          </div>

          <!-- Change status -->
          <div class="flex items-center gap-3 pt-2">
            <span class="text-white/50 text-xs uppercase">{{ $t('admin.feedback.columns.status') }}</span>
            <div class="w-44">
              <Select
                size="xs"
                :model-value="detail.status"
                :options="statusChangeOptions"
                @change="(v) => setStatus(detail!.id, v as FeedbackStatus)"
              />
            </div>
          </div>

          <!-- Collapsible diagnostics -->
          <details v-if="detail.console_logs" class="bg-black/20 rounded p-2">
            <summary class="cursor-pointer text-white/70 text-xs uppercase">{{ $t('admin.feedback.detail.console') }}</summary>
            <pre class="mt-2 text-white/60 text-xs whitespace-pre-wrap break-words max-h-64 overflow-auto">{{ pretty(detail.console_logs) }}</pre>
          </details>
          <details v-if="detail.network_logs" class="bg-black/20 rounded p-2">
            <summary class="cursor-pointer text-white/70 text-xs uppercase">{{ $t('admin.feedback.detail.network') }}</summary>
            <pre class="mt-2 text-white/60 text-xs whitespace-pre-wrap break-words max-h-64 overflow-auto">{{ pretty(detail.network_logs) }}</pre>
          </details>
          <details v-if="detail.page_html" class="bg-black/20 rounded p-2">
            <summary class="cursor-pointer text-white/70 text-xs uppercase">{{ $t('admin.feedback.detail.html') }}</summary>
            <pre class="mt-2 text-white/60 text-xs whitespace-pre-wrap break-words max-h-64 overflow-auto">{{ detail.page_html }}</pre>
          </details>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/ui/Select.vue'
import { useAdminFeedback } from '@/composables/useAdminFeedback'
import type { FeedbackStatus } from '@/types/feedback'

const { t } = useI18n()

const {
  items, total, page, pageSize, isLoading, error,
  filterCategory, filterStatus, filterType,
  detail, isDetailLoading, detailError,
  refresh, applyFilters, setPage, openDetail, closeDetail, setStatus,
} = useAdminFeedback()

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

const PLAYER_TYPES = ['feedback', 'kodik', 'animelib', 'ourenglish', 'hanime', 'raw']

const typeOptions = computed(() => [
  { value: 'all', label: t('admin.feedback.filters.allTypes') },
  ...PLAYER_TYPES.map((v) => ({ value: v, label: v })),
])
const categoryOptions = computed(() => [
  { value: 'all', label: t('admin.feedback.filters.allCategories') },
  { value: 'bug', label: categoryLabel('bug') },
  { value: 'issue', label: categoryLabel('issue') },
  { value: 'feature', label: categoryLabel('feature') },
])
const statusOptions = computed(() => [
  { value: 'all', label: t('admin.feedback.filters.allStatuses') },
  { value: 'new', label: statusLabel('new') },
  { value: 'in_progress', label: statusLabel('in_progress') },
  { value: 'resolved', label: statusLabel('resolved') },
])
const statusChangeOptions = computed(() => [
  { value: 'new', label: statusLabel('new') },
  { value: 'in_progress', label: statusLabel('in_progress') },
  { value: 'resolved', label: statusLabel('resolved') },
])

function categoryLabel(c: string): string {
  if (c === 'bug' || c === 'issue' || c === 'feature') return t(`admin.feedback.category.${c}`)
  return t('admin.feedback.category.other')
}
function statusLabel(s: string): string {
  if (s === 'new' || s === 'in_progress' || s === 'resolved') return t(`admin.feedback.status.${s}`)
  return s
}

function categoryClass(c: string): string {
  switch (c) {
    case 'bug': return 'bg-destructive/20 text-destructive'
    case 'issue': return 'bg-warning/20 text-warning'
    case 'feature': return 'bg-success/20 text-success'
    default: return 'bg-info/20 text-info'
  }
}
function statusClass(s: string): string {
  switch (s) {
    case 'resolved': return 'bg-success/20 text-success'
    case 'in_progress': return 'bg-warning/20 text-warning'
    default: return 'bg-info/20 text-info'
  }
}

// Render the report timestamp; fall back to the id's leading timestamp segment.
function formatDate(iso: string, id: string): string {
  const raw = iso || id.split('_')[0]?.replace('T', ' ') || ''
  const d = new Date(iso)
  if (!Number.isNaN(d.getTime())) return d.toLocaleString()
  return raw
}

function pretty(v: unknown): string {
  if (typeof v === 'string') return v
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

onMounted(refresh)
</script>
