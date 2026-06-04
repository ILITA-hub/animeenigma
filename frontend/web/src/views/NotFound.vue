<template>
  <div class="not-found">
    <div class="content">
      <h1>{{ $t('notFound.code') }}</h1>
      <h2>{{ $t('notFound.title') }}</h2>
      <p>{{ $t('notFound.description') }}</p>
      <router-link to="/" class="home-link">{{ $t('notFound.goHome') }}</router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

useI18n()
</script>

<style scoped>
.not-found {
  min-height: 80vh;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
}

.content h1 {
  font-size: 8rem;
  color: var(--destructive);
  margin-bottom: 1rem;
  font-weight: bold;
}

.content h2 {
  font-size: 2rem;
  color: var(--foreground);
  margin-bottom: 1rem;
}

.content p {
  font-size: 1.2rem;
  color: var(--muted-foreground);
  margin-bottom: 2rem;
}

.home-link {
  display: inline-block;
  padding: 1rem 2rem;
  background: var(--destructive);
  color: white;
  text-decoration: none;
  border-radius: 8px;
  font-size: 1.1rem;
  font-weight: 500;
  transition: background 0.3s;
}

.home-link:hover {
  background: var(--destructive);
}
</style>
