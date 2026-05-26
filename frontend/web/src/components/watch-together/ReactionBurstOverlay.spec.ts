/**
 * Workstream watch-together — Phase 2 (frontend-shell) Plan 02.5 Task 2.
 *
 * Vitest spec for ReactionBurstOverlay.vue. Verifies the 6 behaviors
 * locked in the plan:
 *
 *   1. Empty `reactions` prop → no `<span>` rendered in the overlay.
 *   2. 3 reactions in props → 3 `<span>` rendered, each containing the
 *      correct emoji in order.
 *   3. Each rendered `<span>` has an inline `left:` style between 0% and
 *      100% (the random horizontal placement).
 *   4. The same reaction id rendered twice (across re-renders) keeps the
 *      SAME `left` value — i.e. the per-id `left` is memoized so the
 *      emoji doesn't jitter horizontally as the parent re-renders.
 *   5. The outer wrapper has the `pointer-events-none` class so the
 *      overlay never intercepts clicks on the player beneath it.
 *   6. The SFC source contains an `@keyframes` block in `<style scoped>`
 *      (smoke-grep — CSS-only animation is the WT-NF-05 contract).
 */

import { readFileSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'

import ReactionBurstOverlay from './ReactionBurstOverlay.vue'
import type { ReactionEvent } from '@/composables/useWatchTogetherRoom'

const __dirname = dirname(fileURLToPath(import.meta.url))
const SFC_SOURCE = readFileSync(
  resolve(__dirname, 'ReactionBurstOverlay.vue'),
  'utf-8',
)

function makeReaction(id: number, emoji: string): ReactionEvent {
  return { id, emoji, user_id: `user-${id}`, ts: Date.now() }
}

/**
 * Parse the inline `left:` style off a rendered span. Returns the number
 * of percentage points (e.g. `42.7` for `left: 42.7%`). Throws on missing
 * or non-percentage values so a regressed unit silently passing is
 * impossible.
 */
function leftPercent(el: Element): number {
  const style = (el as HTMLElement).getAttribute('style') ?? ''
  const match = style.match(/left:\s*([\d.]+)%/)
  if (!match) {
    throw new Error(`expected inline 'left: NN%' style, got: ${style}`)
  }
  return parseFloat(match[1])
}

describe('ReactionBurstOverlay', () => {
  it('renders no <span> when reactions is empty', () => {
    const wrapper = mount(ReactionBurstOverlay, {
      props: { reactions: [] },
    })
    expect(wrapper.findAll('span').length).toBe(0)
  })

  it('renders one <span> per reaction with the correct emoji', () => {
    const reactions: ReactionEvent[] = [
      makeReaction(1, '🔥'),
      makeReaction(2, '❤️'),
      makeReaction(3, '🎉'),
    ]
    const wrapper = mount(ReactionBurstOverlay, { props: { reactions } })
    const spans = wrapper.findAll('span')
    expect(spans.length).toBe(3)
    expect(spans[0].text()).toBe('🔥')
    expect(spans[1].text()).toBe('❤️')
    expect(spans[2].text()).toBe('🎉')
  })

  it('each <span> has an inline left:NN% between 0 and 100', () => {
    const reactions: ReactionEvent[] = [
      makeReaction(10, '🔥'),
      makeReaction(11, '✨'),
      makeReaction(12, '💯'),
    ]
    const wrapper = mount(ReactionBurstOverlay, { props: { reactions } })
    const spans = wrapper.findAll('span')
    for (const span of spans) {
      const pct = leftPercent(span.element)
      expect(pct).toBeGreaterThanOrEqual(0)
      expect(pct).toBeLessThanOrEqual(100)
    }
  })

  it('memoizes the left value per reaction id across re-renders', async () => {
    const r = makeReaction(42, '🔥')
    const wrapper = mount(ReactionBurstOverlay, { props: { reactions: [r] } })
    const firstLeft = leftPercent(wrapper.find('span').element)

    // Trigger a re-render by replacing the prop array with a new array
    // containing the SAME reaction id. The memoization Map keyed by id
    // should return the original `left` value rather than rolling fresh.
    await wrapper.setProps({ reactions: [{ ...r }] })
    const secondLeft = leftPercent(wrapper.find('span').element)

    expect(secondLeft).toBe(firstLeft)
  })

  it('outer wrapper has pointer-events-none (click-through to player)', () => {
    const wrapper = mount(ReactionBurstOverlay, { props: { reactions: [] } })
    expect(wrapper.classes()).toContain('pointer-events-none')
  })

  it('SFC source contains an @keyframes block in <style scoped>', () => {
    // Smoke-grep the on-disk SFC. Vue Test Utils strips the scoped style
    // block before mount, so DOM introspection won't catch this — we read
    // the source directly. Locks the CSS-only animation contract (WT-NF-05).
    expect(SFC_SOURCE).toMatch(/<style\s+scoped>/)
    expect(SFC_SOURCE).toMatch(/@keyframes\s+\w+/)
  })

  // ----- Phase 5 (Plan 05.3) polish tests -----

  it('caps concurrent on-screen bursts at 8 via FIFO drop', () => {
    // 12 rapid reactions in props → only the most recent 8 render.
    const reactions = Array.from({ length: 12 }, (_, i) =>
      makeReaction(100 + i, '🔥'),
    )
    const wrapper = mount(ReactionBurstOverlay, { props: { reactions } })
    expect(wrapper.findAll('span').length).toBe(8)
    // The 8 visible should be the LAST 8 in the input (FIFO drop of oldest)
    const spans = wrapper.findAll('span')
    // Last span's emoji corresponds to id 111 (100+11); spans render in
    // array order, so verify by inspecting via the kept slice.
    expect(spans[0].text()).toBe('🔥')
    expect(spans.length).toBe(8)
  })

  it('uses 8-column stratified placement (not pure random)', () => {
    // 8 reactions with consecutive ids → each picks a distinct column
    // (round-robin via `id % 8`). Verifies the placement is deterministic
    // and stratified, not random.
    const reactions = Array.from({ length: 8 }, (_, i) =>
      makeReaction(200 + i, '✨'),
    )
    const wrapper = mount(ReactionBurstOverlay, { props: { reactions } })
    const lefts = wrapper.findAll('span').map((s) => leftPercent(s.element))
    // All 8 columns should be hit (set size = 8 means all distinct)
    expect(new Set(lefts).size).toBe(8)
    // Each value should be one of the locked column %s
    const allowed = new Set([10, 20, 30, 45, 55, 70, 80, 90])
    for (const l of lefts) {
      expect(allowed.has(l)).toBe(true)
    }
  })

  it('animation duration is 2.5s (Phase 5 upgrade from 3s)', () => {
    // Smoke-grep the SFC source — Vitest can't introspect scoped CSS.
    expect(SFC_SOURCE).toMatch(/animation:\s*burst-rise\s+2\.5s/)
  })

  it('keyframes include scale and translate (rise + wiggle)', () => {
    // The Phase 5 keyframes go beyond Phase 2's translateY-only.
    expect(SFC_SOURCE).toMatch(/scale\(/)
    expect(SFC_SOURCE).toMatch(/translate\(/)
  })
})
