import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Tabs from './Tabs.vue'

const baseTabs = [
  { value: 'a', label: 'A' },
  { value: 'b', label: 'B' },
]

describe('Tabs.vue', () => {
  it('renders one role=tab button per tab; active tab has aria-selected=true', () => {
    const w = mount(Tabs, { props: { modelValue: 'a', tabs: baseTabs } })
    const tabs = w.findAll('[role="tab"]')
    expect(tabs).toHaveLength(2)
    expect(tabs[0].attributes('aria-selected')).toBe('true')
    expect(tabs[1].attributes('aria-selected')).toBe('false')
  })

  it('clicking a tab emits update:modelValue with that value', async () => {
    const w = mount(Tabs, { props: { modelValue: 'a', tabs: baseTabs } })
    await w.findAll('[role="tab"]')[1].trigger('click')
    expect(w.emitted('update:modelValue')).toBeTruthy()
    expect(w.emitted('update:modelValue')!.at(-1)).toEqual(['b'])
  })

  it('active tab (default variant) classes contain bg-white/10 + text-white; inactive contains text-white/60', () => {
    const w = mount(Tabs, { props: { modelValue: 'a', tabs: baseTabs } })
    const tabs = w.findAll('[role="tab"]')
    expect(tabs[0].classes()).toContain('bg-white/10')
    expect(tabs[0].classes()).toContain('text-white')
    expect(tabs[1].classes()).toContain('text-white/60')
  })

  it("variant='pills' active classes contain bg-cyan-500/20", () => {
    const w = mount(Tabs, { props: { modelValue: 'a', tabs: baseTabs, variant: 'pills' } })
    expect(w.findAll('[role="tab"]')[0].classes()).toContain('bg-cyan-500/20')
  })

  it("variant='underline' active classes contain border-cyan-400", () => {
    const w = mount(Tabs, { props: { modelValue: 'a', tabs: baseTabs, variant: 'underline' } })
    expect(w.findAll('[role="tab"]')[0].classes()).toContain('border-cyan-400')
  })

  it('tabpanel has id tabpanel-a + aria-labelledby tab-a; #a slot content renders', () => {
    const w = mount(Tabs, {
      props: { modelValue: 'a', tabs: baseTabs },
      slots: { a: '<p>panel-a</p>' },
    })
    const panel = w.find('[role="tabpanel"]')
    expect(panel.attributes('id')).toBe('tabpanel-a')
    expect(panel.attributes('aria-labelledby')).toBe('tab-a')
    expect(panel.text()).toContain('panel-a')
  })

  it('each tab button has id tab-${value} + aria-controls tabpanel-${value}', () => {
    const w = mount(Tabs, { props: { modelValue: 'a', tabs: baseTabs } })
    const tabA = w.findAll('[role="tab"]')[0]
    expect(tabA.attributes('id')).toBe('tab-a')
    expect(tabA.attributes('aria-controls')).toBe('tabpanel-a')
  })

  it('tab.count renders the count pill', () => {
    const w = mount(Tabs, {
      props: { modelValue: 'a', tabs: [{ value: 'a', label: 'A', count: 3 }] },
    })
    const pill = w.find('span.rounded-full')
    expect(pill.exists()).toBe(true)
    expect(pill.text()).toContain('3')
  })

  it('tab.disabled adds cursor-not-allowed', () => {
    const w = mount(Tabs, {
      props: { modelValue: 'a', tabs: [{ value: 'a', label: 'A', disabled: true }] },
    })
    expect(w.findAll('[role="tab"]')[0].classes()).toContain('cursor-not-allowed')
  })
})
