<template>
  <div class="not-found">
    <div class="content">
      <h1>404</h1>
      <h2>Page Not Found</h2>
      <p>The page you're looking for doesn't exist or has been moved.</p>
      <router-link to="/" class="btn">Go Home</router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
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
