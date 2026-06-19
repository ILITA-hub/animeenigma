/**
 * Spec for useWatchTogetherLaunch — the shared create-room → navigate →
 * copy-invite flow used by both InviteButton and the in-player WT button.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

const { pushMock, toastPushMock, createRoomMock } = vi.hoisted(() => ({
  pushMock: vi.fn(),
  toastPushMock: vi.fn(),
  createRoomMock: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }),
}))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: pushMock }) }))
vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ push: toastPushMock, dismiss: vi.fn() }),
}))
vi.mock('@/api/watch-together', () => ({ createRoom: createRoomMock }))

import { useWatchTogetherLaunch } from './useWatchTogetherLaunch'

const PAYLOAD = {
  animeId: 'anime-1',
  episodeId: '3',
  player: 'aeplayer' as const,
  translationId: 'tok-1',
}

beforeEach(() => {
  pushMock.mockReset()
  toastPushMock.mockReset()
  createRoomMock.mockReset()
  // Default: clipboard available + writes succeed.
  Object.assign(navigator, {
    clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
  })
})

describe('useWatchTogetherLaunch', () => {
  it('creates the room with the mapped payload', async () => {
    createRoomMock.mockResolvedValue({ room_id: 'r1', invite_url: 'http://x/r1' })
    const { launch } = useWatchTogetherLaunch()
    await launch(PAYLOAD)
    expect(createRoomMock).toHaveBeenCalledTimes(1)
    expect(createRoomMock).toHaveBeenCalledWith({
      anime_id: 'anime-1',
      episode_id: '3',
      player: 'aeplayer',
      translation_id: 'tok-1',
    })
  })

  it('navigates to the new room', async () => {
    createRoomMock.mockResolvedValue({ room_id: 'r1', invite_url: 'http://x/r1' })
    const { launch } = useWatchTogetherLaunch()
    await launch(PAYLOAD)
    expect(pushMock).toHaveBeenCalledWith('/watch/room/r1')
  })

  it('copies the invite and shows a success toast', async () => {
    createRoomMock.mockResolvedValue({ room_id: 'r1', invite_url: 'http://x/r1' })
    const { launch } = useWatchTogetherLaunch()
    await launch(PAYLOAD)
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('http://x/r1')
    expect(toastPushMock).toHaveBeenCalledWith(
      'watch_together.invite_copied_toast',
      'success',
      4000,
    )
  })

  it('falls back to a manual-copy info toast when clipboard is unavailable', async () => {
    Object.assign(navigator, { clipboard: undefined })
    createRoomMock.mockResolvedValue({ room_id: 'r1', invite_url: 'http://x/r1' })
    const { launch } = useWatchTogetherLaunch()
    await launch(PAYLOAD)
    expect(pushMock).toHaveBeenCalledWith('/watch/room/r1')
    expect(toastPushMock).toHaveBeenCalledWith(
      expect.stringContaining('http://x/r1'),
      'info',
      8000,
    )
  })

  it('shows an error toast and does not navigate when createRoom rejects', async () => {
    createRoomMock.mockRejectedValue(new Error('boom'))
    const { launch } = useWatchTogetherLaunch()
    await launch(PAYLOAD)
    expect(pushMock).not.toHaveBeenCalled()
    expect(toastPushMock).toHaveBeenCalledWith(
      'watch_together.invite_failed_toast',
      'error',
      4000,
    )
  })

  it('refuses an empty translationId for a NON-aeplayer player', async () => {
    const { launch } = useWatchTogetherLaunch()
    await launch({ ...PAYLOAD, player: 'kodik' as const, translationId: '' })
    expect(createRoomMock).not.toHaveBeenCalled()
    expect(toastPushMock).toHaveBeenCalledWith(
      'watch_together.invite_failed_toast',
      'error',
      4000,
    )
  })

  it('ALLOWS an empty translationId for aeplayer (token-less room)', async () => {
    createRoomMock.mockResolvedValue({ room_id: 'r1', invite_url: 'http://x/r1' })
    const { launch } = useWatchTogetherLaunch()
    await launch({ ...PAYLOAD, player: 'aeplayer' as const, translationId: '' })
    expect(createRoomMock).toHaveBeenCalledTimes(1)
    expect(createRoomMock).toHaveBeenCalledWith(
      expect.objectContaining({ player: 'aeplayer', translation_id: '' }),
    )
    expect(pushMock).toHaveBeenCalledWith('/watch/room/r1')
  })

  it('exposes a launching flag that is false after completion', async () => {
    createRoomMock.mockResolvedValue({ room_id: 'r1', invite_url: 'http://x/r1' })
    const { launch, launching } = useWatchTogetherLaunch()
    expect(launching.value).toBe(false)
    await launch(PAYLOAD)
    expect(launching.value).toBe(false)
  })
})
