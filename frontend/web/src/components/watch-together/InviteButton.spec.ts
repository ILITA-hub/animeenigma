/**
 * Workstream watch-together — Phase 02 (frontend-shell) Plan 02.6 Task 2.
 *
 * Vitest spec for InviteButton.vue. Locks the 4-step click flow:
 *
 *   click → createRoom() → router.push(/watch/room/${room_id})
 *         → navigator.clipboard.writeText(invite_url) → toast.push('success')
 *
 * The 9 tests below lock:
 *   1. Click triggers `createRoom` once with the 4 props as payload
 *   2. On success, `router.push('/watch/room/<room_id>')` called once
 *   3. On success, `navigator.clipboard.writeText(invite_url)` called
 *   4. On clipboard success, toast.push called with the `invite_copied_toast`
 *      key and `'success'` type
 *   5. On clipboard absent, toast falls back to manual-copy message
 *   6. On createRoom failure, toast.push called with `'error'` type
 *   7. While createRoom is in flight, button is disabled
 *   8. After createRoom resolves OR rejects, button is re-enabled
 *   9. No `font-bold` in rendered HTML
 *
 * All collaborators are vi.mocked so the spec exercises only the SFC's
 * behavior. The toast / router stubs are exposed as exported `vi.fn()`s so
 * we can assert on them without touching the module-singleton queue.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

// ── Mocks (must come before SFC import) ──────────────────────────────────

const pushMock = vi.fn()
const toastPushMock = vi.fn()
const createRoomMock = vi.fn()

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'en' },
  }),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock }),
}))

vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ push: toastPushMock, dismiss: vi.fn() }),
}))

vi.mock('@/api/watch-together', async () => {
  // Preserve the real module's type exports — the SFC only needs createRoom
  // as a value, but importing real types keeps tsc honest in the SFC.
  const actual = await vi.importActual<typeof import('@/api/watch-together')>(
    '@/api/watch-together',
  )
  return {
    ...actual,
    createRoom: createRoomMock,
  }
})

// Imported AFTER the vi.mock calls so the SFC resolves to the stubs.
import InviteButton from './InviteButton.vue'

const baseProps = {
  animeId: 'anime-uuid',
  episodeId: 'episode-uuid',
  player: 'kodik' as const,
  translationId: 'translation-uuid',
}

const successResponse = {
  room_id: 'room-abc',
  invite_url: 'https://animeenigma.ru/watch/room/room-abc',
  ws_url: 'wss://animeenigma.ru/api/watch-together/ws',
}

function makeClipboard() {
  return { writeText: vi.fn().mockResolvedValue(undefined) }
}

describe('InviteButton', () => {
  beforeEach(() => {
    pushMock.mockReset()
    toastPushMock.mockReset()
    createRoomMock.mockReset()
    // Default: a clipboard that resolves cleanly. Tests that need the
    // no-clipboard branch override via Object.defineProperty.
    Object.defineProperty(globalThis, 'navigator', {
      value: { clipboard: makeClipboard() },
      configurable: true,
      writable: true,
    })
  })

  it('triggers createRoom once with the props as payload on click', async () => {
    createRoomMock.mockResolvedValue(successResponse)
    const wrapper = mount(InviteButton, { props: baseProps })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(createRoomMock).toHaveBeenCalledTimes(1)
    expect(createRoomMock).toHaveBeenCalledWith({
      anime_id: 'anime-uuid',
      episode_id: 'episode-uuid',
      player: 'kodik',
      translation_id: 'translation-uuid',
    })
  })

  it('navigates to /watch/room/${room_id} on createRoom success', async () => {
    createRoomMock.mockResolvedValue(successResponse)
    const wrapper = mount(InviteButton, { props: baseProps })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(pushMock).toHaveBeenCalledTimes(1)
    expect(pushMock).toHaveBeenCalledWith('/watch/room/room-abc')
  })

  it('writes the invite_url to navigator.clipboard on success', async () => {
    createRoomMock.mockResolvedValue(successResponse)
    const wrapper = mount(InviteButton, { props: baseProps })
    const writeText = (navigator as unknown as { clipboard: { writeText: ReturnType<typeof vi.fn> } })
      .clipboard.writeText
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledTimes(1)
    expect(writeText).toHaveBeenCalledWith(successResponse.invite_url)
  })

  it('toasts the invite_copied_toast key with type "success" on clipboard success', async () => {
    createRoomMock.mockResolvedValue(successResponse)
    const wrapper = mount(InviteButton, { props: baseProps })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    // Find the matching toast call (any other defensive toasts shouldn't
    // mask the success path).
    const calls = toastPushMock.mock.calls
    const success = calls.find((c) => c[1] === 'success')
    expect(success).toBeDefined()
    expect(success![0]).toContain('watch_together.invite_copied_toast')
  })

  it('falls back to a manual-copy toast (info) when navigator.clipboard is absent', async () => {
    createRoomMock.mockResolvedValue(successResponse)
    // Strip clipboard entirely.
    Object.defineProperty(globalThis, 'navigator', {
      value: {},
      configurable: true,
      writable: true,
    })
    const wrapper = mount(InviteButton, { props: baseProps })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    // The manual-copy toast carries the invite_url so the user can copy it
    // by hand — assert both the key and the URL are in the message.
    const fallback = toastPushMock.mock.calls.find((c) => c[1] === 'info')
    expect(fallback).toBeDefined()
    expect(fallback![0]).toContain('watch_together.invite_copy_manual')
    expect(fallback![0]).toContain(successResponse.invite_url)
  })

  it('toasts with type "error" when createRoom rejects', async () => {
    createRoomMock.mockRejectedValue(new Error('boom'))
    const wrapper = mount(InviteButton, { props: baseProps })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    const errorCall = toastPushMock.mock.calls.find((c) => c[1] === 'error')
    expect(errorCall).toBeDefined()
    // router.push must NOT have been called on the error path.
    expect(pushMock).not.toHaveBeenCalled()
  })

  it('disables the button while createRoom is in flight', async () => {
    // Resolver we control manually so we can inspect the disabled state
    // BEFORE the promise resolves.
    let resolveFn!: (v: typeof successResponse) => void
    createRoomMock.mockReturnValue(
      new Promise((res) => {
        resolveFn = res
      }),
    )
    const wrapper = mount(InviteButton, { props: baseProps })
    const btn = wrapper.find<HTMLButtonElement>('button')
    await btn.trigger('click')
    // Pending state — button should be disabled.
    expect(btn.element.disabled).toBe(true)
    // Now resolve to unblock; subsequent assertion-cleanup just to keep the
    // promise queue clean.
    resolveFn(successResponse)
    await flushPromises()
  })

  it('re-enables the button after createRoom rejects', async () => {
    createRoomMock.mockRejectedValue(new Error('boom'))
    const wrapper = mount(InviteButton, { props: baseProps })
    const btn = wrapper.find<HTMLButtonElement>('button')
    await btn.trigger('click')
    await flushPromises()
    expect(btn.element.disabled).toBe(false)
  })

  it('uses only font-medium / font-semibold weights (no font-bold)', async () => {
    createRoomMock.mockResolvedValue(successResponse)
    const wrapper = mount(InviteButton, { props: baseProps })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bfont-bold\b/)
    expect(html).not.toMatch(/\bfont-black\b/)
    expect(html).not.toMatch(/\bfont-extrabold\b/)
  })
})
