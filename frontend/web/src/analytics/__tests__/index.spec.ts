import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

describe('analytics singleton', () => {
  let beacon: ReturnType<typeof vi.fn>

  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    beacon = vi.fn().mockReturnValue(true)
    // @ts-expect-error jsdom lacks sendBeacon
    navigator.sendBeacon = beacon
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('does not send before init', async () => {
    const { analytics } = await import('../index')
    analytics.page()
    analytics.track('foo')
    analytics.flushNow()
    expect(beacon).not.toHaveBeenCalled()
  })

  it('page() enqueues a pageview that flushes', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/api/analytics/collect', flushMs: 999999 })
    analytics.page()
    analytics.flushNow()
    expect(beacon).toHaveBeenCalledTimes(1)
    const env = JSON.parse(await (beacon.mock.calls[0][1] as Blob).text())
    expect(env.events[0].event_type).toBe('pageview')
  })

  it('identify sets user_id on subsequent events; reset clears it', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    analytics.identify('u1')
    analytics.track('after_login')
    analytics.flushNow()
    let env = JSON.parse(await (beacon.mock.calls.at(-1)![1] as Blob).text())
    expect(env.user_id).toBe('u1')

    analytics.reset()
    analytics.track('after_logout')
    analytics.flushNow()
    env = JSON.parse(await (beacon.mock.calls.at(-1)![1] as Blob).text())
    expect(env.user_id).toBeNull()
  })

  it('identify is deduped — same user id emits only one identify event', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    analytics.identify('u1')
    analytics.identify('u1') // duplicate — should NOT emit a second identify
    analytics.flushNow()
    const env = JSON.parse(await (beacon.mock.calls.at(-1)![1] as Blob).text())
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
    const env = JSON.parse(await (beacon.mock.calls.at(-1)![1] as Blob).text())
    const click = env.events.find((e: { event_type: string }) => e.event_type === 'click')
    expect(click).toBeTruthy()
    expect(click.el_selector).toContain('button#buy')
    document.body.removeChild(btn)
  })
})
