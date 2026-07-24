import { ref, computed, watch, toValue, type MaybeRefOrGetter } from 'vue'

const STORAGE_KEY_BLOCKED = 'shikimori_blocked'
const STORAGE_KEY_FAILED_URLS = 'shikimori_failed_urls'
const FAILURE_THRESHOLD = 20

const SHIKIMORI_DOMAINS = ['shiki.one', 'shikimori.io', 'shikimori.one']

// Domains the backend image-proxy accepts (mirror of allowedDomains in
// services/streaming/internal/service/image_proxy.go)
const PROXYABLE_DOMAINS = [...SHIKIMORI_DOMAINS, 'cdn.myanimelist.net', 'myanimelist.net']

// Allowed resize widths — a small fixed set so the MinIO cache holds at most
// N variants per poster instead of one per device width.
const PROXY_WIDTHS = [128, 256, 384, 512, 640]

// In-memory set of URLs already counted this session (survives SPA navigation, not reloads)
// sessionStorage set tracks across reloads — same URL won't be double-counted
const failedUrlsMemory = new Set<string>()

function isShikimoriUrl(url: string): boolean {
  try {
    const hostname = new URL(url).hostname.toLowerCase()
    return SHIKIMORI_DOMAINS.some(d => hostname === d || hostname.endsWith('.' + d))
  } catch {
    return false
  }
}

function isBlocked(): boolean {
  try {
    return sessionStorage.getItem(STORAGE_KEY_BLOCKED) === 'true'
  } catch {
    return false
  }
}

function trackFailure(url: string): void {
  try {
    // Skip if this URL was already counted
    if (failedUrlsMemory.has(url)) return

    // Check sessionStorage set (survives reloads)
    const storedJson = sessionStorage.getItem(STORAGE_KEY_FAILED_URLS) || '[]'
    const failedUrls: string[] = JSON.parse(storedJson)
    if (failedUrls.includes(url)) {
      failedUrlsMemory.add(url) // sync to memory
      return
    }

    // New failure — track it
    failedUrlsMemory.add(url)
    failedUrls.push(url)
    sessionStorage.setItem(STORAGE_KEY_FAILED_URLS, JSON.stringify(failedUrls))

    if (failedUrls.length >= FAILURE_THRESHOLD) {
      sessionStorage.setItem(STORAGE_KEY_BLOCKED, 'true')
    }
  } catch {
    // sessionStorage may be unavailable (private browsing, etc.)
  }
}

function proxyUrl(originalUrl: string): string {
  return `/api/streaming/image-proxy?url=${encodeURIComponent(originalUrl)}`
}

function isProxyableUrl(url: string): boolean {
  if (url.startsWith('/api/gacha/images/')) return true
  try {
    const hostname = new URL(url).hostname.toLowerCase()
    return PROXYABLE_DOMAINS.some(d => hostname === d || hostname.endsWith('.' + d))
  } catch {
    return false
  }
}

/**
 * Resized poster URL for card grids/rows. Routes proxyable upstream posters
 * through the backend image-proxy with a `w` (width) param so a 56px row
 * thumbnail downloads ~5-20KB instead of the 300-530KB original. `width`
 * should be the largest CSS pixel width × 2 (retina); it is snapped up to
 * the nearest allowed bucket. Non-proxyable URLs pass through unchanged.
 */
export function cardPosterUrl(originalUrl: string | undefined | null, width: number): string {
  if (!originalUrl) return '/placeholder.svg'
  if (!isProxyableUrl(originalUrl)) return originalUrl
  const w = PROXY_WIDTHS.find(x => x >= width) ?? PROXY_WIDTHS[PROXY_WIDTHS.length - 1]
  return `${proxyUrl(originalUrl)}&w=${w}`
}

export function getImageUrl(originalUrl: string | undefined | null): string {
  if (!originalUrl) return '/placeholder.svg'
  if (!isShikimoriUrl(originalUrl)) return originalUrl
  if (isBlocked()) return proxyUrl(originalUrl)
  return originalUrl
}

export function getImageFallbackUrl(originalUrl: string): string {
  if (isShikimoriUrl(originalUrl)) {
    trackFailure(originalUrl)
  }
  return proxyUrl(originalUrl)
}

export function useImageProxy(originalUrl: MaybeRefOrGetter<string | undefined | null>) {
  const hasFallback = ref(false)

  // New source ⇒ fresh fallback state (route components are reused across
  // navigations, so the url can change while the consumer stays mounted).
  watch(() => toValue(originalUrl), () => {
    hasFallback.value = false
  })

  const imageSrc = computed(() => {
    const url = toValue(originalUrl)
    return hasFallback.value && url ? getImageFallbackUrl(url) : getImageUrl(url)
  })

  function onError() {
    if (hasFallback.value || !toValue(originalUrl)) return
    hasFallback.value = true
  }

  return { imageSrc, onError }
}
