import { describe, it, expect } from 'vitest'
import { mount, config } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import EpisodesPanel from './EpisodesPanel.vue'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

// Real i18n so $t() resolves to en.json text (the assertions below expect the
// English strings, not raw keys) and {n} interpolation works.
const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })
// Append (don't clobber) in case a shared setup ever registers a global plugin.
config.global.plugins = [...(config.global.plugins ?? []), i18n]

const eps: EpisodeOption[] = [
  { key: 1, label: 1, number: 1 },
  { key: 2, label: 2, number: 2 },
  { key: 3, label: 3, number: 3, isFiller: true },
]

describe('EpisodesPanel', () => {
  it('renders one button per episode + a count', () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    expect(w.findAll('[data-test^="episode-"]').length).toBe(3)
    expect(w.text()).toContain('3')
  })

  it('highlights the selected episode', () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 2 } })
    expect(w.find('[data-test="episode-2"]').classes().join(' ')).toContain('brand-cyan')
    expect(w.find('[data-test="episode-1"]').classes().join(' ')).not.toContain('brand-cyan')
  })

  it('emits select with the episode option on click', async () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    await w.find('[data-test="episode-2"]').trigger('click')
    expect(w.emitted('select')?.[0]).toEqual([eps[1]])
  })

  it('dims filler episodes', () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    expect(w.find('[data-test="episode-3"]').classes()).toContain('opacity-50')
  })

  it('shows an empty state when there are no episodes', () => {
    const w = mount(EpisodesPanel, { props: { episodes: [], selectedNumber: null } })
    expect(w.text()).toContain('No episodes from this source')
  })

  it('marks episodes <= watchedUpTo as watched (check icon)', () => {
    const w = mount(EpisodesPanel, {
      props: { episodes: eps, selectedNumber: 3, watchedUpTo: 2 },
    })
    expect(w.find('[data-test="episode-1"] [data-test="ep-watched"]').exists()).toBe(true)
    expect(w.find('[data-test="episode-2"] [data-test="ep-watched"]').exists()).toBe(true)
    expect(w.find('[data-test="episode-3"] [data-test="ep-watched"]').exists()).toBe(false)
  })

  it('marks completed-by-progress episodes as watched too', () => {
    const w = mount(EpisodesPanel, {
      props: {
        episodes: eps,
        selectedNumber: 1,
        progress: { 3: { pct: 1, completed: true } },
      },
    })
    expect(w.find('[data-test="episode-3"] [data-test="ep-watched"]').exists()).toBe(true)
  })

  it('renders a partial-progress underline sized by pct', () => {
    const w = mount(EpisodesPanel, {
      props: {
        episodes: eps,
        selectedNumber: 1,
        progress: { 2: { pct: 0.4, completed: false } },
      },
    })
    const bar = w.find('[data-test="episode-2"] [data-test="ep-progress"]')
    expect(bar.exists()).toBe(true)
    expect(bar.attributes('style')).toContain('40%')
    expect(w.find('[data-test="episode-1"] [data-test="ep-progress"]').exists()).toBe(false)
  })

  it('shows the mark-as-watched action only for logged-in users (canMark)', () => {
    const anon = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    expect(anon.find('[data-test="mark-watched"]').exists()).toBe(false)
    const authed = mount(EpisodesPanel, {
      props: { episodes: eps, selectedNumber: 1, canMark: true },
    })
    expect(authed.find('[data-test="mark-watched"]').exists()).toBe(true)
    expect(authed.find('[data-test="mark-watched"]').text()).toContain('Mark ep. 1 as watched')
  })

  it('emits mark-watched on click and disables when already marked', async () => {
    const w = mount(EpisodesPanel, {
      props: { episodes: eps, selectedNumber: 2, canMark: true },
    })
    await w.find('[data-test="mark-watched"]').trigger('click')
    expect(w.emitted('mark-watched')).toHaveLength(1)

    const done = mount(EpisodesPanel, {
      props: { episodes: eps, selectedNumber: 2, canMark: true, marked: true },
    })
    const btn = done.find('[data-test="mark-watched"]')
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.text()).toContain('Ep. 2 watched')
  })

  it('exposes the episode title as a tooltip', () => {
    const w = mount(EpisodesPanel, {
      props: {
        episodes: [{ key: 1, label: 1, number: 1, title: 'The Journey Begins' }],
        selectedNumber: null,
      },
    })
    expect(w.find('[data-test="episode-1"]').attributes('title')).toBe('1. The Journey Begins')
  })

  // ── V2b bottom-sheet behaviors ────────────────────────────────────────────

  const manyEps = (n: number): EpisodeOption[] =>
    Array.from({ length: n }, (_, i) => ({
      key: i + 1,
      label: i + 1,
      number: i + 1,
      title: `Episode ${i + 1}`,
    }))

  it('shows the episode title text in the strip card (not only a tooltip)', () => {
    const w = mount(EpisodesPanel, {
      props: {
        episodes: [{ key: 1, label: 1, number: 1, title: 'The Journey Begins' }],
        selectedNumber: 1,
      },
    })
    expect(w.find('[data-test="episode-1"]').text()).toContain('The Journey Begins')
  })

  it('adaptive: <=15 eps — no jump input, no grid toggle (strip only)', () => {
    const w = mount(EpisodesPanel, { props: { episodes: manyEps(12), selectedNumber: 1 } })
    expect(w.find('[data-test="ep-strip"]').exists()).toBe(true)
    expect(w.find('[data-test="jump-input"]').exists()).toBe(false)
    expect(w.find('[data-test="view-grid"]').exists()).toBe(false)
  })

  it('adaptive: 16-99 eps — jump input appears, still no grid toggle', () => {
    const w = mount(EpisodesPanel, { props: { episodes: manyEps(40), selectedNumber: 1 } })
    expect(w.find('[data-test="jump-input"]').exists()).toBe(true)
    expect(w.find('[data-test="view-grid"]').exists()).toBe(false)
  })

  it('adaptive: 100+ eps — grid toggle appears and switches to the grid view', async () => {
    const w = mount(EpisodesPanel, { props: { episodes: manyEps(112), selectedNumber: 4 } })
    expect(w.find('[data-test="view-grid"]').exists()).toBe(true)
    expect(w.find('[data-test="ep-grid"]').exists()).toBe(false)

    await w.find('[data-test="view-grid"]').trigger('click')
    expect(w.find('[data-test="ep-grid"]').exists()).toBe(true)
    expect(w.find('[data-test="ep-strip"]').exists()).toBe(false)
    expect(w.findAll('[data-test^="episode-grid-"]').length).toBe(112)

    await w.find('[data-test="view-strip"]').trigger('click')
    expect(w.find('[data-test="ep-strip"]').exists()).toBe(true)
  })

  it('grid cells emit select (click = play, same as strip)', async () => {
    const w = mount(EpisodesPanel, { props: { episodes: manyEps(112), selectedNumber: 4 } })
    await w.find('[data-test="view-grid"]').trigger('click')
    await w.find('[data-test="episode-grid-87"]').trigger('click')
    expect(w.emitted('select')?.[0]).toEqual([
      expect.objectContaining({ number: 87 }),
    ])
  })

  it('shows the next-unwatched chip for long titles and hides it when caught up', () => {
    const behind = mount(EpisodesPanel, {
      props: { episodes: manyEps(40), selectedNumber: 1, watchedUpTo: 3 },
    })
    expect(behind.find('[data-test="next-unwatched"]').text()).toContain('4')

    // Selected episode IS the next unwatched — chip is noise, hide it.
    const onIt = mount(EpisodesPanel, {
      props: { episodes: manyEps(40), selectedNumber: 4, watchedUpTo: 3 },
    })
    expect(onIt.find('[data-test="next-unwatched"]').exists()).toBe(false)
  })

  it('jump scrolls/flashes the target card without selecting it', async () => {
    const w = mount(EpisodesPanel, { props: { episodes: manyEps(40), selectedNumber: 1 } })
    const input = w.find('[data-test="jump-input"]')
    await input.setValue('120') // clamps to 40
    await input.trigger('keydown.enter')
    expect(w.emitted('select')).toBeUndefined()
    expect(w.find('[data-test="episode-40"]').classes()).toContain('ep-flash')
  })

  // ── Upcoming episode placeholder ──────────────────────────────────────────

  it('renders a disabled upcoming placeholder with the eta label', () => {
    const w = mount(EpisodesPanel, {
      props: { episodes: eps, selectedNumber: 1, upcoming: { number: 4, etaLabel: 'in 2 days' } },
    })
    const ph = w.find('[data-test="episode-upcoming"]')
    expect(ph.exists()).toBe(true)
    expect(ph.text()).toContain('airs in 2 days')
    // non-interactive: it is a div, not a button, and emits no select
    expect(ph.element.tagName).not.toBe('BUTTON')
  })

  it('upcoming placeholder without an eta shows the generic not-aired label', () => {
    const w = mount(EpisodesPanel, {
      props: { episodes: eps, selectedNumber: 1, upcoming: { number: 4 } },
    })
    expect(w.find('[data-test="episode-upcoming"]').text()).toContain('not aired yet')
  })

  it('hides the upcoming placeholder when that episode is already loaded', () => {
    const w = mount(EpisodesPanel, {
      props: { episodes: eps, selectedNumber: 1, upcoming: { number: 3, etaLabel: 'soon' } },
    })
    expect(w.find('[data-test="episode-upcoming"]').exists()).toBe(false)
  })

  it('renders no upcoming placeholder when upcoming is null', () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    expect(w.find('[data-test="episode-upcoming"]').exists()).toBe(false)
  })
})
