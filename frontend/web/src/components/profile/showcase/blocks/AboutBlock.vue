<script setup lang="ts">
import { computed } from 'vue'
import { defaultVariant } from '@/types/showcase'
import type { AboutConfig } from '@/types/showcase'

const props = defineProps<{ config: AboutConfig; variant?: string }>()
const v = computed(() => props.variant || defaultVariant('about'))
</script>

<template>
  <!-- quote (default): accent bar + large text + mono status -->
  <div v-if="v === 'quote'" class="about-quote relative h-full rounded-xl border border-border bg-card">
    <p class="q font-semibold text-foreground leading-relaxed">
      {{ config.text }}
    </p>
    <p v-if="config.title" class="sig mt-3 font-medium text-muted-foreground">
      {{ config.title }}
    </p>
  </div>

  <!-- bio: avatar + name + paragraph + chips -->
  <div v-else-if="v === 'bio'" class="h-full rounded-xl border border-border bg-card p-4 md:p-6">
    <div class="flex items-center gap-3.5 mb-3.5">
      <div class="about-av-ring shrink-0 rounded-[14px] p-0.5">
        <div class="flex h-full w-full items-center justify-center rounded-xl bg-card font-semibold text-foreground text-sm">
          {{ config.title ? config.title.slice(0, 2).toUpperCase() : 'AE' }}
        </div>
      </div>
      <div>
        <p class="font-semibold text-foreground text-base leading-tight">{{ config.title }}</p>
      </div>
    </div>
    <p class="text-sm text-muted-foreground leading-relaxed">{{ config.text }}</p>
  </div>

  <!-- terminal: hacker style Neon Tokyo -->
  <div v-else-if="v === 'terminal'" class="h-full rounded-xl border border-border overflow-hidden bg-[#0a0a13]">
    <div class="flex items-center gap-1.5 px-4 py-2.5 border-b border-border">
      <span class="inline-block w-2.5 h-2.5 rounded-full bg-[#ff5f57]"></span>
      <span class="inline-block w-2.5 h-2.5 rounded-full bg-[#febc2e]"></span>
      <span class="inline-block w-2.5 h-2.5 rounded-full bg-[#28c840]"></span>
      <span class="ml-2 text-xs text-muted-foreground font-medium">
        ~/{{ config.title || 'user' }} — zsh
      </span>
    </div>
    <div class="p-4 md:p-5 font-mono text-[13.5px] leading-[1.85]">
      <div>
        <span class="text-success font-medium">➜</span>
        <span class="ml-1 text-primary font-medium">whoami</span>
      </div>
      <div class="text-foreground">{{ config.title }}</div>
      <div>
        <span class="text-success font-medium">➜</span>
        <span class="ml-1 text-primary font-medium">cat about.txt</span>
      </div>
      <div class="text-foreground">{{ config.text }}</div>
      <div>
        <span class="text-success font-medium">➜</span>
        <span class="about-cursor ml-1 inline-block w-2 h-[15px] bg-primary align-middle"></span>
      </div>
    </div>
  </div>

  <!-- minimal: large centered statement, no border -->
  <div v-else-if="v === 'minimal'" class="h-full py-8 px-5 text-center">
    <p class="about-min-big font-semibold text-foreground text-2xl leading-snug tracking-tight max-w-xl mx-auto">
      {{ config.text }}
    </p>
    <p v-if="config.title" class="mt-3.5 text-xs font-medium text-muted-foreground">
      — {{ config.title }}
    </p>
  </div>

  <!-- vn: visual novel dialog box -->
  <div v-else-if="v === 'vn'" class="about-vn relative h-full rounded-xl overflow-hidden flex items-end min-h-[200px]">
    <div class="about-vn-bg absolute inset-0"></div>
    <div class="about-vn-box relative mx-4 mb-4 ml-[calc(120px+16px)] border rounded-xl p-4 md:p-5 border-primary/40 bg-black/80 backdrop-blur">
      <span class="about-nametag absolute -top-3 left-4 text-xs font-semibold px-3 py-1 rounded-lg bg-primary text-primary-foreground shadow-md">
        {{ config.title }}
      </span>
      <p class="text-sm text-foreground leading-relaxed">{{ config.text }}</p>
      <span class="about-arrow absolute right-3 bottom-2 text-primary font-medium text-xs">▼</span>
    </div>
  </div>
</template>

<style scoped>
/* quote variant: gradient accent bar */
.about-quote {
  padding: 26px 26px 26px 30px;
}
.about-quote::before {
  content: '';
  position: absolute;
  left: 0;
  top: 18px;
  bottom: 18px;
  width: 3px;
  border-radius: 3px;
  background: linear-gradient(var(--brand-cyan), var(--brand-pink));
  box-shadow: 0 0 16px var(--cyan-a40);
}
.about-quote .q {
  font-size: 18px;
  line-height: 1.55;
}
.about-quote .sig {
  font-size: 13px;
  font-family: var(--font-mono);
}

/* bio: avatar gradient ring */
.about-av-ring {
  width: 56px;
  height: 56px;
  background: conic-gradient(from 180deg, var(--brand-cyan), var(--brand-violet), var(--brand-pink), var(--brand-cyan));
}

/* terminal: blinking cursor */
.about-cursor {
  animation: about-blink 1.1s steps(1) infinite;
}
@keyframes about-blink {
  50% { opacity: 0; }
}

/* minimal: gradient text on key phrase */
.about-min-big :deep(b),
.about-min-big b {
  background: linear-gradient(135deg, var(--brand-cyan), var(--brand-violet));
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

/* vn: background gradient overlay */
.about-vn-bg {
  background: linear-gradient(120deg, var(--cyan-a20), var(--accent-soft), var(--pink-soft));
}

/* vn: bouncing arrow */
.about-arrow {
  animation: about-bob 1s ease-in-out infinite;
}
@keyframes about-bob {
  50% { transform: translateY(3px); }
}
</style>
