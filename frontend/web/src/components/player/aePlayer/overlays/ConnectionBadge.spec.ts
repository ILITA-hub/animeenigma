import { describe, it, expect } from 'vitest'
import { mount, config } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ConnectionBadge from './ConnectionBadge.vue'

// Real i18n so the tooltip/aria text resolves to en.json copy.
const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })
config.global.plugins = [...(config.global.plugins ?? []), i18n]

describe('ConnectionBadge', () => {
  it('renders nothing when the connection is ok', () => {
    const w = mount(ConnectionBadge, { props: { state: 'ok' } })
    expect(w.find('[data-test="connection-badge"]').exists()).toBe(false)
  })

  it('shows the offline (slashed) glyph and label when offline', () => {
    const w = mount(ConnectionBadge, { props: { state: 'offline' } })
    const badge = w.find('[data-test="connection-badge"]')
    expect(badge.exists()).toBe(true)
    expect(badge.attributes('aria-label')).toBe(en.player.aePlayer.connectionOffline)
    // WifiOff renders; the slow-only exclamation mark does not.
    expect(w.find('.pl-conn-bang').exists()).toBe(false)
  })

  it('shows the wifi + exclamation glyph and slow label when slow', () => {
    const w = mount(ConnectionBadge, { props: { state: 'slow' } })
    const badge = w.find('[data-test="connection-badge"]')
    expect(badge.exists()).toBe(true)
    expect(badge.attributes('aria-label')).toBe(en.player.aePlayer.connectionSlow)
    expect(w.find('.pl-conn-bang').text()).toBe('!')
  })

  it('exposes the state as a modifier class and a polite live region', () => {
    const w = mount(ConnectionBadge, { props: { state: 'slow' } })
    const badge = w.find('[data-test="connection-badge"]')
    expect(badge.classes()).toContain('pl-conn--slow')
    expect(badge.attributes('role')).toBe('status')
    expect(badge.attributes('aria-live')).toBe('polite')
  })
})
