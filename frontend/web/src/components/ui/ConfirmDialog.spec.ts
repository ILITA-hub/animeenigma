/**
 * Spec for ConfirmDialog.vue.
 *
 * ConfirmDialog wraps Modal (Reka Dialog), whose content is portaled and
 * focus-trapped — jsdom can't fully render it. So, like Modal.spec, we drive
 * the exposed handlers directly and assert the emitted events + prop defaults,
 * rather than clicking portaled DOM.
 */

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ConfirmDialog from './ConfirmDialog.vue'

// Stub the heavy Modal child: we only need ConfirmDialog's own logic + props,
// not Reka's portal/focus-trap behaviour (covered by Modal.spec).
const ModalStub = {
  name: 'Modal',
  props: ['modelValue', 'title', 'size', 'closable'],
  emits: ['update:modelValue'],
  template: '<div class="modal-stub"><slot /><slot name="footer" /></div>',
}

function mountDialog(props: Record<string, unknown> = {}) {
  return mount(ConfirmDialog, {
    props: { open: true, ...props },
    global: {
      stubs: { Modal: ModalStub, Button: { template: '<button><slot /></button>' } },
    },
  })
}

describe('ConfirmDialog', () => {
  it('renders the description text when provided', () => {
    const wrapper = mountDialog({ description: 'Delete this forever?' })
    expect(wrapper.text()).toContain('Delete this forever?')
  })

  it('renders default confirm/cancel labels', () => {
    const wrapper = mountDialog()
    expect(wrapper.text()).toContain('Confirm')
    expect(wrapper.text()).toContain('Cancel')
  })

  it('honors custom confirm/cancel labels', () => {
    const wrapper = mountDialog({ confirmText: 'Reset', cancelText: 'Keep' })
    expect(wrapper.text()).toContain('Reset')
    expect(wrapper.text()).toContain('Keep')
  })

  it('onConfirm emits confirm + closes (update:open false)', () => {
    const wrapper = mountDialog()
    ;(wrapper.vm as unknown as { onConfirm: () => void }).onConfirm()
    expect(wrapper.emitted('confirm')).toHaveLength(1)
    expect(wrapper.emitted('update:open')?.[0]).toEqual([false])
  })

  it('onCancel emits cancel + closes (update:open false)', () => {
    const wrapper = mountDialog()
    ;(wrapper.vm as unknown as { onCancel: () => void }).onCancel()
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    expect(wrapper.emitted('update:open')?.[0]).toEqual([false])
  })

  it('Esc/backdrop close (update:model-value false) resolves as cancel', () => {
    const wrapper = mountDialog()
    ;(wrapper.vm as unknown as { onOpenUpdate: (v: boolean) => void }).onOpenUpdate(false)
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    expect(wrapper.emitted('confirm')).toBeUndefined()
  })

  it('does NOT emit cancel when Modal opens (update:model-value true)', () => {
    const wrapper = mountDialog()
    ;(wrapper.vm as unknown as { onOpenUpdate: (v: boolean) => void }).onOpenUpdate(true)
    expect(wrapper.emitted('cancel')).toBeUndefined()
  })
})
