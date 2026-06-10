import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AvatarGroup from './AvatarGroup.vue'
import Avatar from './Avatar.vue'

const items = [
  { name: 'A B' }, { name: 'C D' }, { name: 'E F' },
  { name: 'G H' }, { name: 'I J' }, { name: 'K L' },
]

describe('AvatarGroup.vue', () => {
  it('caps visible avatars at max and shows +N overflow', () => {
    const w = mount(AvatarGroup, { props: { items, max: 4 } })
    expect(w.findAllComponents(Avatar)).toHaveLength(4)
    expect(w.text()).toContain('+2')
  })
  it('shows no overflow chip when items <= max', () => {
    const w = mount(AvatarGroup, { props: { items: items.slice(0, 3), max: 4 } })
    expect(w.findAllComponents(Avatar)).toHaveLength(3)
    expect(w.text()).not.toContain('+')
  })
  it('forwards size to children', () => {
    const w = mount(AvatarGroup, { props: { items: items.slice(0, 2), size: 'sm' } })
    expect(w.findComponent(Avatar).props('size')).toBe('sm')
  })
})
