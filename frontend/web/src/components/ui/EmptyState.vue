<!--
  EmptyState — DS primitive for "nothing here yet" placeholders.

  Replaces the ~30 hand-rolled `text-center py-N text-muted-foreground` blocks
  scattered across views/players. Fully slot-driven so it covers every shape we
  had in the wild:

    1. Text-only (the common case) — pass the message in the default slot:
         <EmptyState>{{ $t('schedule.empty') }}</EmptyState>

    2. Icon + text:
         <EmptyState size="sm">
           <template #icon><Bell class="size-10" /></template>
           {{ $t('notifications.dropdown.empty') }}
         </EmptyState>

    3. Title + description + CTA:
         <EmptyState :title="t('...')" :description="t('...')">
           <template #icon><Inbox class="size-12" /></template>
           <template #action><Button @click="...">{{ t('...') }}</Button></template>
         </EmptyState>

  The container is muted (text-muted-foreground); the #icon slot inherits a
  further-dimmed wrapper, and `title` renders in full foreground so it reads as
  a heading. Render order: icon → title → description → default slot → action.
-->

<script setup lang="ts">
import { type HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { emptyStateVariants, type EmptyStateVariants } from './empty-state-variants'

interface Props {
  title?: string
  description?: string
  size?: EmptyStateVariants['size']
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  size: 'md',
})
</script>

<template>
  <div :class="cn(emptyStateVariants({ size: props.size }), props.class)">
    <div v-if="$slots.icon" class="text-muted-foreground/50" aria-hidden="true">
      <slot name="icon" />
    </div>

    <p v-if="props.title" class="font-medium text-foreground">{{ props.title }}</p>

    <p v-if="props.description" class="text-sm [text-wrap:balance]">{{ props.description }}</p>

    <slot />

    <div v-if="$slots.action" class="mt-4">
      <slot name="action" />
    </div>
  </div>
</template>
