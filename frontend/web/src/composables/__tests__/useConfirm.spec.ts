/**
 * Spec for useConfirm — the promise-based confirm() API.
 *
 * State is module-level, so each test must settle any open dialog (cancel())
 * to avoid leaking a pending resolver into the next test.
 */

import { describe, it, expect, afterEach } from 'vitest'
import { useConfirm } from '../useConfirm'

const { state, confirm, accept, cancel } = useConfirm()

afterEach(() => {
  // Reset shared state between tests.
  if (state.value.open) cancel()
})

describe('useConfirm', () => {
  it('starts closed', () => {
    expect(state.value.open).toBe(false)
  })

  it('confirm() opens the dialog and stores options', () => {
    const p = confirm({ title: 'T', description: 'D', variant: 'destructive' })
    expect(state.value.open).toBe(true)
    expect(state.value.title).toBe('T')
    expect(state.value.description).toBe('D')
    expect(state.value.variant).toBe('destructive')
    cancel()
    return p // settle to avoid unhandled pending promise
  })

  it('accept() resolves the promise true and closes', async () => {
    const p = confirm({ title: 'X' })
    accept()
    await expect(p).resolves.toBe(true)
    expect(state.value.open).toBe(false)
  })

  it('cancel() resolves the promise false and closes', async () => {
    const p = confirm({ title: 'X' })
    cancel()
    await expect(p).resolves.toBe(false)
    expect(state.value.open).toBe(false)
  })

  it('a second confirm() supersedes the first, resolving it false', async () => {
    const first = confirm({ title: 'first' })
    const second = confirm({ title: 'second' })
    expect(state.value.title).toBe('second')
    await expect(first).resolves.toBe(false)
    accept()
    await expect(second).resolves.toBe(true)
  })

  it('confirm() works with no options (all fields undefined, open true)', () => {
    const p = confirm()
    expect(state.value.open).toBe(true)
    expect(state.value.title).toBeUndefined()
    cancel()
    return p
  })
})
