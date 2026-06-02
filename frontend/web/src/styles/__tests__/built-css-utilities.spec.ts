import { describe, it, expect, beforeAll } from 'vitest'
import { execSync } from 'node:child_process'
import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'

// Builds the app once, then asserts the new semantic utilities are emitted.
// Vite may split CSS into multiple hashed chunks under dist/assets; the Tailwind
// utility layer can land in any of them, so we concatenate ALL .css files.
const DIST = join(__dirname, '../../../dist/assets')

describe('canonical token utilities are generated', () => {
  let css = ''
  beforeAll(() => {
    execSync('bun run build', { cwd: join(__dirname, '../../..'), stdio: 'ignore' })
    const cssFiles = readdirSync(DIST).filter((f) => f.endsWith('.css'))
    if (cssFiles.length === 0) throw new Error('no built CSS found in dist/assets')
    css = cssFiles.map((f) => readFileSync(join(DIST, f), 'utf8')).join('\n')
  }, 120_000)

  it.each([
    'bg-primary', 'text-primary-foreground', 'bg-secondary',
    'bg-background', 'text-foreground', 'bg-card', 'bg-popover',
    'bg-muted', 'text-muted-foreground', 'border-border',
    'bg-destructive', 'bg-success-soft', 'bg-brand-pink', 'ring-ring',
  ])('emits .%s', (util) => {
    expect(css).toContain(`.${util}`)
  })
})
