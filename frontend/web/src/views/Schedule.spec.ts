// frontend/web/src/views/Schedule.spec.ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

// ── Module mocks (hoisted before SUT import) ────────────────────────────────

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (k: string) => k,
      locale: { value: 'en' },
    }),
  }
})

vi.mock('@/composables/useAnime', () => ({
  useAnime: () => ({
    loading: { value: false },
    fetchSchedule: vi.fn().mockResolvedValue([
      {
        id: '1',
        name: 'Kaiju',
        name_ru: 'Кайдзю',
        kind: 'TV',
        episodes_aired: 9,
        episodes_count: 12,
        next_episode_at: '2026-06-08T17:00:00Z',
        genres: [{ name: 'Action' }],
      },
    ]),
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: vi.fn(() => ({
    isAuthenticated: false,
  })),
}))

vi.mock('@/stores/watchlist', () => ({
  useWatchlistStore: vi.fn(() => ({
    getStatus: vi.fn().mockReturnValue(null),
    fetchStatuses: vi.fn().mockResolvedValue(undefined),
  })),
}))

// ── Child component stubs ───────────────────────────────────────────────────

vi.mock('@/components/schedule/ScheduleFilters.vue', () => ({
  default: {
    name: 'ScheduleFilters',
    props: ['filters', 'genres', 'loggedIn', 'matchCount', 'total'],
    template: '<div data-testid="schedule-filters-stub" />',
  },
}))

vi.mock('@/components/schedule/MonthView.vue', () => ({
  default: {
    name: 'MonthView',
    props: ['cells'],
    emits: ['open'],
    template: '<div data-testid="month-view-stub" />',
  },
}))

vi.mock('@/components/schedule/WeekView.vue', () => ({
  default: {
    name: 'WeekView',
    props: ['columns'],
    emits: ['open'],
    template: '<div data-testid="week-view-stub" />',
  },
}))

vi.mock('@/components/schedule/TableView.vue', () => ({
  default: {
    name: 'TableView',
    props: ['rows', 'sortKey', 'sortDir'],
    emits: ['sort'],
    template: '<div data-testid="table-view-stub" />',
  },
}))

vi.mock('@/components/schedule/DayModal.vue', () => ({
  default: {
    name: 'DayModal',
    props: ['modelValue', 'date', 'occurrences'],
    template: '<div data-testid="day-modal-stub" />',
  },
}))

// ── SUT (imported after all vi.mock calls) ──────────────────────────────────

import Schedule from './Schedule.vue'

// ── Helpers ─────────────────────────────────────────────────────────────────

function mountView() {
  return mount(Schedule, {
    global: {
      mocks: { $t: (k: string) => k },
    },
  })
}

// ── Tests ────────────────────────────────────────────────────────────────────

describe('Schedule view', () => {
  it('mounts and renders the view toggle buttons', () => {
    const w = mountView()
    expect(w.text()).toContain('schedule.viewMonth')
    expect(w.text()).toContain('schedule.viewWeek')
    expect(w.text()).toContain('schedule.viewTable')
  })
})
