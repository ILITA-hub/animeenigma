/**
 * Vitest spec for FeedbackStatusCard.vue — the AUTO-417 feedback triage
 * loop renderer (feedback_created / feedback_in_progress / feedback_ai_done).
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'en' },
  }),
}))

const pushMock = vi.fn()
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRouter: () => ({ push: pushMock }),
}))

import FeedbackStatusCard from './FeedbackStatusCard.vue'
import type { UserNotification } from '@/types/notification'

function notif(type: string, payload: Record<string, unknown> = {}): UserNotification {
  return {
    id: 'n-1',
    user_id: 'u-1',
    type,
    dedupe_key: `feedback:r-1:${type.replace('feedback_', '')}`,
    payload: { report_id: 'r-1', status: 'created', ...payload },
    read_at: null,
    dismissed_at: null,
    clicked_at: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  } as UserNotification
}

function mountCard(n: UserNotification) {
  return mount(FeedbackStatusCard, {
    props: { notification: n },
    global: {
      mocks: { $t: (key: string) => key },
    },
  })
}

describe('FeedbackStatusCard', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    pushMock.mockClear()
  })

  it('renders the created-stage i18n keys for feedback_created', () => {
    const wrapper = mountCard(notif('feedback_created'))
    expect(wrapper.text()).toContain('notifications.feedback.created.title')
    expect(wrapper.text()).toContain('notifications.feedback.created.body')
  })

  it('renders the inProgress-stage keys for feedback_in_progress', () => {
    const wrapper = mountCard(notif('feedback_in_progress'))
    expect(wrapper.text()).toContain('notifications.feedback.inProgress.title')
  })

  it('renders the aiDone-stage keys for feedback_ai_done', () => {
    const wrapper = mountCard(notif('feedback_ai_done'))
    expect(wrapper.text()).toContain('notifications.feedback.aiDone.title')
    expect(wrapper.text()).toContain('notifications.feedback.aiDone.body')
  })

  it('shows the user description snippet when present, hides it when absent', () => {
    const withDesc = mountCard(notif('feedback_created', { description: 'плеер не работает' }))
    expect(withDesc.text()).toContain('плеер не работает')

    const without = mountCard(notif('feedback_created'))
    expect(without.text()).not.toContain('«')
  })

  it('dims the card when read', () => {
    const n = notif('feedback_created')
    n.read_at = new Date().toISOString()
    const wrapper = mountCard(n)
    expect(wrapper.classes()).toContain('opacity-70')
  })

  it('emits close (and never navigates) on row click', async () => {
    const wrapper = mountCard(notif('feedback_ai_done'))
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
    expect(pushMock).not.toHaveBeenCalled()
  })

  it('uses a distinct accent class per stage', () => {
    expect(mountCard(notif('feedback_created')).find('.text-cyan-400').exists()).toBe(true)
    expect(mountCard(notif('feedback_in_progress')).find('.text-info').exists()).toBe(true)
    expect(mountCard(notif('feedback_ai_done')).find('.text-success').exists()).toBe(true)
  })
})
