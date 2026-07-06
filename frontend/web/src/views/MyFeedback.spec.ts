import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

const listMyReports = vi.fn()
vi.mock('@/api/client', () => ({ userApi: { listMyReports: (...a: unknown[]) => listMyReports(...a) } }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${JSON.stringify(p)}` : k), locale: { value: 'ru' } }),
}))
// FeedbackButton drags in the auth store + i18n singleton via its import chain;
// this page's tests only cover the report list, so mock it out entirely.
vi.mock('@/components/layout/FeedbackButton.vue', () => ({
  default: { name: 'FeedbackButton', template: '<div data-test="feedback-button" />' },
}))

import MyFeedback from './MyFeedback.vue'

const item = (over: Record<string, unknown> = {}) => ({
  id: '2026-06-10T12-00-00_bot_feedback',
  timestamp: '2026-06-10T12:00:00Z',
  player_type: 'feedback',
  category: 'bug',
  description: 'Player is broken',
  status: 'new',
  ...over,
})

function mountPage() {
  return mount(MyFeedback, {
    global: {
      mocks: { $t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${JSON.stringify(p)}` : k) },
      stubs: { Spinner: true },
    },
  })
}

describe('MyFeedback', () => {
  // Braces matter: an implicit `=> mock.mockReset()` RETURNS the mock fn,
  // which vitest calls as a cleanup hook — re-invoking the rejected-promise
  // mock after the test and failing it with the "handled" error.
  beforeEach(() => { listMyReports.mockReset() })

  it('renders rows with status + category labels and description', async () => {
    listMyReports.mockResolvedValue({ data: { data: { items: [item(), item({ id: 'x2', status: 'resolved', category: 'feature', description: 'Add stuff' })], total: 2, page: 1, page_size: 20 } } })
    const w = mountPage()
    await flushPromises()
    expect(w.text()).toContain('myFeedback.status.new')
    expect(w.text()).toContain('myFeedback.status.resolved')
    expect(w.text()).toContain('myFeedback.category.bug')
    expect(w.text()).toContain('myFeedback.category.feature')
    expect(w.text()).toContain('Player is broken')
  })

  it('shows anime context when present', async () => {
    listMyReports.mockResolvedValue({ data: { data: { items: [item({ anime_name: 'Frieren', episode_number: 7, player_type: 'ourenglish' })], total: 1, page: 1, page_size: 20 } } })
    const w = mountPage()
    await flushPromises()
    expect(w.text()).toContain('Frieren')
    expect(w.text()).toContain('myFeedback.episode:{"n":7}')
  })

  it('shows empty state when there are no reports', async () => {
    listMyReports.mockResolvedValue({ data: { data: { items: [], total: 0, page: 1, page_size: 20 } } })
    const w = mountPage()
    await flushPromises()
    expect(w.text()).toContain('myFeedback.empty')
  })

  it('shows error state with retry on failure', async () => {
    // vi.fn keeps the returned promise in mock.results, which the runner
    // flags as an unhandled rejection — pre-attach a no-op catch; the
    // component still receives the same rejected promise.
    const rejected = Promise.reject(new Error('boom'))
    rejected.catch(() => {})
    listMyReports.mockReturnValue(rejected)
    const w = mountPage()
    await flushPromises()
    expect(w.text()).toContain('myFeedback.loadError')
    expect(w.text()).toContain('myFeedback.retry')
  })

  it('unknown statuses degrade to the raw value instead of a broken key', async () => {
    listMyReports.mockResolvedValue({ data: { data: { items: [item({ status: 'weird_future_status' })], total: 1, page: 1, page_size: 20 } } })
    const w = mountPage()
    await flushPromises()
    expect(w.text()).toContain('weird_future_status')
    expect(w.text()).not.toContain('myFeedback.status.weird_future_status')
  })
})
