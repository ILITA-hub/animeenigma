/**
 * Workstream watch-together — Phase 02 (frontend-shell) Plan 02.4 Task 1.
 *
 * Vitest spec for MemberList.vue. Verifies:
 *   1. Renders one <li> per member
 *   2. Member matching `hostUserId` gets the (host) badge
 *   3. Member matching `useAuthStore().user.id` gets the (you) badge
 *   4. Empty members renders the `—` fallback
 *   5. No `font-bold` class anywhere in the rendered HTML
 *   6. i18n keys flow through t() (the stubbed t() echoes the key)
 *   7. Username text is rendered for each member
 *   8. Avatar <img> is rendered when `avatar_url` is set; initial-fallback when absent
 *
 * vue-i18n is stubbed with a t() that echoes the key (plus JSON-encoded params
 * if present). useAuthStore is stubbed to return a configurable user ref so
 * individual tests can flip the (you)-target user without re-importing.
 */

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'

// ── Mocks ────────────────────────────────────────────────────────────────

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

const mockAuthUser = ref<{ id?: string; username?: string } | null>({
  id: 'self-uuid',
  username: 'self',
})
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get user() {
      return mockAuthUser.value
    },
  }),
}))

// Import AFTER vi.mock so the SFC's useI18n + useAuthStore resolve to the stubs.
import MemberList from './MemberList.vue'
import type { Member } from '@/api/watch-together'

function makeMember(userId: string, username: string, avatarUrl = ''): Member {
  return {
    user_id: userId,
    meta: {
      username,
      avatar_url: avatarUrl,
      joined_at: 1_700_000_000,
      last_seen_at: 1_700_000_000,
    },
  }
}

const baseMembers: Member[] = [
  makeMember('host-uuid', 'Alice', 'https://example.com/a.png'),
  makeMember('self-uuid', 'Bob'),
  makeMember('other-uuid', 'Charlie', 'https://example.com/c.png'),
]

function mountList(props: { members: Member[]; hostUserId: string }) {
  return mount(MemberList, { props })
}

describe('MemberList', () => {
  it('renders one <li> per member', () => {
    const wrapper = mountList({ members: baseMembers, hostUserId: 'host-uuid' })
    expect(wrapper.findAll('li')).toHaveLength(3)
  })

  it('renders the (host) badge on the host member only', () => {
    const wrapper = mountList({ members: baseMembers, hostUserId: 'host-uuid' })
    const items = wrapper.findAll('li')
    // Alice is host.
    expect(items[0].text()).toContain('watch_together.host_badge')
    // Bob is NOT host.
    expect(items[1].text()).not.toContain('watch_together.host_badge')
    // Charlie is NOT host.
    expect(items[2].text()).not.toContain('watch_together.host_badge')
  })

  it('renders the (you) badge on the auth-store-matched member only', () => {
    const wrapper = mountList({ members: baseMembers, hostUserId: 'host-uuid' })
    const items = wrapper.findAll('li')
    // Alice is NOT self.
    expect(items[0].text()).not.toContain('watch_together.you_badge')
    // Bob IS self.
    expect(items[1].text()).toContain('watch_together.you_badge')
    // Charlie is NOT self.
    expect(items[2].text()).not.toContain('watch_together.you_badge')
  })

  it('renders the `—` fallback when members is empty', () => {
    const wrapper = mountList({ members: [], hostUserId: 'host-uuid' })
    expect(wrapper.findAll('li')).toHaveLength(0)
    expect(wrapper.text()).toContain('—')
  })

  it('uses only font-medium / font-semibold weights (no font-bold)', () => {
    const wrapper = mountList({ members: baseMembers, hostUserId: 'host-uuid' })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bfont-bold\b/)
    expect(html).not.toMatch(/\bfont-black\b/)
    expect(html).not.toMatch(/\bfont-extrabold\b/)
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('renders i18n keys through t() — the members heading is present', () => {
    const wrapper = mountList({ members: baseMembers, hostUserId: 'host-uuid' })
    // With t() stubbed to echo the key, the literal key appears in the DOM.
    expect(wrapper.text()).toContain('watch_together.members_heading')
  })

  it('renders each member username', () => {
    const wrapper = mountList({ members: baseMembers, hostUserId: 'host-uuid' })
    const text = wrapper.text()
    expect(text).toContain('Alice')
    expect(text).toContain('Bob')
    expect(text).toContain('Charlie')
  })

  it('renders an <img> for members with avatar_url, fallback initial otherwise', () => {
    const wrapper = mountList({ members: baseMembers, hostUserId: 'host-uuid' })
    const imgs = wrapper.findAll('img')
    // Alice + Charlie have avatar URLs; Bob does not.
    expect(imgs).toHaveLength(2)
    expect(imgs[0].attributes('src')).toBe('https://example.com/a.png')
    // Bob's fallback shows the first letter of "Bob".
    const items = wrapper.findAll('li')
    expect(items[1].text()).toContain('B')
  })
})
