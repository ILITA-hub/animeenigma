import { describe, it, expect, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import Modal from './Modal.vue'

// Track every mounted wrapper so afterEach can unmount it. Unmounting matters
// here because useBodyScrollLock keeps a MODULE-LEVEL refcount + captured prior
// overflow; a leaked (never-unmounted) open Modal would hold the lock and skew
// the next test's scroll-lock assertion.
const mounted: VueWrapper[] = []
function mountModal(...args: Parameters<typeof mount>): VueWrapper {
  const w = mount(...args) as VueWrapper
  mounted.push(w)
  return w
}

// RESEARCH Pitfall 6: Reka DialogContent portals to document.body, so we mount
// with attachTo: document.body and query document.body for portaled content
// rather than wrapper.find(). jsdom cannot verify the portal/focus-trap/
// transition/scroll-lock VISUALLY — those go to the in-browser gate. Here we
// assert the logic/props/emit/slot contract we CAN observe in jsdom.

afterEach(() => {
  // Unmount first so useBodyScrollLock releases its refcount, then scrub the
  // body of any portaled Reka nodes + reset overflow for the next test.
  while (mounted.length) mounted.pop()!.unmount()
  document.body.innerHTML = ''
  document.body.style.overflow = ''
})

describe('Modal.vue (Reka Dialog)', () => {
  it('closed (modelValue=false) renders no dialog content in document.body', () => {
    mountModal(Modal, { props: { modelValue: false }, attachTo: document.body })
    expect(document.body.querySelector('[role="dialog"]')).toBeNull()
  })

  it('open (modelValue=true) renders portaled dialog content with the title text', async () => {
    mountModal(Modal, {
      props: { modelValue: true, title: 'Hi' },
      attachTo: document.body,
    })
    await new Promise(r => setTimeout(r, 0))
    const content = document.body.querySelector('[role="dialog"]')
    expect(content).not.toBeNull()
    expect(document.body.textContent).toContain('Hi')
  })

  it('default + #header + #footer slots render when open', async () => {
    mountModal(Modal, {
      props: { modelValue: true },
      slots: {
        default: '<p>body-content</p>',
        header: '<span>custom-header</span>',
        footer: '<button>footer-btn</button>',
      },
      attachTo: document.body,
    })
    await new Promise(r => setTimeout(r, 0))
    expect(document.body.textContent).toContain('body-content')
    expect(document.body.textContent).toContain('custom-header')
    expect(document.body.textContent).toContain('footer-btn')
  })

  it('closing (open -> false) emits update:modelValue false AND close', async () => {
    const w = mountModal(Modal, { props: { modelValue: true }, attachTo: document.body })
    await new Promise(r => setTimeout(r, 0))
    // Drive Reka's open update through the component's bridge handler.
    ;(w.vm as unknown as { onOpenUpdate: (v: boolean) => void }).onOpenUpdate(false)
    await w.vm.$nextTick()
    expect(w.emitted('update:modelValue')).toBeTruthy()
    expect(w.emitted('update:modelValue')!.at(-1)).toEqual([false])
    expect(w.emitted('close')).toBeTruthy()
  })

  it('opening engages the body scroll-lock (useBodyScrollLock side effect)', async () => {
    mountModal(Modal, { props: { modelValue: true }, attachTo: document.body })
    await new Promise(r => setTimeout(r, 0))
    expect(document.body.style.overflow).toBe('hidden')
  })

  it('closeOnEsc=false: escape handler calls preventDefault and does NOT emit close', async () => {
    const w = mountModal(Modal, {
      props: { modelValue: true, closeOnEsc: false },
      attachTo: document.body,
    })
    await new Promise(r => setTimeout(r, 0))
    let prevented = false
    const fakeEvent = { preventDefault: () => { prevented = true } } as Event
    ;(w.vm as unknown as { onEscapeKeyDown: (e: Event) => void }).onEscapeKeyDown(fakeEvent)
    expect(prevented).toBe(true)
    expect(w.emitted('close')).toBeFalsy()
  })

  it('closeOnBackdrop=false: pointer-down-outside handler calls preventDefault', async () => {
    const w = mountModal(Modal, {
      props: { modelValue: true, closeOnBackdrop: false },
      attachTo: document.body,
    })
    await new Promise(r => setTimeout(r, 0))
    let prevented = false
    const fakeEvent = { preventDefault: () => { prevented = true } } as Event
    ;(w.vm as unknown as { onPointerDownOutside: (e: Event) => void }).onPointerDownOutside(fakeEvent)
    expect(prevented).toBe(true)
  })

  it('closeOnEsc=true (default): escape handler does NOT preventDefault (lets Reka close)', async () => {
    const w = mountModal(Modal, { props: { modelValue: true }, attachTo: document.body })
    await new Promise(r => setTimeout(r, 0))
    let prevented = false
    const fakeEvent = { preventDefault: () => { prevented = true } } as Event
    ;(w.vm as unknown as { onEscapeKeyDown: (e: Event) => void }).onEscapeKeyDown(fakeEvent)
    expect(prevented).toBe(false)
  })
})
