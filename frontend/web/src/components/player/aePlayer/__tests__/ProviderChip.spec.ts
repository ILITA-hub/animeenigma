/**
 * ProviderChip recovering/degraded state guard (feed-driven).
 *
 * Verifies that:
 *   (a) A chip in 'recovering' state renders the Recovering label (lime badge)
 *   (b) The chip is selectable/clickable in hacker mode
 *   (c) The chip is NOT selectable without hacker mode
 *   (d) Active and degraded states are unaffected by the recovering branch
 */
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ProviderChip from '../ProviderChip.vue'
import type { ProviderRow } from '@/types/aePlayer'

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

function makeRow(state: ProviderRow['state'], reason?: string): ProviderRow {
  // recovering/degraded arrive from the backend feed as hacker-only + selectable;
  // the chip gates the disabled attribute on hacker mode.
  const hackerOnly = state === 'recovering' || state === 'degraded'
  return {
    id: 'gogoanime',
    label: 'Anitaku',
    group: 'en',
    state,
    selectable: state !== 'no_content',
    hackerOnly,
    order: 80,
    audios: ['sub', 'dub'],
    reason,
  }
}

describe('ProviderChip — recovering state', () => {
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
