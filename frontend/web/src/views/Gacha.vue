<template>
  <!-- /gacha — daily claim card + active banner list -->
  <div class="min-h-screen bg-base pt-20">
    <div class="container mx-auto px-4 py-8 max-w-4xl">
      <!-- Loading state -->
      <div v-if="loadingWallet && loadingBanners" class="flex justify-center py-20">
        <Spinner size="lg" />
      </div>

      <template v-else>
        <!-- Page header -->
        <div class="mb-8">
          <h1 class="text-3xl font-semibold text-white">{{ $t('gacha.nav_item') }}</h1>
          <p class="text-muted-foreground text-sm mt-1">
            <Gem class="inline size-4 mr-1 text-orange-400" aria-hidden="true" />
            <span :aria-label="$t('gacha.balance_chip_aria', { n: balance })">
              {{ balance }} Энигмы
            </span>
          </p>
        </div>

        <!-- Daily claim card -->
        <div class="glass-card p-4 md:p-6 mb-8">
          <h2 class="text-lg font-semibold text-white mb-3">{{ $t('gacha.daily_claim_title') }}</h2>
          <div class="flex flex-wrap items-center gap-4">
            <div class="flex-1 min-w-0">
              <p v-if="dailyStreak > 0" class="text-muted-foreground text-sm">
                {{ $t('gacha.daily_streak_label', { n: dailyStreak }) }}
              </p>
              <p v-if="dailyResult?.claimed && dailyResult.amount" class="text-orange-400 text-sm font-medium mt-1">
                {{ $t('gacha.daily_amount', { n: dailyResult.amount }) }}
              </p>
            </div>
            <Button
              :disabled="alreadyClaimed || loadingDaily"
              variant="outline"
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

        <!-- Banner list loading -->
        <div v-if="loadingBanners" class="flex justify-center py-10">
          <Spinner />
        </div>

        <!-- Empty banners -->
        <div v-else-if="banners.length === 0" class="glass-card p-8 text-center text-muted-foreground">
          {{ $t('gacha.banner_list_empty') }}
        </div>

        <!-- Banner grid -->
        <div v-else class="space-y-4">
          <h2 class="text-lg font-semibold text-white">{{ $t('gacha.banner_list_title') }}</h2>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <router-link
              v-for="banner in banners"
              :key="banner.id"
              :to="`/gacha/${banner.id}`"
              class="glass-card p-4 group flex gap-4 hover:border-orange-400/30 transition-colors"
              :aria-label="$t('gacha.banner_open') + ': ' + banner.name"
            >
              <!-- Banner art thumbnail -->
              <div class="w-20 h-20 rounded-lg overflow-hidden flex-shrink-0 bg-white/5">
                <img
                  v-if="banner.art_path"
                  :src="cardImageUrl(banner.art_path)"
                  :alt="banner.name"
                  class="w-full h-full object-cover"
                />
                <div v-else class="w-full h-full flex items-center justify-center">
                  <Gem class="size-8 text-orange-400/40" aria-hidden="true" />
                </div>
              </div>
              <!-- Banner info -->
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 flex-wrap mb-1">
                  <span class="text-white font-semibold truncate">{{ banner.name }}</span>
                  <Badge v-if="banner.is_standard" variant="default" size="sm" class="flex-shrink-0">
                    {{ $t('gacha.banner_standard_badge') }}
                  </Badge>
                </div>
                <p v-if="banner.description" class="text-muted-foreground text-sm line-clamp-2">
                  {{ banner.description }}
                </p>
              </div>
              <ChevronRight class="size-5 text-muted-foreground group-hover:text-white transition-colors flex-shrink-0 self-center" aria-hidden="true" />
            </router-link>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Gem, ChevronRight } from 'lucide-vue-next'
import { useGachaStore } from '@/stores/gacha'
import { cardImageUrl, type DailyClaimResponse } from '@/api/gacha'
import Spinner from '@/components/ui/Spinner.vue'
import Button from '@/components/ui/Button.vue'
import Badge from '@/components/ui/Badge.vue'
import Alert from '@/components/ui/Alert.vue'

const { t } = useI18n()
void t // suppress unused-var lint

const store = useGachaStore()
const loadingWallet = computed(() => store.loadingWallet)
const loadingBanners = computed(() => store.loadingBanners)
const loadingDaily = computed(() => store.loadingDaily)
const banners = computed(() => store.banners)
const bannersError = computed(() => store.bannersError)
const balance = computed(() => store.balance)
const dailyStreak = computed(() => store.dailyStreak)

const dailyResult = ref<DailyClaimResponse | null>(null)
// Track within-session claim so the button disables after the first press.
const alreadyClaimed = ref(false)

async function onClaimDaily() {
  const res = await store.claimDaily()
  if (res) {
    dailyResult.value = res
    // Backend returns claimed=false when already claimed today; we disable
    // the button only after a fresh claim.
    if (res.claimed) {
      alreadyClaimed.value = true
    } else {
      alreadyClaimed.value = true // already claimed (idempotent response)
    }
  }
}

onMounted(async () => {
  await Promise.all([store.refreshWallet(), store.fetchBanners()])
  // If the user already claimed today, disable the button without showing an
  // error — detect via last_daily_at being today (UTC).
  if (store.wallet?.last_daily_at) {
    const lastDaily = new Date(store.wallet.last_daily_at)
    const today = new Date()
    if (
      lastDaily.getUTCFullYear() === today.getUTCFullYear() &&
      lastDaily.getUTCMonth() === today.getUTCMonth() &&
      lastDaily.getUTCDate() === today.getUTCDate()
    ) {
      alreadyClaimed.value = true
    }
  }
})
</script>
