/**
 * Task 13 — ProviderChip recovering state guard.
 *
 * Verifies that:
 *   (a) A chip in 'recovering' state renders the Recovering label (lime badge)
 *   (b) The chip is selectable/clickable in hacker mode
 *   (c) The chip is NOT selectable without hacker mode
 *   (d) Active and down states are unaffected by the new branch
 */
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ProviderChip from '../ProviderChip.vue'
import type { ProviderRow } from '@/types/aePlayer'

// ProviderChip uses deriveCapLabels which is pure; no network calls to stub.
// lucide-vue-next icons render fine in jsdom.

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

function makeRow(state: ProviderRow['state'], reason?: string): ProviderRow {
  return {
    def: {
      id: 'gogoanime',
      name: 'Anitaku',
      hue: '#22c55e',
      group: 'en',
      audios: ['sub', 'dub'],
      langs: ['en'],
      content: ['common'],
      scraper: true,
      blurb: 'Test provider',
    },
    state,
    reason,
  }
}

describe('ProviderChip — recovering state (Task 13)', () => {
  it('renders the Recovering label for state=recovering', () => {
    const wrapper = mount(ProviderChip, {
      props: { row: makeRow('recovering', 'back online'), hackerMode: true },
      global: { plugins: [i18n] },
    })
    expect(wrapper.text()).toContain('Recovering')
  })

  it('renders the recovering badge with data-test="cap-recovering"', () => {
    const wrapper = mount(ProviderChip, {
      props: { row: makeRow('recovering', 'back online'), hackerMode: true },
      global: { plugins: [i18n] },
    })
    expect(wrapper.find('[data-test="cap-recovering"]').exists()).toBe(true)
  })

  it('recovering chip is selectable (clickable) in hacker mode', async () => {
    const wrapper = mount(ProviderChip, {
      props: { row: makeRow('recovering', 'back online'), hackerMode: true },
      global: { plugins: [i18n] },
    })
    const btn = wrapper.find('button')
    expect(btn.attributes('disabled')).toBeUndefined()
    await btn.trigger('click')
    expect(wrapper.emitted('select')).toBeTruthy()
  })

  it('recovering chip is NOT selectable without hacker mode', async () => {
    const wrapper = mount(ProviderChip, {
      props: { row: makeRow('recovering', 'back online'), hackerMode: false },
      global: { plugins: [i18n] },
    })
    const btn = wrapper.find('button')
    expect(btn.attributes('disabled')).toBeDefined()
    await btn.trigger('click')
    expect(wrapper.emitted('select')).toBeFalsy()
  })

  it('does NOT render the Recovering badge for state=active', () => {
    const wrapper = mount(ProviderChip, {
      props: { row: makeRow('active'), hackerMode: false },
      global: { plugins: [i18n] },
    })
    expect(wrapper.find('[data-test="cap-recovering"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Recovering')
  })

  it('does NOT render the Recovering badge for state=degraded', () => {
    const wrapper = mount(ProviderChip, {
      props: { row: makeRow('degraded', 'ad injection'), hackerMode: true },
      global: { plugins: [i18n] },
    })
    expect(wrapper.find('[data-test="cap-recovering"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="cap-degraded"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Degraded')
  })
})
