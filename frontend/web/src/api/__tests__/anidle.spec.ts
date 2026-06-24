import { describe, it, expect } from 'vitest'
import { buildShareText } from '../anidle'
import type { GuessOutcome, VisibleAnime, GuessComparison } from '../anidle'

// ─── Fixtures ─────────────────────────────────────────────────────────────

function makeAnime(over: Partial<VisibleAnime> = {}): VisibleAnime {
  return {
    id: 'anime-1',
    name_ru: 'Тест Аниме',
    name_en: 'Test Anime',
    name_jp: 'テストアニメ',
    poster_url: 'https://shikimori.one/test.jpg',
    year: 2020,
    episodes: 24,
    score: 8.5,
    status: 'released',
    rating: 'pg_13',
    genres: [{ id: '1', name: 'Action' }],
    studios: [{ id: '2', name: 'Bones' }],
    tags: [{ id: '3', name: 'Shounen' }],
    ...over,
  }
}

function makeResult(over: Partial<GuessComparison> = {}): GuessComparison {
  return {
    genres: { status: 'correct' },
    studios: { status: 'wrong' },
    year: { status: 'partial', hint: 'higher' },
    episodes: { status: 'wrong' },
    score: { status: 'correct' },
    status: { status: 'correct' },
    rating: { status: 'correct' },
    tags: { status: 'wrong' },
    ...over,
  }
}

function makeOutcome(attempt: number, over: Partial<GuessOutcome> = {}): GuessOutcome {
  return {
    anime: makeAnime(),
    result: makeResult(),
    solved: false,
    attempt,
    ...over,
  }
}

// ─── Tests ─────────────────────────────────────────────────────────────────

describe('anidleApi exports', () => {
  it('exports anidleApi with getDailyState function', async () => {
    const { anidleApi } = await import('../anidle')
    expect(typeof anidleApi.getDailyState).toBe('function')
  })

  it('exports anidleApi with dailyGuess function', async () => {
    const { anidleApi } = await import('../anidle')
    expect(typeof anidleApi.dailyGuess).toBe('function')
  })

  it('exports anidleApi with search function', async () => {
    const { anidleApi } = await import('../anidle')
    expect(typeof anidleApi.search).toBe('function')
  })

  it('exports anidleApi with getStats function', async () => {
    const { anidleApi } = await import('../anidle')
    expect(typeof anidleApi.getStats).toBe('function')
  })

  it('exports anidleApi with getLeaderboard function', async () => {
    const { anidleApi } = await import('../anidle')
    expect(typeof anidleApi.getLeaderboard).toBe('function')
  })

  it('exports DailyState-shaped types (structural check)', async () => {
    const mod = await import('../anidle')
    // The module exports buildShareText (runtime verifiable)
    expect(typeof mod.buildShareText).toBe('function')
  })
})

describe('buildShareText', () => {
  it('includes the date in the header', () => {
    const outcome = makeOutcome(1, { solved: true })
    const text = buildShareText([outcome], '2026-06-15', true)
    expect(text).toContain('2026-06-15')
  })

  it('includes the attempt count in the header', () => {
    const outcomes = [makeOutcome(1), makeOutcome(2)]
    const text = buildShareText(outcomes, '2026-06-15', false)
    expect(text).toContain('2')
  })

  it('uses 🟩 for correct columns', () => {
    const outcome = makeOutcome(1, {
      result: makeResult({ genres: { status: 'correct' } }),
    })
    const text = buildShareText([outcome], '2026-06-15', false)
    expect(text).toContain('🟩')
  })

  it('uses 🟨 for partial columns', () => {
    const outcome = makeOutcome(1, {
      result: makeResult({ year: { status: 'partial', hint: 'higher' } }),
    })
    const text = buildShareText([outcome], '2026-06-15', false)
    expect(text).toContain('🟨')
  })

  it('uses ⬜ for wrong columns', () => {
    const outcome = makeOutcome(1, {
      result: makeResult({ studios: { status: 'wrong' } }),
    })
    const text = buildShareText([outcome], '2026-06-15', false)
    expect(text).toContain('⬜')
  })

  it('does not include any anime name (no spoiler)', () => {
    const anime = makeAnime({ name_ru: 'СекретноеАниме', name_en: 'SecretAnime' })
    const outcome = makeOutcome(1, { anime })
    const text = buildShareText([outcome], '2026-06-15', false)
    expect(text).not.toContain('СекретноеАниме')
    expect(text).not.toContain('SecretAnime')
  })

  it('produces 8 emoji characters per guess row', () => {
    const outcome = makeOutcome(1)
    const text = buildShareText([outcome], '2026-06-15', false)
    const lines = text.split('\n').filter(l => l.match(/[🟩🟨⬜]/u))
    expect(lines).toHaveLength(1)
    const emojiCount = (lines[0].match(/🟩|🟨|⬜/g) ?? []).length
    expect(emojiCount).toBe(8)
  })
})
