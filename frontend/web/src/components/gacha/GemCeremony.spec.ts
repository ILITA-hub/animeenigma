import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import GemCeremony from './GemCeremony.vue'

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

function mountCeremony(props: { active: boolean; topTier: 'N' | 'R' | 'SR' | 'SSR' }) {
  return mount(GemCeremony, {
    props,
    global: { plugins: [i18n] },
    attachTo: document.body,
  })
}

describe('GemCeremony', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('does not render when inactive', () => {
    const w = mountCeremony({ active: false, topTier: 'N' })
    expect(document.querySelector('[data-testid="gem-ceremony"]')).toBeNull()
    w.unmount()
  })

  it('renders the summon overlay when active', async () => {
    const w = mountCeremony({ active: true, topTier: 'R' })
    await flushPromises()
    expect(document.querySelector('[data-testid="gem-ceremony"]')).not.toBeNull()
    w.unmount()
  })

  it('SSR run adds the gold class', async () => {
    const w = mountCeremony({ active: true, topTier: 'SSR' })
    await flushPromises()
    expect(document.querySelector('.summon')?.classList.contains('gold')).toBe(true)
    w.unmount()
  })

  it('skip emits done(true) immediately', async () => {
    const w = mountCeremony({ active: true, topTier: 'N' })
    await flushPromises()
    const skip = document.querySelector('[data-testid="ceremony-skip"]') as HTMLButtonElement
    skip.click()
    await flushPromises()
    expect(w.emitted('done')?.[0]).toEqual([true])
    w.unmount()
  })

  it('running the full timeline emits done(false)', async () => {
    const w = mountCeremony({ active: true, topTier: 'R' })
    await flushPromises()
    // Non-SSR total: 700 + 500 + 450 + 420 = 2070ms
    vi.advanceTimersByTime(2100)
    await flushPromises()
    expect(w.emitted('done')?.[0]).toEqual([false])
    w.unmount()
  })

  it('SSR run takes longer than a non-SSR run before completing', async () => {
    const w = mountCeremony({ active: true, topTier: 'SSR' })
    await flushPromises()
    // Non-SSR would be done by 2070ms; SSR is 1200+900+450+420 = 2970ms.
    vi.advanceTimersByTime(2100)
    await flushPromises()
    expect(w.emitted('done')).toBeUndefined()
    vi.advanceTimersByTime(1000)
    await flushPromises()
    expect(w.emitted('done')?.[0]).toEqual([false])
    w.unmount()
  })
})
