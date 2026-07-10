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
                <!-- Compact one-line top row: name · key/open · audience · Save.
                     Audience badge is derived live from the (mutable) roles
                     draft; failSafe is preserved in the payload but no longer
                     surfaced (it's an outage-only lever, not a per-edit knob). -->
                <CardHeader class="flex flex-row flex-wrap items-center justify-between gap-x-3 gap-y-2">
                  <div class="flex flex-wrap items-center gap-x-3 gap-y-1 min-w-0">
                    <CardTitle class="text-base">{{ row.label }}</CardTitle>
                    <span class="inline-flex items-center gap-2 text-xs text-white/40">
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
                    </span>
                  </div>
                  <div class="flex items-center gap-2 shrink-0">
                    <Badge :variant="AUDIENCE_VARIANT[audienceKind(row.roles)]" data-testid="audience-badge">
                      {{ $t(`admin.policy.audience.${audienceKind(row.roles)}`) }}
                    </Badge>
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
                      <!-- Roulette membership rendered as a distinct "role" chip
                           (owner ask): toggles the same `roulette` bool the old
                           per-flag Switch drove; Save persists it. -->
                      <Chip
                        :active="row.roulette"
                        size="sm"
                        class="gap-1"
                        :data-testid="`roulette-chip-${row.key}`"
                        @click="row.roulette = !row.roulette"
                      >
                        <Sparkles class="size-3" aria-hidden="true" />
                        {{ $t('admin.policy.rouletteRole') }}
                      </Chip>
                    </div>
                  </div>

                  <!-- Allow / deny lists -->
                  <div class="grid gap-4 sm:grid-cols-2">
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
                </CardContent>
              </Card>
            </div>

            <!-- Access preview -->
            <div class="glass-card p-4 md:p-6">
              <h2 class="text-base font-semibold text-white mb-1">{{ $t('admin.policy.preview.title') }}</h2>
              <p class="text-white/60 text-sm mb-4">{{ $t('admin.policy.preview.subtitle') }}</p>

              <div class="flex flex-wrap items-center gap-4 mb-2">
                <!-- When a specific user is picked the check evaluates THAT
                     user (real role + overrides), so the hypothetical-identity
                     control is inert — grey it out to signal that. -->
                <SegmentedControl
                  v-model="previewIdentity"
                  :options="identityOptions"
                  :aria-label="$t('admin.policy.preview.identityLabel')"
                  :class="previewUser ? 'opacity-40 pointer-events-none' : ''"
                />
                <div class="flex items-center gap-2">
                  <span class="text-xs text-white/50">{{ $t('admin.policy.preview.userLabel') }}</span>
                  <UserResolveInput
                    mode="chip"
                    :aria-label="$t('admin.policy.preview.userLabel')"
                    @resolve="setPreviewUser"
                  />
                  <Chip
                    v-if="previewUser"
                    removable
                    size="sm"
                    data-testid="preview-user-chip"
                    @remove="clearPreviewUser"
                  >
                    {{ previewUser.username }}<span class="opacity-60"> · {{ previewRoleLabel }}</span>
                  </Chip>
                </div>
              </div>
              <p v-if="previewUser" class="text-xs text-white/50 mb-4" data-testid="preview-user-hint">
                {{ $t('admin.policy.preview.checkingUser', { username: previewUser.username, role: previewRoleLabel }) }}
              </p>

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

        <!-- ─── PROVIDERS TAB (P5 Task 2) ──────────────────────────────
             Facade over catalog's stream_providers table (Task 1): list +
             set policy to any of auto/manual/disabled (all three are admin
             levers — nothing is machine-set). Health is probe-owned — this
             tab does NOT touch it. -->
        <template #providers>
          <div v-if="providersError === '403'" class="glass-card p-4 mb-6 border border-destructive/40">
            <p class="text-destructive">{{ $t('admin.policy.error403') }}</p>
          </div>
          <div v-else-if="providersError" class="glass-card p-4 mb-6 border border-destructive/40">
            <p class="text-destructive">{{ providersError }}</p>
          </div>

          <div v-if="isProvidersLoading" class="flex justify-center py-12">
            <Spinner size="lg" />
          </div>

          <template v-else>
            <div class="mb-6">
              <h2 class="text-base font-semibold text-white">{{ $t('admin.policy.providers.title') }}</h2>
              <p class="text-white/60 text-sm mt-1">{{ $t('admin.policy.providers.intro') }}</p>
            </div>

            <EmptyState
              v-if="providerRows.length === 0"
              :title="$t('admin.policy.providersPlaceholder.title')"
              :description="$t('admin.policy.providersPlaceholder.description')"
            />

            <div v-else class="grid gap-4">
              <Card v-for="row in providerRows" :key="row.name" padding="none" data-testid="provider-card">
                <CardHeader class="flex flex-row flex-wrap items-start justify-between gap-3">
                  <div>
                    <CardTitle class="text-base font-mono">{{ row.name }}</CardTitle>
                    <p class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-white/40">
                      <span>{{ $t('admin.policy.providers.groupLabel') }}: {{ row.group }}</span>
                      <span>{{ $t('admin.policy.providers.engineLabel') }}: {{ row.engine }}</span>
                    </p>
                    <p v-if="row.reason" class="mt-1 text-xs text-white/60">{{ row.reason }}</p>
                    <p v-if="row.description" class="mt-1 text-xs text-white/50">{{ row.description }}</p>
                  </div>
                  <div class="flex items-center gap-3">
                    <Badge :variant="stateVariant(row.derived_state)" :data-testid="`provider-state-${row.name}`">
                      {{ $t(`admin.policy.providers.state.${row.derived_state}`) }}
                    </Badge>
                    <SegmentedControl
                      :model-value="row.policy"
                      :options="providerPolicyOptions"
                      :aria-label="$t('admin.policy.providers.toggleLabel', { name: row.name })"
                      :class="row.saving ? 'opacity-40 pointer-events-none' : ''"
                      :data-testid="`provider-policy-${row.name}`"
                      @update:model-value="(v: string) => onSelectProviderPolicy(row, v as ScraperProviderPolicy)"
                    />
                  </div>
                </CardHeader>
              </Card>
            </div>
          </template>
        </template>

        <!-- ─── MAINTENANCE TAB ─────────────────────────────────────────────
             Pull-config control board: pause/resume each routine + tune safe
             knobs + read last-run status. Host routines' toggles are a PAUSE
             (the systemd timer still fires; the script early-exits). -->
        <template #maintenance>
          <div v-if="maintError === '403'" class="glass-card p-4 mb-6 border border-destructive/40">
            <p class="text-destructive">{{ $t('admin.policy.error403') }}</p>
          </div>
          <div v-else-if="maintError" class="glass-card p-4 mb-6 border border-destructive/40">
            <p class="text-destructive">{{ maintError }}</p>
          </div>

          <div v-if="isMaintLoading" class="flex justify-center py-12">
            <Spinner size="lg" />
          </div>

          <template v-else>
            <div class="mb-6">
              <h2 class="text-base font-semibold text-white">{{ $t('admin.policy.maintenance.title') }}</h2>
              <p class="text-white/60 text-sm mt-1">{{ $t('admin.policy.maintenance.intro') }}</p>
            </div>

            <EmptyState v-if="maintRows.length === 0" class="mb-8">
              {{ $t('admin.policy.maintenance.loadError') }}
            </EmptyState>

            <div v-for="group in maintGroups" :key="group.key" class="mb-8">
              <p class="text-xs uppercase tracking-wide text-white/50 mb-3">{{ $t(group.titleKey) }}</p>
              <div class="grid gap-4">
                <Card v-for="row in group.rows" :key="row.id" padding="none" data-testid="routine-card">
                  <CardHeader class="flex flex-row flex-wrap items-start justify-between gap-3">
                    <div class="min-w-0">
                      <CardTitle class="text-base">{{ routineName(row.id) }}</CardTitle>
                      <p class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-white/40">
                        <span class="font-mono">{{ row.id }}</span>
                      </p>
                      <p class="mt-2 flex flex-wrap items-center gap-2 text-xs text-white/60">
                        <Badge :variant="statusVariant(row)" :data-testid="`routine-status-${row.id}`">
                          {{ statusLabel(row) }}
                        </Badge>
                        <span v-if="row.lastSummary">{{ row.lastSummary }}</span>
                      </p>
                    </div>
                    <div class="flex items-center gap-3 shrink-0">
                      <span class="text-xs text-white/60">
                        {{ row.enabled ? $t('admin.policy.maintenance.enabled') : $t('admin.policy.maintenance.paused') }}
                      </span>
                      <Switch
                        :model-value="row.enabled"
                        :disabled="row.saving"
                        :aria-label="$t('admin.policy.maintenance.toggleLabel', { name: routineName(row.id) })"
                        :data-testid="`routine-switch-${row.id}`"
                        @update:model-value="(v: boolean) => onToggleRoutine(row, v)"
                      />
                    </div>
                  </CardHeader>

                  <CardContent v-if="row.enabled && (routineDescriptor(row.id)?.knobs.length ?? 0) > 0" class="pt-0">
                    <div class="grid gap-4 sm:grid-cols-2">
                      <div v-for="knob in routineDescriptor(row.id)!.knobs" :key="knob.key">
                        <p class="text-xs uppercase tracking-wide text-white/50 mb-2">{{ $t(knob.labelKey) }}</p>

                        <SegmentedControl
                          v-if="knob.type === 'select'"
                          :model-value="String(row.draft[knob.key] ?? '')"
                          :options="segOptions(knob.options)"
                          :aria-label="$t(knob.labelKey)"
                          @update:model-value="(v: string) => (row.draft[knob.key] = v)"
                        />

                        <Input
                          v-else-if="knob.type === 'number'"
                          type="number"
                          :min="knob.min"
                          :max="knob.max"
                          :model-value="String(row.draft[knob.key] ?? '')"
                          :aria-label="$t(knob.labelKey)"
                          @update:model-value="(v: string | number) => (row.draft[knob.key] = v)"
                        />

                        <div v-else-if="knob.type === 'chips'">
                          <div class="flex flex-wrap gap-2 mb-2">
                            <Chip
                              v-for="a in alertList(row)"
                              :key="a"
                              removable
                              size="sm"
                              @remove="removeAlert(row, a)"
                            >
                              {{ a }}
                            </Chip>
                          </div>
                          <Input
                            v-model="alertDraft[row.id]"
                            type="text"
                            :placeholder="$t(knob.placeholderKey)"
                            :aria-label="$t(knob.labelKey)"
                            @keyup.enter="addAlert(row)"
                          />
                        </div>
                      </div>
                    </div>

                    <div class="mt-4 flex justify-end">
                      <Button
                        size="sm"
                        :loading="row.saving"
                        :disabled="row.saving || !isKnobDirty(row)"
                        :data-testid="`routine-save-${row.id}`"
                        @click="saveKnobs(row)"
                      >
                        {{ $t('admin.policy.save') }}
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              </div>
            </div>
          </template>
        </template>
      </Tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ExternalLink, Sparkles } from 'lucide-vue-next'
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Chip,
  EmptyState,
  Input,
  SegmentedControl,
  Spinner,
  Switch,
  Tabs,
} from '@/components/ui'
import UserResolveInput from '@/components/admin/UserResolveInput.vue'
import { useAdminPolicy } from '@/composables/useAdminPolicy'
import type { FeatureFlag, FailSafe } from '@/composables/useAdminPolicy'
import { useAdminProviders } from '@/composables/useAdminProviders'
import type { ScraperProviderPolicy, ScraperProviderWire } from '@/composables/useAdminProviders'
import { useAdminMaintenance } from '@/composables/useAdminMaintenance'
import type { MaintenanceRoutineWire } from '@/composables/useAdminMaintenance'
import { MAINTENANCE_ROUTINES, routineDescriptor } from '@/config/maintenanceRoutines'
import { useOpenFeature } from '@/composables/useOpenFeature'
import { useConfirm } from '@/composables/useConfirm'
import { featureRoute } from '@/config/policyFeatures'
import { adminApi, type ResolvedUser } from '@/api/client'
import { useToast } from '@/composables/useToast'
import type { BadgeVariants } from '@/components/ui/badge-variants'

// Reserved master key: collapsed into `rouletteEnabled` by the backend and
// excluded from the rendered flag roster (per §B3 of the design spec).
const RESERVED_MASTER_KEY = '__roulette__'

const ROLE_OPTIONS = ['admin', 'user', 'everyone'] as const
type RoleOption = (typeof ROLE_OPTIONS)[number]

// Effective-audience summary badge (replaces the old read-only failSafe badge).
// Derived live from the mutable roles draft so it updates as chips toggle. The
// broadest matching role wins: everyone > any signed-in (user) > admin-only;
// an empty/allow-list-only rule reads "restricted".
type AudienceKind = 'everyone' | 'signedIn' | 'adminOnly' | 'restricted'

function audienceKind(roles: string[]): AudienceKind {
  if (roles.includes('everyone')) return 'everyone'
  if (roles.includes('user')) return 'signedIn'
  if (roles.includes('admin')) return 'adminOnly'
  return 'restricted'
}

const AUDIENCE_VARIANT: Record<AudienceKind, NonNullable<BadgeVariants['variant']>> = {
  everyone: 'success',
  signedIn: 'primary',
  adminOnly: 'info',
  restricted: 'default',
}

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
const providers = useAdminProviders()
const { openFeature } = useOpenFeature()
const { confirm } = useConfirm()
const toast = useToast()

const activeTab = ref('features')
const tabDefs = computed(() => [
  { value: 'features', label: t('admin.policy.tabs.features') },
  { value: 'providers', label: t('admin.policy.tabs.providers') },
  { value: 'maintenance', label: t('admin.policy.tabs.maintenance') },
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
  // Snapshot the exact payload BEFORE the await — if the admin edits the row
  // while the PUT is in flight, `row.original` must reflect what was actually
  // sent, not whatever `row` looks like by the time the response lands. That
  // keeps a mid-flight edit dirty so its own Save still fires.
  const payload = {
    roles: [...row.roles],
    allowUsers: [...row.allowUsers],
    denyUsers: [...row.denyUsers],
    roulette: row.roulette,
    failSafe: row.failSafe,
    label: row.label,
  }
  try {
    await policy.setFlag(row.key, payload)
    row.original = snapshotAudience(payload)
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

// A resolved user's role ("admin"/"user"); unknown/missing falls back to
// "user" so the by-user check still evaluates against a signed-in identity.
const previewRoleLabel = computed(() => {
  const role = previewUser.value?.role
  const known = role === 'admin' || role === 'user' ? role : 'user'
  return t(`admin.policy.roles.${known}`)
})

// identity is a plain string: one of PREVIEW_IDENTITIES for the hypothetical
// check, or a resolved user's real role for the by-user check.
function canAccess(row: FlagRow, identity: string, userId?: string): boolean {
  if (identity === 'guest') return false
  if (userId && row.denyUsers.includes(userId)) return false
  if (userId && row.allowUsers.includes(userId)) return true
  if (row.roles.includes('everyone')) return true
  if (identity !== 'anonymous' && row.roles.includes(identity)) return true
  return false
}

const previewResults = computed(() => {
  // Two modes:
  //  - A specific user is selected → evaluate as THAT user: their real role
  //    (from resolve) + their id, so per-user allow/deny overrides apply. The
  //    identity segmented control is ignored (and disabled in the template).
  //    This is the fix for "access check by user": the old code keyed the id
  //    off the identity control and dropped it under the anonymous default, so
  //    picking a user did nothing.
  //  - No user → the hypothetical identity control drives it. anonymous/guest
  //    carry no userID, so per-user overrides don't apply.
  const u = previewUser.value
  const identity = u ? (u.role || 'user') : previewIdentity.value
  const userId = u ? u.id : undefined
  return rows.value.map((row) => ({
    key: row.key,
    label: row.label,
    visible: canAccess(row, identity, userId),
  }))
})

// ─── PROVIDERS TAB (P5 Task 2) ──────────────────────────────────────────
interface ProviderRow extends ScraperProviderWire {
  saving: boolean
}

const isProvidersLoading = ref(true)
const providersError = ref<string | null>(null)
const providerRows = ref<ProviderRow[]>([])

// All three policy values are admin levers (probe auto demote/promote retired
// 2026-07-08 — nothing machine-sets policy): auto = in the failover chain,
// manual = parked out of auto-failover but still manually selectable,
// disabled = dropped entirely.
const PROVIDER_POLICIES: ScraperProviderPolicy[] = ['auto', 'manual', 'disabled']

const providerPolicyOptions = computed(() =>
  PROVIDER_POLICIES.map((p) => ({ value: p, label: t(`admin.policy.providers.policy.${p}`) })),
)


function stateVariant(state: ScraperProviderWire['derived_state']): NonNullable<BadgeVariants['variant']> {
  switch (state) {
    case 'UP':
      return 'success'
    case 'Recovering':
    case 'Degraded':
      return 'warning'
    case 'Down':
      return 'destructive'
    case 'Disabled':
    default:
      return 'default'
  }
}

function handleProvidersError(e: unknown): void {
  const err = e as { response?: { status?: number; data?: { error?: { message?: string } } }; message?: string }
  providersError.value = err.response?.status === 403
    ? '403'
    : (err.response?.data?.error?.message || err.message || t('admin.policy.providers.loadError'))
}

async function loadProviders(): Promise<void> {
  isProvidersLoading.value = true
  providersError.value = null
  try {
    const list = await providers.list()
    providerRows.value = list.map((p) => ({ ...p, saving: false }))
  } catch (e) {
    handleProvidersError(e)
  } finally {
    isProvidersLoading.value = false
  }
}

// Applies the actual policy change: optimistic flip of `policy` (so the
// segmented control reflects the new state immediately), then reconciles with
// the authoritative wire on success (derived_state/reason/health may differ
// from what we guessed) or reverts on failure.
async function applyProviderPolicy(row: ProviderRow, nextPolicy: ScraperProviderPolicy): Promise<void> {
  const previousPolicy = row.policy
  // row.name, not description — description is multi-line operator prose (44398d73).
  const name = row.name
  row.saving = true
  row.policy = nextPolicy
  try {
    const updated = await providers.setPolicy(row.name, nextPolicy)
    Object.assign(row, updated)
    toast.push(t(`admin.policy.providers.toasts.${nextPolicy}.success`, { name }), 'success')
  } catch {
    row.policy = previousPolicy
    toast.push(t(`admin.policy.providers.toasts.${nextPolicy}.error`, { name }), 'error')
  } finally {
    row.saving = false
  }
}

// auto/manual apply directly; disabled is gated behind a destructive confirm
// (disabling drops the provider from playback entirely — manual only parks it
// out of auto-failover, it stays manually selectable). Re-selecting the
// current policy is a no-op.
async function onSelectProviderPolicy(row: ProviderRow, next: ScraperProviderPolicy): Promise<void> {
  if (next === row.policy) return
  if (next !== 'disabled') {
    await applyProviderPolicy(row, next)
    return
  }
  const name = row.name
  const confirmed = await confirm({
    title: t('admin.policy.providers.confirmDisableTitle', { name }),
    description: t('admin.policy.providers.confirmDisableBody', { name }),
    confirmText: t('admin.policy.providers.disableAction'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  })
  if (!confirmed) return
  await applyProviderPolicy(row, 'disabled')
}

// ─── MAINTENANCE TAB ────────────────────────────────────────────────────
interface MaintenanceRow extends MaintenanceRoutineWire {
  saving: boolean
  draft: Record<string, unknown>      // knob edit buffer
  original: string                    // JSON of settings at load/save (dirty check)
}

const maintenance = useAdminMaintenance()
const isMaintLoading = ref(true)
const maintError = ref<string | null>(null)
const maintRows = ref<MaintenanceRow[]>([])
const alertDraft = ref<Record<string, string>>({}) // per-routine "add suppressed alert" input

function toMaintRow(w: MaintenanceRoutineWire): MaintenanceRow {
  return { ...w, saving: false, draft: { ...w.settings }, original: JSON.stringify(w.settings ?? {}) }
}

async function loadMaintenance(): Promise<void> {
  isMaintLoading.value = true
  maintError.value = null
  try {
    const list = await maintenance.list()
    // Render in registry order (host first, then cluster); unknown ids appended.
    const order = new Map(MAINTENANCE_ROUTINES.map((d, i) => [d.id, i]))
    maintRows.value = list
      .map(toMaintRow)
      .sort((a, b) => (order.get(a.id) ?? 999) - (order.get(b.id) ?? 999))
  } catch (e) {
    maintError.value = maintErrText(e)
  } finally {
    isMaintLoading.value = false
  }
}

function maintErrText(e: unknown): string {
  const err = e as { response?: { status?: number; data?: { error?: { message?: string } } }; message?: string }
  return err.response?.status === 403 ? '403' : (err.response?.data?.error?.message || err.message || t('admin.policy.maintenance.loadError'))
}

const maintGroups = computed(() => {
  const host = maintRows.value.filter((r) => (routineDescriptor(r.id)?.group ?? 'host') === 'host')
  const cluster = maintRows.value.filter((r) => routineDescriptor(r.id)?.group === 'cluster')
  return [
    { key: 'host', titleKey: 'admin.policy.maintenance.groups.host', rows: host },
    { key: 'cluster', titleKey: 'admin.policy.maintenance.groups.cluster', rows: cluster },
  ].filter((g) => g.rows.length > 0)
})

function routineName(id: string): string {
  const d = routineDescriptor(id)
  return d ? t(d.nameKey) : id
}

// ── status badge ──
type MaintStatus = 'ok' | 'failed' | 'stale' | 'never'
function routineStatus(row: MaintenanceRow): MaintStatus {
  if (!row.lastRunAt) return 'never'
  const stale = routineDescriptor(row.id)?.staleAfterMs
  if (stale && Date.now() - new Date(row.lastRunAt).getTime() > stale) return 'stale'
  return row.lastOk === false ? 'failed' : 'ok'
}
const STATUS_VARIANT: Record<MaintStatus, NonNullable<BadgeVariants['variant']>> = {
  ok: 'success', failed: 'destructive', stale: 'warning', never: 'default',
}
function statusVariant(row: MaintenanceRow) { return STATUS_VARIANT[routineStatus(row)] }
function statusLabel(row: MaintenanceRow) { return t(`admin.policy.maintenance.status.${routineStatus(row)}`) }

// ── enable/pause (instant, confirm-gated on pause) ──
async function onToggleRoutine(row: MaintenanceRow, enabled: boolean): Promise<void> {
  const name = routineName(row.id)
  if (!enabled) {
    const ok = await confirm({
      title: t('admin.policy.maintenance.confirmPauseTitle', { name }),
      description: t('admin.policy.maintenance.confirmPauseBody', { name }),
      confirmText: t('admin.policy.maintenance.pauseAction'),
      cancelText: t('common.cancel'),
      variant: 'destructive',
    })
    if (!ok) return
  }
  await applyRoutine(row, enabled, currentSettings(row), enabled ? 'toastEnable' : 'toastPause')
}

// ── save knobs (explicit) ──
function currentSettings(row: MaintenanceRow): Record<string, unknown> {
  // Coerce number knobs from their string Input values.
  const out: Record<string, unknown> = { ...row.settings, ...row.draft }
  for (const k of routineDescriptor(row.id)?.knobs ?? []) {
    if (k.type === 'number' && out[k.key] !== undefined && out[k.key] !== '') out[k.key] = Number(out[k.key])
  }
  return out
}
function isKnobDirty(row: MaintenanceRow): boolean {
  return JSON.stringify(currentSettings(row)) !== row.original
}
async function saveKnobs(row: MaintenanceRow): Promise<void> {
  await applyRoutine(row, row.enabled, currentSettings(row), 'toastSave')
}

async function applyRoutine(row: MaintenanceRow, enabled: boolean, settings: Record<string, unknown>, kind: 'toastEnable' | 'toastPause' | 'toastSave'): Promise<void> {
  const prevEnabled = row.enabled
  const name = routineName(row.id)
  row.saving = true
  row.enabled = enabled
  try {
    await maintenance.setRoutine(row.id, { enabled, settings })
    row.settings = settings
    row.draft = { ...settings }
    row.original = JSON.stringify(settings)
    toast.push(t(`admin.policy.maintenance.${kind}Success`, { name }), 'success')
  } catch {
    row.enabled = prevEnabled
    toast.push(t(`admin.policy.maintenance.${kind}Error`, { name }), 'error')
  } finally {
    row.saving = false
  }
}

// ── suppressed-alerts chip editor (maintenance_bot) ──
function alertList(row: MaintenanceRow): string[] {
  const v = row.draft['suppressed_alerts']
  return Array.isArray(v) ? (v as string[]) : []
}
function addAlert(row: MaintenanceRow): void {
  const val = (alertDraft.value[row.id] || '').trim()
  if (!val) return
  const cur = alertList(row)
  if (!cur.includes(val)) row.draft['suppressed_alerts'] = [...cur, val]
  alertDraft.value[row.id] = ''
}
function removeAlert(row: MaintenanceRow, v: string): void {
  row.draft['suppressed_alerts'] = alertList(row).filter((x) => x !== v)
}

function segOptions(opts: string[]) { return opts.map((o) => ({ value: o, label: o })) }

onMounted(() => {
  load()
  loadProviders()
  loadMaintenance()
})
</script>
