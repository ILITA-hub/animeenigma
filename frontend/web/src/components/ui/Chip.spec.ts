import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

import Chip from './Chip.vue'

describe('Chip', () => {
  it('renders as a toggle button with aria-pressed', () => {
    const w = mount(Chip, { props: { active: true }, slots: { default: 'Смотрю' } })
    const btn = w.find('button')
    expect(btn.exists()).toBe(true)
    expect(btn.attributes('aria-pressed')).toBe('true')
    expect(btn.text()).toContain('Смотрю')
  })

  it('applies the active tint only when active', () => {
    expect(mount(Chip, { props: { active: true } }).classes().join(' ')).toContain('bg-primary/15')
    expect(mount(Chip, { props: { active: false } }).classes().join(' ')).toContain('bg-white/5')
  })

  it('renders the count suffix', () => {
    const w = mount(Chip, { props: { count: 42 }, slots: { default: 'Все' } })
    expect(w.find('[data-testid="chip-count"]').text()).toBe('(42)')
  })

  it('removable mode: span root, ✕ emits remove', async () => {
    const w = mount(Chip, { props: { removable: true, active: true }, slots: { default: 'Жанр' } })
    expect(w.element.tagName).toBe('SPAN')
    expect(w.attributes('aria-pressed')).toBeUndefined()
    await w.find('[data-testid="chip-remove"]').trigger('click')
    expect(w.emitted('remove')).toBeTruthy()
  })

  it('click on the toggle reaches the parent listener', async () => {
    const onClick = vi.fn()
    const w = mount(Chip, { attrs: { onClick }, slots: { default: 'x' } })
    await w.find('button').trigger('click')
    expect(onClick).toHaveBeenCalled()
  })
})
