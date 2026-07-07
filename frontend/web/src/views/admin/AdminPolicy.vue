<template>
  <!-- /admin/policy — RBAC-and-roulette P3 (Task 7): runtime "steering wheel"
       for the P1 policy-service / P2 gateway FeatureGate. Absorbs
       AdminSecretFeatures.vue's master roulette switch; per-flag rows now
       also edit role + per-user audience and preview live access. -->
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-6xl">
      <div class="mb-6">
        <h1 class="text-3xl font-semibold text-white">{{ $t('admin.policy.title') }}</h1>
        <p class="text-white/60 text-sm mt-1">{{ $t('admin.policy.subtitle') }}</p>
      </div>

      <Tabs v-model="activeTab" :tabs="tabDefs" variant="underline">
        <!-- ─── FEATURES TAB ──────────────────────────────────────────── -->
        <template #features>
          <div v-if="error === '403'" class="glass-card p-4 mb-6 border border-destructive/40">
            <p class="text-destructive">{{ $t('admin.policy.error403') }}</p>
          </div>
          <div v-else-if="error" class="glass-card p-4 mb-6 border border-destructive/40">
            <p class="text-destructive">{{ error }}</p>
          </div>

          <div v-if="isLoading" class="flex justify-center py-12">
            <Spinner size="lg" />
          </div>

          <template v-else>
            <!-- Master roulette switch -->
            <div class="glass-card p-4 md:p-6 mb-6 flex items-center justify-between gap-4">
              <div>
                <h2 class="text-base font-semibold text-white">{{ $t('admin.policy.master.label') }}</h2>
                <p class="text-white/60 text-sm mt-1">{{ $t('admin.policy.master.hint') }}</p>
              </div>
              <Switch
                :model-value="rouletteEnabled"
                :disabled="rouletteSaving"
                :aria-label="$t('admin.policy.master.label')"
                data-testid="master-roulette-switch"
                @update:model-value="onToggleRoulette"
              />
            </div>

            <EmptyState v-if="rows.length === 0" class="mb-8">
              {{ $t('admin.policy.noFeatures') }}
            </EmptyState>

            <div v-else class="grid gap-4 mb-8">
              <Card v-for="row in rows" :key="row.key" padding="none" data-testid="flag-card">
                <CardHeader class="flex flex-row flex-wrap items-start justify-between gap-3">
                  <div>
                    <CardTitle class="text-base">{{ row.label }}</CardTitle>
                    <p class="mt-1 flex flex-wrap items-center gap-2 text-xs text-white/40">
                      <span class="font-mono">{{ row.key }}</span>
                      <a
                        v-if="featureRoute(row.key)"
                        :href="featureRoute(row.key)"
                        target="_blank"
                        rel="noopener noreferrer"
                        class="inline-flex items-center gap-1 text-brand-cyan hover:underline"
                        :aria-label="$t('admin.policy.openAria', { label: row.label })"
                        @click="openFeature($event, featureRoute(row.key)!)"
                      >
                        {{ $t('admin.policy.openLabel') }}
                        <ExternalLink class="size-3" aria-hidden="true" />
                      </a>
                    </p>
                  </div>
                  <div class="flex items-center gap-3">
                    <Badge
                      v-if="row.failSafe === 'admin'"
                      class="bg-brand-violet/20 text-brand-violet"
                      data-testid="failsafe-badge"
                    >
                      {{ $t('admin.policy.failSafe.admin') }}
                    </Badge>
                    <Badge v-else variant="success" data-testid="failsafe-badge">
                      {{ $t('admin.policy.failSafe.everyone') }}
                    </Badge>
                    <Switch
                      :model-value="row.roulette"
                      :disabled="row.saving"
                      :aria-label="$t('admin.policy.rouletteToggleLabel')"
                      :data-testid="`roulette-switch-${row.key}`"
                      @update:model-value="(v: boolean) => (row.roulette = v)"
                    />
                  </div>
                </CardHeader>

                <CardContent class="pt-0">
                  <!-- Roles -->
                  <div class="mb-4">
                    <p class="text-xs uppercase tracking-wide text-white/50 mb-2">
                      {{ $t('admin.policy.rolesLabel') }}
                    </p>
                    <div class="flex flex-wrap gap-2">
                      <Chip
                        v-for="role in ROLE_OPTIONS"
                        :key="role"
                        :active="row.roles.includes(role)"
                        size="sm"
                        :data-testid="`role-chip-${row.key}-${role}`"
                        @click="toggleRole(row, role)"
                      >
                        {{ $t(`admin.policy.roles.${role}`) }}
                      </Chip>
                    </div>
                  </div>

                  <!-- Allow / deny lists -->
                  <div class="grid gap-4 sm:grid-cols-2 mb-4">
                    <div>
                      <p class="text-xs uppercase tracking-wide text-white/50 mb-1">
                        {{ $t('admin.policy.allow.label') }}
                      </p>
                      <p class="text-white/40 text-xs mb-2">{{ $t('admin.policy.allow.hint') }}</p>
                      <div class="flex flex-wrap gap-2 mb-2">
                        <Chip
                          v-for="id in row.allowUsers"
                          :key="id"
                          removable
                          size="sm"
                          @remove="removeAllowUser(row, id)"
                        >
                          {{ chipLabel(row.allowUserMap, id) }}
                        </Chip>
                      </div>
                      <UserResolveInput mode="chip" @resolve="(u) => addAllowUser(row, u)" />
                    </div>
                    <div>
                      <p class="text-xs uppercase tracking-wide text-white/50 mb-1">
                        {{ $t('admin.policy.deny.label') }}
                      </p>
                      <p class="text-white/40 text-xs mb-2">{{ $t('admin.policy.deny.hint') }}</p>
                      <div class="flex flex-wrap gap-2 mb-2">
                        <Chip
                          v-for="id in row.denyUsers"
                          :key="id"
                          removable
                          size="sm"
                          @remove="removeDenyUser(row, id)"
                        >
                          {{ chipLabel(row.denyUserMap, id) }}
                        </Chip>
                      </div>
                      <UserResolveInput mode="chip" @resolve="(u) => addDenyUser(row, u)" />
                    </div>
                  </div>

                  <div class="flex justify-end">
                    <Button
                      size="sm"
                      :loading="row.saving"
                      :disabled="row.saving || !isDirty(row)"
                      :data-testid="`save-button-${row.key}`"
                      @click="saveRow(row)"
                    >
                      {{ $t('admin.policy.save') }}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            </div>

            <!-- Access preview -->
            <div class="glass-card p-4 md:p-6">
              <h2 class="text-base font-semibold text-white mb-1">{{ $t('admin.policy.preview.title') }}</h2>
              <p class="text-white/60 text-sm mb-4">{{ $t('admin.policy.preview.subtitle') }}</p>

              <div class="flex flex-wrap items-center gap-4 mb-4">
                <SegmentedControl
                  v-model="previewIdentity"
                  :options="identityOptions"
                  :aria-label="$t('admin.policy.preview.identityLabel')"
                />
                <div class="flex items-center gap-2">
                  <UserResolveInput mode="chip" @resolve="setPreviewUser" />
                  <Chip v-if="previewUser" removable size="sm" @remove="clearPreviewUser">
                    {{ previewUser.username }}
                  </Chip>
                </div>
              </div>

              <ul class="grid gap-2 sm:grid-cols-2">
                <li
                  v-for="result in previewResults"
                  :key="result.key"
                  class="flex items-center justify-between gap-2 rounded-lg bg-white/5 px-3 py-2 text-sm"
                  :data-testid="`preview-result-${result.key}`"
                >
                  <span class="text-white/80">{{ result.label }}</span>
                  <Badge :variant="result.visible ? 'success' : 'default'">
                    {{ result.visible ? $t('admin.policy.preview.visible') : $t('admin.policy.preview.hidden') }}
                  </Badge>
                </li>
              </ul>
            </div>
          </template>
        </template>

        <!-- ─── PROVIDERS TAB (placeholder — P4) ──────────────────────── -->
        <template #providers>
          <EmptyState
            :title="$t('admin.policy.providersPlaceholder.title')"
            :description="$t('admin.policy.providersPlaceholder.description')"
          />
        </template>
      </Tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ExternalLink } from 'lucide-vue-next'
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Chip,
  EmptyState,
  SegmentedControl,
  Spinner,
  Switch,
  Tabs,
} from '@/components/ui'
import UserResolveInput from '@/components/admin/UserResolveInput.vue'
import { useAdminPolicy } from '@/composables/useAdminPolicy'
import type { FeatureFlag, FailSafe } from '@/composables/useAdminPolicy'
import { useOpenFeature } from '@/composables/useOpenFeature'
import { featureRoute } from '@/config/policyFeatures'
import { adminApi, type ResolvedUser } from '@/api/client'
import { useToast } from '@/composables/useToast'

// Reserved master key: collapsed into `rouletteEnabled` by the backend and
// excluded from the rendered flag roster (per §B3 of the design spec).
const RESERVED_MASTER_KEY = '__roulette__'

const ROLE_OPTIONS = ['admin', 'user', 'everyone'] as const
type RoleOption = (typeof ROLE_OPTIONS)[number]

const PREVIEW_IDENTITIES = ['anonymous', 'guest', 'user', 'admin'] as const
type PreviewIdentity = (typeof PREVIEW_IDENTITIES)[number]

interface FlagAudience {
  roles: string[]
  roulette: boolean
  allowUsers: string[]
  denyUsers: string[]
}

interface FlagRow extends FlagAudience {
  key: string
  label: string
  failSafe: FailSafe
  allowUserMap: Record<string, string>
  denyUserMap: Record<string, string>
  saving: boolean
  original: FlagAudience
}

const { t } = useI18n()
const policy = useAdminPolicy()
const { openFeature } = useOpenFeature()
const toast = useToast()

const activeTab = ref('features')
const tabDefs = computed(() => [
  { value: 'features', label: t('admin.policy.tabs.features') },
  { value: 'providers', label: t('admin.policy.tabs.providers') },
])

const isLoading = ref(true)
const error = ref<string | null>(null)
const rouletteEnabled = ref(false)
const rouletteSaving = ref(false)
const rows = ref<FlagRow[]>([])

function handleError(e: unknown): void {
  const err = e as { response?: { status?: number; data?: { error?: { message?: string } } }; message?: string }
  error.value = err.response?.status === 403
    ? '403'
    : (err.response?.data?.error?.message || err.message || 'Failed')
}

// Standard resolver envelope is {success,data} (mirrors UserResolveInput.vue's
// own unwrap) — a deleted/unresolvable user falls back to the raw id so a
// stale allow/deny entry never breaks the page.
function unwrapUser(data: unknown): ResolvedUser {
  const d = data as { data?: ResolvedUser }
  return d && typeof d === 'object' && 'data' in d ? (d.data as ResolvedUser) : (data as ResolvedUser)
}

async function resolveUsernames(ids: string[]): Promise<Record<string, string>> {
  const entries = await Promise.all(ids.map(async (id): Promise<[string, string]> => {
    try {
      const res = await adminApi.resolveUser(id)
      return [id, unwrapUser(res.data).username]
    } catch {
      return [id, id]
    }
  }))
  return Object.fromEntries(entries)
}

function snapshotAudience(source: FlagAudience): FlagAudience {
  return {
    roles: [...source.roles],
    roulette: source.roulette,
    allowUsers: [...source.allowUsers],
    denyUsers: [...source.denyUsers],
  }
}

async function buildRow(flag: FeatureFlag): Promise<FlagRow> {
  const [allowUserMap, denyUserMap] = await Promise.all([
    resolveUsernames(flag.allowUsers),
    resolveUsernames(flag.denyUsers),
  ])
  return {
    key: flag.key,
    label: flag.label,
    failSafe: flag.failSafe,
    roles: [...flag.roles],
    roulette: flag.roulette,
    allowUsers: [...flag.allowUsers],
    denyUsers: [...flag.denyUsers],
    allowUserMap,
    denyUserMap,
    saving: false,
    original: snapshotAudience(flag),
  }
}

async function load(): Promise<void> {
  isLoading.value = true
  error.value = null
  try {
    const res = await policy.list()
    rouletteEnabled.value = res.rouletteEnabled
    const nonMaster = res.flags.filter((f) => f.key !== RESERVED_MASTER_KEY)
    rows.value = await Promise.all(nonMaster.map(buildRow))
  } catch (e) {
    handleError(e)
  } finally {
    isLoading.value = false
  }
}

async function onToggleRoulette(enabled: boolean): Promise<void> {
  const previous = rouletteEnabled.value
  rouletteEnabled.value = enabled
  rouletteSaving.value = true
  try {
    await policy.setRoulette(enabled)
    toast.push(t('admin.policy.toastRouletteSuccess'), 'success')
  } catch {
    rouletteEnabled.value = previous
    toast.push(t('admin.policy.toastRouletteError'), 'error')
  } finally {
    rouletteSaving.value = false
  }
}

function sameMembers(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  const sa = [...a].sort()
  const sb = [...b].sort()
  return sa.every((v, i) => v === sb[i])
}

function isDirty(row: FlagRow): boolean {
  return row.roulette !== row.original.roulette
    || !sameMembers(row.roles, row.original.roles)
    || !sameMembers(row.allowUsers, row.original.allowUsers)
    || !sameMembers(row.denyUsers, row.original.denyUsers)
}

function toggleRole(row: FlagRow, role: RoleOption): void {
  const idx = row.roles.indexOf(role)
  if (idx >= 0) row.roles.splice(idx, 1)
  else row.roles.push(role)
}

function chipLabel(map: Record<string, string>, id: string): string {
  const username = map[id] ?? id
  return `${username} #${id.slice(0, 8)}`
}

function addAllowUser(row: FlagRow, user: ResolvedUser): void {
  if (!row.allowUsers.includes(user.id)) row.allowUsers.push(user.id)
  row.allowUserMap[user.id] = user.username
}

function removeAllowUser(row: FlagRow, id: string): void {
  row.allowUsers = row.allowUsers.filter((u) => u !== id)
}

function addDenyUser(row: FlagRow, user: ResolvedUser): void {
  if (!row.denyUsers.includes(user.id)) row.denyUsers.push(user.id)
  row.denyUserMap[user.id] = user.username
}

function removeDenyUser(row: FlagRow, id: string): void {
  row.denyUsers = row.denyUsers.filter((u) => u !== id)
}

async function saveRow(row: FlagRow): Promise<void> {
  row.saving = true
  try {
    await policy.setFlag(row.key, {
      roles: [...row.roles],
      allowUsers: [...row.allowUsers],
      denyUsers: [...row.denyUsers],
      roulette: row.roulette,
      failSafe: row.failSafe,
      label: row.label,
    })
    row.original = snapshotAudience(row)
    toast.push(t('admin.policy.toastSaveSuccess', { label: row.label }), 'success')
  } catch {
    toast.push(t('admin.policy.toastSaveError', { label: row.label }), 'error')
  } finally {
    row.saving = false
  }
}

// ─── Access preview (§B5) — client-side mirror of the P1 canAccess order:
// guest -> deny · deny-list -> deny · allow-list -> allow · everyone -> allow
// · role -> allow · else deny. Pure preview, no writes.
const previewIdentity = ref<PreviewIdentity>('anonymous')
const previewUser = ref<ResolvedUser | null>(null)

const identityOptions = computed(() =>
  PREVIEW_IDENTITIES.map((id) => ({ value: id, label: t(`admin.policy.preview.identity.${id}`) })),
)

function setPreviewUser(user: ResolvedUser): void {
  previewUser.value = user
}

function clearPreviewUser(): void {
  previewUser.value = null
}

function canAccess(row: FlagRow, identity: PreviewIdentity, userId?: string): boolean {
  if (identity === 'guest') return false
  if (userId && row.denyUsers.includes(userId)) return false
  if (userId && row.allowUsers.includes(userId)) return true
  if (row.roles.includes('everyone')) return true
  if (identity !== 'anonymous' && row.roles.includes(identity)) return true
  return false
}

const previewResults = computed(() =>
  rows.value.map((row) => ({
    key: row.key,
    label: row.label,
    visible: canAccess(row, previewIdentity.value, previewUser.value?.id),
  })),
)

onMounted(load)
</script>
