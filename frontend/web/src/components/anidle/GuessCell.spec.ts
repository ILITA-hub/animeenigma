import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import GuessCell from './GuessCell.vue'

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

function mountCell(props: { status: 'correct' | 'partial' | 'wrong'; value: string | number; hint?: 'higher' | 'lower' | null }) {
  return mount(GuessCell, {
    props,
    global: { plugins: [i18n] },
  })
}

describe('GuessCell', () => {
  it('applies bg-success for correct status', () => {
    const w = mountCell({ status: 'correct', value: 'Action' })
    expect(w.classes()).toContain('bg-success')
  })

  it('applies bg-warning for partial status', () => {
    const w = mountCell({ status: 'partial', value: 2020 })
    expect(w.classes()).toContain('bg-warning')
  })

  it('applies bg-muted for wrong status', () => {
    const w = mountCell({ status: 'wrong', value: 'Bones' })
    expect(w.classes()).toContain('bg-muted')
  })

  it('renders ↑ indicator for hint=higher', () => {
    const w = mountCell({ status: 'partial', value: 2018, hint: 'higher' })
    expect(w.text()).toContain('↑')
  })

  it('renders ↓ indicator for hint=lower', () => {
    const w = mountCell({ status: 'partial', value: 2025, hint: 'lower' })
    expect(w.text()).toContain('↓')
  })

  it('renders no arrow when hint is absent', () => {
    const w = mountCell({ status: 'correct', value: 'Action' })
    expect(w.text()).not.toContain('↑')
    expect(w.text()).not.toContain('↓')
  })

  it('does not use off-palette color classes (bg-green/yellow/gray)', () => {
    const wCorrect = mountCell({ status: 'correct', value: 'x' })
    const wPartial = mountCell({ status: 'partial', value: 'x' })
    const wWrong = mountCell({ status: 'wrong', value: 'x' })
    for (const w of [wCorrect, wPartial, wWrong]) {
      const html = w.html()
      expect(html).not.toMatch(/bg-green-/)
      expect(html).not.toMatch(/bg-yellow-/)
      expect(html).not.toMatch(/bg-gray-/)
      expect(html).not.toMatch(/bg-emerald-/)
    }
  })

  it('displays the value as text', () => {
    const w = mountCell({ status: 'correct', value: 'My Studio' })
    expect(w.text()).toContain('My Studio')
  })
})
