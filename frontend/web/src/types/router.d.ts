// Typed route meta — makes the layout/auth contracts on route definitions
// typo-proof (an unaugmented RouteMeta accepts any key, so a misspelled
// `fullBleed` would compile clean and silently get the default offset).
import 'vue-router'

declare module 'vue-router' {
  interface RouteMeta {
    /** i18n key for the document title, applied by the afterEach hook. */
    titleKey?: string
    /** Route requires an authenticated user (router guard). */
    requiresAuth?: boolean
    /** Route requires an admin user (router guard). */
    requiresAdmin?: boolean
    /**
     * Raw-library operator surface: admin OR librarian role (router guard;
     * mirrors the gateway's LibraryRoleMiddleware on /api/library/*).
     */
    requiresLibraryAccess?: boolean
    /**
     * Page design runs behind the transparent fixed header (full-bleed
     * hero) — App.vue's <main> skips the default --header-offset padding
     * and the page owns its own top clearance.
     */
    fullBleed?: boolean
  }
}
