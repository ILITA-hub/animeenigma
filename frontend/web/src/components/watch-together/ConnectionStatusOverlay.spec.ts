/**
 * Workstream watch-together — Phase 03 (player-sync) Plan 03.5 Task 3.
 *
 * Vitest spec for ConnectionStatusOverlay.vue. Verifies:
 *   1. status='open' → nothing rendered (composable's happy path)
 *   2. status='idle' → nothing rendered (pre-connect)
 *   3. status='connecting' → nothing rendered (initial open in progress)
 *   4. status='failed' → nothing rendered (terminal — WatchTogetherView's
 *      own error branches own this case, not this overlay)
 *   5. status='reconnecting' → banner with reconnecting_indicator key
 *   6. status='closed' → banner with connection_status_closed key
 *   7. spinner element has `animate-spin` class
 *   8. wrapper has `pointer-events-none`; banner has `pointer-events-auto`
 *
 * `vue-i18n` is stubbed so t() echoes the key for assertion.
 */

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

import ConnectionStatusOverlay from './ConnectionStatusOverlay.vue'
import Spinner from '@/components/ui/Spinner.vue'
import type { ConnectionStatus } from '@/composables/useWatchTogetherRoom'

function mountAt(status: ConnectionStatus) {
  return mount(ConnectionStatusOverlay, { props: { status } })
}

describe('ConnectionStatusOverlay', () => {
  it('renders nothing when status is `open`', () => {
    const wrapper = mountAt('open')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })

  it('renders nothing when status is `idle`', () => {
    const wrapper = mountAt('idle')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })

  it('renders nothing when status is `connecting`', () => {
    const wrapper = mountAt('connecting')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })

  it('renders nothing when status is `failed` (WatchTogetherView owns the terminal UX)', () => {
    const wrapper = mountAt('failed')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })

  it('renders the reconnecting_indicator banner when status is `reconnecting`', () => {
    const wrapper = mountAt('reconnecting')
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('watch_together.reconnecting_indicator')
  })

  it('renders the connection_status_closed banner when status is `closed`', () => {
    const wrapper = mountAt('closed')
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('watch_together.connection_status_closed')
  })

  it('renders a Spinner while reconnecting', () => {
    const wrapper = mountAt('reconnecting')
    // The inline animate-spin span was replaced by the shared <Spinner>
    // primitive (dual-arc donut, role="status").
    expect(wrapper.findComponent(Spinner).exists()).toBe(true)
  })

  it('wrapper is pointer-events-none and the banner box is pointer-events-auto', () => {
    const wrapper = mountAt('reconnecting')
    const outer = wrapper.find('[role="status"]')
    expect(outer.classes()).toContain('pointer-events-none')
    // The banner inside re-enables pointer events so AT can read it.
    const innerHtml = outer.html()
    expect(innerHtml).toContain('pointer-events-auto')
  })

  it('uses only font-medium / font-semibold weights (no font-bold)', () => {
    const wrapper = mountAt('reconnecting')
    const html = wrapper.html()
    expect(html).not.toMatch(/\bfont-bold\b/)
    expect(html).not.toMatch(/\bfont-black\b/)
    expect(html).not.toMatch(/\bfont-extrabold\b/)
    expect(html).toMatch(/font-medium|font-semibold/)
  })
})
