import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

// --- Mocks -----------------------------------------------------------------
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, fallback?: string) => fallback ?? k }),
}))

const toggleReaction = vi.fn()
vi.mock('@/api/client', () => ({
  reviewApi: { toggleReaction: (...a: unknown[]) => toggleReaction(...a) },
}))

const authState = { isAuthenticated: true }
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState,
}))

const toastPush = vi.fn()
vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ push: toastPush }),
}))

import ReviewReactions from './ReviewReactions.vue'

const baseProps = {
  reviewId: 'rev-1',
  animeId: 'anime-1',
}

describe('ReviewReactions', () => {
  beforeEach(() => {
    toggleReaction.mockReset()
    toastPush.mockReset()
    authState.isAuthenticated = true
  })

  it('renders all 12 emoji pills', () => {
    const w = mount(ReviewReactions, { props: baseProps })
    const pills = w.findAll('button')
    expect(pills).toHaveLength(12)
  })

  it('seeds counts and highlights the viewer-reacted pill', () => {
    const w = mount(ReviewReactions, {
      props: {
        ...baseProps,
        initialReactions: [{ emoji: '👍', count: 3, reacted_by_me: true }],
      },
    })
    const likePill = w.findAll('button').find((b) => b.text().includes('👍'))!
    expect(likePill.text()).toContain('3')
    expect(likePill.attributes('aria-pressed')).toBe('true')
    expect(likePill.classes()).toContain('bg-cyan-500/20')
  })

  it('hides the count badge when an emoji has zero reactions', () => {
    const w = mount(ReviewReactions, { props: baseProps })
    const likePill = w.findAll('button').find((b) => b.text().includes('👍'))!
    expect(likePill.text().replace('👍', '').trim()).toBe('')
  })

  it('prompts login (no API call) when unauthenticated', async () => {
    authState.isAuthenticated = false
    const w = mount(ReviewReactions, { props: baseProps })
    await w.findAll('button')[0].trigger('click')
    expect(toggleReaction).not.toHaveBeenCalled()
    expect(toastPush).toHaveBeenCalledWith('anime.reactions.login_prompt', 'info')
  })

  it('toggles a reaction and reconciles from the server response', async () => {
    toggleReaction.mockResolvedValue({
      data: { data: { added: true, counts: [{ emoji: '👍', count: 1, reacted_by_me: true }] } },
    })
    const w = mount(ReviewReactions, { props: baseProps })
    const likePill = w.findAll('button').find((b) => b.text().includes('👍'))!
    await likePill.trigger('click')
    await flushPromises()
    expect(toggleReaction).toHaveBeenCalledWith('anime-1', 'rev-1', '👍')
    expect(likePill.text()).toContain('1')
    expect(likePill.attributes('aria-pressed')).toBe('true')
  })

  it('collapses the count to zero when the last reaction is toggled off', async () => {
    toggleReaction.mockResolvedValue({
      data: { data: { added: false, counts: [] } },
    })
    const w = mount(ReviewReactions, {
      props: {
        ...baseProps,
        initialReactions: [{ emoji: '❤️', count: 1, reacted_by_me: true }],
      },
    })
    const lovePill = w.findAll('button').find((b) => b.text().includes('❤️'))!
    expect(lovePill.text()).toContain('1')
    await lovePill.trigger('click')
    await flushPromises()
    expect(lovePill.attributes('aria-pressed')).toBe('false')
    expect(lovePill.text().replace('❤️', '').trim()).toBe('')
  })

  it('shows an error toast when the API call fails', async () => {
    toggleReaction.mockRejectedValue(new Error('boom'))
    const w = mount(ReviewReactions, { props: baseProps })
    await w.findAll('button')[0].trigger('click')
    await flushPromises()
    expect(toastPush).toHaveBeenCalledWith('anime.reactions.error', 'error')
  })
})
