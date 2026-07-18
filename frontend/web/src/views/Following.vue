<template>
  <div class="mx-auto min-h-screen max-w-4xl px-4 pb-16 pt-24">
    <header class="mb-6">
      <p class="mb-2 text-sm font-medium uppercase tracking-wider text-brand-cyan">
        {{ $t('following.eyebrow') }}
      </p>
      <h1 class="text-3xl font-semibold text-foreground">{{ $t('following.title') }}</h1>
      <p class="mt-2 text-muted-foreground">{{ $t('following.subtitle') }}</p>
    </header>

    <div v-if="loading" class="flex justify-center py-16">
      <Spinner size="lg" />
    </div>

    <EmptyState
      v-else-if="users.length === 0"
      :title="$t('following.emptyTitle')"
      :description="$t('following.emptyDescription')"
      class="glass-card"
    >
      <template #icon><Users class="size-12" /></template>
    </EmptyState>

    <template v-else>
      <div class="mb-5 flex gap-2 overflow-x-auto pb-2" role="tablist" :aria-label="$t('following.filterLabel')">
        <button
          type="button"
          role="tab"
          :aria-selected="selectedUserId === ''"
          class="flex shrink-0 items-center gap-2 rounded-full border px-3 py-2 text-sm transition-colors"
          :class="selectedUserId === '' ? 'border-brand-cyan bg-brand-cyan/15 text-brand-cyan' : 'border-border bg-card text-muted-foreground hover:text-foreground'"
          @click="selectedUserId = ''"
        >
          <Users class="size-4" />
          {{ $t('following.all') }}
        </button>
        <button
          v-for="user in users"
          :key="user.id"
          type="button"
          role="tab"
          :aria-selected="selectedUserId === user.id"
          class="flex shrink-0 items-center gap-2 rounded-full border px-3 py-2 text-sm transition-colors"
          :class="selectedUserId === user.id ? 'border-brand-cyan bg-brand-cyan/15 text-brand-cyan' : 'border-border bg-card text-muted-foreground hover:text-foreground'"
          @click="selectedUserId = user.id"
        >
          <Avatar :src="user.avatar" :name="user.username" size="xs" class="size-6" />
          @{{ user.username }}
        </button>
      </div>

      <ActivityFeed
        :key="selectedUserId || 'all'"
        source="following"
        :user-id="selectedUserId"
        title-key="following.feedTitle"
        empty-key="following.noActivity"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Users } from 'lucide-vue-next'
import ActivityFeed from '@/components/ActivityFeed.vue'
import { Avatar, EmptyState, Spinner } from '@/components/ui'
import { followingApi } from '@/api/client'

interface FollowedUser {
  id: string
  username: string
  public_id?: string
  avatar?: string
  followed_at: string
}

const users = ref<FollowedUser[]>([])
const loading = ref(true)
const selectedUserId = ref('')

onMounted(async () => {
  try {
    const response = await followingApi.list()
    const data = response.data?.data || response.data
    users.value = data?.users || []
  } catch (error) {
    console.error('Failed to load followed users:', error)
  } finally {
    loading.value = false
  }
})
</script>
