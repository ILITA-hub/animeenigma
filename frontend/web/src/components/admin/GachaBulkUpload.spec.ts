import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import ja from '@/locales/ja.json'
import GachaBulkUpload from './GachaBulkUpload.vue'

vi.mock('@/api/gacha', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/gacha')>()
  return {
    ...actual,
    gachaAdminApi: {
      uploadFile: vi.fn(),
      createCard: vi.fn(),
    },
  }
})

import { gachaAdminApi } from '@/api/gacha'

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en, ru, ja } })

// Modal renders via reka-ui's DialogPortal (a Teleport internally); jsdom
// leaves it out of the mounted tree even with `stubs: { teleport: true }`
// (that string stub doesn't intercept reka's own <Teleport> component
// reference). AdminGacha.spec.ts hits the same wall and stubs Modal outright
// with a v-if shell that still renders the default + footer slots — mirrored
// here so the drop zone / file input / retry button are actually queryable.
const ModalStub = {
  props: ['modelValue', 'title'],
  template: '<div data-testid="modal" v-if="modelValue"><slot /><slot name="footer" /></div>',
}

function mountDialog() {
  return mount(GachaBulkUpload, {
    props: { modelValue: true },
    global: {
      plugins: [i18n],
      stubs: { teleport: true, Modal: ModalStub },
    },
  })
}

/** jsdom file inputs are read-only — override `files` to simulate a pick. */
async function pickFiles(wrapper: ReturnType<typeof mountDialog>, files: File[]) {
  const input = wrapper.find('[data-testid="bulk-file-input"]')
  Object.defineProperty(input.element, 'files', { value: files, configurable: true })
  await input.trigger('change')
  await flushPromises()
}

describe('GachaBulkUpload', () => {
  beforeEach(() => {
    vi.mocked(gachaAdminApi.uploadFile).mockReset()
    vi.mocked(gachaAdminApi.createCard).mockReset()
  })

  it('uploads each picked file and creates a disabled draft card per file', async () => {
    vi.mocked(gachaAdminApi.uploadFile).mockResolvedValue({
      data: { data: { image_path: 'cards/x.png', image_url: '/api/gacha/images/cards/x.png' } },
    } as never)
    vi.mocked(gachaAdminApi.createCard).mockResolvedValue({ data: { data: {} } } as never)

    const wrapper = mountDialog()
    await pickFiles(wrapper, [
      new File(['a'], 'Emilia.png', { type: 'image/png' }),
      new File(['b'], 'Rem.webp', { type: 'image/webp' }),
    ])

    expect(gachaAdminApi.uploadFile).toHaveBeenCalledTimes(2)
    expect(gachaAdminApi.uploadFile).toHaveBeenCalledWith(expect.any(File), 'cards')
    expect(gachaAdminApi.createCard).toHaveBeenCalledTimes(2)
    expect(gachaAdminApi.createCard).toHaveBeenCalledWith({
      name: 'Emilia',
      source_title: '',
      rarity: 'N',
      enabled: false,
      image_path: 'cards/x.png',
      back_path: '',
      group_ids: [],
    })
  })

  it('falls back to the unnamed label when the file stem is empty', async () => {
    vi.mocked(gachaAdminApi.uploadFile).mockResolvedValue({
      data: { data: { image_path: 'cards/y.png', image_url: '' } },
    } as never)
    vi.mocked(gachaAdminApi.createCard).mockResolvedValue({ data: { data: {} } } as never)

    const wrapper = mountDialog()
    await pickFiles(wrapper, [new File(['a'], '.png', { type: 'image/png' })])

    expect(gachaAdminApi.createCard).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'Unnamed' })
    )
  })

  it('marks failed files and re-runs them via the retry button', async () => {
    vi.mocked(gachaAdminApi.uploadFile).mockRejectedValueOnce(new Error('boom'))
    const wrapper = mountDialog()
    await pickFiles(wrapper, [new File(['a'], 'Fail.png', { type: 'image/png' })])

    expect(gachaAdminApi.createCard).not.toHaveBeenCalled()
    const retry = wrapper.find('[data-testid="bulk-retry-btn"]')
    expect(retry.exists()).toBe(true)

    vi.mocked(gachaAdminApi.uploadFile).mockResolvedValue({
      data: { data: { image_path: 'cards/z.png', image_url: '' } },
    } as never)
    vi.mocked(gachaAdminApi.createCard).mockResolvedValue({ data: { data: {} } } as never)
    await retry.trigger('click')
    await flushPromises()

    expect(gachaAdminApi.createCard).toHaveBeenCalledTimes(1)
  })

  it('tops up workers so a second overlapping batch is not stuck behind a single busy worker', async () => {
    let resolveFirstUpload!: (v: unknown) => void
    vi.mocked(gachaAdminApi.uploadFile).mockImplementationOnce(
      () => new Promise(resolve => { resolveFirstUpload = resolve as (v: unknown) => void })
    )
    vi.mocked(gachaAdminApi.uploadFile).mockResolvedValue({
      data: { data: { image_path: 'cards/x.png', image_url: '' } },
    } as never)
    vi.mocked(gachaAdminApi.createCard).mockResolvedValue({ data: { data: {} } } as never)

    const wrapper = mountDialog()

    // First pick: 1 file whose upload promise stays open — this alone used to
    // permanently exhaust 2 of the 3 workers under the old `running` boolean.
    await pickFiles(wrapper, [new File(['a'], 'Slow.png', { type: 'image/png' })])

    // Second, overlapping pick: 3 more files with instantly-resolving mocks.
    await pickFiles(wrapper, [
      new File(['b'], 'B.png', { type: 'image/png' }),
      new File(['c'], 'C.png', { type: 'image/png' }),
      new File(['d'], 'D.png', { type: 'image/png' }),
    ])

    // The top-up must have revived workers to pick up the new batch even
    // though the first worker is still stuck on the held-open upload —
    // proven by the call count reaching all 4 files before it is released.
    expect(gachaAdminApi.uploadFile).toHaveBeenCalledTimes(4)

    resolveFirstUpload({ data: { data: { image_path: 'cards/slow.png', image_url: '' } } })
    await flushPromises()

    const statuses = wrapper.findAll('li').map(li => li.text())
    expect(statuses.every(s => s.startsWith('✓'))).toBe(true)
  })

  it('does not re-upload an already-uploaded blob when retrying a createCard failure', async () => {
    vi.mocked(gachaAdminApi.uploadFile).mockResolvedValue({
      data: { data: { image_path: 'cards/x.png', image_url: '' } },
    } as never)
    vi.mocked(gachaAdminApi.createCard).mockRejectedValueOnce(new Error('boom'))

    const wrapper = mountDialog()
    await pickFiles(wrapper, [new File(['a'], 'Emilia.png', { type: 'image/png' })])

    expect(gachaAdminApi.uploadFile).toHaveBeenCalledTimes(1)
    expect(gachaAdminApi.createCard).toHaveBeenCalledTimes(1)

    vi.mocked(gachaAdminApi.createCard).mockResolvedValue({ data: { data: {} } } as never)
    const retry = wrapper.find('[data-testid="bulk-retry-btn"]')
    await retry.trigger('click')
    await flushPromises()

    expect(gachaAdminApi.uploadFile).toHaveBeenCalledTimes(1)
    expect(gachaAdminApi.createCard).toHaveBeenCalledTimes(2)
  })

  it('blocks a Modal close (Escape/backdrop/X) while a batch is running, and allows it once finished', async () => {
    let resolveUpload!: (v: unknown) => void
    vi.mocked(gachaAdminApi.uploadFile).mockImplementation(
      () => new Promise(resolve => { resolveUpload = resolve as (v: unknown) => void })
    )

    const wrapper = mountDialog()
    const input = wrapper.find('[data-testid="bulk-file-input"]')
    Object.defineProperty(input.element, 'files', {
      value: [new File(['a'], 'Slow.png', { type: 'image/png' })],
      configurable: true,
    })
    await input.trigger('change')
    await flushPromises()

    // Upload is still pending -> the run is active. Simulate Modal forwarding
    // an Escape/backdrop/X close the same way it forwards a real one: emitting
    // update:modelValue(false) on the Modal stub.
    const modal = wrapper.findComponent(ModalStub)
    await modal.vm.$emit('update:modelValue', false)
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()

    // Let the run finish.
    vi.mocked(gachaAdminApi.createCard).mockResolvedValue({ data: { data: {} } } as never)
    resolveUpload({ data: { data: { image_path: 'cards/slow.png', image_url: '' } } })
    await flushPromises()

    // Now a close request should propagate normally.
    await modal.vm.$emit('update:modelValue', false)
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]])
  })
})
