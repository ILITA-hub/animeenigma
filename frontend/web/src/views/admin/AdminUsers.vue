<template>
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-7xl">
      <!-- Header -->
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <h1 class="text-3xl font-semibold text-white">{{ $t('admin.users.title') }}</h1>
        <span v-if="!isLoading && error !== '403'" class="text-sm text-white/50">
          {{ $t('admin.users.totalCount', { count: total }) }}
        </span>
      </div>

      <!-- Filters -->
      <div class="flex flex-wrap items-end gap-3 mb-6">
        <div class="flex-1 min-w-[220px]">
          <Input
            v-model="query"
            size="sm"
            type="search"
            clearable
            :label="$t('admin.users.searchLabel')"
            :placeholder="$t('admin.users.searchPlaceholder')"
          />
        </div>
        <Select
          v-model="roleFilter"
          size="sm"
          :options="roleFilterOptions"
          :label="$t('admin.users.roleFilter')"
          @change="applyFilters"
        />
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

      <!-- Empty -->
      <div v-else-if="items.length === 0" class="glass-card p-8 text-center text-white/60">
        <p>{{ $t('admin.users.empty') }}</p>
      </div>

      <!-- Table -->
      <div v-else class="glass-card overflow-x-auto">
        <table class="w-full text-sm text-white">
          <thead class="bg-black/40 backdrop-blur">
            <tr class="text-white/70 text-xs uppercase">
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.users.colUser') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.users.colPublicId') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.users.colRole') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.users.colTelegram') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.users.colJoined') }}</th>
              <th scope="col" class="px-3 py-2 text-right">{{ $t('admin.users.colActions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="u in items" :key="u.id" class="border-t border-white/10 hover:bg-white/5">
              <td class="px-3 py-2">
                <div class="flex items-center gap-2">
                  <Avatar :src="u.avatar" :name="u.username" size="sm" />
                  <span>{{ u.username }}</span>
                </div>
              </td>
              <td class="px-3 py-2 font-mono text-white/70 text-xs">{{ u.public_id }}</td>
              <td class="px-3 py-2">
                <div class="flex items-center gap-2">
                  <Badge :variant="roleBadgeVariant(u.role)" size="sm">{{ roleLabel(u.role) }}</Badge>
                  <Select
                    :model-value="u.role"
                    size="xs"
                    :options="assignableRoleOptions"
                    :disabled="u.id === myId"
                    :aria-label="$t('admin.users.changeRoleAria', { user: u.username })"
                    @change="(v) => onRoleChange(u, String(v))"
                  />
                </div>
              </td>
              <td class="px-3 py-2 text-white/70 text-xs">
                <template v-if="u.telegram_id">
                  <span class="font-mono">{{ u.telegram_id }}</span>
                  <span v-if="tgName(u)" class="text-white/50"> · {{ tgName(u) }}</span>
                </template>
                <span v-else class="text-white/30">—</span>
              </td>
              <td class="px-3 py-2 text-white/60 text-xs">{{ formatDate(u.created_at) }}</td>
              <td class="px-3 py-2 text-right whitespace-nowrap">
                <Button
                  v-if="u.public_id"
                  :href="`/user/${u.public_id}`"
                  target="_blank"
                  rel="noopener noreferrer"
                  variant="soft"
                  size="xs"
                  :aria-label="$t('admin.users.openProfileAria', { user: u.username })"
                >
                  <template #icon><ExternalLink /></template>
                  {{ $t('admin.users.openProfile') }}
                </Button>
                <span v-else class="text-white/30">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Pagination -->
      <div class="mt-6 flex justify-center">
        <PaginationBar :current-page="page" :total-pages="totalPages" @update:current-page="setPage" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ExternalLink } from 'lucide-vue-next'
import Input from '@/components/ui/Input.vue'
import Select from '@/components/ui/Select.vue'
import { Spinner, Badge, Avatar, PaginationBar, Button } from '@/components/ui'
import { useConfirm } from '@/composables/useConfirm'
import { useAdminUsers } from '@/composables/useAdminUsers'
import { useAuthStore } from '@/stores/auth'
import type { AdminUser } from '@/api/client'

const { t } = useI18n()
const { confirm } = useConfirm()
const auth = useAuthStore()
const myId = computed(() => auth.user?.id)

const {
  items, total, page, pageSize, isLoading, error,
  query, roleFilter,
  refresh, applyFilters, setPage, changeRole,
} = useAdminUsers()

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

const roleFilterOptions = computed(() => [
  { value: 'all', label: t('admin.users.roleAll') },
  { value: 'user', label: t('admin.users.roleUser') },
  { value: 'librarian', label: t('admin.users.roleLibrarian') },
  { value: 'admin', label: t('admin.users.roleAdmin') },
])
const assignableRoleOptions = computed(() => [
  { value: 'user', label: t('admin.users.roleUser') },
  { value: 'librarian', label: t('admin.users.roleLibrarian') },
  { value: 'admin', label: t('admin.users.roleAdmin') },
])

function roleLabel(role: string): string {
  const key = `admin.users.role${role.charAt(0).toUpperCase()}${role.slice(1)}`
  const label = t(key)
  return label === key ? role : label
}
function roleBadgeVariant(role: string): 'primary' | 'warning' | 'default' {
  if (role === 'admin') return 'primary'
  if (role === 'librarian') return 'warning'
  return 'default'
}
function tgName(u: AdminUser): string {
  return u.telegram_username || u.telegram_first_name || ''
}
function formatDate(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString()
}

async function onRoleChange(u: AdminUser, role: string) {
  if (role === u.role) return
  const ok = await confirm({
    title: t('admin.users.confirmRoleTitle'),
    description: t('admin.users.confirmRoleDesc', { user: u.username, role: roleLabel(role) }),
    confirmText: t('admin.users.confirmRoleConfirm'),
    cancelText: t('common.cancel'),
  })
  if (!ok) {
    // Reset the Select's optimistic value by re-fetching the current rows.
    await refresh()
    return
  }
  try {
    await changeRole(u.id, role)
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: { message?: string } } }; message?: string }
    error.value = err.response?.data?.error?.message || err.message || t('admin.users.errorGeneric')
    await refresh()
  }
}

// Debounce free-text search (300ms) — mirrors AdminFeedback.
let searchDebounce: ReturnType<typeof setTimeout> | null = null
watch(query, () => {
  if (searchDebounce) clearTimeout(searchDebounce)
  searchDebounce = setTimeout(() => applyFilters(), 300)
})
onUnmounted(() => {
  if (searchDebounce) clearTimeout(searchDebounce)
})

onMounted(refresh)
</script>
