<template>
  <div class="not-found">
    <div class="content">
      <h1>{{ $t('notFound.code') }}</h1>
      <h2>{{ $t('notFound.title') }}</h2>
      <p>{{ $t('notFound.description') }}</p>
      <router-link to="/" class="btn">{{ $t('notFound.goHome') }}</router-link>
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
  color: #ff6b6b;
  margin-bottom: 1rem;
  font-weight: bold;
}

.content h2 {
  font-size: 2rem;
  color: #fff;
  margin-bottom: 1rem;
}

.content p {
  font-size: 1.2rem;
  color: #999;
  margin-bottom: 2rem;
}

.btn {
  display: inline-block;
  padding: 1rem 2rem;
  background: #ff6b6b;
  color: white;
  text-decoration: none;
  border-radius: 8px;
  font-size: 1.1rem;
  transition: background 0.3s;
}

.btn:hover {
  background: #ff5252;
}
</style>
