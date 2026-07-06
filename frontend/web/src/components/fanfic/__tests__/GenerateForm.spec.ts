import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import ja from '@/locales/ja.json'
import GenerateForm from '../GenerateForm.vue'
import type { CharacterCardModel } from '@/types/character'

// vi.mock factories are hoisted above imports — anything they close over must
// itself be created via vi.hoisted() (a plain `const` here would still be in
// its temporal-dead-zone when the hoisted mock factory runs).
const { confirmMock } = vi.hoisted(() => ({ confirmMock: vi.fn() }))

vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => ({ confirm: confirmMock }),
}))

vi.mock('@/api/client', () => ({
  animeApi: { search: vi.fn().mockResolvedValue({ data: { data: [] } }) },
  charactersApi: { getAnimeCharacters: vi.fn().mockResolvedValue({ data: { data: [] } }) },
}))

vi.mock('@/api/fanfic', () => ({
  fanficApi: {
    tags: vi.fn().mockResolvedValue([
      { slug: 'fluff', ru: 'флафф', en: 'fluff' },
      { slug: 'angst', ru: 'ангст', en: 'angst' },
    ]),
  },
}))

const i18n = createI18n({ legacy: false, locale: 'ru', messages: { en, ru, ja } })

function mountForm() {
  return mount(GenerateForm, { global: { plugins: [i18n] } })
}

function fakeCharacter(id: string): CharacterCardModel {
  return { id, name: `Character ${id}`, image: '', role: 'main' }
}

interface GenerateFormVm {
  selectedAnime: unknown
  selectAnime: (item: Record<string, unknown>) => void
  selectedCharacters: { id: string; name: string }[]
  toggleCharacter: (c: CharacterCardModel) => void
  MAX_CHARACTERS: number
  selectedTags: string[]
  customTagInput: string
  addCustomTag: () => void
  MAX_TAGS: number
  rating: string
  onRatingChange: (v: string) => Promise<void>
  prompt: string
  MAX_PROMPT: number
  promptLength: number
  promptOverLimit: boolean
  canGenerate: boolean
  buildInput: () => unknown
  onSubmit: () => void
}

describe('GenerateForm', () => {
  it('renders the core fields (prompt textarea + generate button)', async () => {
    const wrapper = mountForm()
    await flushPromises()
    expect(wrapper.find('textarea').exists()).toBe(true)
    expect(wrapper.text()).toContain(ru.fanfic.form.generate)
  })

  it('disables the generate button until an anime is selected and a prompt is entered', async () => {
    const wrapper = mountForm()
    await flushPromises()
    const vm = wrapper.vm as unknown as GenerateFormVm
    expect(vm.canGenerate).toBe(false)

    const button = wrapper.findAll('button').find((b) => b.text().includes(ru.fanfic.form.generate))
    expect(button?.attributes('disabled')).toBeDefined()

    vm.selectAnime({ id: 'anime-1', shikimori_id: '123', name: 'Test Anime', poster_url: 'http://x/p.jpg' })
    vm.prompt = 'Write something sweet'
    await wrapper.vm.$nextTick()
    expect(vm.canGenerate).toBe(true)
  })

  it('builds the correct GenerateInput and emits it on submit', async () => {
    const wrapper = mountForm()
    await flushPromises()
    const vm = wrapper.vm as unknown as GenerateFormVm

    vm.selectAnime({ id: 'anime-1', shikimori_id: '123', name: 'Test Anime', poster_url: 'http://x/p.jpg' })
    vm.toggleCharacter(fakeCharacter('char-1'))
    vm.selectedTags.push('fluff')
    vm.prompt = 'A cozy story'
    await wrapper.vm.$nextTick()

    const input = vm.buildInput() as {
      anime: { id: string }
      characters: { id: string }[]
      tags: string[]
      prompt: string
    }
    expect(input.anime.id).toBe('anime-1')
    expect(input.characters).toEqual([{ id: 'char-1', name: 'Character char-1' }])
    expect(input.tags).toEqual(['fluff'])
    expect(input.prompt).toBe('A cozy story')

    vm.onSubmit()
    expect(wrapper.emitted('generate')).toBeTruthy()
    expect(wrapper.emitted('generate')![0]).toEqual([input])
  })

  it('enforces the <=6 character and <=8 tag caps', async () => {
    const wrapper = mountForm()
    await flushPromises()
    const vm = wrapper.vm as unknown as GenerateFormVm

    for (let i = 0; i < 9; i++) {
      vm.toggleCharacter(fakeCharacter(`char-${i}`))
    }
    expect(vm.selectedCharacters.length).toBe(vm.MAX_CHARACTERS)

    for (let i = 0; i < 12; i++) {
      vm.customTagInput = `tag-${i}`
      vm.addCustomTag()
    }
    expect(vm.selectedTags.length).toBe(vm.MAX_TAGS)
  })

  it('blocks generation once the prompt exceeds the 2000-char cap', async () => {
    const wrapper = mountForm()
    await flushPromises()
    const vm = wrapper.vm as unknown as GenerateFormVm

    vm.selectAnime({ id: 'anime-1', shikimori_id: '123', name: 'Test Anime', poster_url: 'http://x/p.jpg' })
    vm.prompt = 'a'.repeat(2000)
    await wrapper.vm.$nextTick()
    expect(vm.promptLength).toBe(2000)
    expect(vm.promptOverLimit).toBe(false)
    expect(vm.canGenerate).toBe(true)

    vm.prompt = 'a'.repeat(2001)
    await wrapper.vm.$nextTick()
    expect(vm.promptLength).toBe(2001)
    expect(vm.promptOverLimit).toBe(true)
    expect(vm.canGenerate).toBe(false)

    const button = wrapper.findAll('button').find((b) => b.text().includes(ru.fanfic.form.generate))
    expect(button?.attributes('disabled')).toBeDefined()
  })

  it('gates the Explicit rating behind a confirm dialog', async () => {
    const wrapper = mountForm()
    await flushPromises()
    const vm = wrapper.vm as unknown as GenerateFormVm

    confirmMock.mockResolvedValueOnce(false)
    await vm.onRatingChange('explicit')
    expect(vm.rating).toBe('teen')

    confirmMock.mockResolvedValueOnce(true)
    await vm.onRatingChange('explicit')
    expect(vm.rating).toBe('explicit')
  })
})
