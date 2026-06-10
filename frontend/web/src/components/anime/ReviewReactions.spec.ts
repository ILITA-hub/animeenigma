import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'

// --- Mocks -----------------------------------------------------------------
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, fallback?: string) => (typeof fallback === 'string' ? fallback : k) }),
}))

const toggleReaction = vi.fn()
const adminRemoveReaction = vi.fn()
vi.mock('@/api/client', () => ({
  reviewApi: {
    toggleReaction: (...a: unknown[]) => toggleReaction(...a),
    adminRemoveReaction: (...a: unknown[]) => adminRemoveReaction(...a),
  },
}))

const authState = { isAuthenticated: true, isAdmin: false }
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState,
}))

const toastPush = vi.fn()
vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ push: toastPush }),
}))

import ReviewReactions from './ReviewReactions.vue'

const baseProps = { reviewId: 'rev-1', animeId: 'anime-1' }

// Both popovers are Teleported to <body> (z-index escape from the glass-card
// stacking context); stub teleport so they render inline for assertions.
const mountRR = (props: Record<string, unknown>) =>
  mount(ReviewReactions, { props, global: { stubs: { teleport: true } } } as never)

// Reaction pills carry aria-pressed; the add button carries aria-expanded;
// the picker is a role="menu"; the who-reacted popover is role="tooltip".
const pills = (w: VueWrapper) => w.findAll('button[aria-pressed]')
const addBtn = (w: VueWrapper) => w.find('button[aria-expanded]')
const pickerBtns = (w: VueWrapper) => w.findAll('[role="menu"] button')

describe('ReviewReactions (Discord/TG style)', () => {
  beforeEach(() => {
    toggleReaction.mockReset()
    adminRemoveReaction.mockReset()
    toastPush.mockReset()
    authState.isAuthenticated = true
    authState.isAdmin = false
  })

  it('renders only emojis that have reactions, not the full palette', () => {
    const w = mountRR({
      ...baseProps,
      initialReactions: [
        { emoji: '👍', count: 3, reacted_by_me: true },
        { emoji: '❤️', count: 1, reacted_by_me: false },
      ],
    })
    expect(pills(w)).toHaveLength(2)
    expect(addBtn(w).exists()).toBe(true)
    expect(pickerBtns(w)).toHaveLength(0) // palette hidden until picker opens
  })

  it('shows the count and highlights the viewer-reacted pill', () => {
    const w = mountRR({ ...baseProps, initialReactions: [{ emoji: '👍', count: 3, reacted_by_me: true }] })
    const like = pills(w)[0]
    expect(like.text()).toContain('3')
    expect(like.attributes('aria-pressed')).toBe('true')
    expect(like.classes()).toContain('bg-cyan-500/20')
  })

  it('with no reactions shows only the add button', () => {
    const w = mountRR(baseProps)
    expect(pills(w)).toHaveLength(0)
    expect(addBtn(w).exists()).toBe(true)
  })

  it('hides the add button on your own review and disables pills', () => {
    const w = mountRR({
      ...baseProps,
      isOwnReview: true,
      initialReactions: [{ emoji: '👍', count: 1, reacted_by_me: false }],
    })
    expect(addBtn(w).exists()).toBe(false)
    expect(pills(w)[0].attributes('disabled')).toBeDefined()
  })

  it('opens the 12-emoji picker when add is clicked', async () => {
    const w = mountRR(baseProps)
    await addBtn(w).trigger('click')
    expect(pickerBtns(w)).toHaveLength(12)
  })

  it('picking an emoji calls toggle and reconciles from the response', async () => {
    toggleReaction.mockResolvedValue({
      data: { data: { added: true, counts: [{ emoji: '👍', count: 1, reacted_by_me: true, users: ['bob'] }] } },
    })
    const w = mountRR(baseProps)
    await addBtn(w).trigger('click')
    const likePick = pickerBtns(w).find((b) => b.text().includes('👍'))!
    await likePick.trigger('click')
    await flushPromises()
    expect(toggleReaction).toHaveBeenCalledWith('anime-1', 'rev-1', '👍')
    expect(pills(w)).toHaveLength(1)
    expect(pills(w)[0].text()).toContain('1')
  })

  it('clicking your current pill removes it (count collapses to none)', async () => {
    toggleReaction.mockResolvedValue({ data: { data: { added: false, counts: [] } } })
    const w = mountRR({
      ...baseProps,
      initialReactions: [{ emoji: '❤️', count: 1, reacted_by_me: true }],
    })
    await pills(w)[0].trigger('click')
    await flushPromises()
    expect(toggleReaction).toHaveBeenCalledWith('anime-1', 'rev-1', '❤️')
    expect(pills(w)).toHaveLength(0)
  })

  it('switching emoji replaces the prior one (one per person)', async () => {
    toggleReaction.mockResolvedValue({
      data: { data: { added: true, counts: [{ emoji: '❤️', count: 1, reacted_by_me: true, users: ['bob'] }] } },
    })
    const w = mountRR({
      ...baseProps,
      initialReactions: [{ emoji: '👍', count: 1, reacted_by_me: true, users: ['bob'] }],
    })
    await addBtn(w).trigger('click')
    const lovePick = pickerBtns(w).find((b) => b.text().includes('❤️'))!
    await lovePick.trigger('click')
    await flushPromises()
    expect(pills(w)).toHaveLength(1)
    expect(pills(w)[0].text()).toContain('❤️')
  })

  it('highlights every viewer-reacted emoji in the picker (admin multi)', async () => {
    const w = mountRR({
      ...baseProps,
      initialReactions: [
        { emoji: '👍', count: 1, reacted_by_me: true },
        { emoji: '❤️', count: 1, reacted_by_me: true },
      ],
    })
    await addBtn(w).trigger('click')
    const highlighted = pickerBtns(w).filter((b) => b.classes().includes('bg-cyan-500/20'))
    expect(highlighted).toHaveLength(2)
  })

  it('reveals who reacted on focus and badges the System reactor', async () => {
    const w = mountRR({
      ...baseProps,
      initialReactions: [{ emoji: '👍', count: 2, reacted_by_me: false, users: ['alice', 'AnimeEnigma'] }],
    })
    await pills(w)[0].trigger('focus')
    const pop = w.find('[role="tooltip"]')
    expect(pop.exists()).toBe(true)
    expect(pop.text()).toContain('alice')
    expect(pop.text()).toContain('AnimeEnigma')
  })

  it('non-admins get no remove buttons in the who-reacted popover', async () => {
    const w = mountRR({
      ...baseProps,
      initialReactions: [
        {
          emoji: '👍',
          count: 1,
          reacted_by_me: false,
          users: ['alice'],
          reactors: [{ user_id: 'user-A', username: 'alice' }],
        },
      ],
    })
    await pills(w)[0].trigger('focus')
    expect(w.findAll('[data-testid="reaction-admin-remove"]')).toHaveLength(0)
  })

  it('admins can remove a specific user’s reaction from the popover', async () => {
    authState.isAdmin = true
    adminRemoveReaction.mockResolvedValue({ data: { data: { counts: [] } } })
    const w = mountRR({
      ...baseProps,
      initialReactions: [
        {
          emoji: '👍',
          count: 1,
          reacted_by_me: false,
          users: ['alice'],
          reactors: [{ user_id: 'user-A', username: 'alice' }],
        },
      ],
    })
    await pills(w)[0].trigger('focus')
    const removeBtns = w.findAll('[data-testid="reaction-admin-remove"]')
    expect(removeBtns).toHaveLength(1)
    await removeBtns[0].trigger('click')
    await flushPromises()
    expect(adminRemoveReaction).toHaveBeenCalledWith('anime-1', 'rev-1', '👍', 'user-A')
    expect(pills(w)).toHaveLength(0) // reconciled from the fresh counts
  })

  it('prompts login (no API call) when unauthenticated', async () => {
    authState.isAuthenticated = false
    const w = mountRR(baseProps)
    await addBtn(w).trigger('click')
    expect(toggleReaction).not.toHaveBeenCalled()
    expect(toastPush).toHaveBeenCalledWith('anime.reactions.login_prompt', 'info')
  })

  it('shows an error toast when the API call fails', async () => {
    toggleReaction.mockRejectedValue(new Error('boom'))
    const w = mountRR({
      ...baseProps,
      initialReactions: [{ emoji: '👍', count: 1, reacted_by_me: false }],
    })
    await pills(w)[0].trigger('click')
    await flushPromises()
    expect(toastPush).toHaveBeenCalledWith('anime.reactions.error', 'error')
  })
})
