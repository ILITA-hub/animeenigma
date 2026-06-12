import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (k: string) => k,
    locale: { value: 'ru' },
  }),
}))

import DatePicker from './DatePicker.vue'

function mountPicker(props: Record<string, unknown> = {}) {
  return mount(DatePicker, {
    props: { placeholder: 'Начало', ...props },
    attachTo: document.body,
  })
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('DatePicker', () => {
  it('shows the placeholder when no value is set', () => {
    const w = mountPicker()
    const trigger = w.find('[data-testid="datepicker-trigger"]')
    expect(trigger.exists()).toBe(true)
    expect(trigger.text()).toContain('Начало')
  })

  it('shows the locale-formatted date when a value is set', () => {
    const w = mountPicker({ modelValue: '2026-04-05' })
    expect(w.find('[data-testid="datepicker-trigger"]').text()).toContain('05.04.2026')
  })

  it('opens a calendar popover with day cells on trigger click', async () => {
    const w = mountPicker({ modelValue: '2026-04-05' })
    await w.find('[data-testid="datepicker-trigger"]').trigger('click')
    // Content is portaled to body
    const cells = document.body.querySelectorAll('[data-reka-calendar-cell-trigger]')
    expect(cells.length).toBeGreaterThan(27)
  })

  it('clear emits an empty string', async () => {
    const w = mountPicker({ modelValue: '2026-04-05' })
    await w.find('[data-testid="datepicker-trigger"]').trigger('click')
    const clearBtn = document.body.querySelector('[data-testid="datepicker-clear"]') as HTMLButtonElement
    expect(clearBtn).toBeTruthy()
    clearBtn.click()
    await w.vm.$nextTick()
    expect(w.emitted('update:modelValue')![0][0]).toBe('')
  })

  it('today button emits an ISO yyyy-mm-dd date', async () => {
    const w = mountPicker()
    await w.find('[data-testid="datepicker-trigger"]').trigger('click')
    const todayBtn = document.body.querySelector('[data-testid="datepicker-today"]') as HTMLButtonElement
    todayBtn.click()
    await w.vm.$nextTick()
    const emitted = w.emitted('update:modelValue')![0][0] as string
    expect(emitted).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  it('selecting a day emits that ISO date and closes', async () => {
    const w = mountPicker({ modelValue: '2026-04-05' })
    await w.find('[data-testid="datepicker-trigger"]').trigger('click')
    const cell = document.body.querySelector('[data-reka-calendar-cell-trigger][data-value="2026-04-18"]') as HTMLElement
    expect(cell).toBeTruthy()
    cell.click()
    await w.vm.$nextTick()
    expect(w.emitted('update:modelValue')![0][0]).toBe('2026-04-18')
  })
})
