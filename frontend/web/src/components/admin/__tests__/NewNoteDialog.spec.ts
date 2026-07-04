import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

vi.mock('@/api/client', () => ({
  adminApi: { createNote: vi.fn() },
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k }) }))

import { adminApi } from '@/api/client'
import NewNoteDialog from '../NewNoteDialog.vue'

const createSpy = adminApi.createNote as ReturnType<typeof vi.fn>

beforeEach(() => vi.clearAllMocks())

describe('NewNoteDialog', () => {
  it('blocks submit when description is empty', async () => {
    const w = mount(NewNoteDialog, { props: { open: true } })
    await (w.vm as unknown as { submit: () => Promise<void> }).submit()
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('posts the note and emits created on success', async () => {
    createSpy.mockResolvedValue({ data: { data: { id: 'note-1', status: 'new' } } })
    const w = mount(NewNoteDialog, { props: { open: true } })
    const vm = w.vm as unknown as { description: string; submit: () => Promise<void> }
    vm.description = 'dark mode'
    await vm.submit()
    await flushPromises()
    expect(createSpy).toHaveBeenCalledWith({ category: undefined, description: 'dark mode' })
    expect(w.emitted('created')?.[0]).toEqual(['note-1'])
  })
})
