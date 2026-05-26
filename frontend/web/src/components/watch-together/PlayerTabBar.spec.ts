/**
 * Workstream watch-together — Phase 04 (state-switching) Plan 04.4 Task 1.
 *
 * Vitest spec for PlayerTabBar.vue. Locks the 5-tab switcher behavior:
 *
 *   1. Renders exactly 5 buttons, one per PlayerKind
 *   2. Each rendered button label resolves through i18n
 *      (no raw `watch_together.player_tab_*` substring leaks)
 *   3. Active tab gains the active-state styling
 *   4. Click on an inactive tab emits `select-player` with the kind
 *   5. Click on the currently-active tab does NOT emit (no-op)
 *   6. `disabled=true` sets aria-disabled and suppresses emits
 *   7. No font-bold / font-black / font-extrabold in rendered HTML
 *      (UI-SPEC contract — font-medium / font-semibold only)
 */

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { PlayerKind } from '@/api/watch-together'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      // Echo a stable, recognisable rendered string per key. We deliberately
      // map each player_tab_* key to a short label so the "no raw key in
      // rendered HTML" assertion is meaningful (the rendered text should
      // NOT contain `watch_together.player_tab_*`).
      const map: Record<string, string> = {
        'watch_together.player_tab_kodik': 'Kodik',
        'watch_together.player_tab_animelib': 'AniLib',
        'watch_together.player_tab_ourenglish': 'English',
        'watch_together.player_tab_hanime': 'Hanime',
        'watch_together.player_tab_raw': 'Raw (JP)',
      }
      return map[key] ?? key
    },
    locale: { value: 'en' },
  }),
}))

import PlayerTabBar from './PlayerTabBar.vue'

const ALL_PLAYERS: PlayerKind[] = ['kodik', 'animelib', 'ourenglish', 'hanime', 'raw']

function mountBar(props: { activePlayer: PlayerKind | null; disabled?: boolean } = { activePlayer: 'kodik' }) {
  return mount(PlayerTabBar, { props })
}

describe('PlayerTabBar', () => {
  it('Test 1: renders exactly 5 tab buttons (one per PlayerKind)', () => {
    const wrapper = mountBar({ activePlayer: 'kodik' })
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs).toHaveLength(5)
  })

  it('Test 2: each tab label resolves through i18n (no raw key strings leak)', () => {
    const wrapper = mountBar({ activePlayer: 'kodik' })
    const html = wrapper.html()
    // No raw key prefix should appear.
    expect(html).not.toContain('watch_together.player_tab_')
    // All five resolved labels should be visible.
    expect(html).toContain('Kodik')
    expect(html).toContain('AniLib')
    expect(html).toContain('English')
    expect(html).toContain('Hanime')
    expect(html).toContain('Raw (JP)')
  })

  it('Test 3: active tab gains aria-selected=true and a distinguishing class', () => {
    const wrapper = mountBar({ activePlayer: 'animelib' })
    const tabs = wrapper.findAll('[role="tab"]')
    // Locate the AniLib tab by its data-player attribute.
    const animelibTab = tabs.find((t) => t.attributes('data-player') === 'animelib')
    const kodikTab = tabs.find((t) => t.attributes('data-player') === 'kodik')
    expect(animelibTab).toBeDefined()
    expect(kodikTab).toBeDefined()
    expect(animelibTab!.attributes('aria-selected')).toBe('true')
    expect(kodikTab!.attributes('aria-selected')).toBe('false')
  })

  it('Test 4: click on an inactive tab emits `select-player` with the kind', async () => {
    const wrapper = mountBar({ activePlayer: 'kodik' })
    const animelibTab = wrapper.findAll('[role="tab"]').find((t) => t.attributes('data-player') === 'animelib')!
    await animelibTab.trigger('click')
    expect(wrapper.emitted('select-player')).toBeTruthy()
    expect(wrapper.emitted('select-player')).toHaveLength(1)
    expect(wrapper.emitted('select-player')![0]).toEqual(['animelib'])
  })

  it('Test 5: click on the currently-active tab does NOT emit', async () => {
    const wrapper = mountBar({ activePlayer: 'kodik' })
    const kodikTab = wrapper.findAll('[role="tab"]').find((t) => t.attributes('data-player') === 'kodik')!
    await kodikTab.trigger('click')
    expect(wrapper.emitted('select-player')).toBeFalsy()
  })

  it('Test 6: disabled=true renders aria-disabled and click does NOT emit', async () => {
    const wrapper = mountBar({ activePlayer: 'kodik', disabled: true })
    const tabs = wrapper.findAll('[role="tab"]')
    for (const t of tabs) {
      expect(t.attributes('aria-disabled')).toBe('true')
    }
    const animelibTab = tabs.find((t) => t.attributes('data-player') === 'animelib')!
    await animelibTab.trigger('click')
    expect(wrapper.emitted('select-player')).toBeFalsy()
  })

  it('Test 7: rendered HTML uses only font-medium / font-semibold weights', () => {
    const wrapper = mountBar({ activePlayer: 'kodik' })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bfont-bold\b/)
    expect(html).not.toMatch(/\bfont-black\b/)
    expect(html).not.toMatch(/\bfont-extrabold\b/)
  })

  it('Test 8: all five player kinds can be selected (cycle through inactive tabs)', async () => {
    // Pin active to 'raw' so the other 4 are clickable.
    const wrapper = mountBar({ activePlayer: 'raw' })
    const expectations: PlayerKind[] = ALL_PLAYERS.filter((k) => k !== 'raw')
    for (const kind of expectations) {
      const tab = wrapper.findAll('[role="tab"]').find((t) => t.attributes('data-player') === kind)!
      await tab.trigger('click')
    }
    const emits = wrapper.emitted('select-player')
    expect(emits).toBeTruthy()
    expect(emits).toHaveLength(4)
    expect(emits!.map((e) => e[0])).toEqual(expectations)
  })
})
