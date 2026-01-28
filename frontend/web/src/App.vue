<template>
  <div id="app">
    <header v-if="!isFullscreen">
      <nav class="navbar">
        <div class="nav-brand">
          <router-link to="/">AnimeEnigma</router-link>
        </div>
        <div class="nav-links">
          <router-link to="/">Home</router-link>
          <router-link to="/browse">Browse</router-link>
          <router-link to="/search">Search</router-link>
          <router-link to="/game">Game Rooms</router-link>
          <router-link v-if="authStore.isAuthenticated" to="/profile">Profile</router-link>
          <button v-if="!authStore.isAuthenticated" @click="login">Login</button>
          <button v-else @click="logout">Logout</button>
        </div>
      </nav>
    </header>

    <main :class="{ fullscreen: isFullscreen }">
      <router-view />
    </main>

    <footer v-if="!isFullscreen">
      <p>&copy; 2024 AnimeEnigma. All rights reserved.</p>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const authStore = useAuthStore()

const isFullscreen = computed(() => route.name === 'watch')

const login = () => {
  // TODO: Implement login modal or redirect
  console.log('Login clicked')
}

const logout = () => {
  authStore.logout()
}
</script>

<style scoped>
#app {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

.navbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem 2rem;
  background: #1a1a1a;
  border-bottom: 2px solid #333;
}

.nav-brand a {
  font-size: 1.5rem;
  font-weight: bold;
  color: #ff6b6b;
  text-decoration: none;
}

.nav-links {
  display: flex;
  gap: 1.5rem;
  align-items: center;
}

.nav-links a {
  color: #fff;
  text-decoration: none;
  transition: color 0.3s;
}

.nav-links a:hover,
.nav-links a.router-link-active {
  color: #ff6b6b;
}

.nav-links button {
  padding: 0.5rem 1rem;
  background: #ff6b6b;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  transition: background 0.3s;
}

.nav-links button:hover {
  background: #ff5252;
}

main {
  flex: 1;
  background: #0f0f0f;
}

main.fullscreen {
  padding: 0;
}

footer {
  padding: 1rem;
  text-align: center;
  background: #1a1a1a;
  color: #888;
  border-top: 2px solid #333;
}
</style>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
  background: #0f0f0f;
  color: #fff;
}
</style>
