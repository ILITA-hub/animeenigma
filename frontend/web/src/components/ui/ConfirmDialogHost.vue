<!--
  ConfirmDialogHost — single global mount point for the promise-based
  useConfirm() API. Mounted once in App.vue (alongside <Toaster />). It binds
  the module-level useConfirm state to a <ConfirmDialog> and routes its events
  back to accept() / cancel(), which resolve the pending confirm() promise.

  Direct import of ConfirmDialog (not the @/components/ui barrel) keeps the
  dependency graph tight and avoids pulling unrelated barrel members.
-->

<script setup lang="ts">
import ConfirmDialog from './ConfirmDialog.vue'
import { useConfirm } from '@/composables/useConfirm'

const { state, accept, cancel } = useConfirm()
</script>

<template>
  <ConfirmDialog
    :open="state.open"
    :title="state.title"
    :description="state.description"
    :confirm-text="state.confirmText"
    :cancel-text="state.cancelText"
    :variant="state.variant"
    @confirm="accept"
    @cancel="cancel"
  />
</template>
