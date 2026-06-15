<template>
  <!-- Phase 17 (UX-33) admin list view for editorial collections. -->
  <div class="min-h-screen bg-base pt-20">
    <div class="container mx-auto px-4 py-8 max-w-7xl">
      <!-- Header -->
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <div>
          <h1 class="text-3xl font-semibold text-white">{{ $t('admin.collections.title') }}</h1>
        </div>
        <div class="flex items-center gap-3">
          <router-link
            to="/admin/collections/new"
            class="px-4 py-2 rounded-md bg-cyan-500/80 hover:bg-cyan-500 text-white font-medium text-sm transition"
          >
            + {{ $t('admin.collections.createNew') }}
          </router-link>
        </div>
      </div>

      <!-- Error states -->
      <div v-if="error === '403'" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ $t('admin.recs.error403') }}</p>
      </div>
      <div v-else-if="error" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ error }}</p>
      </div>

      <!-- Loading -->
      <div v-if="isLoading" class="flex justify-center py-12">
        <Spinner size="lg" />
      </div>

      <!-- Empty state -->
      <div v-else-if="collections.length === 0" class="glass-card p-8 text-center text-white/60">
        <p>{{ $t('admin.collections.itemsEmpty') }}</p>
      </div>

      <!-- Table -->
      <div v-else class="glass-card overflow-x-auto">
        <table class="w-full text-sm text-white">
          <thead class="bg-black/40 backdrop-blur">
            <tr class="text-white/70 text-xs uppercase">
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.collections.tableTitle') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.collections.tableSlug') }}</th>
              <th scope="col" class="px-3 py-2 text-right">{{ $t('admin.collections.tableItems') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.collections.tableUpdated') }}</th>
              <th scope="col" class="px-3 py-2 text-right">{{ $t('admin.collections.tableActions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="c in collections"
              :key="c.id"
              class="border-t border-white/10 hover:bg-white/5"
            >
              <td class="px-3 py-2">
                <span>{{ c.title }}</span>
                <span
                  v-if="!c.published"
                  class="ml-2 px-2 py-0.5 rounded text-[10px] font-mono bg-warning/20 text-warning"
                >{{ $t('admin.collections.draftPill') }}</span>
              </td>
              <td class="px-3 py-2 font-mono text-white/70 text-xs">{{ c.slug }}</td>
              <td class="px-3 py-2 text-right font-mono">{{ c.item_count }}</td>
              <td class="px-3 py-2 text-white/60 text-xs">{{ formatRelative(c.updated_at) }}</td>
              <td class="px-3 py-2 text-right space-x-2">
                <router-link
                  :to="`/admin/collections/${c.id}`"
                  class="px-3 py-1 rounded bg-white/10 hover:bg-white/20 text-xs"
                >{{ $t('admin.collections.edit') }}</router-link>
                <button
                  type="button"
                  class="px-3 py-1 rounded bg-destructive/30 hover:bg-destructive/50 text-xs text-destructive"
                  @click="onDelete(c)"
                >{{ $t('admin.collections.delete') }}</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminApi, type Collection } from '@/api/client'
import { Spinner } from '@/components/ui'
import { useConfirm } from '@/composables/useConfirm'

const { t } = useI18n()
const { confirm } = useConfirm()

const collections = ref<Collection[]>([])
const isLoading = ref(true)
const error = ref<string | null>(null)

async function load() {
  isLoading.value = true
  error.value = null
  try {
    const resp = await adminApi.listCollections()
    const data = (resp.data && 'data' in (resp.data as object))
      ? ((resp.data as { data: Collection[] }).data)
      : (resp.data as Collection[])
    collections.value = Array.isArray(data) ? data : []
  } catch (e: unknown) {
    const err = e as { response?: { status?: number; data?: { error?: { message?: string } } }; message?: string }
    if (err.response?.status === 403) {
      error.value = '403'
    } else {
      error.value = err.response?.data?.error?.message || err.message || 'Failed to load'
    }
  } finally {
    isLoading.value = false
  }
}

async function onDelete(c: Collection) {
  if (!(await confirm({
    title: t('common.confirmTitle'),
    description: t('admin.collections.deleteConfirm'),
    confirmText: t('common.delete'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  }))) return
  try {
    await adminApi.deleteCollection(c.id)
    await load()
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: { message?: string } } }; message?: string }
    alert(err.response?.data?.error?.message || err.message || 'Delete failed')
  }
}

// Lightweight "X minutes/hours/days ago" formatter — avoids pulling in
// date-fns just for this view.
function formatRelative(iso: string): string {
  if (!iso) return ''
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const diffSec = Math.round((Date.now() - then) / 1000)
  if (diffSec < 60) return `${diffSec}s`
  const min = Math.round(diffSec / 60)
  if (min < 60) return `${min}m`
  const hr = Math.round(min / 60)
  if (hr < 48) return `${hr}h`
  const day = Math.round(hr / 24)
  return `${day}d`
}

onMounted(load)
</script>
