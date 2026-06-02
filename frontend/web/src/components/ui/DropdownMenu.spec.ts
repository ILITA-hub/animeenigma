import { describe, it, expect, afterEach, beforeAll } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import DropdownMenu from './DropdownMenu.vue'

// jsdom lacks ResizeObserver, which Reka's popper (DropdownMenuContent) touches
// when open. Polyfill a no-op locally so the open-portal mount doesn't throw.
// Scoped to this spec — does NOT touch shared test infra.
beforeAll(() => {
  if (!('ResizeObserver' in globalThis)) {
    (globalThis as unknown as { ResizeObserver: unknown }).ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  }
})

// RESEARCH Pitfall 6: Reka DropdownMenuContent portals to document.body, which
// jsdom cannot fully render (no popper layout/focus). So we assert on the
// trigger slot + the controlled-open emit bridge rather than the portaled
// content DOM. The anchored-mode `reference` prop (which lets Plan 04 anchor the
// menu to the anime-card kebab WITHOUT a literal trigger child) is exercised
// LIVE in Plan 04's kebab rebuild; here we only assert it is an accepted prop.

const mounted: VueWrapper[] = []
function mountDD(...args: Parameters<typeof mount>): VueWrapper {
  const w = mount(...args) as VueWrapper
  mounted.push(w)
  return w
}

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
  document.body.innerHTML = ''
})

describe('DropdownMenu.vue (Reka DropdownMenu)', () => {
  it('renders a DropdownMenuRoot with the #trigger slot content inside the trigger', () => {
    const w = mountDD(DropdownMenu, {
      slots: { trigger: '<button>Open menu</button>' },
    })
    // The trigger slot is rendered (the trigger button is in the wrapper DOM).
    expect(w.text()).toContain('Open menu')
  })

  it('controlled open=true portals the default-slot menu body to document.body', async () => {
    mountDD(DropdownMenu, {
      props: { open: true },
      slots: {
        trigger: '<button>T</button>',
        default: '<div data-test="item">Item A</div>',
      },
      attachTo: document.body,
    })
    await new Promise(r => setTimeout(r, 0))
    expect(document.body.textContent).toContain('Item A')
  })

  it('emitting a Reka open-change re-emits update:open through the bridge', async () => {
    const w = mountDD(DropdownMenu, {
      props: { open: false },
      slots: { trigger: '<button>T</button>' },
    })
    ;(w.vm as unknown as { onOpenUpdate: (v: boolean) => void }).onOpenUpdate(true)
    await w.vm.$nextTick()
    expect(w.emitted('update:open')).toBeTruthy()
    expect(w.emitted('update:open')!.at(-1)).toEqual([true])
  })

  it('exposes token-driven content classes (bg-popover, rounded, border)', () => {
    const w = mountDD(DropdownMenu, { slots: { trigger: '<button>T</button>' } })
    const cls = (w.vm as unknown as { contentClasses: string }).contentClasses
    expect(cls).toContain('bg-popover')
    expect(cls).toContain('text-popover-foreground')
    expect(cls).toMatch(/rounded-/)
    expect(cls).toContain('border')
  })

  it('accepts a `reference` prop for anchored mode (Plan 04 kebab) without a trigger', () => {
    // Virtual element with the PopperContent-compatible bounding-rect source.
    const virtualEl = {
      getBoundingClientRect: () =>
        ({ x: 0, y: 0, width: 0, height: 0, top: 0, left: 0, right: 0, bottom: 0 }) as DOMRect,
    }
    const w = mountDD(DropdownMenu, {
      props: { open: true, reference: virtualEl },
      slots: { default: '<div>Anchored</div>' },
      attachTo: document.body,
    })
    // The prop is accepted (anchored mode); identity may be proxied by Vue, so
    // assert the bounding-rect source survives rather than reference identity.
    const props = (w.vm as unknown as { $props: { reference?: { getBoundingClientRect: () => DOMRect } } }).$props
    expect(typeof props.reference?.getBoundingClientRect).toBe('function')
  })

  it('forwards align/side/sideOffset props with sensible defaults', () => {
    const w = mountDD(DropdownMenu, { slots: { trigger: '<button>T</button>' } })
    const p = (w.vm as unknown as { $props: { align: string; side: string; sideOffset: number } }).$props
    expect(p.align).toBe('start')
    expect(p.side).toBe('bottom')
    expect(p.sideOffset).toBe(4)
  })
})
