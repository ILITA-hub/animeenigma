import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createRouter, createMemoryHistory } from 'vue-router'
import en from '@/locales/en.json'
import type { UserStats } from '@/api/anidle'

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/', component: { template: '<div/>' } },
    { path: '/auth', component: { template: '<div/>' } },
  ],
})

function makeStats(over: Partial<UserStats> = {}): UserStats {
  return {
    user_id: 'u1',
    games_played: 42,
    games_won: 30,
    current_streak: 5,
    max_streak: 12,
    guess_distribution: { '1': 3, '2': 10, '3': 8, '4': 5, '5': 4 },
    last_played_date: '2026-06-15',
    updated_at: '2026-06-15T12:00:00Z',
    ...over,
  }
}

async function mountStats(props: { stats: UserStats | null; isAuthenticated: boolean }) {
  const StatsPanel = (await import('./StatsPanel.vue')).default
  return mount(StatsPanel, {
    props,
    global: {
      plugins: [i18n, router],
      stubs: {
        LoadingState: { template: '<div class="loading-state"/>' },
        Card: { template: '<div class="card"><slot/></div>' },
      },
    },
  })
}

describe('StatsPanel', () => {
  it('shows guest notice when isAuthenticated=false', async () => {
    const w = await mountStats({ stats: null, isAuthenticated: false })
    // The i18n key stats_guest_notice
    expect(w.text()).toContain('Sign in to save your stats')
  })

  it('guest notice contains a link to /auth', async () => {
    const w = await mountStats({ stats: null, isAuthenticated: false })
    const link = w.find('a[href="/auth"]')
    expect(link.exists()).toBe(true)
  })

  it('shows games_played count when stats is provided', async () => {
    const w = await mountStats({ stats: makeStats({ games_played: 42 }), isAuthenticated: true })
    expect(w.text()).toContain('42')
  })

  it('shows current_streak count', async () => {
    const w = await mountStats({ stats: makeStats({ current_streak: 5 }), isAuthenticated: true })
    expect(w.text()).toContain('5')
  })

  it('shows bg-success in histogram bars (no off-palette color)', async () => {
    const w = await mountStats({ stats: makeStats(), isAuthenticated: true })
    expect(w.html()).toContain('bg-success')
    expect(w.html()).not.toContain('bg-green-')
    expect(w.html()).not.toContain('bg-emerald-')
  })

  it('shows LoadingState when authenticated but stats is null', async () => {
    const w = await mountStats({ stats: null, isAuthenticated: true })
    expect(w.find('.loading-state').exists()).toBe(true)
  })
})
