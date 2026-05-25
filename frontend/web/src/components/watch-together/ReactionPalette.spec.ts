/**
 * Workstream watch-together — Phase 2 (frontend-shell) Plan 02.5 Task 1.
 *
 * Vitest spec for ReactionPalette.vue. Verifies the 7 behaviors locked in
 * the plan:
 *
 *   1. Renders exactly 24 buttons (the locked REACTION_WHITELIST size).
 *   2. Each button's text content matches the corresponding REACTION_WHITELIST
 *      entry IN ORDER (so the palette stays in lockstep with the Go server
 *      filter; reorderings cause whitelist-vs-display mismatches).
 *   3. Clicking the first button calls `sendReaction('🔥')` exactly once.
 *   4. Clicking the SAME button twice within 200ms calls `sendReaction` ONCE
 *      (client-side throttle — server already rate-limits at 5/sec).
 *   5. After 200ms, the throttle releases and a second click fires.
 *   6. Each button carries `aria-label` matching its emoji (screen-reader
 *      contract — "fire emoji" is what assistive tech speaks).
 *   7. No `font-bold` / `font-extrabold` / `font-black` in rendered HTML
 *      (project rule: only `font-medium` / `font-semibold` are allowed).
 *
 * `vue-i18n` is stubbed with an echoing `t()` so we don't depend on the
 * production locale files for SFC tests (matches the spotlight cards'
 * pattern).
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'en' },
  }),
}))

// Imported AFTER vi.mock so the SFC's useI18n() resolves to the stub.
import ReactionPalette from './ReactionPalette.vue'
import { REACTION_WHITELIST } from '@/types/watch-together'

describe('ReactionPalette', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders exactly 24 buttons (the locked REACTION_WHITELIST size)', () => {
    const sendReaction = vi.fn()
    const wrapper = mount(ReactionPalette, { props: { sendReaction } })
    const buttons = wrapper.findAll('button')
    expect(buttons.length).toBe(24)
    // Sanity check — REACTION_WHITELIST itself is 24. If this ever drifts
    // the palette test will catch the source-of-truth break.
    expect(REACTION_WHITELIST.length).toBe(24)
  })

  it('each button text matches REACTION_WHITELIST in order', () => {
    const sendReaction = vi.fn()
    const wrapper = mount(ReactionPalette, { props: { sendReaction } })
    const buttons = wrapper.findAll('button')
    REACTION_WHITELIST.forEach((emoji, idx) => {
      expect(buttons[idx].text()).toBe(emoji)
    })
  })

  it('clicking the first button calls sendReaction with the first emoji', async () => {
    const sendReaction = vi.fn()
    const wrapper = mount(ReactionPalette, { props: { sendReaction } })
    await wrapper.findAll('button')[0].trigger('click')
    expect(sendReaction).toHaveBeenCalledTimes(1)
    expect(sendReaction).toHaveBeenCalledWith(REACTION_WHITELIST[0])
  })

  it('clicking the same button twice within 200ms is throttled to one call', async () => {
    const sendReaction = vi.fn()
    const wrapper = mount(ReactionPalette, { props: { sendReaction } })
    const button = wrapper.findAll('button')[0]
    await button.trigger('click')
    // Advance only 100ms — still inside the throttle window.
    vi.advanceTimersByTime(100)
    await button.trigger('click')
    expect(sendReaction).toHaveBeenCalledTimes(1)
  })

  it('after 200ms the throttle releases and a second click fires', async () => {
    const sendReaction = vi.fn()
    const wrapper = mount(ReactionPalette, { props: { sendReaction } })
    const button = wrapper.findAll('button')[0]
    await button.trigger('click')
    vi.advanceTimersByTime(250)
    await button.trigger('click')
    expect(sendReaction).toHaveBeenCalledTimes(2)
  })

  it('each button has aria-label matching its emoji', () => {
    const sendReaction = vi.fn()
    const wrapper = mount(ReactionPalette, { props: { sendReaction } })
    const buttons = wrapper.findAll('button')
    REACTION_WHITELIST.forEach((emoji, idx) => {
      expect(buttons[idx].attributes('aria-label')).toBe(emoji)
    })
  })

  it('uses no forbidden font-weight utilities (only font-medium / font-semibold allowed)', () => {
    const sendReaction = vi.fn()
    const wrapper = mount(ReactionPalette, { props: { sendReaction } })
    const html = wrapper.html()
    expect(html).not.toMatch(/font-bold/)
    expect(html).not.toMatch(/font-extrabold/)
    expect(html).not.toMatch(/font-black/)
  })
})
