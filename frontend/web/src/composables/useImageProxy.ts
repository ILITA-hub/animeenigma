import { ref } from 'vue'

const STORAGE_KEY_BLOCKED = 'shikimori_blocked'
const STORAGE_KEY_FAILURES = 'shikimori_failures'
const FAILURE_THRESHOLD = 3

const SHIKIMORI_DOMAINS = ['shiki.one', 'shikimori.io', 'shikimori.one']

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

function incrementFailures(): void {
  try {
    const count = parseInt(sessionStorage.getItem(STORAGE_KEY_FAILURES) || '0', 10) + 1
    sessionStorage.setItem(STORAGE_KEY_FAILURES, String(count))
    if (count >= FAILURE_THRESHOLD) {
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
  if (!originalUrl) return ''
  if (!isShikimoriUrl(originalUrl)) return originalUrl
  if (isBlocked()) return proxyUrl(originalUrl)
  return originalUrl
}

export function getImageFallbackUrl(originalUrl: string): string {
  if (isShikimoriUrl(originalUrl)) {
    incrementFailures()
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
