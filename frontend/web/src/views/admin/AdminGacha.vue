<template>
  <!-- /admin/gacha — Gacha manager with Cards / Groups / Banners tabs -->
  <div class="min-h-screen bg-base pt-20">
    <div class="container mx-auto px-4 py-8 max-w-7xl">
      <!-- Header -->
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <div>
          <h1 class="text-3xl font-semibold text-white">{{ $t('gacha.admin.title') }}</h1>
          <p class="text-white/60 text-sm mt-1">{{ $t('gacha.admin.desc') }}</p>
        </div>
      </div>

      <!-- Error banner -->
      <Alert v-if="pageError" variant="destructive" class="mb-4" dismissible @dismiss="pageError = null">
        {{ pageError }}
      </Alert>

      <!-- Tabs: Cards / Groups / Banners -->
      <Tabs v-model="activeTab" :tabs="tabDefs" variant="underline">

        <!-- ─── CARDS TAB ─────────────────────────────────────────────── -->
        <template #cards>
          <div class="flex flex-wrap items-center justify-between gap-3 mb-4">
            <!-- Filters -->
            <div class="flex flex-wrap gap-2">
              <Select
                v-model="cardFilterRarity"
                :options="rarityFilterOptions"
                :placeholder="$t('gacha.admin.filter_rarity')"
                size="sm"
                class="w-32"
              />
              <Select
                v-model="cardFilterGroup"
                :options="groupFilterOptions"
                :placeholder="$t('gacha.admin.filter_group')"
                size="sm"
                class="w-40"
              />
              <label class="flex items-center gap-1.5 text-sm text-white/70 cursor-pointer select-none">
                <Checkbox v-model="cardFilterEnabled" />
                {{ $t('gacha.admin.filter_enabled_only') }}
              </label>
            </div>
            <Button size="sm" @click="openCardCreate">
              + {{ $t('gacha.admin.card_create') }}
            </Button>
          </div>

          <!-- Loading -->
          <div v-if="loadingCards" class="flex justify-center py-10">
            <Spinner />
          </div>

          <!-- Empty -->
          <div v-else-if="filteredCards.length === 0" class="glass-card p-8 text-center text-muted-foreground">
            {{ $t('gacha.admin.filter_all') }} — no cards
          </div>

          <!-- Cards table -->
          <div v-else class="glass-card overflow-x-auto" data-testid="cards-tab-table">
            <table class="w-full text-sm text-white">
              <thead class="bg-black/40 backdrop-blur text-white/70 text-xs uppercase">
                <tr>
                  <th class="px-3 py-2 text-left w-12">{{ $t('gacha.admin.card_image') }}</th>
                  <th class="px-3 py-2 text-left">{{ $t('gacha.admin.card_name') }}</th>
                  <th class="px-3 py-2 text-left">{{ $t('gacha.admin.card_source') }}</th>
                  <th class="px-3 py-2 text-left">{{ $t('gacha.admin.card_rarity') }}</th>
                  <th class="px-3 py-2 text-center">{{ $t('gacha.admin.card_enabled') }}</th>
                  <th class="px-3 py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="card in filteredCards"
                  :key="card.id"
                  class="border-t border-white/10 hover:bg-white/5"
                >
                  <td class="px-3 py-2">
                    <img
                      :src="cardImageUrl(card.image_path)"
                      :alt="card.name"
                      class="w-8 h-10 object-cover rounded"
                    />
                  </td>
                  <td class="px-3 py-2 font-medium">{{ card.name }}</td>
                  <td class="px-3 py-2 text-white/60 text-xs">{{ card.source_title }}</td>
                  <td class="px-3 py-2">
                    <span :class="['text-xs font-semibold px-1.5 py-0.5 rounded', rarityBadgeClass(card.rarity)]">
                      {{ card.rarity }}
                    </span>
                  </td>
                  <td class="px-3 py-2 text-center">
                    <span :class="card.enabled ? 'text-teal-400' : 'text-muted-foreground'">
                      {{ card.enabled ? '✓' : '–' }}
                    </span>
                  </td>
                  <td class="px-3 py-2 text-right space-x-2">
                    <Button variant="ghost" size="sm" @click="openCardEdit(card)">
                      <Pencil class="size-4" />
                    </Button>
                    <Button variant="ghost" size="sm" class="text-destructive" @click="confirmCardDelete(card)">
                      <Trash2 class="size-4" />
                    </Button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>

        <!-- ─── GROUPS TAB ───────────────────────────────────────────────── -->
        <template #groups>
          <div class="flex justify-end mb-4">
            <Button size="sm" @click="openGroupCreate">
              + {{ $t('gacha.admin.group_create') }}
            </Button>
          </div>

          <div v-if="loadingGroups" class="flex justify-center py-10">
            <Spinner />
          </div>
          <div v-else-if="groups.length === 0" class="glass-card p-8 text-center text-muted-foreground">
            No groups yet
          </div>
          <div v-else class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            <div
              v-for="group in groups"
              :key="group.id"
              class="glass-card p-4"
            >
              <div class="flex items-center justify-between mb-3">
                <span class="font-semibold text-white">{{ group.name }}</span>
                <div class="flex gap-1">
                  <Button variant="ghost" size="sm" @click="openGroupRename(group)">
                    <Pencil class="size-4" />
                  </Button>
                  <Button variant="ghost" size="sm" class="text-destructive" @click="confirmGroupDelete(group)">
                    <Trash2 class="size-4" />
                  </Button>
                </div>
              </div>
              <p class="text-white/50 text-xs">{{ group.name }}</p>
            </div>
          </div>
        </template>

        <!-- ─── BANNERS TAB ──────────────────────────────────────────────── -->
        <template #banners>
          <div class="flex justify-end mb-4">
            <Button size="sm" @click="openBannerCreate">
              + {{ $t('gacha.admin.banner_create') }}
            </Button>
          </div>

          <div v-if="loadingBanners" class="flex justify-center py-10">
            <Spinner />
          </div>
          <div v-else-if="banners.length === 0" class="glass-card p-8 text-center text-muted-foreground">
            No banners yet
          </div>
          <div v-else class="glass-card overflow-x-auto">
            <table class="w-full text-sm text-white">
              <thead class="bg-black/40 backdrop-blur text-white/70 text-xs uppercase">
                <tr>
                  <th class="px-3 py-2 text-left">{{ $t('gacha.admin.banner_name') }}</th>
                  <th class="px-3 py-2 text-center">{{ $t('gacha.admin.banner_standard') }}</th>
                  <th class="px-3 py-2 text-center">{{ $t('gacha.admin.banner_enabled') }}</th>
                  <th class="px-3 py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="banner in banners"
                  :key="banner.id"
                  class="border-t border-white/10 hover:bg-white/5"
                >
                  <td class="px-3 py-2 font-medium">{{ banner.name }}</td>
                  <td class="px-3 py-2 text-center">
                    <span class="text-xs">{{ banner.is_standard ? '✓' : '–' }}</span>
                  </td>
                  <td class="px-3 py-2 text-center">
                    <span :class="banner.enabled ? 'text-teal-400 text-xs' : 'text-muted-foreground text-xs'">
                      {{ banner.enabled ? '✓' : '–' }}
                    </span>
                  </td>
                  <td class="px-3 py-2 text-right space-x-2">
                    <Button variant="ghost" size="sm" @click="openBannerEdit(banner)">
                      <Pencil class="size-4" />
                    </Button>
                    <Button variant="ghost" size="sm" class="text-destructive" @click="confirmBannerDelete(banner)">
                      <Trash2 class="size-4" />
                    </Button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
      </Tabs>
    </div>
  </div>

  <!-- ─── CARD CREATE/EDIT DIALOG ───────────────────────────────────────── -->
  <Modal
    v-model="showCardDialog"
    :title="editCard ? $t('gacha.admin.card_edit') : $t('gacha.admin.card_create')"
    closable
    size="lg"
  >
    <div class="space-y-4">
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.card_name') }}</label>
        <Input v-model="cardForm.name" placeholder="Character name" />
      </div>
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.card_source') }}</label>
        <Input v-model="cardForm.source_title" placeholder="Source anime title" />
      </div>
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.card_rarity') }}</label>
        <Select v-model="cardForm.rarity" :options="rarityOptions" />
      </div>
      <div class="flex items-center gap-2">
        <Checkbox v-model="cardForm.enabled" />
        <label class="text-white/70 text-sm cursor-pointer">{{ $t('gacha.admin.card_enabled') }}</label>
      </div>

      <!-- Image upload: file OR URL -->
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.card_image') }}</label>
        <!-- Image preview -->
        <div v-if="imagePreview" class="mb-2">
          <img
            :src="imagePreview"
            alt="Preview"
            class="w-20 h-28 object-cover rounded border border-white/20"
          />
        </div>
        <div class="flex flex-col gap-2">
          <!-- File upload -->
          <div class="flex items-center gap-2">
            <label
              class="inline-flex items-center gap-1.5 cursor-pointer rounded-xl border border-white/20 bg-white/5 hover:bg-white/10 px-3 py-1.5 text-sm font-medium text-white transition"
            >
              <Upload class="size-4" aria-hidden="true" />
              {{ $t('gacha.admin.card_image_or') }}
              <input
                ref="fileInput"
                type="file"
                accept="image/*"
                class="sr-only"
                @change="onFileChange"
              />
            </label>
          </div>
          <!-- URL input -->
          <Input
            v-model="cardForm.imageUrl"
            :placeholder="$t('gacha.admin.card_image_url_placeholder')"
            @blur="onImageUrlBlur"
          />
        </div>
        <p v-if="uploadError" class="text-destructive text-xs mt-1">{{ $t('gacha.admin.upload_error') }}</p>
        <p v-if="uploading" class="text-muted-foreground text-xs mt-1">{{ $t('gacha.admin.upload_uploading') }}</p>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <Button variant="outline" @click="showCardDialog = false">
          {{ $t('gacha.admin.card_cancel') }}
        </Button>
        <Button :disabled="savingCard" @click="saveCard">
          {{ $t('gacha.admin.card_save') }}
        </Button>
      </div>
    </template>
  </Modal>

  <!-- ─── GROUP CREATE/RENAME DIALOG ───────────────────────────────────── -->
  <Modal
    v-model="showGroupDialog"
    :title="editGroup ? $t('gacha.admin.group_rename') : $t('gacha.admin.group_create')"
    closable
  >
    <div>
      <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.group_name') }}</label>
      <Input v-model="groupForm.name" placeholder="Group name" />
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <Button variant="outline" @click="showGroupDialog = false">
          {{ $t('gacha.admin.card_cancel') }}
        </Button>
        <Button :disabled="savingGroup" @click="saveGroup">
          {{ $t('gacha.admin.card_save') }}
        </Button>
      </div>
    </template>
  </Modal>

  <!-- ─── BANNER CREATE/EDIT DIALOG ────────────────────────────────────── -->
  <Modal
    v-model="showBannerDialog"
    :title="editBanner ? $t('gacha.admin.banner_edit') : $t('gacha.admin.banner_create')"
    closable
    size="lg"
  >
    <div class="space-y-4">
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.banner_name') }}</label>
        <Input v-model="bannerForm.name" placeholder="Banner name" />
      </div>
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.banner_description') }}</label>
        <Input v-model="bannerForm.description" placeholder="Short description" />
      </div>
      <div class="flex flex-wrap gap-4">
        <label class="flex items-center gap-2">
          <Checkbox v-model="bannerForm.is_standard" />
          <span class="text-white/70 text-sm">{{ $t('gacha.admin.banner_standard') }}</span>
        </label>
        <label class="flex items-center gap-2">
          <Checkbox v-model="bannerForm.enabled" />
          <span class="text-white/70 text-sm">{{ $t('gacha.admin.banner_enabled') }}</span>
        </label>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.banner_active_from') }}</label>
          <Input v-model="bannerForm.active_from" type="datetime-local" />
        </div>
        <div>
          <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.banner_active_to') }}</label>
          <Input v-model="bannerForm.active_to" type="datetime-local" />
        </div>
      </div>

      <!-- Banner cards section -->
      <div v-if="editBanner">
        <p class="text-white/70 text-xs mb-1">{{ $t('gacha.admin.banner_cards_section') }}</p>
        <div class="flex gap-2 mb-2">
          <Input
            v-model="bannerCardIds"
            placeholder="card-id-1, card-id-2"
            class="flex-1"
          />
          <Button variant="outline" size="sm" @click="addBannerCards">
            {{ $t('gacha.admin.banner_add_cards') }}
          </Button>
        </div>
        <div class="flex gap-2">
          <Select
            v-model="bannerGroupId"
            :options="groupSelectOptions"
            :placeholder="$t('gacha.admin.banner_add_group')"
            class="flex-1"
          />
          <Button variant="outline" size="sm" @click="addBannerGroup">
            Add group
          </Button>
        </div>
        <p v-if="bannerGroupAdded" class="text-teal-400 text-xs mt-1">
          {{ $t('gacha.admin.banner_group_added') }}
        </p>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <Button variant="outline" @click="showBannerDialog = false">
          {{ $t('gacha.admin.banner_cancel') }}
        </Button>
        <Button :disabled="savingBanner" @click="saveBanner">
          {{ $t('gacha.admin.banner_save') }}
        </Button>
      </div>
    </template>
  </Modal>

  <!-- ─── DELETE CONFIRM DIALOG ─────────────────────────────────────────── -->
  <Modal v-model="showDeleteDialog" :title="deleteTarget?.label ?? ''" closable>
    <p class="text-white/80">{{ deleteTarget?.confirmMsg }}</p>
    <template #footer>
      <div class="flex justify-end gap-2">
        <Button variant="outline" @click="showDeleteDialog = false">
          {{ $t('gacha.admin.card_cancel') }}
        </Button>
        <Button variant="destructive" :disabled="deleting" @click="runDelete">
          {{ $t('gacha.admin.card_delete') }}
        </Button>
      </div>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Pencil, Trash2, Upload } from 'lucide-vue-next'
import {
  gachaAdminApi,
  cardImageUrl,
  type GachaCard,
  type GachaGroup,
  type GachaBanner,
  type Rarity,
} from '@/api/gacha'
import Tabs from '@/components/ui/Tabs.vue'
import Modal from '@/components/ui/Modal.vue'
import Input from '@/components/ui/Input.vue'
import Button from '@/components/ui/Button.vue'
import Select from '@/components/ui/Select.vue'
import Checkbox from '@/components/ui/Checkbox.vue'
import Spinner from '@/components/ui/Spinner.vue'
import Alert from '@/components/ui/Alert.vue'

const { t } = useI18n()

// ── Tabs ──────────────────────────────────────────────────────────────────────
const activeTab = ref<'cards' | 'groups' | 'banners'>('cards')
const tabDefs = [
  { value: 'cards',   label: t('gacha.admin.tab_cards') },
  { value: 'groups',  label: t('gacha.admin.tab_groups') },
  { value: 'banners', label: t('gacha.admin.tab_banners') },
]

// ── Shared state ──────────────────────────────────────────────────────────────
const pageError = ref<string | null>(null)

// ── Cards state ───────────────────────────────────────────────────────────────
const cards = ref<GachaCard[]>([])
const loadingCards = ref(false)
const cardFilterRarity = ref<string>('all')
const cardFilterGroup = ref<string>('all')
const cardFilterEnabled = ref(false)

const rarityFilterOptions = [
  { value: 'all', label: t('gacha.admin.filter_all') },
  { value: 'N', label: 'N' },
  { value: 'R', label: 'R' },
  { value: 'SR', label: 'SR' },
  { value: 'SSR', label: 'SSR' },
]

const filteredCards = computed(() => {
  return cards.value.filter(c => {
    if (cardFilterRarity.value !== 'all' && c.rarity !== cardFilterRarity.value) return false
    if (cardFilterEnabled.value && !c.enabled) return false
    return true
  })
})

// ── Groups state ──────────────────────────────────────────────────────────────
const groups = ref<GachaGroup[]>([])
const loadingGroups = ref(false)

const groupFilterOptions = computed(() => [
  { value: 'all', label: t('gacha.admin.filter_all') },
  ...groups.value.map(g => ({ value: g.id, label: g.name })),
])

const groupSelectOptions = computed(() =>
  groups.value.map(g => ({ value: g.id, label: g.name })),
)

// ── Banners state ─────────────────────────────────────────────────────────────
const banners = ref<GachaBanner[]>([])
const loadingBanners = ref(false)

// ── Card dialog ───────────────────────────────────────────────────────────────
const showCardDialog = ref(false)
const editCard = ref<GachaCard | null>(null)
const savingCard = ref(false)
const uploading = ref(false)
const uploadError = ref(false)
const imagePreview = ref<string | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

const cardForm = ref({
  name: '',
  source_title: '',
  rarity: 'N' as Rarity,
  enabled: true,
  imagePath: '',   // final stored path (returned from upload or existing)
  imageUrl: '',    // user-typed URL
})

function resetCardForm() {
  cardForm.value = { name: '', source_title: '', rarity: 'N', enabled: true, imagePath: '', imageUrl: '' }
  imagePreview.value = null
  uploadError.value = false
  uploading.value = false
}

function openCardCreate() {
  editCard.value = null
  resetCardForm()
  showCardDialog.value = true
}

function openCardEdit(card: GachaCard) {
  editCard.value = card
  cardForm.value = {
    name: card.name,
    source_title: card.source_title,
    rarity: card.rarity,
    enabled: card.enabled,
    imagePath: card.image_path,
    imageUrl: '',
  }
  imagePreview.value = card.image_path ? cardImageUrl(card.image_path) : null
  uploadError.value = false
  uploading.value = false
  showCardDialog.value = true
}

async function onFileChange(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  uploading.value = true
  uploadError.value = false
  try {
    const form = new FormData()
    form.append('file', file)
    form.append('kind', 'card')
    const res = await gachaAdminApi.uploadFile(form)
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    const path = data?.data?.image_path ?? ''
    cardForm.value.imagePath = path
    imagePreview.value = path ? cardImageUrl(path) : null
  } catch {
    uploadError.value = true
  } finally {
    uploading.value = false
  }
}

async function onImageUrlBlur() {
  const url = cardForm.value.imageUrl.trim()
  if (!url) return
  uploading.value = true
  uploadError.value = false
  try {
    const res = await gachaAdminApi.uploadUrl({ image_url: url, kind: 'card' })
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    const path = data?.data?.image_path ?? ''
    cardForm.value.imagePath = path
    imagePreview.value = path ? cardImageUrl(path) : url
  } catch {
    uploadError.value = true
    imagePreview.value = url
  } finally {
    uploading.value = false
  }
}

async function saveCard() {
  savingCard.value = true
  pageError.value = null
  try {
    const payload = {
      name: cardForm.value.name,
      source_title: cardForm.value.source_title,
      rarity: cardForm.value.rarity,
      enabled: cardForm.value.enabled,
      image_path: cardForm.value.imagePath,
    }
    if (editCard.value) {
      await gachaAdminApi.updateCard(editCard.value.id, payload)
    } else {
      await gachaAdminApi.createCard(payload)
    }
    showCardDialog.value = false
    await loadCards()
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    savingCard.value = false
  }
}

// ── Group dialog ──────────────────────────────────────────────────────────────
const showGroupDialog = ref(false)
const editGroup = ref<GachaGroup | null>(null)
const savingGroup = ref(false)
const groupForm = ref({ name: '' })

function openGroupCreate() {
  editGroup.value = null
  groupForm.value = { name: '' }
  showGroupDialog.value = true
}

function openGroupRename(group: GachaGroup) {
  editGroup.value = group
  groupForm.value = { name: group.name }
  showGroupDialog.value = true
}

async function saveGroup() {
  savingGroup.value = true
  pageError.value = null
  try {
    if (editGroup.value) {
      await gachaAdminApi.renameGroup(editGroup.value.id, { name: groupForm.value.name })
    } else {
      await gachaAdminApi.createGroup({ name: groupForm.value.name })
    }
    showGroupDialog.value = false
    await loadGroups()
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    savingGroup.value = false
  }
}

// ── Banner dialog ─────────────────────────────────────────────────────────────
const showBannerDialog = ref(false)
const editBanner = ref<GachaBanner | null>(null)
const savingBanner = ref(false)
const bannerCardIds = ref('')
const bannerGroupId = ref<string>('')
const bannerGroupAdded = ref(false)

const bannerForm = ref({
  name: '',
  description: '',
  is_standard: false,
  enabled: true,
  active_from: '',
  active_to: '',
  sort_order: 0,
})

function openBannerCreate() {
  editBanner.value = null
  bannerForm.value = { name: '', description: '', is_standard: false, enabled: true, active_from: '', active_to: '', sort_order: 0 }
  bannerCardIds.value = ''
  bannerGroupId.value = ''
  bannerGroupAdded.value = false
  showBannerDialog.value = true
}

function openBannerEdit(banner: GachaBanner) {
  editBanner.value = banner
  bannerForm.value = {
    name: banner.name,
    description: banner.description ?? '',
    is_standard: banner.is_standard,
    enabled: banner.enabled,
    active_from: banner.active_from ?? '',
    active_to: banner.active_to ?? '',
    sort_order: banner.sort_order ?? 0,
  }
  bannerCardIds.value = ''
  bannerGroupId.value = ''
  bannerGroupAdded.value = false
  showBannerDialog.value = true
}

async function saveBanner() {
  savingBanner.value = true
  pageError.value = null
  try {
    const payload = {
      name: bannerForm.value.name,
      description: bannerForm.value.description,
      is_standard: bannerForm.value.is_standard,
      enabled: bannerForm.value.enabled,
      active_from: bannerForm.value.active_from || undefined,
      active_to: bannerForm.value.active_to || undefined,
      sort_order: bannerForm.value.sort_order,
    }
    if (editBanner.value) {
      await gachaAdminApi.updateBanner(editBanner.value.id, payload)
    } else {
      await gachaAdminApi.createBanner(payload)
    }
    showBannerDialog.value = false
    await loadBanners()
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    savingBanner.value = false
  }
}

async function addBannerCards() {
  if (!editBanner.value || !bannerCardIds.value.trim()) return
  const ids = bannerCardIds.value.split(',').map(s => s.trim()).filter(Boolean)
  try {
    await gachaAdminApi.addBannerCards(editBanner.value.id, { card_ids: ids })
    bannerCardIds.value = ''
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  }
}

async function addBannerGroup() {
  if (!editBanner.value || !bannerGroupId.value) return
  try {
    await gachaAdminApi.addGroupCardsToBanner(editBanner.value.id, bannerGroupId.value)
    bannerGroupAdded.value = true
    bannerGroupId.value = ''
    setTimeout(() => { bannerGroupAdded.value = false }, 3000)
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  }
}

// ── Delete confirm ────────────────────────────────────────────────────────────
const showDeleteDialog = ref(false)
const deleting = ref(false)
const deleteTarget = ref<{ label: string; confirmMsg: string; action: () => Promise<void> } | null>(null)

function confirmCardDelete(card: GachaCard) {
  deleteTarget.value = {
    label: card.name,
    confirmMsg: t('gacha.admin.card_delete_confirm'),
    action: async () => {
      await gachaAdminApi.deleteCard(card.id)
      await loadCards()
    },
  }
  showDeleteDialog.value = true
}

function confirmGroupDelete(group: GachaGroup) {
  deleteTarget.value = {
    label: group.name,
    confirmMsg: t('gacha.admin.group_delete_confirm'),
    action: async () => {
      await gachaAdminApi.deleteGroup(group.id)
      await loadGroups()
    },
  }
  showDeleteDialog.value = true
}

function confirmBannerDelete(banner: GachaBanner) {
  deleteTarget.value = {
    label: banner.name,
    confirmMsg: t('gacha.admin.banner_delete_confirm'),
    action: async () => {
      await gachaAdminApi.deleteBanner(banner.id)
      await loadBanners()
    },
  }
  showDeleteDialog.value = true
}

async function runDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  pageError.value = null
  try {
    await deleteTarget.value.action()
    showDeleteDialog.value = false
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    deleting.value = false
  }
}

// ── Data loaders ──────────────────────────────────────────────────────────────
async function loadCards() {
  loadingCards.value = true
  try {
    const res = await gachaAdminApi.listCards()
    cards.value = ((res as { data?: { data?: GachaCard[] } }).data?.data ?? [])
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    loadingCards.value = false
  }
}

async function loadGroups() {
  loadingGroups.value = true
  try {
    const res = await gachaAdminApi.listGroups()
    groups.value = ((res as { data?: { data?: GachaGroup[] } }).data?.data ?? [])
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    loadingGroups.value = false
  }
}

async function loadBanners() {
  loadingBanners.value = true
  try {
    const res = await gachaAdminApi.listBanners()
    banners.value = ((res as { data?: { data?: GachaBanner[] } }).data?.data ?? [])
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    loadingBanners.value = false
  }
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function extractMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return String(err)
}

const rarityOptions = [
  { value: 'N', label: 'N' },
  { value: 'R', label: 'R' },
  { value: 'SR', label: 'SR' },
  { value: 'SSR', label: 'SSR' },
]

function rarityBadgeClass(rarity: Rarity): string {
  switch (rarity) {
    case 'SSR': return 'bg-orange-400/20 text-orange-400'
    case 'SR':  return 'bg-indigo-400/20 text-indigo-400'
    case 'R':   return 'bg-teal-400/20 text-teal-400'
    default:    return 'bg-white/10 text-white/60'
  }
}

onMounted(async () => {
  await Promise.all([loadCards(), loadGroups(), loadBanners()])
})
</script>
