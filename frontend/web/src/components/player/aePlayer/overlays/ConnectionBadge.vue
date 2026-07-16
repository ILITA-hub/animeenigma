<template>
  <div
    v-if="state !== 'ok'"
    class="pl-conn"
    :class="`pl-conn--${state}`"
    role="status"
    aria-live="polite"
    :title="label"
    :aria-label="label"
    data-test="connection-badge"
  >
    <!-- Offline: the slashed glyph says it on its own. -->
    <WifiOff v-if="state === 'offline'" class="pl-conn-glyph" aria-hidden="true" />
    <!-- Slow: wifi glyph + a small exclamation mark. -->
    <span v-else class="pl-conn-wifi">
      <Wifi class="pl-conn-glyph" aria-hidden="true" />
      <span class="pl-conn-bang" aria-hidden="true">!</span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Wifi, WifiOff } from 'lucide-vue-next'
import type { ConnectionState } from '../connectionHealth'

const props = defineProps<{ state: ConnectionState }>()

const { t } = useI18n()

// Honest, soft wording (owner's call): describe the condition, don't accuse — it
// could be a laggy CDN rather than the user's ISP. Also the accessible name.
const label = computed(() =>
  props.state === 'offline'
    ? t('player.aePlayer.connectionOffline')
    : t('player.aePlayer.connectionSlow'),
)
</script>

<style scoped>
/* Always-on (NOT gated on chrome idle): a stall matters most while the viewer is
   passively watching with the controls hidden. Top-left is the conventional
   network-signal corner; z-7 sits above the top scrim (z-6), below HUD/controls. */
.pl-conn {
  position: absolute;
  top: 16px;
  left: 16px;
  z-index: 7;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--brand-cyan);
  filter: drop-shadow(0 0 6px var(--cyan-a60));
  pointer-events: none;
  animation: pl-conn-blink 1.8s ease-in-out infinite;
}

.pl-conn-glyph {
  width: 26px;
  height: 26px;
}

.pl-conn-wifi {
  position: relative;
  display: inline-flex;
}

.pl-conn-bang {
  position: absolute;
  right: -3px;
  bottom: -3px;
  font-size: 14px;
  font-weight: 700;
  line-height: 1;
  color: var(--brand-cyan);
  text-shadow: 0 1px 2px var(--black-a60);
}

@keyframes pl-conn-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.28; }
}

/* Respect reduced-motion: hold it solid instead of blinking. */
@media (prefers-reduced-motion: reduce) {
  .pl-conn {
    animation: none;
    opacity: 1;
  }
}
</style>
