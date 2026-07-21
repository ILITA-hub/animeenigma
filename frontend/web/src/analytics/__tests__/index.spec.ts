import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

describe('analytics singleton', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  function lastEnvelope() {
    const init = fetchMock.mock.calls.at(-1)![1] as RequestInit
    return JSON.parse(init.body as string)
  }

  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('does not send before init', async () => {
    const { analytics } = await import('../index')
    analytics.page()
    analytics.track('foo')
    analytics.flushNow()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('page() enqueues a pageview that flushes', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/api/analytics/collect', flushMs: 999999 })
    analytics.page()
    analytics.flushNow()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const env = lastEnvelope()
    expect(env.events[0].event_type).toBe('pageview')
  })

  it('identify sets user_id on subsequent events; reset clears it', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    analytics.identify('u1')
    analytics.track('after_login')
    analytics.flushNow()
    let env = lastEnvelope()
    expect(env.user_id).toBe('u1')

    analytics.reset()
    analytics.track('after_logout')
    analytics.flushNow()
    env = lastEnvelope()
    expect(env.user_id).toBeNull()
  })

  it('identify is deduped — same user id emits only one identify event', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    analytics.identify('u1')
    analytics.identify('u1') // duplicate — should NOT emit a second identify
    analytics.flushNow()
    const env = lastEnvelope()
    const identifies = env.events.filter((e: { event_type: string }) => e.event_type === 'identify')
    expect(identifies).toHaveLength(1)
  })

  it('a click on the document is autocaptured after init', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    const btn = document.createElement('button')
    btn.id = 'buy'
    document.body.appendChild(btn)
    btn.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    analytics.flushNow()
    const env = lastEnvelope()
    const click = env.events.find((e: { event_type: string }) => e.event_type === 'click')
    expect(click).toBeTruthy()
    expect(click.el_selector).toContain('button#buy')
    document.body.removeChild(btn)
  })
})
