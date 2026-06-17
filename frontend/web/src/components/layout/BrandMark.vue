<template>
  <!-- Neon Tokyo brand mark: 28×28 rounded square with cyan→pink gradient,
       4px inset cutout showing --color-base, "AE" centered in cyan. -->
  <span class="brand-mark" aria-hidden="true" />
</template>

<style scoped>
.brand-mark {
  display: inline-block;
  width: 28px;
  height: 28px;
  border-radius: var(--r-sm, 8px);
  background: linear-gradient(135deg, var(--brand-cyan) 0%, var(--brand-pink) 100%);
  box-shadow: var(--accent-glow, 0 0 30px var(--accent-line));
  position: relative;
  flex-shrink: 0;
}

/* Inset cutout showing page base colour */
.brand-mark::after {
  content: "";
  position: absolute;
  inset: 4px;
  border-radius: 5px;
  background: var(--color-base, #08080f);
}

/* "AE" letters sit above the cutout (z-index 1) */
.brand-mark::before {
  content: "AE";
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  font-family: var(--font-display);
  font-weight: 800;
  font-size: 11px;
  color: var(--brand-cyan);
  z-index: 1;
  line-height: 1;
}
</style>
