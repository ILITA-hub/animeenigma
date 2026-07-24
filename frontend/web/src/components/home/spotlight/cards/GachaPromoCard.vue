<template>
  <SpotlightCardShell
    accent="violet"
    icon="gem"
    :kicker="t('spotlight.gachaPromo.title')"
    backdrop="gradient-mesh"
  >
    <!--
      «Лудка» launch promo (2026-07-24). Static feature card: violet accent
      (meta/service per the brand triad), gem kicker icon echoing the pull
      ceremony's ◆, a CSS-only card-deck fan on the right (no image
      fetches — the card can never show a skeleton), rarity chips in the
      SAME hues the gacha UI uses (PullSummary.vue: SSR orange / SR indigo /
      R teal), and economy numbers straight from the backend payload.
    -->
    <template #background-extra>
      <div
        aria-hidden="true"
        class="absolute inset-0 bg-gradient-to-r from-brand-violet/25 via-transparent to-transparent"
      />
    </template>

    <template #kicker-extra>
      <span class="rounded-full px-2 py-0.5 text-[10px] font-semibold bg-brand-violet/20 text-brand-violet">
        {{ t('spotlight.gachaPromo.newBadge') }}
      </span>
    </template>

    <div class="flex-1 min-h-0 flex flex-col md:flex-row gap-4 md:gap-8 md:items-center">
      <div class="flex-1 min-w-0 max-w-[600px]">
        <h3
          class="text-2xl md:text-3xl font-display font-semibold text-white leading-tight"
          data-testid="gacha-headline"
        >
          {{ t('spotlight.gachaPromo.headline') }}
        </h3>

        <p class="mt-2 text-[15px] leading-relaxed text-white/70 line-clamp-3 [text-wrap:pretty]">
          {{ t('spotlight.gachaPromo.description') }}
        </p>

        <div class="mt-3 flex flex-wrap items-center gap-2" data-testid="rarity-row">
          <span class="rarity-chip bg-white/10 text-white/70">N</span>
          <span class="rarity-chip bg-teal-400/15 text-teal-400">R</span>
          <span class="rarity-chip bg-indigo-400/15 text-indigo-400">SR</span>
          <span class="rarity-chip bg-orange-400/15 text-orange-400">SSR</span>
          <span class="text-[13px] text-muted-foreground font-medium">
            {{ t('spotlight.gachaPromo.pityHint', { n: data.pity_ssr_at }) }}
          </span>
        </div>

        <p class="mt-3 text-[13px] text-muted-foreground font-medium" data-testid="gacha-costs">
          {{ t('spotlight.gachaPromo.costsLine', { one: data.pull_cost_single, ten: data.pull_cost_ten }) }}
        </p>
        <p class="text-[13px] text-muted-foreground">
          {{ t('spotlight.gachaPromo.earnHint') }}
        </p>
      </div>

      <!-- Decorative deck fan — pure CSS, desktop only. -->
      <div class="promo-deck relative flex-shrink-0 self-center hidden md:block" aria-hidden="true">
        <div class="absolute -inset-4 bg-brand-violet/20 blur-3xl rounded-full" />
        <span class="promo-card promo-c1 border border-white/10 bg-gradient-to-br from-brand-violet/20 to-transparent text-brand-violet/50">◆</span>
        <span class="promo-card promo-c2 border border-white/10 bg-gradient-to-br from-brand-violet/25 to-transparent text-brand-violet/60">◆</span>
        <span class="promo-card promo-c3 border border-brand-violet/40 bg-gradient-to-br from-brand-violet/30 to-transparent text-brand-violet">◆</span>
      </div>
    </div>

    <template #cta>
      <router-link
        to="/gacha"
        :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']"
        data-testid="gacha-cta"
      >
        <Gem class="w-4 h-4" aria-hidden="true" />
        {{ t('spotlight.gachaPromo.cta') }}
      </router-link>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Gem } from 'lucide-vue-next'
import type { GachaPromoData } from '@/types/spotlight'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'

defineProps<{ data: GachaPromoData }>()
const { t } = useI18n()
</script>

<style scoped>
.rarity-chip {
  border-radius: 9999px;
  padding: 2px 10px;
  font-family: var(--font-mono);
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 0.08em;
}
.promo-deck { width: 148px; height: 156px; }
.promo-card {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 92px;
  height: 130px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  font-size: 26px;
}
.promo-c1 { transform: translate(-50%, -50%) rotate(-14deg) translateX(-22px); }
.promo-c2 { transform: translate(-50%, -50%) rotate(-2deg) translateY(-4px); }
.promo-c3 { transform: translate(-50%, -50%) rotate(11deg) translateX(22px) translateY(-8px); }
</style>
