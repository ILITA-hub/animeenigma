import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/api/client', () => ({ apiClient: { post: vi.fn().mockResolvedValue({}) } }))
import { apiClient } from '@/api/client'
import { emitRecWatchedIfRecent } from '../recsAnalytics'

describe('emitRecWatchedIfRecent', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('emits rec_watched and removes the click (fire-once)', async () => {
    localStorage.setItem('recentRecClicks', JSON.stringify([
      { anime_id: 'a1', signal_id: 's1', pinned: false, timestamp: Date.now() },
    ]))
    await emitRecWatchedIfRecent('a1', 'player')
    expect(apiClient.post).toHaveBeenCalledWith('/events/rec', expect.objectContaining({
      event_type: 'rec_watched', anime_id: 'a1', signal_id: 's1', source_route: 'player',
    }))
    await emitRecWatchedIfRecent('a1', 'player')
    expect(apiClient.post).toHaveBeenCalledTimes(1) // fire-once
  })

  it('does nothing without a recent click', async () => {
    await emitRecWatchedIfRecent('a2', 'player')
    expect(apiClient.post).not.toHaveBeenCalled()
  })

  it('honors the 7-day window', async () => {
    localStorage.setItem('recentRecClicks', JSON.stringify([
      { anime_id: 'a1', signal_id: 's1', pinned: false, timestamp: Date.now() - 8 * 24 * 3600 * 1000 },
    ]))
    await emitRecWatchedIfRecent('a1', 'player')
    expect(apiClient.post).not.toHaveBeenCalled()
  })
})
