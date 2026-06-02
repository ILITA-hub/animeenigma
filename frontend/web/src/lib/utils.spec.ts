import { describe, it, expect } from 'vitest'
import { cn } from './utils'

describe('cn()', () => {
  it('concatenates string class lists', () => {
    expect(cn('a', 'b')).toBe('a b')
  })

  it('drops falsy values (clsx)', () => {
    expect(cn('px-2', false && 'hidden', 'py-1')).toBe('px-2 py-1')
  })

  it('de-dupes conflicting Tailwind utilities (last wins)', () => {
    expect(cn('bg-red-500', 'bg-blue-500')).toBe('bg-blue-500')
  })

  it('merges custom color tokens in the bg group (last wins) — guards A1', () => {
    expect(cn('bg-primary', 'bg-brand-pink')).toBe('bg-brand-pink')
  })

  it('accepts array input, last conflicting wins', () => {
    expect(cn(['p-2', 'p-4'])).toBe('p-4')
  })
})
