/**
 * Workstream watch-together — Phase 2 (frontend-shell) Plan 02.1 / Task 2.
 *
 * Vitest spec for `@/api/watch-together`. Mocks `@/api/client` (so the real
 * axios instance — with its auth interceptors — isn't invoked) and asserts:
 *   1. createRoom POSTs the body and returns the unwrapped envelope.
 *   2. getRoom GETs the room and returns the RoomSnapshot from `data`.
 *   3. getRoom on 410 throws RoomGoneError.
 *   4. deleteRoom on 403 throws RoomForbiddenError.
 *   5. deleteRoom on 204 resolves to undefined.
 *   6. Room IDs containing `/` are URL-encoded.
 *
 * Mock pattern matches `composables/useSpotlight.spec.ts` — `vi.mock(...)`
 * hoists before imports so the API client's `apiClient` resolves to the
 * spy, then we import-after-mock to grab the typed handle.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock @/api/client BEFORE importing the SUT — vi.mock is hoisted to the
// top of the file by vitest, so the apiClient symbol the SUT module
// captures resolves to the spy at import time.
vi.mock('@/api/client', () => ({
  apiClient: {
    post: vi.fn(),
    get: vi.fn(),
    delete: vi.fn(),
  },
}))

// Imported AFTER vi.mock so the alias resolves to the stub.
import { apiClient } from '@/api/client'
import { createRoom, getRoom, deleteRoom } from '@/api/watch-together'
import {
  RoomGoneError,
  RoomForbiddenError,
  type CreateRoomRequest,
  type CreateRoomResponse,
  type RoomSnapshot,
} from '@/types/watch-together'

const postSpy = apiClient.post as ReturnType<typeof vi.fn>
const getSpy = apiClient.get as ReturnType<typeof vi.fn>
const deleteSpy = apiClient.delete as ReturnType<typeof vi.fn>

beforeEach(() => {
  postSpy.mockReset()
  getSpy.mockReset()
  deleteSpy.mockReset()
})

// Helpers ───────────────────────────────────────────────────────────────

function axiosErrorWithStatus(status: number): Error & { isAxiosError: true; response: { status: number } } {
  // Shape the SUT recognizes via axios.isAxiosError + response.status.
  // We construct a plain object that satisfies the runtime check — the
  // real axios.AxiosError exposes the same surface plus a few more
  // fields the SUT never reads.
  const err = new Error(`request failed with status ${status}`) as Error & {
    isAxiosError: true
    response: { status: number }
  }
  err.isAxiosError = true
  err.response = { status }
  return err
}

function sampleSnapshot(): RoomSnapshot {
  return {
    room: {
      id: 'r1',
      created_at: 1700000000,
      anime_id: 'a1',
      episode_id: 'e1',
      player: 'kodik',
      translation_id: 't1',
      playback_state: 'paused',
      playback_time: 0,
      playback_time_updated_at: 1700000000000,
      host_user_id: 'u1',
    },
    members: [],
    messages: [],
    protocol_version: '1.0',
  }
}

// ───────────────────────────────────────────────────────────────────────

describe('createRoom', () => {
  it('POSTs /watch-together/rooms with the request body and returns unwrapped data', async () => {
    const req: CreateRoomRequest = {
      anime_id: 'a1',
      episode_id: 'e1',
      player: 'kodik',
      translation_id: 't1',
    }
    const resp: CreateRoomResponse = {
      room_id: 'r1',
      invite_url: 'https://animeenigma.ru/watch/room/r1',
      ws_url: 'wss://animeenigma.ru/api/watch-together/ws?room=r1',
    }
    postSpy.mockResolvedValueOnce({ data: { success: true, data: resp } })

    const result = await createRoom(req)

    expect(postSpy).toHaveBeenCalledTimes(1)
    expect(postSpy).toHaveBeenCalledWith('/watch-together/rooms', req)
    expect(result).toEqual(resp)
  })

  it('handles bare-data responses (no envelope) defensively', async () => {
    const req: CreateRoomRequest = {
      anime_id: 'a1',
      episode_id: 'e1',
      player: 'kodik',
      translation_id: 't1',
    }
    const resp: CreateRoomResponse = {
      room_id: 'r1',
      invite_url: '/watch/room/r1',
      ws_url: 'ws://x/api/watch-together/ws?room=r1',
    }
    // Some test harnesses bypass libs/httputil and return the bare payload.
    // The unwrap helper's fallback should still surface the data.
    postSpy.mockResolvedValueOnce({ data: resp })

    const result = await createRoom(req)

    expect(result).toEqual(resp)
  })
})

describe('getRoom', () => {
  it('GETs /watch-together/rooms/{id} and returns the RoomSnapshot from data', async () => {
    const snap = sampleSnapshot()
    getSpy.mockResolvedValueOnce({ data: { success: true, data: snap } })

    const result = await getRoom('r1')

    expect(getSpy).toHaveBeenCalledTimes(1)
    expect(getSpy).toHaveBeenCalledWith('/watch-together/rooms/r1')
    expect(result).toEqual(snap)
  })

  it('URL-encodes room IDs containing slashes', async () => {
    const snap = sampleSnapshot()
    getSpy.mockResolvedValueOnce({ data: { data: snap } })

    await getRoom('abc/def')

    expect(getSpy).toHaveBeenCalledWith('/watch-together/rooms/abc%2Fdef')
  })

  it('throws RoomGoneError on 410', async () => {
    getSpy.mockRejectedValueOnce(axiosErrorWithStatus(410))

    await expect(getRoom('r1')).rejects.toBeInstanceOf(RoomGoneError)
  })

  it('rethrows non-410 axios errors verbatim', async () => {
    const err = axiosErrorWithStatus(500)
    getSpy.mockRejectedValueOnce(err)

    await expect(getRoom('r1')).rejects.toBe(err)
  })
})

describe('deleteRoom', () => {
  it('DELETEs /watch-together/rooms/{id} and resolves to undefined on 204', async () => {
    deleteSpy.mockResolvedValueOnce({ status: 204, data: '' })

    await expect(deleteRoom('r1')).resolves.toBeUndefined()
    expect(deleteSpy).toHaveBeenCalledWith('/watch-together/rooms/r1')
  })

  it('throws RoomForbiddenError on 403', async () => {
    deleteSpy.mockRejectedValueOnce(axiosErrorWithStatus(403))

    await expect(deleteRoom('r1')).rejects.toBeInstanceOf(RoomForbiddenError)
  })

  it('throws RoomGoneError on 410', async () => {
    deleteSpy.mockRejectedValueOnce(axiosErrorWithStatus(410))

    await expect(deleteRoom('r1')).rejects.toBeInstanceOf(RoomGoneError)
  })

  it('URL-encodes room IDs containing slashes', async () => {
    deleteSpy.mockResolvedValueOnce({ status: 204, data: '' })

    await deleteRoom('abc/def')

    expect(deleteSpy).toHaveBeenCalledWith('/watch-together/rooms/abc%2Fdef')
  })

  it('rethrows non-403/410 axios errors verbatim', async () => {
    const err = axiosErrorWithStatus(500)
    deleteSpy.mockRejectedValueOnce(err)

    await expect(deleteRoom('r1')).rejects.toBe(err)
  })
})

describe('guest auth header (logged-out invite-link join)', () => {
  it('getRoom attaches a Bearer header when a guest token is passed', async () => {
    getSpy.mockResolvedValueOnce({ data: { data: sampleSnapshot() } })

    await getRoom('r1', 'guest.jwt.token')

    expect(getSpy).toHaveBeenCalledWith('/watch-together/rooms/r1', {
      headers: { Authorization: 'Bearer guest.jwt.token' },
    })
  })

  it('getRoom sends NO explicit config when no token is passed (authenticated path)', async () => {
    getSpy.mockResolvedValueOnce({ data: { data: sampleSnapshot() } })

    await getRoom('r1')

    expect(getSpy).toHaveBeenCalledWith('/watch-together/rooms/r1')
  })

  it('getRoom sends NO explicit config for a null/undefined token (guest without a token yet)', async () => {
    getSpy.mockResolvedValueOnce({ data: { data: sampleSnapshot() } })

    await getRoom('r1', null)

    expect(getSpy).toHaveBeenCalledWith('/watch-together/rooms/r1')
  })
})
