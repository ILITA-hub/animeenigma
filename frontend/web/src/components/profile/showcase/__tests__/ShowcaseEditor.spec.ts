import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ShowcaseEditor from '../ShowcaseEditor.vue'
import type { ShowcaseBlock } from '@/types/showcase'

vi.mock('vuedraggable', () => ({
  default: {
    name: 'draggable',
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<div><slot v-for="(el, idx) in modelValue" :element="el" :index="idx" name="item" /></div>',
  },
}))

const blocks: ShowcaseBlock[] = [
  { type: 'about', order: 0, config: { text: 'hi' } },
  { type: 'stats', order: 1, config: {} },
]

const mountEditor = () =>
  mount(ShowcaseEditor, {
    props: { userId: 'u1', modelValue: blocks },
    global: { mocks: { $t: (k: string) => k }, stubs: { teleport: true } },
  })

describe('ShowcaseEditor', () => {
  it('renders one row per block', () => {
    const w = mountEditor()
    expect(w.text()).toContain('showcase.block.about')
    expect(w.text()).toContain('showcase.block.stats')
  })

  it('emits save with re-numbered order on save', async () => {
    const w = mountEditor()
    await w.find('[data-test="showcase-save"]').trigger('click')
    const emitted = w.emitted('save')
    expect(emitted).toBeTruthy()
    const payload = emitted![0][0] as ShowcaseBlock[]
    expect(payload.map((b) => b.order)).toEqual([0, 1])
  })

  it('removes a block', async () => {
    const w = mountEditor()
    await w.find('[data-test="showcase-remove-0"]').trigger('click')
    await w.find('[data-test="showcase-save"]').trigger('click')
    const payload = w.emitted('save')![0][0] as ShowcaseBlock[]
    expect(payload).toHaveLength(1)
  })
})
