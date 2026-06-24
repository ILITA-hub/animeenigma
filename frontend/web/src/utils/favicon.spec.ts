import { describe, it, expect, beforeEach } from 'vitest'
import { faviconVariantForPath, setFaviconVariant } from './favicon'

describe('faviconVariantForPath', () => {
  it('uses the admin variant for /admin and nested admin routes', () => {
    expect(faviconVariantForPath('/admin')).toBe('admin')
    expect(faviconVariantForPath('/admin/')).toBe('admin')
    expect(faviconVariantForPath('/admin/feedback')).toBe('admin')
    expect(faviconVariantForPath('/admin/recs/123')).toBe('admin')
  })

  it('uses the default variant for the main site', () => {
    expect(faviconVariantForPath('/')).toBe('default')
    expect(faviconVariantForPath('/anime/abc')).toBe('default')
    // must not match a non-admin path that merely starts with "admin"
    expect(faviconVariantForPath('/administrative')).toBe('default')
  })
})

describe('setFaviconVariant', () => {
  beforeEach(() => {
    document.head.innerHTML = `
      <link id="favicon-ico" rel="icon" href="/favicon.ico">
      <link id="favicon-32" rel="icon" href="/favicon-32x32.png">
      <link id="favicon-16" rel="icon" href="/favicon-16x16.png">
    `
    // reset module-level applied state by toggling through both variants
    setFaviconVariant('admin')
    setFaviconVariant('default')
  })

  it('swaps every tab-icon link to the admin (cat) hrefs', () => {
    setFaviconVariant('admin')
    const ico = document.getElementById('favicon-ico') as HTMLLinkElement
    const png32 = document.getElementById('favicon-32') as HTMLLinkElement
    expect(ico.getAttribute('href')).toBe('/favicon-admin.ico')
    expect(png32.getAttribute('href')).toBe('/favicon-admin-32x32.png')
  })

  it('restores the brand-mark hrefs for the default variant', () => {
    setFaviconVariant('admin')
    setFaviconVariant('default')
    const ico = document.getElementById('favicon-ico') as HTMLLinkElement
    expect(ico.getAttribute('href')).toBe('/favicon.ico')
  })
})
