<template>
  <section
    ref="heroRef"
    class="relative h-screen min-h-[600px] max-h-[900px] overflow-hidden"
    @mousemove="handleMouseMove"
    @mouseleave="handleMouseLeave"
  >
    <!-- Background Layer (Static Poster) -->
    <div
      class="absolute inset-0 bg-cover bg-center bg-no-repeat"
      :style="{ backgroundImage: `url(${backgroundImage})` }"
    />

    <!-- Gradient Overlay -->
    <div class="absolute inset-0 bg-gradient-to-b from-base/30 via-base/50 to-base" />

    <!-- Volumetric Lighting Effects -->
    <div
      class="absolute inset-0 pointer-events-none"
      :style="{ transform: `translate3d(${parallax.x * 0.02}px, ${parallax.y * 0.02}px, 0)` }"
    >
      <div class="absolute top-1/4 left-1/4 w-96 h-96 bg-cyan-400/10 rounded-full blur-[100px]" />
      <div class="absolute bottom-1/3 right-1/4 w-80 h-80 bg-pink-500/10 rounded-full blur-[80px]" />
    </div>

    <!-- Midground Parallax Layer -->
    <div
      v-if="!prefersReducedMotion"
      class="absolute inset-0 transition-transform duration-100 ease-out"
      :style="{ transform: `translate3d(${parallax.x * 0.015}px, ${parallax.y * 0.015}px, 0)` }"
    >
      <slot name="midground" />
    </div>

    <!-- Foreground Layer with Mouse Tilt -->
    <div
      v-if="!prefersReducedMotion"
      class="absolute inset-0 transition-transform duration-200 ease-out"
      :style="{ transform: `translate3d(${parallax.x * 0.03}px, ${parallax.y * 0.03}px, 0)` }"
    >
      <slot name="foreground" />
    </div>

    <!-- Floating Particles -->
    <div
      v-if="!prefersReducedMotion && isVisible"
      class="absolute inset-0 pointer-events-none overflow-hidden"
    >
      <div
        v-for="particle in particles"
        :key="particle.id"
        class="absolute rounded-full bg-white/20"
        :style="particle.style"
      />
    </div>

    <!-- Content Overlay -->
    <div class="absolute inset-0 flex flex-col items-center justify-center px-4 text-center">
      <div class="max-w-2xl">
        <!-- Title -->
        <h1
          class="text-4xl md:text-5xl lg:text-6xl font-bold mb-4 text-white"
          :class="{ 'animate-fade-in': mounted }"
        >
          <span class="text-glow-cyan">{{ $t('hero.tagline') }}</span>
        </h1>

        <!-- Subtitle -->
        <p
          class="text-lg md:text-xl text-white/70 mb-8 max-w-lg mx-auto"
          :class="{ 'animate-slide-up': mounted }"
          style="animation-delay: 0.1s"
        >
          <slot name="subtitle">
            {{ subtitle }}
          </slot>
        </p>

        <!-- CTA Buttons -->
        <div
          class="flex flex-col sm:flex-row gap-4 justify-center"
          :class="{ 'animate-slide-up': mounted }"
          style="animation-delay: 0.2s"
        >
          <Button size="lg" @click="$router.push('/browse')">
            {{ $t('hero.browse') }}
          </Button>
          <Button size="lg" variant="outline" @click="$emit('signin')">
            {{ $t('hero.signin') }}
          </Button>
        </div>
      </div>
    </div>

    <!-- Scroll Indicator -->
    <div
      v-if="!prefersReducedMotion"
      class="absolute bottom-8 left-1/2 -translate-x-1/2 flex flex-col items-center gap-2 text-white/50 animate-pulse-glow"
    >
      <span class="text-sm">{{ $t('hero.scroll') }}</span>
      <svg class="w-6 h-6 animate-bounce" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
      </svg>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useMediaQuery, useIntersectionObserver } from '@vueuse/core'
import Button from '@/components/ui/Button.vue'

interface Props {
  backgroundImage?: string
  subtitle?: string
}

withDefaults(defineProps<Props>(), {
  backgroundImage: '/images/hero-bg.jpg',
  subtitle: '',
})

defineEmits<{
  signin: []
}>()

const heroRef = ref<HTMLElement | null>(null)
const mounted = ref(false)
const isVisible = ref(true)
const prefersReducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')

// Parallax state
const parallax = reactive({ x: 0, y: 0 })

// Mouse movement handler for parallax effect
const handleMouseMove = (event: MouseEvent) => {
  if (prefersReducedMotion.value) return

  const rect = heroRef.value?.getBoundingClientRect()
  if (!rect) return

  const centerX = rect.width / 2
  const centerY = rect.height / 2
  const mouseX = event.clientX - rect.left
  const mouseY = event.clientY - rect.top

  parallax.x = (mouseX - centerX) / centerX * 20
  parallax.y = (mouseY - centerY) / centerY * 20
}

const handleMouseLeave = () => {
  parallax.x = 0
  parallax.y = 0
}

// Generate floating particles
interface Particle {
  id: number
  style: {
    left: string
    top: string
    width: string
    height: string
    animation: string
    opacity: number
  }
}

const particles = computed<Particle[]>(() => {
  if (prefersReducedMotion.value) return []

  return Array.from({ length: 12 }, (_, i) => ({
    id: i,
    style: {
      left: `${Math.random() * 100}%`,
      top: `${Math.random() * 100}%`,
      width: `${Math.random() * 4 + 2}px`,
      height: `${Math.random() * 4 + 2}px`,
      animation: `float ${6 + Math.random() * 4}s ease-in-out infinite`,
      animationDelay: `${Math.random() * 5}s`,
      opacity: Math.random() * 0.5 + 0.2,
    } as Particle['style'],
  }))
})

// Visibility observer for performance
useIntersectionObserver(
  heroRef,
  ([entry]) => {
    isVisible.value = entry?.isIntersecting ?? true
  },
  { threshold: 0.1 }
)

onMounted(() => {
  mounted.value = true
})
</script>

<style scoped>
@keyframes float {
  0%, 100% {
    transform: translateY(0) translateX(0);
  }
  25% {
    transform: translateY(-20px) translateX(10px);
  }
  50% {
    transform: translateY(-10px) translateX(-5px);
  }
  75% {
    transform: translateY(-30px) translateX(5px);
  }
}
</style>
