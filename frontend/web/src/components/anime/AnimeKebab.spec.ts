import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

import AnimeKebab from './AnimeKebab.vue'

describe('AnimeKebab', () => {
  it('keeps the default top-right geometry', () => {
    const w = mount(AnimeKebab)
    const cls = w.find('button').classes()
    expect(cls).toContain('absolute')
    expect(cls).toContain('top-2')
    expect(cls).toContain('right-2')
    expect(cls).toContain('w-9')
    expect(cls).toContain('h-9')
  })

  it('lets a caller override size/position via class', () => {
    const w = mount(AnimeKebab, { props: { class: 'static w-12 h-12' } })
    const cls = w.find('button').classes()
    // tailwind-merge resolves position-type (absolute→static) and size (w-9/h-9→w-12/h-12)
    // conflicts in favour of the caller. Inset classes (top-2, right-2) are a separate
    // utility group and survive — they are harmless on a static element.
    expect(cls).toContain('w-12')
    expect(cls).toContain('h-12')
    expect(cls).not.toContain('w-9')
    expect(cls).not.toContain('h-9')
    expect(cls).toContain('static')
    expect(cls).not.toContain('absolute')
    expect(cls).toContain('top-2')
    expect(cls).toContain('right-2')
  })

  it('emits open with the button element on click', async () => {
    const w = mount(AnimeKebab)
    await w.find('button').trigger('click')
    const ev = w.emitted('open')
    expect(ev).toBeTruthy()
    expect(ev![0][0]).toBeInstanceOf(HTMLButtonElement)
  })
})
