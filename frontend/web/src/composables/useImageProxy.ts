import { ref } from 'vue'

const STORAGE_KEY_BLOCKED = 'shikimori_blocked'
const STORAGE_KEY_FAILED_URLS = 'shikimori_failed_urls'
const FAILURE_THRESHOLD = 20

const SHIKIMORI_DOMAINS = ['shiki.one', 'shikimori.io', 'shikimori.one']

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

export function useImageProxy(originalUrl: string | undefined | null) {
  const src = ref(getImageUrl(originalUrl))
  const hasFallback = ref(false)

  function onError() {
    if (hasFallback.value || !originalUrl) return
    hasFallback.value = true
    src.value = getImageFallbackUrl(originalUrl)
  }

  return { imageSrc: src, onError }
}
