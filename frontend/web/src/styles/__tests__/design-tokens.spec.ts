import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

const css = readFileSync(join(__dirname, '../main.css'), 'utf8')

describe('canonical tokens declared', () => {
  it.each(['--background', '--foreground', '--primary', '--primary-foreground',
    '--secondary', '--muted', '--muted-foreground', '--border', '--input', '--ring',
    '--brand-cyan', '--brand-pink', '--info', '--destructive', '--success-soft'])(
    'declares %s', (t) => {
      expect(css).toMatch(new RegExp(`\\${t}\\s*:`))
    })
})

describe('deprecated tokens are aliased to canonical ones (value-preserving)', () => {
  it('--pink aliases --brand-pink', () => {
    expect(css).toMatch(/--pink:\s*var\(--brand-pink\)/)
  })
  it('--ink aliases --foreground', () => {
    expect(css).toMatch(/--ink:\s*var\(--foreground\)/)
  })
  it('--ink-3 aliases --muted-foreground', () => {
    expect(css).toMatch(/--ink-3:\s*var\(--muted-foreground\)/)
  })
  it('--accent resolves to the shadcn hover surface (flipped from brand-cyan in P5/05-04)', () => {
    // Plan 05-04 (DS-MIGRATE-05) flipped --accent from its temporary brand-cyan
    // alias to the neutral shadcn hover surface (var(--elevated)); the temp alias
    // was deleted. var(--brand-cyan) is no longer the correct target.
    expect(css).toMatch(/--accent:\s*var\(--elevated\)/)
  })
})

// The .btn-* re-points are the highest-regression-risk edit (a wrong target
// token would silently shift a rendered color). Pin them so a future
// accidental re-point to a non-equivalent token is caught. [^}] matches
// across newlines, so these span the multi-line rule body.
describe('.btn-* classes reference canonical tokens (value-preserving)', () => {
  it('.btn-primary background uses --primary', () => {
    expect(css).toMatch(/\.btn-primary\s*\{[^}]*background-color:\s*var\(--primary\)/)
  })
  it('.btn-primary text uses --primary-foreground', () => {
    expect(css).toMatch(/\.btn-primary\s*\{[^}]*color:\s*var\(--primary-foreground\)/)
  })
  it('.btn-secondary background uses --brand-pink', () => {
    expect(css).toMatch(/\.btn-secondary\s*\{[^}]*background-color:\s*var\(--brand-pink\)/)
  })
  it('.btn-secondary text uses --brand-pink-foreground', () => {
    expect(css).toMatch(/\.btn-secondary\s*\{[^}]*color:\s*var\(--brand-pink-foreground\)/)
  })
  it('.btn:focus-visible ring uses --ring', () => {
    expect(css).toMatch(/\.btn:focus-visible\s*\{[^}]*var\(--ring\)/)
  })
})
