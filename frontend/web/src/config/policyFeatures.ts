/**
 * RBAC-and-roulette policy admin UI (Task 6) — static registry mapping a
 * policy feature key (services/policy `FeatureFlag.key`) to the same-origin
 * relative route that surfaces it in the app. The admin policy view
 * (`AdminPolicy.vue`, Task 7) uses this to render an "open" affordance next
 * to each feature row; a key absent from the map renders no link.
 *
 * Keep in sync with src/router/index.ts. Deliberately NOT derived from the
 * router at runtime — the policy-service feature keys are a small, curated
 * set and a static map keeps this file trivially auditable in review.
 */
export const POLICY_FEATURE_ROUTES: Record<string, string> = {
  fanfic: '/fanfics',
  gacha: '/gacha',
  'profile-wall': '/profile',
  anidle: '/anidle',
  'showcase-editor': '/profile#showcase',
  themes: '/themes',
  status: '/status',
  game: '/game',
  downloads: '/downloads',
  'my-feedback': '/my-feedback',
  following: '/following',
}

/** Returns the relative route for a feature key, or undefined when the key
 *  has no in-app surface to link to. */
export function featureRoute(key: string): string | undefined {
  return POLICY_FEATURE_ROUTES[key]
}
