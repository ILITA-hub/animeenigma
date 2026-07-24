<template>
  <!-- Admin dashboard landing — Vue replacement for the old hardcoded gateway
       HTML page. Lists every admin tool as a styled Neon-Tokyo card. Auth is
       enforced upstream (gateway /admin JWT+AdminRole) AND by the router guard
       (meta.requiresAdmin), so this only ever renders for admins. -->
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-7xl">
      <!-- Header -->
      <div class="mb-8">
        <h1 class="text-3xl font-semibold text-white">{{ $t('admin.dashboard.title') }}</h1>
        <p class="text-white/60 text-sm mt-1">{{ $t('admin.dashboard.subtitle') }}</p>
      </div>

      <!-- Tool card grid -->
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        <component
          :is="tool.external ? 'a' : 'router-link'"
          v-for="tool in tools"
          :key="tool.key"
          v-bind="tool.external ? { href: tool.to } : { to: tool.to }"
          class="admin-tool-card glass-card p-6 group"
          data-testid="admin-tool-card"
        >
          <div class="flex items-start gap-4">
            <span class="admin-tool-icon" :class="`admin-tool-icon--${tool.accent}`" aria-hidden="true">
              <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" :d="tool.icon" />
              </svg>
            </span>
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <h2 class="text-base font-semibold text-white truncate">{{ $t(tool.label) }}</h2>
                <span v-if="tool.external" class="text-white/30 text-[10px] uppercase tracking-wide">↗</span>
              </div>
              <p class="text-white/55 text-sm mt-1">{{ $t(tool.desc) }}</p>
            </div>
          </div>
        </component>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
interface AdminTool {
  key: string
  to: string
  label: string
  desc: string
  icon: string
  accent: 'cyan' | 'pink'
  external?: boolean
}

// Heroicons-style single-path SVGs (stroke). Kept inline to avoid a new dep.
const tools: AdminTool[] = [
  {
    key: 'grafana',
    to: '/admin/grafana/',
    external: true,
    label: 'admin.dashboard.grafana',
    desc: 'admin.dashboard.grafanaDesc',
    accent: 'cyan',
    icon: 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z',
  },
  {
    key: 'recs',
    to: '/admin/recs',
    label: 'admin.recs.title',
    desc: 'admin.dashboard.recsDesc',
    accent: 'pink',
    icon: 'M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z',
  },
  {
    key: 'feedback',
    to: '/admin/feedback',
    label: 'admin.feedback.title',
    desc: 'admin.dashboard.feedbackDesc',
    accent: 'pink',
    icon: 'M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z',
  },
  {
    key: 'collections',
    to: '/admin/collections',
    label: 'admin.collections.title',
    desc: 'admin.dashboard.collectionsDesc',
    accent: 'cyan',
    icon: 'M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10',
  },
  {
    key: 'users',
    to: '/admin/users',
    label: 'admin.users.title',
    desc: 'admin.dashboard.usersDesc',
    accent: 'cyan',
    icon: 'M17 20h5v-2a4 4 0 00-3-3.87M9 20H4v-2a4 4 0 013-3.87m6-1.13a4 4 0 10-4-4 4 4 0 004 4z',
  },
  {
    key: 'rawLibrary',
    to: '/admin/raw-library',
    label: 'player.adminLibrary.title',
    desc: 'admin.dashboard.rawLibraryDesc',
    accent: 'pink',
    icon: 'M9 17V7m0 10a2 2 0 01-2 2H5a2 2 0 01-2-2V7a2 2 0 012-2h2a2 2 0 012 2m0 10a2 2 0 002 2h2a2 2 0 002-2M9 7a2 2 0 012-2h2a2 2 0 012 2m0 10V7m0 10a2 2 0 002 2h2a2 2 0 002-2V7a2 2 0 00-2-2h-2a2 2 0 00-2 2',
  },
  {
    key: 'gacha',
    to: '/admin/gacha',
    label: 'gacha.admin.title',
    desc: 'gacha.admin.desc',
    accent: 'cyan',
    icon: 'M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
  },
  {
    key: 'policy',
    to: '/admin/policy',
    label: 'admin.policy.title',
    desc: 'admin.dashboard.policyDesc',
    accent: 'pink',
    icon: 'M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z',
  },
]
</script>

<style scoped>
.admin-tool-card {
  display: block;
  text-decoration: none;
  transition: border-color 0.18s ease, transform 0.18s ease, background-color 0.18s ease;
}

.admin-tool-card:hover {
  border-color: var(--white-a20);
  background-color: var(--white-a8);
  transform: translateY(-2px);
}

.admin-tool-icon {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border-radius: 12px;
  flex-shrink: 0;
  border: 1px solid transparent;
}

.admin-tool-icon--cyan {
  color: var(--brand-cyan);
  background: color-mix(in srgb, var(--brand-cyan) 12%, transparent);
  border-color: color-mix(in srgb, var(--brand-cyan) 30%, transparent);
}

.admin-tool-icon--pink {
  color: var(--brand-pink);
  background: color-mix(in srgb, var(--brand-pink) 12%, transparent);
  border-color: color-mix(in srgb, var(--brand-pink) 30%, transparent);
}
</style>
