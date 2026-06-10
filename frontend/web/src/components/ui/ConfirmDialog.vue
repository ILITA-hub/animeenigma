<!--
  ConfirmDialog — themed replacement for the native `window.confirm()`.

  Presentational only: it renders a small Modal with a title, a description,
  and a Cancel / Confirm button pair. It owns NO state — `open` is controlled
  by the parent, and it emits `confirm` / `cancel` (plus `update:open` so it
  can be used with `v-model:open`).

  Two ways to use it:
    1. Declaratively, with v-model:open + @confirm in a specific view.
    2. Imperatively via `useConfirm()` (promise-based) — the global
       <ConfirmDialogHost> (mounted once in App.vue) drives this component
       from module-level state, so any call site can do:
         if (!(await confirm({ title, description, variant: 'destructive' }))) return

  Closing via Esc / backdrop / the dismissed state counts as a CANCEL — the
  same semantics as native confirm() returning false. There is no close (X)
  button: a confirm dialog must be answered with one of the two buttons (or
  dismissed, which is cancel).
-->

<script setup lang="ts">
import Modal from './Modal.vue'
import Button from './Button.vue'

interface Props {
  open: boolean
  title?: string
  description?: string
  confirmText?: string
  cancelText?: string
  /** Confirm-button styling. `destructive` for irreversible actions. */
  variant?: 'default' | 'destructive'
}

const props = withDefaults(defineProps<Props>(), {
  confirmText: 'Confirm',
  cancelText: 'Cancel',
  variant: 'default',
})

const emit = defineEmits<{
  confirm: []
  cancel: []
  'update:open': [value: boolean]
}>()

function onConfirm() {
  emit('update:open', false)
  emit('confirm')
}

function onCancel() {
  emit('update:open', false)
  emit('cancel')
}

// Esc / backdrop / any Modal-driven close resolves as a cancel. We bind only
// @update:model-value (NOT @close too) so Modal's twin close+update emits don't
// fire cancel twice.
function onOpenUpdate(value: boolean) {
  if (!value) onCancel()
}

// Exposed so the co-located spec can exercise the handlers without depending on
// Reka's portaled/focus-trapped DOM, which jsdom cannot fully simulate (same
// pattern as Modal.spec).
defineExpose({ onConfirm, onCancel, onOpenUpdate })
</script>

<template>
  <Modal
    :model-value="props.open"
    :title="props.title"
    size="sm"
    :closable="false"
    @update:model-value="onOpenUpdate"
  >
    <p v-if="props.description" class="text-sm text-white/80 [overflow-wrap:anywhere]">
      {{ props.description }}
    </p>

    <template #footer>
      <Button variant="ghost" @click="onCancel">{{ props.cancelText }}</Button>
      <Button :variant="props.variant" @click="onConfirm">{{ props.confirmText }}</Button>
    </template>
  </Modal>
</template>
