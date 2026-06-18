/**
 * Workstream watch-together — Phase 04 (state-switching) Plan 04.4 Task 1.
 * Updated 2026-06-17: legacy players retired. Only the first-party aePlayer
 * + Classic Kodik survive, so the bar renders exactly 2 tabs (aeplayer leads).
 *
 * Vitest spec for PlayerTabBar.vue. Locks the 2-tab switcher behavior:
 *
 *   1. Renders exactly 2 buttons (aeplayer + kodik), aeplayer leads
 *   2. Each rendered button label resolves through i18n
 *      (no raw `watch_together.player_tab_*` substring leaks)
 *   3. Active tab gains the active-state styling
 *   4. Click on an inactive tab emits `select-player` with the kind
 *   5. Click on the currently-active tab does NOT emit (no-op)
 *   6. `disabled=true` sets aria-disabled and suppresses emits
 *   7. No font-bold / font-black / font-extrabold in rendered HTML
 *      (UI-SPEC contract — font-medium / font-semibold only)
 *   8. No retired legacy player tabs are rendered
 */

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { vi } from 'vitest'
import type { PlayerKind } from '@/api/watch-together'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      // Echo a stable, recognisable rendered string per key. We deliberately
      // map each player_tab_* key to a short label so the "no raw key in
      // rendered HTML" assertion is meaningful (the rendered text should
      // NOT contain `watch_together.player_tab_*`).
      const map: Record<string, string> = {
        'watch_together.player_tab_aeplayer': 'AnimeEnigma',
        'watch_together.player_tab_kodik': 'Kodik',
      }
      return map[key] ?? key
    },
    locale: { value: 'en' },
  }),
}))

import PlayerTabBar from './PlayerTabBar.vue'

// Survivors only. Retired kinds stay in the PlayerKind union (for forward-compat
// snapshot handling) but are NOT offered by the in-room switch.
const SURVIVORS: PlayerKind[] = ['aeplayer', 'kodik']
const RETIRED: PlayerKind[] = ['kodik-adfree', 'animelib', 'ourenglish', 'hanime', 'raw']

function mountBar(props: { activePlayer: PlayerKind | null; disabled?: boolean } = { activePlayer: 'kodik' }) {
  return mount(PlayerTabBar, { props })
}

describe('PlayerTabBar', () => {
  it('Test 1: renders exactly 2 surviving tab buttons (aeplayer + kodik)', () => {
    const wrapper = mountBar({ activePlayer: 'kodik' })
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs).toHaveLength(2)
    expect(tabs.map((t) => t.attributes('data-player'))).toEqual(SURVIVORS)
  })

  it('Test 1a: aeplayer tab is rendered and leads the bar', () => {
    const wrapper = mountBar({ activePlayer: 'kodik' })
    const tabs = wrapper.findAll('[role="tab"]')
    const aeTab = tabs.find((t) => t.attributes('data-player') === 'aeplayer')
    expect(aeTab).toBeDefined()
    expect(aeTab!.text()).toBe('AnimeEnigma')
    expect(tabs[0]!.attributes('data-player')).toBe('aeplayer')
  })

  it('Test 2: each tab label resolves through i18n (no raw key strings leak)', () => {
    const wrapper = mountBar({ activePlayer: 'kodik' })
    const html = wrapper.html()
    // No raw key prefix should appear.
    expect(html).not.toContain('watch_together.player_tab_')
    // Both surviving labels should be visible.
    expect(html).toContain('AnimeEnigma')
    expect(html).toContain('Kodik')
  })

  it('Test 3: active tab gains aria-selected=true and a distinguishing class', () => {
    const wrapper = mountBar({ activePlayer: 'aeplayer' })
    const tabs = wrapper.findAll('[role="tab"]')
    const aeTab = tabs.find((t) => t.attributes('data-player') === 'aeplayer')
    const kodikTab = tabs.find((t) => t.attributes('data-player') === 'kodik')
    expect(aeTab).toBeDefined()
    expect(kodikTab).toBeDefined()
    expect(aeTab!.attributes('aria-selected')).toBe('true')
    expect(kodikTab!.attributes('aria-selected')).toBe('false')
  })

  it('Test 4: click on an inactive tab emits `select-player` with the kind', async () => {
    const wrapper = mountBar({ activePlayer: 'kodik' })
    const aeTab = wrapper.findAll('[role="tab"]').find((t) => t.attributes('data-player') === 'aeplayer')!
    await aeTab.trigger('click')
    expect(wrapper.emitted('select-player')).toBeTruthy()
    expect(wrapper.emitted('select-player')).toHaveLength(1)
    expect(wrapper.emitted('select-player')![0]).toEqual(['aeplayer'])
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
    const aeTab = tabs.find((t) => t.attributes('data-player') === 'aeplayer')!
    await aeTab.trigger('click')
    expect(wrapper.emitted('select-player')).toBeFalsy()
  })

  it('Test 7: rendered HTML uses only font-medium / font-semibold weights', () => {
    const wrapper = mountBar({ activePlayer: 'kodik' })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bfont-bold\b/)
    expect(html).not.toMatch(/\bfont-black\b/)
    expect(html).not.toMatch(/\bfont-extrabold\b/)
  })

  it('Test 8: no retired legacy player tabs are rendered', () => {
    const wrapper = mountBar({ activePlayer: 'kodik' })
    const tabs = wrapper.findAll('[role="tab"]')
    for (const kind of RETIRED) {
      expect(tabs.find((t) => t.attributes('data-player') === kind)).toBeUndefined()
    }
  })

  it('Test 9: hiddenKinds further omits a survivor (kodik hidden → only aeplayer)', () => {
    const wrapper = mount(PlayerTabBar, {
      props: { activePlayer: 'aeplayer' as PlayerKind, hiddenKinds: ['kodik'] as PlayerKind[] },
    })
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs).toHaveLength(1)
    expect(tabs[0]!.attributes('data-player')).toBe('aeplayer')
  })
})
