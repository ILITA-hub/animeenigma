<template>
  <!-- Admin feedback browser: Project Board: feedback, TODOs and ideas — read, triage + quick-capture. -->
  <div class="min-h-screen bg-base pt-20">
    <div class="container mx-auto px-4 py-8 max-w-7xl">
      <!-- Header -->
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <div>
          <h1 class="text-3xl font-semibold text-white">{{ $t('admin.feedback.title') }}</h1>
          <p class="text-white/60 text-sm mt-1">{{ $t('admin.feedback.subtitle') }}</p>
        </div>
        <div class="flex items-center gap-3">
          <!-- View toggle: table / kanban -->
          <SegmentedControl
            :model-value="viewMode"
            :options="[
              { value: 'table', label: $t('admin.feedback.viewTable') },
              { value: 'kanban', label: $t('admin.feedback.viewKanban') },
            ]"
            @update:model-value="setViewMode($event as ViewMode)"
          />
          <span class="text-white/50 text-sm">{{ $t('admin.feedback.total', { n: total }) }}</span>
          <button
            type="button"
            class="px-4 py-2 rounded-md bg-white/5 hover:bg-white/10 border border-white/10 text-white font-medium text-sm transition"
            @click="showNewNote = true"
          >
            {{ $t('admin.feedback.newNote.button') }}
          </button>
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
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 mb-6">
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
          v-model="filterSource"
          size="sm"
          :options="sourceOptions"
          :label="$t('admin.feedback.filters.source')"
          @change="applyFilters"
        />
        <!-- Status: multi-select checkbox picker (any combination of statuses) -->
        <div class="w-full">
          <label class="block text-sm font-medium text-white/70 mb-2">{{ $t('admin.feedback.filters.status') }}</label>
          <Popover v-model:open="statusMenuOpen" align="start" class="w-56 p-2">
            <template #trigger>
              <button
                type="button"
                class="w-full flex items-center justify-between gap-2 bg-white/5 border border-white/10 text-white px-3 py-2 text-sm rounded-lg touch-target transition-all duration-200 cursor-pointer hover:border-white/20 focus:outline-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50 data-[state=open]:ring-2 data-[state=open]:ring-cyan-500/50"
                aria-haspopup="listbox"
              >
                <span :class="filterStatuses.length ? 'text-white' : 'text-white/30'">{{ statusSummary }}</span>
                <ChevronDown
                  class="size-4 text-white/50 transition-transform duration-200"
                  :class="statusMenuOpen ? 'rotate-180' : ''"
                  aria-hidden="true"
                />
              </button>
            </template>

            <div class="space-y-0.5">
              <div class="flex items-center justify-between gap-2 px-1 pb-2 mb-1 border-b border-white/10">
                <button type="button" class="text-xs text-white/60 hover:text-white transition-colors" @click="setStatusFilter(ACTIVE_STATUSES)">
                  {{ $t('admin.feedback.status.active') }}
                </button>
                <button type="button" class="text-xs text-white/60 hover:text-white transition-colors" @click="setStatusFilter([])">
                  {{ $t('admin.feedback.filters.allStatuses') }}
                </button>
              </div>
              <label
                v-for="s in STATUS_ORDER"
                :key="s"
                class="flex items-center gap-2 px-1 py-1.5 rounded-md hover:bg-white/5 cursor-pointer transition-colors"
              >
                <Checkbox :model-value="filterStatuses.includes(s)" @update:model-value="(v) => toggleStatus(s, v as boolean)" />
                <span class="text-sm text-white/80">{{ statusLabel(s) }}</span>
              </label>
            </div>
          </Popover>
        </div>
        <Input
          v-model="filterUsername"
          size="sm"
          type="search"
          clearable
          :label="$t('admin.feedback.filters.username')"
          :placeholder="$t('admin.feedback.filters.usernamePlaceholder')"
        />
        <div>
          <label class="block text-sm font-medium text-white/70 mb-2">{{ $t('admin.feedback.filters.dateFrom') }}</label>
          <DatePicker
            v-model="filterDateFrom"
            class="w-full"
            :placeholder="$t('admin.feedback.filters.dateFrom')"
            :title="$t('admin.feedback.filters.dateFrom')"
          />
        </div>
        <div>
          <label class="block text-sm font-medium text-white/70 mb-2">{{ $t('admin.feedback.filters.dateTo') }}</label>
          <DatePicker
            v-model="filterDateTo"
            class="w-full"
            :placeholder="$t('admin.feedback.filters.dateTo')"
            :title="$t('admin.feedback.filters.dateTo')"
          />
        </div>
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
                <Badge size="sm" :variant="kindVariant(r.kind || 'feedback')" class="text-[10px] font-mono uppercase mr-1">
                  {{ kindLabel(r.kind || 'feedback') }}
                </Badge>
                <Badge size="sm" :variant="categoryVariant(r.category)" class="text-[10px] font-mono uppercase">
                  {{ categoryLabel(r.category) }}
                </Badge>
                <span v-if="r.source" class="ml-2 text-white/40 text-[10px] uppercase">{{ sourceLabel(r.source) }}</span>
              </td>
              <td class="px-3 py-2 whitespace-nowrap text-white/80">{{ r.username || r.user_id }}</td>
              <td class="px-3 py-2 text-white/70 max-w-md truncate">
                <span v-if="r.attachments?.length" class="mr-1 text-white/50" :title="`${r.attachments.length}`">📎{{ r.attachments.length }}</span>
                {{ r.description || '—' }}
              </td>
              <td class="px-3 py-2 whitespace-nowrap text-white/50 text-xs">{{ formatDate(r.timestamp, r.id) }}</td>
              <td class="px-3 py-2 whitespace-nowrap" @click.stop>
                <Select
                  :model-value="r.status"
                  size="xs"
                  :options="rowStatusOptions"
                  :trigger-class="'w-auto ' + statusSelectClass(r.status)"
                  @change="(v) => setStatus(r.id, v as FeedbackStatus)"
                />
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
            <Badge size="sm" class="font-semibold uppercase" :class="statusPillClass(col.status)">
              {{ statusLabel(col.status) }}
            </Badge>
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
                <div class="flex items-center gap-1.5">
                  <Badge size="sm" :variant="kindVariant(r.kind || 'feedback')" class="text-[10px] font-mono uppercase">
                    {{ kindLabel(r.kind || 'feedback') }}
                  </Badge>
                  <Badge size="sm" :variant="categoryVariant(r.category)" class="text-[10px] font-mono uppercase">
                    {{ categoryLabel(r.category) }}
                  </Badge>
                </div>
                <span class="text-white/40 text-[10px] whitespace-nowrap">{{ formatDate(r.timestamp, r.id) }}</span>
              </div>
              <p class="text-white/80 text-xs line-clamp-3 break-words">{{ r.description || '—' }}</p>
              <div class="flex items-center justify-between gap-2 mt-2 text-[11px] text-white/50">
                <span class="truncate">{{ r.username || r.user_id }}</span>
                <span class="flex items-center gap-1.5 whitespace-nowrap">
                  <span v-if="r.attachments?.length" class="text-white/40">📎{{ r.attachments.length }}</span>
                  <span v-if="r.source" class="text-white/30 uppercase">{{ sourceLabel(r.source) }}</span>
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

    <NewNoteDialog v-model:open="showNewNote" @created="refresh" />

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
            <Badge size="sm" :variant="categoryVariant(detail.category)" class="text-[10px] font-mono uppercase">
              {{ categoryLabel(detail.category) }}
            </Badge>
            <Badge size="sm" class="text-[10px] font-mono uppercase" :class="statusPillClass(detail.status)">
              {{ statusLabel(detail.status) }}
            </Badge>
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
            <Select
              :model-value="detail.status"
              size="sm"
              :options="rowStatusOptions"
              :trigger-class="'w-auto ' + statusSelectClass(detail.status)"
              @change="(v) => setStatus(detail!.id, v as FeedbackStatus)"
            />
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
import { Check, Link as LinkIcon, ChevronDown } from 'lucide-vue-next'
import { adminApi } from '@/api/client'
import Input from '@/components/ui/Input.vue'
import Select from '@/components/ui/Select.vue'
import SegmentedControl from '@/components/ui/SegmentedControl.vue'
import { Spinner, DatePicker, Badge, Popover, Checkbox } from '@/components/ui'
import { useAdminFeedback } from '@/composables/useAdminFeedback'
import NewNoteDialog from '@/components/admin/NewNoteDialog.vue'
import { FEEDBACK_CATEGORIES, isFeedbackCategory } from '@/types/feedback'
import type { FeedbackStatus, FeedbackCategory } from '@/types/feedback'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const {
  items, total, page, pageSize, isLoading, error,
  filterCategory, filterStatuses, filterSource, filterType, filterUsername, filterDateFrom, filterDateTo,
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

const showNewNote = ref(false)

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
  // Kanban shows every status as a column, so pull a bigger page
  // (columns should be ~complete, not just the table's page).
  page.value = 1
  pageSize.value = m === 'kanban' ? 200 : 50
  refresh()
}

// --- Kanban: group the loaded rows into status columns ---
const STATUS_ORDER: FeedbackStatus[] = ['new', 'in_progress', 'ai_done', 'resolved', 'not_relevant']
// Active = everything except dismissed; used as the default selection and the
// "Active" quick-action in the status filter.
const ACTIVE_STATUSES: FeedbackStatus[] = ['new', 'in_progress', 'ai_done', 'resolved']
const kanbanColumns = computed(() => {
  // Empty selection = show every column; otherwise only the chosen statuses,
  // kept in canonical STATUS_ORDER.
  const sel = filterStatuses.value
  const order = sel.length ? STATUS_ORDER.filter((s) => sel.includes(s)) : STATUS_ORDER
  return order.map((status) => ({
    status,
    items: items.value.filter((i) => i.status === status),
  }))
})

// --- Status multi-select filter (Popover + checkbox list) ---
const statusMenuOpen = ref(false)

// Same-set check (order-independent) so the trigger can name common presets.
const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every((x) => b.includes(x))

const statusSummary = computed(() => {
  const sel = filterStatuses.value
  if (sel.length === 0 || sel.length === STATUS_ORDER.length) return t('admin.feedback.filters.allStatuses')
  if (sameSet(sel, ACTIVE_STATUSES)) return t('admin.feedback.status.active')
  if (sel.length === 1) return statusLabel(sel[0])
  return t('admin.feedback.filters.statusSelected', { n: sel.length })
})

function toggleStatus(s: string, checked: boolean): void {
  const next = checked
    ? [...filterStatuses.value, s]
    : filterStatuses.value.filter((x) => x !== s)
  filterStatuses.value = next
  applyFilters()
}

function setStatusFilter(list: string[]): void {
  filterStatuses.value = [...list]
  applyFilters()
}

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
  ...FEEDBACK_CATEGORIES.map((c) => ({ value: c, label: categoryLabel(c) })),
])
const SOURCES = ['feedback_form', 'telegram', 'api', 'manual'] as const
const kindLabel = (k: string) => t(`admin.feedback.kind.${k}`)
const sourceLabel = (s: string) => t(`admin.feedback.source.${s}`)
const sourceOptions = computed(() => [
  { value: 'all', label: t('admin.feedback.filters.allSources') },
  ...SOURCES.map((v) => ({ value: v, label: sourceLabel(v) })),
])

type KindBadge = 'info' | 'primary' | 'warning'
const KIND_VARIANT: Record<string, KindBadge> = { feedback: 'info', todo: 'primary', idea: 'warning' }
const kindVariant = (k: string): KindBadge => KIND_VARIANT[k] ?? 'info'

// Per-report status control options (no 'all' pseudo-status). Shared by the
// table-row picker and the detail-modal picker so both expose every status
// (the detail modal previously omitted 'ai_done').
const rowStatusOptions = computed(() => [
  { value: 'new', label: statusLabel('new') },
  { value: 'in_progress', label: statusLabel('in_progress') },
  { value: 'ai_done', label: statusLabel('ai_done') },
  { value: 'resolved', label: statusLabel('resolved') },
  { value: 'not_relevant', label: statusLabel('not_relevant') },
])

function categoryLabel(c: string): string {
  if (isFeedbackCategory(c)) return t(`admin.feedback.category.${c}`)
  return t('admin.feedback.category.other')
}
function statusLabel(s: string): string {
  if (s === 'new' || s === 'in_progress' || s === 'ai_done' || s === 'resolved' || s === 'not_relevant') return t(`admin.feedback.status.${s}`)
  return s
}

// Single source of truth for per-status styling (kills drift across the three
// places status colour was previously duplicated). Every status drives its pill,
// dropdown trigger and row accent from ONE token-bound palette so all three agree:
//   - `pill`   → bg+text classes for the status Badge (passed as a class override).
//   - `select` → tints the status-dropdown trigger (overrides its neutral base).
//   - `accent` → left border colour on each table row.
// ai_done is brand-violet (purple) — its distinguishing colour for the
// "AI says done, awaiting human verify" triage state. `new` is the info-blue
// token so purple stays unique to ai_done.
const STATUS_META: Record<FeedbackStatus, { pill: string; select: string; accent: string }> = {
  new:          { pill: 'bg-info/20 text-info',                 select: 'bg-info/20 text-info border border-info/40',                        accent: 'border-info' },
  in_progress:  { pill: 'bg-warning/20 text-warning',           select: 'bg-warning/20 text-warning border border-warning/40',               accent: 'border-warning' },
  ai_done:      { pill: 'bg-brand-violet/20 text-brand-violet', select: 'bg-brand-violet/20 text-brand-violet border border-brand-violet/40', accent: 'border-brand-violet' },
  resolved:     { pill: 'bg-success/20 text-success',           select: 'bg-success/20 text-success border border-success/40',               accent: 'border-success' },
  not_relevant: { pill: 'bg-muted text-muted-foreground',       select: 'bg-muted text-muted-foreground border border-muted-foreground/40',  accent: 'border-muted-foreground' },
}
const statusMeta = (s: string) => STATUS_META[s as FeedbackStatus] ?? STATUS_META.new
const statusPillClass = (s: string): string => statusMeta(s).pill
const statusSelectClass = (s: string): string => statusMeta(s).select
const statusAccentBorder = (s: string): string => statusMeta(s).accent

// Category → Badge variant. Admin keeps colour-coded categories (richer than the
// user-facing MyFeedback view, which renders all categories as a neutral badge).
type CategoryBadge = 'destructive' | 'warning' | 'success' | 'info'
const CATEGORY_VARIANT: Record<FeedbackCategory, CategoryBadge> = {
  bug: 'destructive',
  issue: 'warning',
  feature: 'success',
}
// CATEGORY_VARIANT stays exhaustive over FeedbackCategory; the guard maps any
// legacy/empty value off-list to the neutral 'info' badge.
const categoryVariant = (c: string): CategoryBadge => (isFeedbackCategory(c) ? CATEGORY_VARIANT[c] : 'info')

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
  // Honor a persisted kanban preference on first load: wider page.
  if (viewMode.value === 'kanban') {
    pageSize.value = 200
    filterSource.value = 'all'
  }
  refresh()
  // Deep link: ?id=<report-id> opens that report's detail straight away.
  const qid = route.query.id
  const id = Array.isArray(qid) ? qid[0] : qid
  if (typeof id === 'string' && id !== '') openDetail(id)
})
</script>
