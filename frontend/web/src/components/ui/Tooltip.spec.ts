import { describe, it, expect, afterEach, beforeAll } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { TooltipProvider } from 'reka-ui'
import Tooltip from './Tooltip.vue'

// jsdom lacks ResizeObserver, which Reka's popper (TooltipContent) touches when
// open. Polyfill a no-op locally. Scoped to this spec — no shared infra change.
beforeAll(() => {
  if (!('ResizeObserver' in globalThis)) {
    ;(globalThis as unknown as { ResizeObserver: unknown }).ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  }
})

// RESEARCH Pitfall 6: TooltipContent portals to body and jsdom won't render the
// popper/open-on-hover timing. We assert the trigger slot + token-driven content
// classes (exposed) rather than the portaled tooltip DOM. TooltipRoot REQUIRES a
// TooltipProvider ancestor (mounted once in App.vue in prod) — so each mount here
// wraps Tooltip in a real TooltipProvider host.

const mounted: VueWrapper[] = []
function mountInProvider(slots: Record<string, string>, props: Record<string, unknown> = {}): VueWrapper {
  const Host = defineComponent({
    setup() {
      return () => h(TooltipProvider, {}, () => h(Tooltip, props, slots))
    },
  })
  const w = mount(Host, { attachTo: document.body }) as VueWrapper
  mounted.push(w)
  return w
}

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
  document.body.innerHTML = ''
})

describe('Tooltip.vue (Reka Tooltip)', () => {
  it('renders the #trigger slot content', () => {
    const w = mountInProvider({ trigger: '<button>Hover me</button>' })
    expect(w.text()).toContain('Hover me')
  })

  it('exposes token-driven content classes (bg-popover, text-popover-foreground, sizing)', () => {
    const w = mountInProvider({ trigger: '<button>T</button>', default: 'Tip text' })
    const tip = w.findComponent(Tooltip)
    const cls = (tip.vm as unknown as { contentClasses: string }).contentClasses
    expect(cls).toContain('bg-popover')
    expect(cls).toContain('text-popover-foreground')
    expect(cls).toContain('rounded-md')
    expect(cls).toContain('px-3')
    expect(cls).toContain('py-1.5')
    expect(cls).toContain('text-xs')
  })

  it('accepts a delayDuration prop (forwarded to TooltipRoot)', () => {
    const w = mountInProvider({ trigger: '<button>T</button>' }, { delayDuration: 100 })
    const tip = w.findComponent(Tooltip)
    const p = (tip.vm as unknown as { $props: { delayDuration?: number } }).$props
    expect(p.delayDuration).toBe(100)
  })
})
