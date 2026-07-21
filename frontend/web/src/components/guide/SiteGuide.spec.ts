import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SiteGuide from './SiteGuide.vue'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('SiteGuide', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
  })

  it('walks through the guide and closes on the final step', async () => {
    const wrapper = mount(SiteGuide, { props: { modelValue: true }, attachTo: document.body })

    expect(wrapper.text()).toContain('siteGuide.steps.home.title')
    for (let i = 0; i < 5; i += 1) {
      await wrapper.get('[data-testid="site-guide-next"]').trigger('click')
    }
    expect(wrapper.text()).toContain('siteGuide.steps.feedback.title')
    await wrapper.get('[data-testid="site-guide-next"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]])
    wrapper.unmount()
  })

  it('highlights a visible matching interface element', async () => {
    const target = document.createElement('div')
    target.dataset.siteGuide = 'brand'
    target.getBoundingClientRect = () => ({
      top: 20, left: 30, right: 130, bottom: 60, width: 100, height: 40, x: 30, y: 20, toJSON: () => ({}),
    })
    document.body.appendChild(target)

    const wrapper = mount(SiteGuide, { props: { modelValue: true }, attachTo: document.body })
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="site-guide-spotlight"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('supports Escape and arrow-key navigation', async () => {
    const wrapper = mount(SiteGuide, { props: { modelValue: true }, attachTo: document.body })
    const panel = wrapper.get('[data-testid="site-guide-panel"]')

    await panel.trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.text()).toContain('siteGuide.steps.search.title')
    await panel.trigger('keydown', { key: 'ArrowLeft' })
    expect(wrapper.text()).toContain('siteGuide.steps.home.title')
    await panel.trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]])
    wrapper.unmount()
  })

  it('keeps the catalog interactive while waiting for a player-tour title', () => {
    const wrapper = mount(SiteGuide, {
      props: { modelValue: true, mode: 'player-picker' },
      attachTo: document.body,
    })

    expect(wrapper.text()).toContain('siteGuide.picker.title')
    expect(wrapper.find('[data-testid="site-guide-blocker"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="site-guide-next"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('runs a separate player tour and keeps player chrome awake', async () => {
    const wrapper = mount(SiteGuide, {
      props: { modelValue: true, mode: 'player' },
      attachTo: document.body,
    })

    expect(wrapper.text()).toContain('siteGuide.playerSteps.screen.title')
    expect(document.body.classList.contains('site-guide-player-active')).toBe(true)
    await wrapper.setProps({ modelValue: false })
    expect(document.body.classList.contains('site-guide-player-active')).toBe(false)
    wrapper.unmount()
  })

  it('moves the panel above a low target so Feedback remains visible', async () => {
    const feedback = document.createElement('div')
    feedback.dataset.siteGuide = 'feedback'
    feedback.getBoundingClientRect = () => ({
      top: 450, left: 30, right: 130, bottom: 500, width: 100, height: 50, x: 30, y: 450, toJSON: () => ({}),
    })
    document.body.appendChild(feedback)
    const wrapper = mount(SiteGuide, { props: { modelValue: true }, attachTo: document.body })

    for (let i = 0; i < 5; i += 1) {
      await wrapper.get('[data-testid="site-guide-next"]').trigger('click')
    }
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="site-guide-panel"]').classes())
      .toContain('top-[calc(var(--header-offset)+1rem)]')
    wrapper.unmount()
  })
})
