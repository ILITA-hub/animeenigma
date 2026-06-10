<template>
  <span :class="cn(avatarVariants({ size }), props.class)">
    <span :class="cn('flex size-full items-center justify-center overflow-hidden rounded-full bg-brand-cyan/15 font-semibold leading-none text-brand-cyan', fallbackClass)">
      <img
        v-if="src && !errored"
        :src="src"
        :alt="name ?? ''"
        class="size-full object-cover"
        loading="lazy"
        @error="errored = true"
      />
      <template v-else>{{ initials }}</template>
    </span>
    <span
      v-if="status"
      :class="cn('absolute bottom-0 right-0 rounded-full ring-2 ring-background', avatarDotSize[size], avatarDotColor[status])"
    />
    <!-- Overlay slot — absolutely-positioned adornments (edit button, live dot, …) -->
    <slot />
  </span>
</template>

<script setup lang="ts">
import { ref, computed, watch, type HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import {
  avatarVariants, avatarInitials, avatarDotSize, avatarDotColor,
  type AvatarSize, type AvatarStatus,
} from './avatar-variants'

interface Props {
  src?: string
  name?: string
  size?: AvatarSize
  status?: AvatarStatus
  /** Extra classes for the inner fallback/image chip — e.g. a per-user hashed bg color. */
  fallbackClass?: HTMLAttributes['class']
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), { size: 'md' })
const errored = ref(false)
watch(() => props.src, () => { errored.value = false })
const initials = computed(() => avatarInitials(props.name))
</script>
