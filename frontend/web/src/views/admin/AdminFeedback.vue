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
          <!-- View toggle: table / kanban -->
          <div class="inline-flex rounded-md border border-white/10 overflow-hidden">
            <button
              type="button"
              class="px-3 py-2 text-sm font-medium transition"
              :class="viewMode === 'table' ? 'bg-cyan-500/80 text-white' : 'bg-white/5 text-white/60 hover:bg-white/10'"
              @click="setViewMode('table')"
            >
              {{ $t('admin.feedback.viewTable') }}
            </button>
            <button
              type="button"
              class="px-3 py-2 text-sm font-medium transition"
              :class="viewMode === 'kanban' ? 'bg-cyan-500/80 text-white' : 'bg-white/5 text-white/60 hover:bg-white/10'"
              @click="setViewMode('kanban')"
            >
              {{ $t('admin.feedback.viewKanban') }}
            </button>
          </div>
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
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-6 gap-3 mb-6">
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
          v-if="viewMode === 'table'"
          v-model="filterStatus"
          size="sm"
          :options="statusOptions"
          :label="$t('admin.feedback.filters.status')"
          @change="applyFilters"
        />
        <Input
          v-model="filterUsername"
          size="sm"
          type="search"
          clearable
          :label="$t('admin.feedback.filters.username')"
          :placeholder="$t('admin.feedback.filters.usernamePlaceholder')"
        />
        <Input
          v-model="filterDateFrom"
          size="sm"
          type="date"
          :label="$t('admin.feedback.filters.dateFrom')"
        />
        <Input
          v-model="filterDateTo"
          size="sm"
          type="date"
          :label="$t('admin.feedback.filters.dateTo')"
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
        <Spinner size="lg" />
      </div>

      <!-- Empty state -->
      <div v-else-if="items.length === 0" class="glass-card p-8 text-center text-white/60">
        <p>{{ $t('admin.feedback.empty') }}</p>
      </div>

      <!-- Table -->
      <div v-else-if="viewMode === 'table'" class="glass-card overflow-x-auto">
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
              tabindex="0"
              class="border-t border-white/10 hover:bg-white/5 cursor-pointer transition"
              @click="openReport(r.id)"
              @keydown.enter="(e) => { if (!(e.target as HTMLElement).closest('select, button, a')) openReport(r.id) }"
            >
              <td class="px-3 py-2 whitespace-nowrap border-l-4" :class="statusAccentBorder(r.status)">
                <span class="px-2 py-0.5 rounded text-[10px] font-mono uppercase" :class="categoryClass(r.category)">
                  {{ categoryLabel(r.category) }}
                </span>
                <span v-if="r.player_type && r.player_type !== 'feedback'" class="ml-2 text-white/40 text-xs">
                  {{ r.player_type }}
                </span>
              </td>
              <td class="px-3 py-2 whitespace-nowrap text-white/80">{{ r.username || r.user_id }}</td>
              <td class="px-3 py-2 text-white/70 max-w-md truncate">
                <span v-if="r.attachments?.length" class="mr-1 text-white/50" :title="`${r.attachments.length}`">📎{{ r.attachments.length }}</span>
                {{ r.description || '—' }}
              </td>
              <td class="px-3 py-2 whitespace-nowrap text-white/50 text-xs">{{ formatDate(r.timestamp, r.id) }}</td>
              <td class="px-3 py-2 whitespace-nowrap" @click.stop>
                <select
                  :value="r.status"
                  :class="['rounded px-2 py-1 text-[11px] font-medium cursor-pointer outline-none transition-colors', statusSelectClass(r.status)]"
                  @change="setStatus(r.id, ($event.target as HTMLSelectElement).value as FeedbackStatus)"
                >
                  <option class="bg-popover text-white" value="new">{{ statusLabel('new') }}</option>
                  <option class="bg-popover text-white" value="in_progress">{{ statusLabel('in_progress') }}</option>
                  <option class="bg-popover text-white" value="ai_done">{{ statusLabel('ai_done') }}</option>
                  <option class="bg-popover text-white" value="resolved">{{ statusLabel('resolved') }}</option>
                  <option class="bg-popover text-white" value="not_relevant">{{ statusLabel('not_relevant') }}</option>
                </select>
              </td>
              <td class="px-3 py-2 text-right whitespace-nowrap">
                <button
                  type="button"
                  class="p-1.5 rounded bg-white/10 hover:bg-white/20 align-middle mr-2"
                  :title="copiedId === r.id ? $t('admin.feedback.copied') : $t('admin.feedback.copyLink')"
                  :aria-label="$t('admin.feedback.copyLink')"
                  @click.stop="copyReportLink(r.id)"
                >
                  <Check v-if="copiedId === r.id" class="w-3.5 h-3.5 text-success" />
                  <LinkIcon v-else class="w-3.5 h-3.5 text-white/70" />
                </button>
                <button
                  type="button"
                  class="px-3 py-1 rounded bg-white/10 hover:bg-white/20 text-xs"
                  @click.stop="openReport(r.id)"
                >
                  {{ $t('admin.feedback.view') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Kanban -->
      <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
        <div
          v-for="col in kanbanColumns"
          :key="col.status"
          class="glass-card p-3 transition-colors"
          :class="dragOverStatus === col.status ? 'ring-2 ring-cyan-400/60' : ''"
          @dragover.prevent="dragOverStatus = col.status"
          @drop="onDrop(col.status)"
        >
          <div class="flex items-center justify-between mb-3 px-1">
            <span class="px-2 py-0.5 rounded text-xs font-semibold uppercase" :class="statusClass(col.status)">
              {{ statusLabel(col.status) }}
            </span>
            <span class="text-white/40 text-xs font-mono">{{ col.items.length }}</span>
          </div>

          <div class="space-y-2 min-h-[80px]">
            <div
              v-for="r in col.items"
              :key="r.id"
              draggable="true"
              tabindex="0"
              role="button"
              class="rounded-lg bg-white/5 border border-white/10 hover:border-white/20 hover:bg-white/10 p-3 cursor-grab active:cursor-grabbing transition"
              :class="draggingId === r.id ? 'opacity-40' : ''"
              @dragstart="onDragStart(r.id, $event)"
              @dragend="onDragEnd"
              @click="openReport(r.id)"
              @keydown.enter.prevent="openReport(r.id)"
            >
              <div class="flex items-center justify-between gap-2 mb-1.5">
                <span class="px-2 py-0.5 rounded text-[10px] font-mono uppercase" :class="categoryClass(r.category)">
                  {{ categoryLabel(r.category) }}
                </span>
                <span class="text-white/40 text-[10px] whitespace-nowrap">{{ formatDate(r.timestamp, r.id) }}</span>
              </div>
              <p class="text-white/80 text-xs line-clamp-3 break-words">{{ r.description || '—' }}</p>
              <div class="flex items-center justify-between gap-2 mt-2 text-[11px] text-white/50">
                <span class="truncate">{{ r.username || r.user_id }}</span>
                <span class="flex items-center gap-1.5 whitespace-nowrap">
                  <span v-if="r.attachments?.length" class="text-white/40">📎{{ r.attachments.length }}</span>
                  <span v-if="r.player_type && r.player_type !== 'feedback'" class="text-white/30">{{ r.player_type }}</span>
                </span>
              </div>
            </div>

            <p v-if="col.items.length === 0" class="text-white/30 text-xs text-center py-4">—</p>
          </div>
        </div>
      </div>

      <!-- Pagination (table only) -->
      <div v-if="!isLoading && viewMode === 'table' && total > pageSize" class="flex items-center justify-center gap-4 mt-6 text-sm text-white/70">
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
      @click.self="closeReport"
    >
      <div class="glass-card w-full max-w-3xl p-4 md:p-6 lg:p-8 my-8">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-xl font-semibold text-white">{{ $t('admin.feedback.detail.title') }}</h2>
          <button type="button" class="text-white/60 hover:text-white text-2xl leading-none" @click="closeReport">×</button>
        </div>

        <div v-if="isDetailLoading" class="flex justify-center py-12">
          <Spinner size="lg" />
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

          <!-- Report ID + shareable deep link -->
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-white/40 text-xs uppercase">{{ $t('admin.feedback.detail.id') }}</span>
            <code class="px-2 py-0.5 rounded bg-black/30 text-cyan-300 text-xs font-mono break-all">{{ detail.id }}</code>
            <button
              type="button"
              class="inline-flex items-center gap-1 px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-xs text-white/80"
              @click="copyReportLink(detail.id)"
            >
              <Check v-if="copiedId === detail.id" class="w-3.5 h-3.5 text-success" />
              <LinkIcon v-else class="w-3.5 h-3.5" />
              {{ copiedId === detail.id ? $t('admin.feedback.copied') : $t('admin.feedback.copyLink') }}
            </button>
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

          <!-- Telegram context (bot-mirrored entries) -->
          <div v-if="detail.telegram_meta" class="space-y-1 text-white/70 text-xs bg-black/20 rounded p-2">
            <div>
              <span class="text-white/40 uppercase">Telegram</span>
              <span v-if="detail.telegram_meta.from_admin" class="ml-2 px-1.5 py-0.5 rounded bg-warning/20 text-warning">{{ $t('admin.feedback.detail.fromAdmin') }}</span>
            </div>
            <div v-if="detail.telegram_meta.forwarded_from">
              <span class="text-white/40">{{ $t('admin.feedback.detail.forwardedFrom') }}:</span> {{ detail.telegram_meta.forwarded_from }}
            </div>
            <div v-if="detail.telegram_meta.reply_to" class="whitespace-pre-wrap break-words">
              <span class="text-white/40">{{ $t('admin.feedback.detail.replyTo') }}:</span> {{ detail.telegram_meta.reply_to }}
            </div>
          </div>

          <!-- Attachments (blob-fetched with auth, images inline) -->
          <div v-if="detail.attachments?.length">
            <div class="text-white/50 text-xs uppercase mb-2">{{ $t('admin.feedback.detail.attachments') }} ({{ detail.attachments.length }})</div>
            <div class="grid grid-cols-2 sm:grid-cols-3 gap-3">
              <div v-for="a in loadedAttachments" :key="a.name" class="rounded-lg border border-white/10 bg-black/20 overflow-hidden">
                <a v-if="a.isImage && a.url" :href="a.url" target="_blank" rel="noopener noreferrer">
                  <img :src="a.url" :alt="a.name" class="w-full h-32 object-cover hover:opacity-80 transition" />
                </a>
                <div class="px-2 py-1.5 flex items-center justify-between gap-2 text-xs">
                  <span class="truncate text-white/70">{{ a.name }}</span>
                  <span v-if="a.error" class="text-destructive whitespace-nowrap">{{ $t('admin.feedback.detail.attachmentLoadError') }}</span>
                  <a v-else-if="a.url" :href="a.url" :download="a.name" class="text-cyan-300 hover:underline whitespace-nowrap">{{ $t('admin.feedback.detail.download') }}</a>
                </div>
              </div>
            </div>
          </div>

          <!-- Change status -->
          <div class="flex items-center gap-3 pt-2">
            <span class="text-white/50 text-xs uppercase">{{ $t('admin.feedback.columns.status') }}</span>
            <select
              :value="detail.status"
              :class="['rounded px-3 py-1.5 text-xs font-medium cursor-pointer outline-none transition-colors', statusSelectClass(detail.status)]"
              @change="setStatus(detail!.id, ($event.target as HTMLSelectElement).value as FeedbackStatus)"
            >
              <option class="bg-popover text-white" value="new">{{ statusLabel('new') }}</option>
              <option class="bg-popover text-white" value="in_progress">{{ statusLabel('in_progress') }}</option>
              <option class="bg-popover text-white" value="resolved">{{ statusLabel('resolved') }}</option>
              <option class="bg-popover text-white" value="not_relevant">{{ statusLabel('not_relevant') }}</option>
            </select>
          </div>

          <!-- Status history (append-only triage log; starts 2026-06-10) -->
          <div v-if="detail.status_history?.length" class="bg-black/20 rounded p-3">
            <p class="text-white/50 text-xs uppercase mb-2">{{ $t('admin.feedback.detail.history') }}</p>
            <ol class="space-y-1.5">
              <li v-for="(tr, i) in detail.status_history" :key="i" class="flex items-center gap-2 flex-wrap text-xs">
                <span class="text-white/40 whitespace-nowrap">{{ formatDate(tr.at, '') }}</span>
                <span class="text-white/70">{{ statusLabel(tr.from) }}</span>
                <span class="text-white/40" aria-hidden="true">→</span>
                <span class="text-white">{{ statusLabel(tr.to) }}</span>
                <span v-if="tr.by" class="text-white/40">· {{ tr.by }}</span>
              </li>
            </ol>
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
import { computed, onBeforeUnmount, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { Check, Link as LinkIcon } from 'lucide-vue-next'
import { adminApi } from '@/api/client'
import Input from '@/components/ui/Input.vue'
import Select from '@/components/ui/Select.vue'
import { Spinner } from '@/components/ui'
import { useAdminFeedback } from '@/composables/useAdminFeedback'
import type { FeedbackStatus } from '@/types/feedback'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const {
  items, total, page, pageSize, isLoading, error,
  filterCategory, filterStatus, filterType, filterUsername, filterDateFrom, filterDateTo,
  detail, isDetailLoading, detailError,
  refresh, applyFilters, setPage, openDetail, closeDetail, setStatus,
} = useAdminFeedback()

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

// Username filter is free-text — debounce so we don't refetch per keystroke.
let usernameDebounce: ReturnType<typeof setTimeout> | null = null
watch(filterUsername, () => {
  if (usernameDebounce) clearTimeout(usernameDebounce)
  usernameDebounce = setTimeout(() => applyFilters(), 300)
})
onUnmounted(() => {
  if (usernameDebounce) clearTimeout(usernameDebounce)
})

// Date pickers fire once per selection — no debounce needed.
watch([filterDateFrom, filterDateTo], () => applyFilters())

// --- Deep-linking: /admin/feedback?id=<report-id> opens that report ---
// openReport/closeReport wrap the composable's openDetail/closeDetail and keep
// the `id` query param in sync so any open report has a shareable URL.
function openReport(id: string): void {
  if (route.query.id !== id) router.replace({ query: { ...route.query, id } })
  openDetail(id)
}
function closeReport(): void {
  if (route.query.id) {
    const q = { ...route.query }
    delete q.id
    router.replace({ query: q })
  }
  closeDetail()
}

// Copy a shareable deep link for a report; copiedId drives transient ✓ feedback.
const copiedId = ref<string | null>(null)
let copiedTimeout: ReturnType<typeof setTimeout> | null = null
async function copyReportLink(id: string): Promise<void> {
  const url = `${window.location.origin}/admin/feedback?id=${encodeURIComponent(id)}`
  try {
    await navigator.clipboard.writeText(url)
    copiedId.value = id
    if (copiedTimeout) clearTimeout(copiedTimeout)
    copiedTimeout = setTimeout(() => (copiedId.value = null), 1500)
  } catch {
    // Clipboard can be unavailable (insecure context) — ignore silently.
  }
}

// --- View mode (table | kanban), persisted in localStorage ---
const VIEW_KEY = 'admin_feedback_view'
type ViewMode = 'table' | 'kanban'
const viewMode = ref<ViewMode>(
  typeof localStorage !== 'undefined' && localStorage.getItem(VIEW_KEY) === 'kanban' ? 'kanban' : 'table',
)

function setViewMode(m: ViewMode): void {
  if (viewMode.value === m) return
  viewMode.value = m
  try {
    localStorage.setItem(VIEW_KEY, m)
  } catch {
    // localStorage can throw in privacy modes — ignore.
  }
  // Kanban shows every status as a column, so drop the status filter and pull
  // a bigger page (columns should be ~complete, not just the table's page).
  page.value = 1
  pageSize.value = m === 'kanban' ? 200 : 50
  if (m === 'kanban') filterStatus.value = 'all'
  refresh()
}

// --- Kanban: group the loaded rows into status columns ---
const STATUS_ORDER: FeedbackStatus[] = ['new', 'in_progress', 'ai_done', 'resolved', 'not_relevant']
const kanbanColumns = computed(() =>
  STATUS_ORDER.map((status) => ({
    status,
    items: items.value.filter((i) => i.status === status),
  })),
)

// --- Native HTML5 drag-and-drop between columns ---
const draggingId = ref<string | null>(null)
const dragOverStatus = ref<string | null>(null)

function onDragStart(id: string, e: DragEvent): void {
  draggingId.value = id
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', id)
  }
}
function onDragEnd(): void {
  draggingId.value = null
  dragOverStatus.value = null
}
function onDrop(status: FeedbackStatus): void {
  const id = draggingId.value
  draggingId.value = null
  dragOverStatus.value = null
  if (!id) return
  const row = items.value.find((i) => i.id === id)
  if (row && row.status !== status) setStatus(id, status)
}

// 'telegram' entries are mirrored into the store by the maintenance bot.
const PLAYER_TYPES = ['feedback', 'telegram', 'kodik', 'animelib', 'ourenglish', 'hanime', 'raw']

// --- Attachments: fetched as blobs (Bearer auth), rendered via object URLs ---
interface LoadedAttachment {
  name: string
  url: string
  isImage: boolean
  error?: boolean
}
const loadedAttachments = ref<LoadedAttachment[]>([])

function clearAttachmentUrls(): void {
  for (const a of loadedAttachments.value) {
    if (a.url) URL.revokeObjectURL(a.url)
  }
  loadedAttachments.value = []
}

watch(detail, async (d) => {
  clearAttachmentUrls()
  if (!d?.attachments?.length) return
  const id = d.id
  const results = await Promise.all(
    d.attachments.map(async (name): Promise<LoadedAttachment> => {
      try {
        const res = await adminApi.getReportAttachment(id, name)
        const blob = res.data as Blob
        return { name, url: URL.createObjectURL(blob), isImage: (blob.type || '').startsWith('image/') }
      } catch {
        return { name, url: '', isImage: false, error: true }
      }
    }),
  )
  // The detail may have been closed/switched while blobs were in flight.
  if (detail.value?.id === id) {
    loadedAttachments.value = results
  } else {
    for (const a of results) {
      if (a.url) URL.revokeObjectURL(a.url)
    }
  }
})

onBeforeUnmount(clearAttachmentUrls)

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
  { value: 'ai_done', label: statusLabel('ai_done') },
  { value: 'resolved', label: statusLabel('resolved') },
  { value: 'not_relevant', label: statusLabel('not_relevant') },
])

function categoryLabel(c: string): string {
  if (c === 'bug' || c === 'issue' || c === 'feature') return t(`admin.feedback.category.${c}`)
  return t('admin.feedback.category.other')
}
function statusLabel(s: string): string {
  if (s === 'new' || s === 'in_progress' || s === 'ai_done' || s === 'resolved' || s === 'not_relevant') return t(`admin.feedback.status.${s}`)
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
    case 'ai_done': return 'bg-indigo-500/20 text-indigo-300'
    case 'in_progress': return 'bg-warning/20 text-warning'
    case 'not_relevant': return 'bg-muted text-muted-foreground'
    default: return 'bg-info/20 text-info'
  }
}
// Colored <select> control reflecting the current status at a glance.
function statusSelectClass(s: string): string {
  switch (s) {
    case 'resolved': return 'bg-success/20 text-success border border-success/40'
    case 'ai_done': return 'bg-indigo-500/20 text-indigo-300 border border-indigo-400/40'
    case 'in_progress': return 'bg-warning/20 text-warning border border-warning/40'
    case 'not_relevant': return 'bg-muted text-muted-foreground border border-muted-foreground/40'
    default: return 'bg-info/20 text-info border border-info/40'
  }
}
// Left accent border on each list row, colored by status.
function statusAccentBorder(s: string): string {
  switch (s) {
    case 'resolved': return 'border-success'
    case 'ai_done': return 'border-indigo-400'
    case 'in_progress': return 'border-warning'
    case 'not_relevant': return 'border-muted-foreground'
    default: return 'border-info'
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

onMounted(() => {
  // Honor a persisted kanban preference on first load: wider page + no status filter.
  if (viewMode.value === 'kanban') {
    pageSize.value = 200
    filterStatus.value = 'all'
  }
  refresh()
  // Deep link: ?id=<report-id> opens that report's detail straight away.
  const qid = route.query.id
  const id = Array.isArray(qid) ? qid[0] : qid
  if (typeof id === 'string' && id !== '') openDetail(id)
})
</script>
