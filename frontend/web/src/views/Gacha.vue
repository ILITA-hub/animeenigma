<template>
  <!-- /gacha — all-in-one Variant C: daily-claim header + banner slider + spin
       dock + drops modal + gem ceremony → 3D viewer → summary pull flow. -->
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-5xl">
      <!-- Loading state -->
      <div v-if="loadingWallet && loadingBanners" class="flex justify-center py-20">
        <Spinner size="lg" />
      </div>

      <template v-else>
        <!-- Page header: title + daily claim -->
        <div class="flex items-end justify-between gap-4 flex-wrap mb-6">
          <div>
            <h1 class="text-3xl font-semibold text-white">{{ $t('gacha.page_title') }}</h1>
            <p class="text-muted-foreground text-sm mt-1">
              <Gem class="inline size-4 mr-1 text-orange-400" aria-hidden="true" />
              <span :aria-label="$t('gacha.balance_chip_aria', { n: balance })">
                {{ balance }} {{ $t('gacha.balance_unit') }}
              </span>
            </p>
          </div>

          <!-- Daily claim card -->
          <div class="glass-card flex items-center gap-4 px-4 py-3">
            <div>
              <div class="text-sm font-semibold text-white">{{ $t('gacha.daily_claim_title') }}</div>
              <div v-if="dailyStreak > 0" class="text-muted-foreground text-xs">
                {{ $t('gacha.daily_streak_label', { n: dailyStreak }) }}
              </div>
            </div>
            <Button
              size="sm"
              :disabled="alreadyClaimed || loadingDaily"
              @click="onClaimDaily"
            >
              <span v-if="loadingDaily">…</span>
              <span v-else-if="alreadyClaimed">{{ $t('gacha.daily_claimed_text') }}</span>
              <span v-else>{{ $t('gacha.daily_claim_button') }}</span>
            </Button>
          </div>
        </div>

        <!-- Banners error -->
        <Alert v-if="bannersError" variant="destructive" class="mb-6">{{ bannersError }}</Alert>

        <!-- Empty banners -->
        <div v-else-if="banners.length === 0" class="glass-card p-8 text-center text-muted-foreground">
          {{ $t('gacha.banner_list_empty') }}
        </div>

        <!-- Slider + dock -->
        <div v-else>
          <GachaSlider v-model="activeIndex" :banners="banners" />
          <SpinDock
            v-if="activeBanner"
            :pity="activeBanner.my_pity"
            :pity-threshold="activeBanner.pity_threshold"
            :balance="balance"
            :cost-x1="COST_X1"
            :cost-x10="COST_X10"
            :loading="loadingPull"
            @drops="showDrops = true"
            @pull="onPull"
          />

          <!-- Pull error -->
          <Alert
            v-if="pullError"
            variant="destructive"
            class="mt-6"
            dismissible
            @dismiss="store.pullError = null"
          >
            {{ pullError }}
          </Alert>
        </div>
      </template>
    </div>

    <!-- Drops modal -->
    <DropsModal
      v-if="activeBanner"
      v-model="showDrops"
      :banner-name="activeBanner.name"
      :cards="activeBanner.cards"
    />

    <!-- Gem ceremony -->
    <GemCeremony
      :active="ceremonyActive"
      :top-tier="topTier"
      @done="onCeremonyDone"
    />

    <!-- 3D viewer -->
    <CardViewer3D
      :active="viewerActive"
      :cards="pullResult?.cards ?? []"
      @done="onViewerDone"
    />

    <!-- Summary -->
    <PullSummary
      v-if="pullResult && activeBanner"
      v-model="showSummary"
      :cards="pullResult.cards"
      :banner-name="activeBanner.name"
      :balance="balance"
      :pity="pullResult.pity"
      :pity-threshold="activeBanner.pity_threshold"
      :cost-x10="COST_X10"
      :instant="true"
      @again="onAgain"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useMediaQuery } from '@vueuse/core'
import { Gem } from 'lucide-vue-next'
import { useGachaStore } from '@/stores/gacha'
import { type DailyClaimResponse, type PullResult, type Rarity } from '@/api/gacha'
import Spinner from '@/components/ui/Spinner.vue'
import Button from '@/components/ui/Button.vue'
import Alert from '@/components/ui/Alert.vue'
import GachaSlider from '@/components/gacha/GachaSlider.vue'
import SpinDock from '@/components/gacha/SpinDock.vue'
import DropsModal from '@/components/gacha/DropsModal.vue'
import GemCeremony from '@/components/gacha/GemCeremony.vue'
import CardViewer3D from '@/components/gacha/CardViewer3D.vue'
import PullSummary from '@/components/gacha/PullSummary.vue'

const { t } = useI18n()
void t

const route = useRoute()
const store = useGachaStore()

const COST_X1 = 100
const COST_X10 = 900

const reducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')

const loadingWallet = computed(() => store.loadingWallet)
const loadingBanners = computed(() => store.loadingBanners)
const loadingDaily = computed(() => store.loadingDaily)
const loadingPull = computed(() => store.loadingPull)
const banners = computed(() => store.banners)
const bannersError = computed(() => store.bannersError)
const pullError = computed(() => store.pullError)
const balance = computed(() => store.balance)
const dailyStreak = computed(() => store.dailyStreak)

// ── Slider selection (preselect from ?banner= query) ──────────────────────────
const activeIndex = ref(0)
const activeBanner = computed(() => banners.value[activeIndex.value] ?? null)

function preselectFromQuery() {
  const id = route.query.banner as string | undefined
  if (!id) return
  const idx = banners.value.findIndex((b) => b.id === id)
  if (idx >= 0) activeIndex.value = idx
}

// ── Daily claim ───────────────────────────────────────────────────────────────
const dailyResult = ref<DailyClaimResponse | null>(null)
const alreadyClaimed = ref(false)

async function onClaimDaily() {
  const res = await store.claimDaily()
  if (res) {
    dailyResult.value = res
    alreadyClaimed.value = true
  }
}

// ── Pull flow: ceremony → viewer → summary ────────────────────────────────────
const showDrops = ref(false)
const pullResult = ref<PullResult | null>(null)
const ceremonyActive = ref(false)
const viewerActive = ref(false)
const showSummary = ref(false)

const TIER_RANK: Record<Rarity, number> = { N: 0, R: 1, SR: 2, SSR: 3 }
const topTier = computed<Rarity>(() => {
  const cards = pullResult.value?.cards ?? []
  return cards.reduce<Rarity>(
    (max, o) => (TIER_RANK[o.card.rarity] > TIER_RANK[max] ? o.card.rarity : max),
    'N',
  )
})

async function onPull(mode: 'x1' | 'x10') {
  if (!activeBanner.value) return
  const res = await store.pull(activeBanner.value.id, mode)
  if (!res) return
  pullResult.value = res
  // Refresh banners so pity bar reflects the new state.
  await store.fetchBanners()
  // Reduced motion → skip the spectacle straight to the summary grid.
  if (reducedMotion.value) {
    showSummary.value = true
  } else {
    ceremonyActive.value = true
  }
}

function onCeremonyDone(skipped: boolean) {
  ceremonyActive.value = false
  if (skipped) {
    showSummary.value = true
  } else {
    viewerActive.value = true
  }
}

function onViewerDone() {
  viewerActive.value = false
  showSummary.value = true
}

function onAgain() {
  showSummary.value = false
  void onPull('x10')
}

onMounted(async () => {
  await Promise.all([store.refreshWallet(), store.fetchBanners()])
  preselectFromQuery()
  // Detect today's claim (UTC) to disable the button without an error.
  if (store.wallet?.last_daily_at) {
    const last = new Date(store.wallet.last_daily_at)
    const now = new Date()
    if (
      last.getUTCFullYear() === now.getUTCFullYear() &&
      last.getUTCMonth() === now.getUTCMonth() &&
      last.getUTCDate() === now.getUTCDate()
    ) {
      alreadyClaimed.value = true
    }
  }
})

// Re-preselect if banners load after mount or the query changes.
watch(banners, preselectFromQuery)
watch(() => route.query.banner, preselectFromQuery)
</script>
