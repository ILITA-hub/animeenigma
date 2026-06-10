<template>
  <div :class="cn('flex items-center', props.class)">
    <Avatar
      v-for="(a, i) in visible"
      :key="i"
      v-bind="a"
      :size="size"
      :class="cn('ring-2 ring-background', i > 0 && '-ml-2.5')"
    />
    <span
      v-if="overflow > 0"
      :class="cn('relative -ml-2.5 inline-flex items-center justify-center rounded-full bg-muted font-mono text-muted-foreground ring-2 ring-background', sizeClass)"
    >+{{ overflow }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import Avatar from './Avatar.vue'
import { avatarVariants, type AvatarSize, type AvatarStatus } from './avatar-variants'

interface AvatarItem { src?: string; name?: string; status?: AvatarStatus }

interface Props {
  items: AvatarItem[]
  max?: number
  size?: AvatarSize
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), { max: 4, size: 'md' })
const visible = computed(() => props.items.slice(0, props.max))
const overflow = computed(() => Math.max(0, props.items.length - props.max))
const sizeClass = computed(() => avatarVariants({ size: props.size }))
</script>
