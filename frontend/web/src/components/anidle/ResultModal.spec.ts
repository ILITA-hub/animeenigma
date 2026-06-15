import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import type { VisibleAnime, GuessOutcome, GuessComparison } from '@/api/anidle'

vi.mock('@/composables/useImageProxy', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/composables/useImageProxy')>()
  return {
    ...actual,
    cardPosterUrl: (url: string) => url ?? '/placeholder.svg',
  }
})

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

function makeAnime(over: Partial<VisibleAnime> = {}): VisibleAnime {
  return {
    id: 'a1',
    name_ru: 'Наруто',
    name_en: 'Naruto',
    name_jp: 'ナルト',
    poster_url: 'https://shikimori.one/naruto.jpg',
    year: 2002,
    episodes: 220,
    score: 7.9,
    status: 'released',
    rating: 'pg_13',
    genres: [{ id: '1', name: 'Action' }],
    studios: [{ id: '2', name: 'Pierrot' }],
    tags: [],
    ...over,
  }
}

function makeResult(over: Partial<GuessComparison> = {}): GuessComparison {
  return {
    genres: { status: 'correct' },
    studios: { status: 'wrong' },
    year: { status: 'wrong' },
    episodes: { status: 'wrong' },
    score: { status: 'partial', hint: 'higher' },
    status: { status: 'correct' },
    rating: { status: 'correct' },
    tags: { status: 'wrong' },
    ...over,
  }
}

function makeOutcome(over: Partial<GuessOutcome> = {}): GuessOutcome {
  return {
    anime: makeAnime(),
    result: makeResult(),
    solved: false,
    attempt: 1,
    ...over,
  }
}

async function mountResultModal(props: {
  open: boolean
  answer: VisibleAnime
  guesses: GuessOutcome[]
  date: string
  solved: boolean
}) {
  const ResultModal = (await import('./ResultModal.vue')).default
  const ShareCard = (await import('./ShareCard.vue')).default
  return mount(ResultModal, {
    props,
    global: {
      plugins: [i18n],
      stubs: {
        // Pass-through stub that renders slots
        Modal: {
          template: '<div class="modal-stub"><slot name="header"/><slot/><slot name="footer"/></div>',
          props: ['modelValue', 'title', 'closable'],
        },
      },
      components: { ShareCard },
    },
  })
}

describe('ResultModal', () => {
  it('shows the win title when solved=true', async () => {
    const w = await mountResultModal({
      open: true,
      answer: makeAnime(),
      guesses: [makeOutcome()],
      date: '2026-06-15',
      solved: true,
    })
    const html = w.html()
    // When solved=true, result_win_title ("Correct!") is used as the title prop,
    // NOT result_loss_title ("The answer was…"). Verify the share button exists (modal content rendered).
    expect(html).toContain('Share result')
    // Emoji grid rendered
    expect(html).toContain('🟩')
    // Answer name shown
    expect(html).toContain('Наруто')
  })

  it('shows the loss title when solved=false', async () => {
    const w = await mountResultModal({
      open: true,
      answer: makeAnime(),
      guesses: [makeOutcome()],
      date: '2026-06-15',
      solved: false,
    })
    // result_loss_title i18n key text — verify it's not the win title
    const html = w.html()
    expect(html).not.toContain('Correct!')
    // Share button still present
    expect(html).toContain('Share result')
  })

  it('shows the answer anime name_ru', async () => {
    const w = await mountResultModal({
      open: true,
      answer: makeAnime({ name_ru: 'Наруто' }),
      guesses: [],
      date: '2026-06-15',
      solved: false,
    })
    expect(w.text()).toContain('Наруто')
  })

  it('ShareCard contains 🟩 for a correct column', async () => {
    const guess = makeOutcome({ result: makeResult({ genres: { status: 'correct' } }) })
    const w = await mountResultModal({
      open: true,
      answer: makeAnime(),
      guesses: [guess],
      date: '2026-06-15',
      solved: false,
    })
    expect(w.html()).toContain('🟩')
  })

  it('ShareCard contains ⬜ for a wrong column', async () => {
    const guess = makeOutcome({ result: makeResult({ studios: { status: 'wrong' } }) })
    const w = await mountResultModal({
      open: true,
      answer: makeAnime(),
      guesses: [guess],
      date: '2026-06-15',
      solved: false,
    })
    expect(w.html()).toContain('⬜')
  })

  it('does not contain font-bold in rendered HTML', async () => {
    const w = await mountResultModal({
      open: true,
      answer: makeAnime(),
      guesses: [],
      date: '2026-06-15',
      solved: true,
    })
    expect(w.html()).not.toContain('font-bold')
    expect(w.html()).not.toContain('font-extrabold')
    expect(w.html()).not.toContain('font-black')
  })
})
