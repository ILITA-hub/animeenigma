<template>
  <!--
    ButtonGroup — semantic wrapper for ARIA toggle groups.

    Phase 5 (UX-12) of the 2026-05-12 UX audit workstream: introduces the
    role="group" + aria-label shell expected by the WAI-ARIA "Toggle Button"
    pattern for radio-like and toggle-like button rows. The component is
    deliberately style-agnostic — children render their own classes — so the
    5 existing toggle surfaces (Anime RU/EN, provider chips, Themes type
    filter, Game answer options, Navbar mobile-lang) can keep their bespoke
    visual treatments while sharing the ARIA semantics.

    Required prop: `label` — accessible name for the group (visible label or
    short description). Optional `pressed` is not a prop here because each
    child button binds its own `aria-pressed` from its own selected-state
    computation; the wrapper only provides the group role.
  -->
  <div
    role="group"
    :aria-label="label"
    :class="containerClass"
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  containerClass?: string
}>()
</script>
