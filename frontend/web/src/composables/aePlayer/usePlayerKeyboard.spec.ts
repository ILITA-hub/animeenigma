import { afterEach, describe, expect, it, vi } from 'vitest'
import { computed, ref, type Ref } from 'vue'
import { usePlayerKeyboard, type PlayerKeyboardDeps } from './usePlayerKeyboard'
import type { PlayerState } from './usePlayerState'

function key(keyName: string): KeyboardEvent {
  return {
    key: keyName,
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    shiftKey: false,
    target: document.body,
    preventDefault: vi.fn(),
  } as unknown as KeyboardEvent
}

function setup() {
  const root = document.createElement('div')
  const video = document.createElement('video')
  root.append(video)
  document.body.append(root)

  const isPointerInside = ref(false)
  const togglePlay = vi.fn()
  const state = {
    volume: ref(50),
    muted: ref(false),
    subOffset: ref(0),
  } as unknown as PlayerState
  const deps: PlayerKeyboardDeps = {
    rootRef: ref(root),
    videoRef: ref(video) as Ref<HTMLVideoElement | null>,
    isPointerInside,
    state,
    openMenu: ref(null),
    browseOpen: ref(false),
    wakeUi: vi.fn(),
    closeMenus: vi.fn(),
    toggleMenu: vi.fn(),
    togglePlay,
    onSeekRel: vi.fn(),
    onSetVolume: vi.fn(),
    onToggleMute: vi.fn(),
    onToggleFullscreen: vi.fn(),
    onTogglePip: vi.fn(),
    writeProgress: vi.fn(),
    anime_hasNextEp: computed(() => false),
    showNextEpisode: ref(false),
    showNextEpChip: computed(() => false),
    goToNextEpisode: vi.fn(),
  }

  return { root, isPointerInside, togglePlay, keyboard: usePlayerKeyboard(deps) }
}

afterEach(() => {
  vi.restoreAllMocks()
  Reflect.deleteProperty(document, 'fullscreenElement')
  document.body.replaceChildren()
})

describe('usePlayerKeyboard ownership', () => {
  it('does not hijack page hotkeys before the player has been activated', () => {
    const { togglePlay, keyboard } = setup()

    keyboard.onKeydown(key(' '))

    expect(togglePlay).not.toHaveBeenCalled()
  })

  it('keeps hotkeys active when Safari transiently resets pointer/focus state', () => {
    const { isPointerInside, togglePlay, keyboard } = setup()
    isPointerInside.value = true
    keyboard.onKeydown(key(' '))

    isPointerInside.value = false
    keyboard.onDocumentFocusIn({ target: document.body } as unknown as FocusEvent)
    keyboard.onKeydown(key(' '))

    expect(togglePlay).toHaveBeenCalledTimes(2)
  })

  it('releases keyboard ownership after an explicit pointer interaction outside', () => {
    const { isPointerInside, togglePlay, keyboard } = setup()
    isPointerInside.value = true
    keyboard.onKeydown(key(' '))
    isPointerInside.value = false

    const outside = document.createElement('button')
    document.body.append(outside)
    keyboard.onDocumentPointerDown({ target: outside } as unknown as PointerEvent)
    keyboard.onKeydown(key(' '))

    expect(togglePlay).toHaveBeenCalledTimes(1)
  })

  it('releases keyboard ownership when focus moves to a real outside control', () => {
    const { isPointerInside, togglePlay, keyboard } = setup()
    isPointerInside.value = true
    keyboard.onKeydown(key(' '))
    isPointerInside.value = false

    const outside = document.createElement('input')
    document.body.append(outside)
    keyboard.onDocumentFocusIn({ target: outside } as unknown as FocusEvent)
    keyboard.onKeydown(key(' '))

    expect(togglePlay).toHaveBeenCalledTimes(1)
  })

  it('treats the player fullscreen element as active even without pointer or focus', () => {
    const { root, togglePlay, keyboard } = setup()
    Object.defineProperty(document, 'fullscreenElement', {
      configurable: true,
      get: () => root,
    })

    keyboard.onKeydown(key(' '))

    expect(togglePlay).toHaveBeenCalledOnce()
  })
})
