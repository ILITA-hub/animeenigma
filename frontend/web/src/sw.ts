/// <reference lib="webworker" />
declare let self: ServiceWorkerGlobalScope & {
  __WB_MANIFEST: Array<import('workbox-precaching').PrecacheEntry | string>
}

import { precacheAndRoute, createHandlerBoundToURL, getCacheKeyForURL } from 'workbox-precaching'
import { NavigationRoute, registerRoute } from 'workbox-routing'
import { clientsClaim } from 'workbox-core'
import { NAV_DENYLIST, edgeAssetToOriginPath, isOfflinePath } from './pwa/swRoutes'

self.skipWaiting()
clientsClaim()
precacheAndRoute(self.__WB_MANIFEST)

// SPA shell for navigations (denylist: API/OG/proxied admin/etc).
registerRoute(new NavigationRoute(createHandlerBoundToURL('/index.html'), { denylist: NAV_DENYLIST }))

// Downloaded-episode namespace. Task 8 replaces this 404 stub with
// handleOfflineRequest (Cache Storage + MP4 Range).
registerRoute(
  ({ url }) => isOfflinePath(url.pathname),
  async () => new Response('offline store not implemented', { status: 404 }),
)

// RU edge chunk fallback: network first, precached origin copy when offline.
registerRoute(
  ({ url }) => edgeAssetToOriginPath(url.href, self.location.origin) !== null,
  async ({ request }) => {
    try {
      return await fetch(request)
    } catch {
      const originUrl = edgeAssetToOriginPath(request.url, self.location.origin)
      const key = originUrl ? getCacheKeyForURL(originUrl) : undefined
      const cached = key ? await caches.match(key) : undefined
      return cached ?? Response.error()
    }
  },
)
