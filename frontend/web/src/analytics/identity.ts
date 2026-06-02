// Analytics identity: anonymous id (reuses the app-wide anon id used for the
// X-Anon-ID header) + an optional resolved user id persisted across reloads.
import { getOrCreateAnonId, resetAnonId } from '@/utils/anonId'

const UID_KEY = 'aenig_analytics_uid'

export function getAnonId(): string {
  return getOrCreateAnonId()
}

export function resetAnon(): string {
  return resetAnonId()
}

export function getUserId(): string | null {
  try {
    return localStorage.getItem(UID_KEY)
  } catch {
    return null
  }
}

export function setUserId(id: string): void {
  try {
    localStorage.setItem(UID_KEY, id)
  } catch {
    // ignore
  }
}

export function clearUserId(): void {
  try {
    localStorage.removeItem(UID_KEY)
  } catch {
    // ignore
  }
}
