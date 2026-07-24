import { ref } from 'vue'
import { adminApi, type AdminUser, type AdminUsersListResponse } from '@/api/client'

function unwrap<T>(data: unknown): T {
  const d = data as { data?: T }
  return d && typeof d === 'object' && 'data' in d ? (d.data as T) : (data as T)
}

function mapErr(e: unknown): string {
  const obj = e as { response?: { status?: number; data?: { error?: { message?: string } } }; message?: string }
  if (obj?.response?.status === 403) return '403'
  return obj?.response?.data?.error?.message || obj?.message || 'admin.users.errorGeneric'
}

export function useAdminUsers() {
  const items = ref<AdminUser[]>([])
  const total = ref(0)
  const page = ref(1)
  const pageSize = ref(25)
  const isLoading = ref(false)
  const error = ref<string | null>(null)

  const query = ref('')
  // 'all' is the "no filter" sentinel — reka-ui Select forbids empty-string values.
  const roleFilter = ref('all')

  async function refresh(): Promise<void> {
    isLoading.value = true
    error.value = null
    try {
      const res = await adminApi.listUsers({
        q: query.value.trim() || undefined,
        role: roleFilter.value && roleFilter.value !== 'all' ? roleFilter.value : undefined,
        page: page.value,
        page_size: pageSize.value,
      })
      const env = unwrap<AdminUsersListResponse>(res.data)
      items.value = env.items ?? []
      total.value = env.total ?? 0
      page.value = env.page ?? 1
      pageSize.value = env.page_size ?? pageSize.value
    } catch (e: unknown) {
      error.value = mapErr(e)
      items.value = []
      total.value = 0
    } finally {
      isLoading.value = false
    }
  }

  // applyFilters resets to page 1 and reloads — call from filter @change / search.
  function applyFilters(): Promise<void> {
    page.value = 1
    return refresh()
  }

  function setPage(p: number): Promise<void> {
    if (p < 1) return Promise.resolve()
    page.value = p
    return refresh()
  }

  // changeRole calls the API and swaps the updated row in place. Throws on
  // failure so the caller can surface the error + refresh.
  async function changeRole(id: string, role: string): Promise<void> {
    const res = await adminApi.updateUserRole(id, role)
    const updated = unwrap<AdminUser>(res.data)
    const idx = items.value.findIndex((u) => u.id === id)
    if (idx !== -1 && updated) items.value[idx] = updated
  }

  return { items, total, page, pageSize, isLoading, error, query, roleFilter, refresh, applyFilters, setPage, changeRole }
}
