// Route-conditional tab favicon.
//
// The main site shows the Neon-Tokyo brand-mark (the "AE" gradient square,
// matching the header). The legacy «APPROVED» cat seal is retired to the
// admin panel only — so /admin/* pages swap the tab icon to favicon-admin.*.
//
// The default (brand-mark) hrefs ship statically in index.html; this only
// overrides them when entering/leaving admin. apple-touch-icon (installed /
// home-screen app identity) is intentionally NOT swapped — it always reflects
// the main app.

export type FaviconVariant = 'default' | 'admin'

// link element id -> { default href, admin href }
const LINKS: Record<string, Record<FaviconVariant, string>> = {
  'favicon-ico': { default: '/favicon.ico', admin: '/favicon-admin.ico' },
  'favicon-32': { default: '/favicon-32x32.png', admin: '/favicon-admin-32x32.png' },
  'favicon-16': { default: '/favicon-16x16.png', admin: '/favicon-admin-16x16.png' },
}

let applied: FaviconVariant | null = null

/** Which favicon a given route path should use. */
export function faviconVariantForPath(path: string): FaviconVariant {
  return path === '/admin' || path.startsWith('/admin/') ? 'admin' : 'default'
}

/** Point the tab-icon <link>s at the chosen variant (no-op if unchanged). */
export function setFaviconVariant(variant: FaviconVariant): void {
  if (variant === applied || typeof document === 'undefined') return
  applied = variant
  for (const [id, hrefs] of Object.entries(LINKS)) {
    const el = document.getElementById(id) as HTMLLinkElement | null
    if (el) el.href = hrefs[variant]
  }
}
