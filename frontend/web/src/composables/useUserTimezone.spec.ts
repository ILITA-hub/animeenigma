import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUserTimezone } from './useUserTimezone'
import { useAuthStore } from '@/stores/auth'
import { browserTimezone } from './useTimezonePref'

describe('useUserTimezone', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('falls back to the browser zone when logged out', () => {
    const { timezone } = useUserTimezone()
    expect(timezone.value).toBe(browserTimezone)
  })

  it('uses the account timezone when set', () => {
    const auth = useAuthStore()
    auth.setUser({ id: '1', username: 'u', email: '', role: 'user', timezone: 'Asia/Tokyo' })
    const { timezone } = useUserTimezone()
    expect(timezone.value).toBe('Asia/Tokyo')
  })

  it('ignores an invalid stored zone', () => {
    const auth = useAuthStore()
    auth.setUser({ id: '1', username: 'u', email: '', role: 'user', timezone: 'Not/AZone' })
    const { timezone } = useUserTimezone()
    expect(timezone.value).toBe(browserTimezone)
  })

  it('does not throw without an active Pinia', () => {
    setActivePinia(undefined as never)
    const { timezone } = useUserTimezone()
    expect(timezone.value).toBe(browserTimezone)
  })
})
