/// <reference lib="webworker" />
declare let self: ServiceWorkerGlobalScope & {
  __WB_MANIFEST: Array<import('workbox-precaching').PrecacheEntry | string>
}

import { precacheAndRoute, createHandlerBoundToURL, getCacheKeyForURL } from 'workbox-precaching'
import { NavigationRoute, registerRoute } from 'workbox-routing'
import { clientsClaim } from 'workbox-core'
import { NAV_DENYLIST, edgeAssetToOriginPath, isOfflinePath } from './pwa/swRoutes'
import { handleOfflineRequest } from './pwa/offlineServe'

self.skipWaiting()
clientsClaim()
precacheAndRoute(self.__WB_MANIFEST)

// SPA shell for navigations (denylist: API/OG/proxied admin/etc).
registerRoute(new NavigationRoute(createHandlerBoundToURL('/index.html'), { denylist: NAV_DENYLIST }))

// Downloaded-episode namespace: serves from Cache Storage with MP4 Range support.
registerRoute(
  ({ url }) => isOfflinePath(url.pathname),
  ({ request }) => handleOfflineRequest(request),
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
