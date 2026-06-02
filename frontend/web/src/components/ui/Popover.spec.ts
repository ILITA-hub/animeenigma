import { describe, it, expect, afterEach, beforeAll } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import Popover from './Popover.vue'

// jsdom lacks ResizeObserver, which Reka's popper (PopoverContent) touches when
// open. Polyfill a no-op locally so the open-portal mount doesn't throw and
// null out wrapper.vm. Scoped to this spec — does NOT touch shared test infra.
beforeAll(() => {
  if (!('ResizeObserver' in globalThis)) {
    ;(globalThis as unknown as { ResizeObserver: unknown }).ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  }
})

// RESEARCH Pitfall 6: PopoverContent portals to body; jsdom can't render the
// popper. We assert the trigger slot + the v-model:open emit bridge + the
// token-driven content classes (exposed), not the portaled DOM.

const mounted: VueWrapper[] = []
function mountPop(...args: Parameters<typeof mount>): VueWrapper {
  const w = mount(...args) as VueWrapper
  mounted.push(w)
  return w
}

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
  document.body.innerHTML = ''
})

describe('Popover.vue (Reka Popover)', () => {
  it('renders the #trigger slot content', () => {
    const w = mountPop(Popover, { slots: { trigger: '<button>Open</button>' } })
    expect(w.text()).toContain('Open')
  })

  it('open=true portals the default-slot content to document.body', async () => {
    mountPop(Popover, {
      props: { open: true },
      slots: { trigger: '<button>T</button>', default: '<div>Body here</div>' },
      attachTo: document.body,
    })
    await new Promise(r => setTimeout(r, 0))
    expect(document.body.textContent).toContain('Body here')
  })

  it('Reka open-change re-emits update:open through the bridge', async () => {
    const w = mountPop(Popover, {
      props: { open: false },
      slots: { trigger: '<button>T</button>' },
    })
    ;(w.vm as unknown as { onOpenUpdate: (v: boolean) => void }).onOpenUpdate(true)
    await w.vm.$nextTick()
    expect(w.emitted('update:open')).toBeTruthy()
    expect(w.emitted('update:open')!.at(-1)).toEqual([true])
  })

  it('exposes token-driven content classes (bg-popover, border, rounded-xl, p-4)', () => {
    const w = mountPop(Popover, { slots: { trigger: '<button>T</button>' } })
    const cls = (w.vm as unknown as { contentClasses: string }).contentClasses
    expect(cls).toContain('bg-popover')
    expect(cls).toContain('text-popover-foreground')
    expect(cls).toContain('border')
    expect(cls).toContain('rounded-xl')
    expect(cls).toContain('p-4')
  })
})
