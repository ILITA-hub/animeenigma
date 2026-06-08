// frontend/web/src/composables/schedule/__tests__/filterSort.spec.ts
import { describe, it, expect } from 'vitest'
import { applyFilters, sortByTime, sortCellHybrid, availableGenres } from '../filterSort'
import { emptyFilters, type ScheduleAnime, type Occurrence } from '../types'

const A = (o: Partial<ScheduleAnime>): ScheduleAnime => ({ id: 'x', name: 'X', ...o })

const list: ScheduleAnime[] = [
  A({ id: '1', name: 'Kaiju No. 8', name_ru: 'Кайдзю №8', kind: 'TV', genres: [{ name: 'Action' }] }),
  A({ id: '2', name: 'Frieren', name_ru: 'Фрирен', kind: 'TV', genres: [{ name: 'Fantasy' }, { name: 'Drama' }] }),
  A({ id: '3', name: 'Dandadan', name_ru: 'Данданадан', kind: 'ONA', genres: [{ name: 'Comedy' }] }),
]

describe('applyFilters', () => {
  const statusOf = (id: string) => (id === '1' ? 'watching' : id === '2' ? 'planned' : null)

  it('no filters returns all', () => {
    expect(applyFilters(list, emptyFilters(), statusOf)).toHaveLength(3)
  })
  it('search matches russian or original (case-insensitive substring)', () => {
    const f = { ...emptyFilters(), search: 'frie' }
    expect(applyFilters(list, f, statusOf).map(a => a.id)).toEqual(['2'])
    const f2 = { ...emptyFilters(), search: 'кайдзю' }
    expect(applyFilters(list, f2, statusOf).map(a => a.id)).toEqual(['1'])
  })
  it('myList keeps only watching/planned', () => {
    const f = { ...emptyFilters(), myList: true }
    expect(applyFilters(list, f, statusOf).map(a => a.id).sort()).toEqual(['1', '2'])
  })
  it('genre filter matches any selected genre', () => {
    const f = { ...emptyFilters(), genres: new Set(['Drama']) }
    expect(applyFilters(list, f, statusOf).map(a => a.id)).toEqual(['2'])
  })
  it('type filter matches kind', () => {
    const f = { ...emptyFilters(), types: new Set(['ONA']) }
    expect(applyFilters(list, f, statusOf).map(a => a.id)).toEqual(['3'])
  })
  it('combines filters (AND across dimensions)', () => {
    const f = { ...emptyFilters(), myList: true, types: new Set(['TV']) }
    expect(applyFilters(list, f, statusOf).map(a => a.id).sort()).toEqual(['1', '2'])
  })
})

describe('availableGenres', () => {
  it('returns distinct genres by name, sorted', () => {
    expect(availableGenres(list).map(g => g.name)).toEqual(['Action', 'Comedy', 'Drama', 'Fantasy'])
  })
})

describe('sorting', () => {
  const occ = (id: string, mins: number): Occurrence => ({
    anime: A({ id, name: id }),
    episode: 1,
    date: new Date(2026, 5, 13, 0, mins),
  })

  it('sortByTime is ascending by date', () => {
    const out = sortByTime([occ('late', 120), occ('early', 30)])
    expect(out.map(o => o.anime.id)).toEqual(['early', 'late'])
  })

  it('sortCellHybrid puts priority anime first, then by time', () => {
    const isPriority = (a: ScheduleAnime) => a.id === 'fav'
    const out = sortCellHybrid([occ('a', 30), occ('fav', 120), occ('b', 10)], isPriority)
    expect(out.map(o => o.anime.id)).toEqual(['fav', 'b', 'a'])
  })

  it('sortCellHybrid with no priority falls back to time', () => {
    const out = sortCellHybrid([occ('a', 30), occ('b', 10)], () => false)
    expect(out.map(o => o.anime.id)).toEqual(['b', 'a'])
  })
})
