<template>
  <!-- /gacha/:id — banner spin screen with pity, pool, and result dialog -->
  <div class="min-h-screen bg-base pt-20">
    <div class="container mx-auto px-4 py-8 max-w-4xl">
      <!-- Loading state -->
      <div v-if="loadingBanners" class="flex justify-center py-20">
        <Spinner size="lg" />
      </div>

      <!-- Banner not found -->
      <div v-else-if="!banner" class="glass-card p-8 text-center text-muted-foreground">
        <p>Banner not found</p>
        <router-link to="/gacha" class="text-orange-400 hover:text-orange-300 mt-2 inline-block">
          ← Back
        </router-link>
      </div>

      <template v-else>
        <!-- Banner art header -->
        <div class="relative rounded-xl overflow-hidden mb-6 h-48 md:h-64 bg-white/5">
          <img
            v-if="banner.art_path"
            :src="cardImageUrl(banner.art_path)"
            :alt="banner.name"
            class="w-full h-full object-cover"
          />
          <div class="absolute inset-0 bg-gradient-to-t from-base via-base/40 to-transparent" />
          <div class="absolute bottom-4 left-4 right-4">
            <div class="flex items-center gap-2 mb-1 flex-wrap">
              <h1 class="text-2xl font-semibold text-white">{{ banner.name }}</h1>
              <Badge v-if="banner.is_standard" variant="default" size="sm">
                {{ $t('gacha.banner_standard_badge') }}
              </Badge>
            </div>
            <p v-if="banner.description" class="text-white/70 text-sm">{{ banner.description }}</p>
          </div>
        </div>

        <!-- Balance + Pity row -->
        <div class="glass-card p-4 mb-6 flex flex-wrap items-center gap-6">
          <!-- Balance -->
          <div class="flex items-center gap-2">
            <Gem class="size-5 text-orange-400" aria-hidden="true" />
            <span class="text-white font-semibold">{{ balance }}</span>
            <span class="text-muted-foreground text-sm">Энигмы</span>
          </div>

          <!-- Pity progress -->
          <div class="flex-1 min-w-[160px]">
            <p class="text-muted-foreground text-xs mb-1">
              {{ $t('gacha.spin_pity_label', { n: pity, max: banner.pity_threshold }) }}
            </p>
            <div class="h-1.5 bg-white/10 rounded-full overflow-hidden">
              <div
                class="h-full bg-orange-400 rounded-full transition-all"
                :style="{ width: `${Math.min((pity / banner.pity_threshold) * 100, 100)}%` }"
                role="progressbar"
                :aria-valuenow="pity"
                :aria-valuemax="banner.pity_threshold"
              />
            </div>
          </div>
        </div>

        <!-- Pull buttons -->
        <div class="flex flex-wrap gap-3 mb-8">
          <Button
            :disabled="balance < COST_X1 || loadingPull"
            variant="outline"
            class="flex-1 sm:flex-none"
            @click="onPull('x1')"
          >
            <Gem class="size-4 mr-1.5 text-orange-400" aria-hidden="true" />
            {{ balance < COST_X1
              ? $t('gacha.spin_insufficient', { n: COST_X1 })
              : $t('gacha.spin_x1_cost', { n: COST_X1 }) }}
          </Button>

          <Button
            :disabled="balance < COST_X10 || loadingPull"
            class="flex-1 sm:flex-none"
            @click="onPull('x10')"
          >
            <Gem class="size-4 mr-1.5" aria-hidden="true" />
            {{ balance < COST_X10
              ? $t('gacha.spin_insufficient', { n: COST_X10 })
              : $t('gacha.spin_x10_cost', { n: COST_X10 }) }}
          </Button>
        </div>

        <!-- Pull error -->
        <Alert v-if="pullError" variant="destructive" class="mb-6" dismissible @dismiss="store.pullError = null">
          {{ pullError }}
        </Alert>

        <!-- Card pool grid -->
        <div class="mb-4">
          <h2 class="text-lg font-semibold text-white mb-3">{{ $t('gacha.pool_title') }}</h2>
          <div class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 gap-3">
            <div
              v-for="card in sortedPoolCards"
              :key="card.id"
              class="relative rounded-lg overflow-hidden aspect-[2/3] bg-white/5"
            >
              <img
                :src="cardImageUrl(card.image_path)"
                :alt="card.owned ? card.name : '???'"
                class="w-full h-full object-cover"
                :style="card.owned ? '' : 'filter: brightness(0)'"
              />
              <!-- Rarity accent border -->
              <div :class="['absolute inset-0 rounded-lg ring-1 ring-inset', rarityRingClass(card.rarity)]" />
              <!-- Owned badge -->
              <span
                v-if="card.owned"
                class="absolute top-1 right-1 bg-teal-500/80 text-white text-[10px] font-semibold px-1 rounded"
              >{{ $t('gacha.pool_owned_badge') }}</span>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>

  <!-- Result dialog -->
  <Modal
    v-model="showResult"
    :title="$t('gacha.result_dialog_title')"
    closable
    @update:model-value="onResultClose"
  >
    <div class="grid grid-cols-3 sm:grid-cols-5 gap-3 mt-2">
      <div
        v-for="(pulled, idx) in pullResult?.cards ?? []"
        :key="idx"
        class="relative rounded-lg overflow-hidden aspect-[2/3] bg-white/5"
      >
        <img
          :src="cardImageUrl(pulled.card.image_path)"
          :alt="pulled.card.name"
          class="w-full h-full object-cover"
        />
        <!-- Rarity frame -->
        <div :class="['absolute inset-0 rounded-lg ring-2 ring-inset', rarityRingClass(pulled.card.rarity)]" />
        <!-- Rarity label -->
        <span :class="['absolute bottom-0 left-0 right-0 text-center text-[10px] font-semibold py-0.5', rarityTextClass(pulled.card.rarity)]">
          {{ pulled.card.rarity }}
        </span>
        <!-- NEW badge -->
        <span
          v-if="pulled.new"
          class="absolute top-1 left-1 bg-orange-500 text-white text-[10px] font-semibold px-1 rounded"
        >{{ $t('gacha.result_new_badge') }}</span>
        <!-- Dupe badge -->
        <span
          v-else-if="pulled.count > 1"
          class="absolute top-1 left-1 bg-white/20 text-white text-[10px] font-semibold px-1 rounded"
        >{{ $t('gacha.result_dupe_badge', { n: pulled.count }) }}</span>
      </div>
    </div>

    <template #footer>
      <Button variant="outline" @click="onResultClose(false)">
        {{ $t('gacha.result_close') }}
      </Button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Gem } from 'lucide-vue-next'
import { useGachaStore } from '@/stores/gacha'
import { cardImageUrl, type PullResult, type Rarity } from '@/api/gacha'
import Spinner from '@/components/ui/Spinner.vue'
import Button from '@/components/ui/Button.vue'
import Badge from '@/components/ui/Badge.vue'
import Alert from '@/components/ui/Alert.vue'
import Modal from '@/components/ui/Modal.vue'

const { t } = useI18n()
void t

const route = useRoute()
const store = useGachaStore()
const bannerId = computed(() => route.params.id as string)

const COST_X1 = 100
const COST_X10 = 900

const loadingBanners = computed(() => store.loadingBanners)
const loadingPull = computed(() => store.loadingPull)
const pullError = computed(() => store.pullError)
const balance = computed(() => store.balance)

// Find this banner from the store
const banner = computed(() => store.banners.find(b => b.id === bannerId.value) ?? null)
const pity = computed(() => banner.value?.my_pity ?? 0)

// Sort pool cards SSR→N (highest rarity first)
const rarityOrder: Rarity[] = ['SSR', 'SR', 'R', 'N']
const sortedPoolCards = computed(() => {
  if (!banner.value) return []
  return [...banner.value.cards].sort(
    (a, b) => rarityOrder.indexOf(a.rarity) - rarityOrder.indexOf(b.rarity),
  )
})

// Result dialog
const showResult = ref(false)
const pullResult = ref<PullResult | null>(null)

async function onPull(mode: 'x1' | 'x10') {
  const res = await store.pull(bannerId.value, mode)
  if (res) {
    pullResult.value = res
    showResult.value = true
    // Refresh banners after pull to update pity
    await store.fetchBanners()
  }
}

function onResultClose(val: boolean) {
  if (!val) {
    showResult.value = false
  }
}

// ── Rarity styling helpers (exempt hues: teal/indigo/orange) ──────────────
function rarityRingClass(rarity: Rarity): string {
  switch (rarity) {
    case 'SSR': return 'ring-orange-400/60'
    case 'SR':  return 'ring-indigo-400/60'
    case 'R':   return 'ring-teal-400/60'
    default:    return 'ring-white/20'
  }
}

function rarityTextClass(rarity: Rarity): string {
  switch (rarity) {
    case 'SSR': return 'bg-orange-500/70 text-white'
    case 'SR':  return 'bg-indigo-500/70 text-white'
    case 'R':   return 'bg-teal-500/70 text-white'
    default:    return 'bg-white/20 text-white/80'
  }
}

onMounted(async () => {
  await Promise.all([store.refreshWallet(), store.fetchBanners()])
})
</script>
