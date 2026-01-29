<template>
  <div class="profile">
    <div class="container">
      <h1>User Profile</h1>
      <div v-if="authStore.user" class="profile-content">
        <div class="profile-header">
          <div class="avatar">
            <img :src="authStore.user.avatar || '/default-avatar.png'" :alt="authStore.user.username" />
          </div>
          <div class="user-info">
            <h2>{{ authStore.user.username }}</h2>
            <p>{{ authStore.user.email }}</p>
            <span class="badge">{{ authStore.user.role }}</span>
          </div>
        </div>

        <div class="profile-sections">
          <section class="section">
            <h3>Watchlist</h3>
            <p class="coming-soon">Your watchlist will appear here</p>
          </section>

          <section class="section">
            <h3>Watch History</h3>
            <p class="coming-soon">Your watch history will appear here</p>
          </section>

          <section class="section">
            <h3>Settings</h3>
            <button @click="logout" class="btn btn-danger">Logout</button>
          </section>
        </div>
      </div>
      <div v-else class="loading">Loading...</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useRouter } from 'vue-router'

const authStore = useAuthStore()
const router = useRouter()

const logout = () => {
  authStore.logout()
  router.push('/')
}

onMounted(async () => {
  if (!authStore.user) {
    await authStore.fetchUser()
  }
})
</script>

<style scoped>
.profile {
  min-height: 80vh;
  padding: 2rem;
}

.container {
  max-width: 1000px;
  margin: 0 auto;
}

h1 {
  font-size: 2.5rem;
  margin-bottom: 2rem;
  color: #fff;
}

.profile-header {
  display: flex;
  gap: 2rem;
  align-items: center;
  margin-bottom: 3rem;
  padding: 2rem;
  background: #1a1a1a;
  border-radius: 12px;
}

.avatar {
  width: 120px;
  height: 120px;
  border-radius: 50%;
  overflow: hidden;
  border: 4px solid #ff6b6b;
}

.avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.user-info h2 {
  font-size: 1.8rem;
  margin-bottom: 0.5rem;
  color: #fff;
}

.user-info p {
  color: #999;
  margin-bottom: 0.5rem;
}

.badge {
  display: inline-block;
  padding: 0.25rem 0.75rem;
  background: #ff6b6b;
  border-radius: 20px;
  font-size: 0.85rem;
  text-transform: uppercase;
}

.profile-sections {
  display: flex;
  flex-direction: column;
  gap: 2rem;
}

.section {
  padding: 2rem;
  background: #1a1a1a;
  border-radius: 12px;
}

.section h3 {
  font-size: 1.5rem;
  margin-bottom: 1rem;
  color: #fff;
}

.coming-soon {
  color: #999;
  font-style: italic;
}

.btn {
  padding: 0.75rem 1.5rem;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 1rem;
  transition: all 0.3s;
}

.btn-danger {
  background: #ff6b6b;
  color: white;
}

.btn-danger:hover {
  background: #ff5252;
}

.loading {
  text-align: center;
  padding: 3rem;
  color: #999;
}
</style>
