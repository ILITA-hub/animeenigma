<!-- frontend/web/src/views/MyFeedback.vue -->
<!-- User-facing "my feedback" page: every message the user sent through the
     feedback button / player report buttons, with its triage status mapped
     to friendly labels. Read-only mirror of the admin feedback archive,
     scoped server-side to the authenticated user (GET /api/users/reports). -->
<template>
  <div class="min-h-screen bg-background pt-20">
    <div class="container mx-auto px-4 py-8 max-w-3xl">
      <div class="flex items-center justify-between gap-3 flex-wrap mb-2">
        <h1 class="text-2xl font-bold text-foreground font-display">{{ $t('myFeedback.title') }}</h1>
        <span v-if="total > 0" class="text-sm text-muted-foreground">{{ $t('myFeedback.total', { n: total }) }}</span>
      </div>
      <p class="text-sm text-muted-foreground mb-4">{{ $t('myFeedback.subtitle') }}</p>

      <div class="flex items-end gap-3 flex-wrap mb-6">
        <div class="w-40">
          <Input v-model="dateFrom" size="sm" type="date" :label="$t('myFeedback.filter.from')" />
        </div>
        <div class="w-40">
          <Input v-model="dateTo" size="sm" type="date" :label="$t('myFeedback.filter.to')" />
        </div>
        <Button v-if="dateFrom || dateTo" variant="ghost" size="sm" class="mb-0.5" @click="resetDates">
          {{ $t('myFeedback.filter.reset') }}
        </Button>
      </div>

      <div v-if="loading" class="flex justify-center py-12">
        <Spinner size="lg" />
      </div>

      <div v-else-if="error" class="glass-card p-6 text-center">
        <p class="text-destructive text-sm">{{ error }}</p>
        <Button variant="secondary" size="sm" class="mt-3" @click="load(page)">{{ $t('myFeedback.retry') }}</Button>
      </div>

      <EmptyState v-else-if="items.length === 0">{{ $t('myFeedback.empty') }}</EmptyState>

      <template v-else>
        <div class="space-y-3">
          <div v-for="it in items" :key="it.id" class="glass-card p-4 md:p-6">
            <div class="flex items-center gap-2 flex-wrap">
              <Badge size="sm" variant="default">{{ categoryLabel(it.category) }}</Badge>
              <span class="text-xs text-muted-foreground">{{ formatDate(it.timestamp) }}</span>
              <Badge size="sm" :variant="statusVariant(it.status)" class="ml-auto">{{ statusLabel(it.status) }}</Badge>
            </div>
            <p v-if="it.anime_name" class="mt-2 text-xs text-primary">
              {{ it.anime_name }}<span v-if="it.episode_number != null"> · {{ $t('myFeedback.episode', { n: it.episode_number }) }}</span>
            </p>
            <p class="mt-2 text-sm text-foreground/90 whitespace-pre-wrap break-words">{{ it.description || $t('myFeedback.noText') }}</p>
            <p v-if="it.status_updated_at" class="mt-2 text-[11px] text-muted-foreground">
              {{ $t('myFeedback.statusUpdated', { date: formatDate(it.status_updated_at) }) }}
            </p>
            <details v-if="it.status_history?.length" class="mt-2">
              <summary class="cursor-pointer text-[11px] text-primary/80 hover:text-primary">
                {{ $t('myFeedback.history', { n: it.status_history.length }) }}
              </summary>
              <ol class="mt-1.5 space-y-1 border-l border-white/10 pl-3">
                <li v-for="(tr, i) in it.status_history" :key="i" class="text-[11px] text-muted-foreground">
                  <span class="text-foreground/70">{{ statusLabel(tr.from) }}</span>
                  <span aria-hidden="true"> → </span>
                  <span class="text-foreground/90">{{ statusLabel(tr.to) }}</span>
                  · {{ formatDate(tr.at) }}
                </li>
              </ol>
            </details>
          </div>
        </div>

        <div v-if="totalPages > 1" class="mt-6">
          <PaginationBar :current-page="page" :total-pages="totalPages" @update:current-page="load($event)" />
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { userApi } from '@/api/client'
import { dayStartISO, dayEndISO } from '@/utils/time'
import type { MyFeedbackItem, MyFeedbackResponse, FeedbackStatus } from '@/types/feedback'
import { Badge, Button, Spinner } from '@/components/ui'
import Input from '@/components/ui/Input.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import PaginationBar from '@/components/ui/PaginationBar.vue'

const { t, locale } = useI18n()

const dateFrom = ref('')
const dateTo = ref('')

const items = ref<MyFeedbackItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(true)
const error = ref('')

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

const STATUS_VARIANT: Record<FeedbackStatus, 'default' | 'primary' | 'success' | 'warning' | 'info'> = {
  new: 'info',
  in_progress: 'warning',
  ai_done: 'primary',
  resolved: 'success',
  not_relevant: 'default',
}

const statusVariant = (s: string) => STATUS_VARIANT[s as FeedbackStatus] ?? 'default'
const statusLabel = (s: string) =>
  ['new', 'in_progress', 'ai_done', 'resolved', 'not_relevant'].includes(s)
    ? t('myFeedback.status.' + s)
    : s
const categoryLabel = (c: string) =>
  ['bug', 'issue', 'feature'].includes(c) ? t('myFeedback.category.' + c) : t('myFeedback.category.other')

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const loc = locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US'
  return d.toLocaleString(loc, { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

async function load(p: number) {
  loading.value = true
  error.value = ''
  try {
    const resp = await userApi.listMyReports({
      page: p,
      page_size: pageSize.value,
      from: dayStartISO(dateFrom.value),
      to: dayEndISO(dateTo.value),
    })
    const data = (resp.data as { data?: MyFeedbackResponse }).data ?? (resp.data as MyFeedbackResponse)
    items.value = data.items ?? []
    total.value = data.total ?? 0
    page.value = data.page ?? p
  } catch {
    error.value = t('myFeedback.loadError')
  } finally {
    loading.value = false
  }
}

function resetDates() {
  dateFrom.value = ''
  dateTo.value = ''
}

watch([dateFrom, dateTo], () => load(1))

onMounted(() => load(1))
</script>
