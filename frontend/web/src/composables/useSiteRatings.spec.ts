import { describe, it, expect, vi } from 'vitest'

const getBatchRatings = vi.fn()
vi.mock('@/api/client', () => ({
  reviewApi: { getBatchRatings: (ids: string[]) => getBatchRatings(ids) },
}))

import { useSiteRatings } from './useSiteRatings'

describe('useSiteRatings', () => {
  it('parses the backend envelope { data: { ratings: { [id]: rating } } }', async () => {
    getBatchRatings.mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          ratings: {
            'id-a': { anime_id: 'id-a', average_score: 8.5, total_reviews: 2 },
          },
        },
      },
    })
    const { ratings, fetchRatings } = useSiteRatings()
    await fetchRatings(['id-a'])
    expect(ratings.value['id-a']).toEqual({ anime_id: 'id-a', average_score: 8.5, total_reviews: 2 })
  })

  it('omits zero-review anime and negative-caches misses (no refetch)', async () => {
    getBatchRatings.mockResolvedValueOnce({
      data: { success: true, data: { ratings: {} } },
    })
    const { ratings, fetchRatings } = useSiteRatings()
    await fetchRatings(['id-b'])
    expect(ratings.value['id-b']).toBeUndefined()

    await fetchRatings(['id-b'])
    expect(getBatchRatings).toHaveBeenCalledTimes(2) // 1 from previous test + 1 here, not 3
  })

  it('still accepts a flat array payload', async () => {
    getBatchRatings.mockResolvedValueOnce({
      data: [{ anime_id: 'id-c', average_score: 7.1, total_reviews: 4 }],
    })
    const { ratings, fetchRatings } = useSiteRatings()
    await fetchRatings(['id-c'])
    expect(ratings.value['id-c']?.average_score).toBe(7.1)
  })
})
