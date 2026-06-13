import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import WatchlistBulkBar from './WatchlistBulkBar.vue'

const statusOptions = [
  { value: 'watching', label: 'Watching' },
  { value: 'completed', label: 'Completed' },
]

function mountBar(count = 3) {
  return mount(WatchlistBulkBar, {
    props: { count, statusOptions },
    global: { mocks: { $t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${JSON.stringify(p)}` : k) } },
  })
}

describe('WatchlistBulkBar', () => {
  it('renders the selected count', () => {
    const wrapper = mountBar(5)
    expect(wrapper.text()).toContain('profile.bulk.selected')
    expect(wrapper.text()).toContain('5')
  })

  it('emits remove when the remove button is clicked', async () => {
    const wrapper = mountBar()
    const btn = wrapper.findAll('button').find((b) => b.text().includes('profile.bulk.remove'))
    await btn!.trigger('click')
    expect(wrapper.emitted('remove')).toBeTruthy()
  })

  it('emits clear when the clear button is clicked', async () => {
    const wrapper = mountBar()
    const btn = wrapper.findAll('button').find((b) => b.text().includes('profile.bulk.clear'))
    await btn!.trigger('click')
    expect(wrapper.emitted('clear')).toBeTruthy()
  })

  it('emits set-status with the chosen status', async () => {
    const wrapper = mountBar()
    const select = wrapper.findComponent({ name: 'Select' })
    await select.vm.$emit('update:modelValue', 'completed')
    expect(wrapper.emitted('set-status')?.[0]).toEqual(['completed'])
  })

  it('does not emit set-status for an empty value', async () => {
    const wrapper = mountBar()
    const select = wrapper.findComponent({ name: 'Select' })
    await select.vm.$emit('update:modelValue', '')
    expect(wrapper.emitted('set-status')).toBeFalsy()
  })
})
