import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

// useI18n() is imported directly in the SFC; stub it module-wide so the label
// keys echo through. importOriginal keeps createI18n (reached via the
// @/components/ui barrel's import graph) intact for isolated runs.
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (k: string) => k, locale: { value: 'en' } }),
}))

import WatchTogetherButton from './WatchTogetherButton.vue'

const global = {
  stubs: {
    Spinner: { template: '<span class="spinner-stub" />' },
  },
}

describe('WatchTogetherButton (launcher)', () => {
  it('renders the wt-launch data-test attribute', () => {
    const w = mount(WatchTogetherButton, { global })
    expect(w.find('[data-test="wt-launch"]').exists()).toBe(true)
  })

  it('no longer shows a WIP badge', () => {
    const w = mount(WatchTogetherButton, { global })
    expect(w.text()).not.toContain('WIP')
  })

  it('emits "launch" when clicked (enabled)', async () => {
    const w = mount(WatchTogetherButton, { global })
    await w.find('button').trigger('click')
    expect(w.emitted('launch')).toHaveLength(1)
  })

  it('is disabled when the disabled prop is set', () => {
    const w = mount(WatchTogetherButton, { props: { disabled: true }, global })
    expect(w.find('button').attributes('disabled')).toBeDefined()
  })

  it('is disabled while loading and shows the spinner', () => {
    const w = mount(WatchTogetherButton, { props: { loading: true }, global })
    expect(w.find('button').attributes('disabled')).toBeDefined()
    expect(w.find('.spinner-stub').exists()).toBe(true)
  })

  it('does not emit "launch" when clicked while disabled', async () => {
    const w = mount(WatchTogetherButton, { props: { disabled: true }, global })
    await w.find('button').trigger('click')
    expect(w.emitted('launch')).toBeUndefined()
  })

  it('uses the invite-button i18n key for its accessible label', () => {
    const w = mount(WatchTogetherButton, { global })
    expect(w.find('button').attributes('aria-label')).toBe(
      'watch_together.invite_button_label',
    )
  })
})
