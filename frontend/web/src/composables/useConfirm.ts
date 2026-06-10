import { ref, readonly, type DeepReadonly, type Ref } from 'vue'

/**
 * useConfirm — promise-based replacement for the native `window.confirm()`.
 *
 * State is module-level so every call site shares ONE active dialog (mirrors
 * useToast). The global <ConfirmDialogHost> (mounted in App.vue) renders a
 * <ConfirmDialog> bound to this state and wires its events back to accept() /
 * cancel().
 *
 * Usage:
 *   const { confirm } = useConfirm()
 *   if (!(await confirm({
 *     title: t('profile.advanced.resetTitle'),
 *     description: t('profile.advanced.resetConfirm'),
 *     confirmText: t('common.reset'),
 *     cancelText: t('common.cancel'),
 *     variant: 'destructive',
 *   }))) return
 *   // ...proceed with the destructive action
 *
 * The returned promise resolves `true` when the user confirms and `false` when
 * they cancel / dismiss (Esc, backdrop) — exactly like native confirm().
 */

export interface ConfirmOptions {
  title?: string
  description?: string
  confirmText?: string
  cancelText?: string
  variant?: 'default' | 'destructive'
}

export interface ConfirmState extends ConfirmOptions {
  open: boolean
}

const state = ref<ConfirmState>({ open: false })
let resolver: ((value: boolean) => void) | null = null

function settle(result: boolean): void {
  if (resolver) {
    resolver(result)
    resolver = null
  }
  state.value = { ...state.value, open: false }
}

function confirm(options: ConfirmOptions = {}): Promise<boolean> {
  // If a dialog is already open, the new request supersedes it; resolve the
  // stale one as cancelled so its awaiter never hangs.
  if (resolver) {
    resolver(false)
    resolver = null
  }
  state.value = { ...options, open: true }
  return new Promise<boolean>((resolve) => {
    resolver = resolve
  })
}

export function useConfirm(): {
  state: DeepReadonly<Ref<ConfirmState>>
  confirm: (options?: ConfirmOptions) => Promise<boolean>
  accept: () => void
  cancel: () => void
} {
  return {
    state: readonly(state),
    confirm,
    accept: () => settle(true),
    cancel: () => settle(false),
  }
}
