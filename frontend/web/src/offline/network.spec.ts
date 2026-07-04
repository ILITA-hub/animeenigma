// src/offline/network.spec.ts
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { isCellular, allowCellularThisSession, setAllowCellularThisSession, onConnectionChange, _resetNetworkForTests } from './network'

function stubConnection(value: unknown) {
  Object.defineProperty(navigator, 'connection', { value, configurable: true })
}

describe('offline/network', () => {
  beforeEach(() => _resetNetworkForTests())
  afterEach(() => stubConnection(undefined))

  it('unknown/absent connection type is NOT cellular', () => {
    stubConnection(undefined)
    expect(isCellular()).toBe(false)
    stubConnection({}) // API present, type undefined (desktop Chrome)
    expect(isCellular()).toBe(false)
    stubConnection({ type: 'wifi' })
    expect(isCellular()).toBe(false)
  })

  it('type === cellular is cellular', () => {
    stubConnection({ type: 'cellular' })
    expect(isCellular()).toBe(true)
  })

  it('session override defaults off, sticks after set', () => {
    expect(allowCellularThisSession()).toBe(false)
    setAllowCellularThisSession(true)
    expect(allowCellularThisSession()).toBe(true)
  })

  it('onConnectionChange subscribes when API present, no-ops when absent', () => {
    const add = vi.fn(); const remove = vi.fn()
    stubConnection({ type: 'wifi', addEventListener: add, removeEventListener: remove })
    const off = onConnectionChange(() => {})
    expect(add).toHaveBeenCalledWith('change', expect.any(Function))
    off()
    expect(remove).toHaveBeenCalled()
    stubConnection(undefined)
    expect(() => onConnectionChange(() => {})()).not.toThrow()
  })
})
