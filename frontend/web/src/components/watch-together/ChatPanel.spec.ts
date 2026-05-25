/**
 * Workstream watch-together — Phase 02 (frontend-shell) Plan 02.4 Task 2.
 *
 * Vitest spec for ChatPanel.vue. Verifies:
 *   1. 3 messages → 3 <li> items rendered (username + body visible)
 *   2. Empty messages → `watch_together.empty_chat` rendered
 *   3. Typing in textarea + clicking send emits `send` exactly once with
 *      the trimmed body
 *   4. After send, the textarea is cleared
 *   5. Empty draft → send button is disabled
 *   6. Whitespace-only draft → send button is disabled
 *   7. Char counter visible only when draft.length > 400
 *   8. Enter key (without Shift) emits `send`; Shift+Enter does NOT emit
 *   9. No font-bold / font-black / font-extrabold in rendered HTML
 *  10. Auto-scroll: when messages length increases AND the list was at-
 *      bottom, scrollTop is set to scrollHeight on nextTick (JSDOM mock)
 *  11. Own-message bubble uses `bg-primary/10`; other messages use
 *      `bg-foreground/5`
 *
 * `vue-i18n` is stubbed so t() echoes the key (params JSON-encoded so the
 * char-counter interpolation is observable).
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'

// ── Mocks ────────────────────────────────────────────────────────────────

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

import ChatPanel from './ChatPanel.vue'
import type { ChatMessage } from '@/api/watch-together'

function makeMsg(
  id: string,
  userId: string,
  username: string,
  body: string,
  ts = 1_700_000_000_000,
): ChatMessage {
  return { id, user_id: userId, username, body, ts }
}

const baseMessages: ChatMessage[] = [
  makeMsg('m1', 'alice-uuid', 'Alice', 'hello'),
  makeMsg('m2', 'self-uuid', 'Bob', 'hi!'),
  makeMsg('m3', 'alice-uuid', 'Alice', 'how are you?'),
]

function mountChat(props: { messages: ChatMessage[]; currentUserId?: string }) {
  return mount(ChatPanel, {
    props: {
      currentUserId: 'self-uuid',
      ...props,
    },
  })
}

describe('ChatPanel', () => {
  beforeEach(() => {
    // Reset scroll behavior — JSDOM doesn't implement scrolling natively,
    // so each test installs whatever scrollHeight / clientHeight values it
    // needs on a per-element basis via Object.defineProperty.
  })

  it('renders one <li> per message with username + body', () => {
    const wrapper = mountChat({ messages: baseMessages })
    const items = wrapper.findAll('li')
    expect(items).toHaveLength(3)
    expect(items[0].text()).toContain('Alice')
    expect(items[0].text()).toContain('hello')
    expect(items[1].text()).toContain('Bob')
    expect(items[1].text()).toContain('hi!')
    expect(items[2].text()).toContain('how are you?')
  })

  it('renders the empty-chat key when messages is empty', () => {
    const wrapper = mountChat({ messages: [] })
    expect(wrapper.findAll('li')).toHaveLength(0)
    expect(wrapper.text()).toContain('watch_together.empty_chat')
  })

  it('emits `send` with the trimmed body when send button is clicked', async () => {
    const wrapper = mountChat({ messages: [] })
    const textarea = wrapper.find('textarea')
    await textarea.setValue('  hello world  ')
    const sendBtn = wrapper.find('button[type="button"]')
    await sendBtn.trigger('click')
    expect(wrapper.emitted('send')).toBeTruthy()
    expect(wrapper.emitted('send')).toHaveLength(1)
    expect(wrapper.emitted('send')![0]).toEqual(['hello world'])
  })

  it('clears the textarea after a successful send', async () => {
    const wrapper = mountChat({ messages: [] })
    const textarea = wrapper.find<HTMLTextAreaElement>('textarea')
    await textarea.setValue('a message')
    await wrapper.find('button[type="button"]').trigger('click')
    expect(textarea.element.value).toBe('')
  })

  it('disables the send button when draft is empty', () => {
    const wrapper = mountChat({ messages: [] })
    const btn = wrapper.find<HTMLButtonElement>('button[type="button"]')
    expect(btn.element.disabled).toBe(true)
  })

  it('disables the send button when draft is whitespace-only', async () => {
    const wrapper = mountChat({ messages: [] })
    await wrapper.find('textarea').setValue('   \n  ')
    const btn = wrapper.find<HTMLButtonElement>('button[type="button"]')
    expect(btn.element.disabled).toBe(true)
  })

  it('shows the char counter only when draft length > 400', async () => {
    const wrapper = mountChat({ messages: [] })
    // Short draft → no counter.
    await wrapper.find('textarea').setValue('short')
    expect(wrapper.text()).not.toContain('watch_together.chat_char_count')
    // Long draft (>400) → counter visible.
    await wrapper.find('textarea').setValue('a'.repeat(401))
    expect(wrapper.text()).toContain('watch_together.chat_char_count')
  })

  it('emits `send` on Enter without Shift; does NOT emit on Shift+Enter', async () => {
    const wrapper = mountChat({ messages: [] })
    const textarea = wrapper.find('textarea')
    await textarea.setValue('first')
    await textarea.trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(wrapper.emitted('send')).toHaveLength(1)
    expect(wrapper.emitted('send')![0]).toEqual(['first'])

    // After auto-clear, type again and try Shift+Enter — should NOT emit.
    await textarea.setValue('second')
    await textarea.trigger('keydown', { key: 'Enter', shiftKey: true })
    expect(wrapper.emitted('send')).toHaveLength(1)
  })

  it('uses only font-medium / font-semibold weights', () => {
    const wrapper = mountChat({ messages: baseMessages })
    const html = wrapper.html()
    expect(html).not.toMatch(/\bfont-bold\b/)
    expect(html).not.toMatch(/\bfont-black\b/)
    expect(html).not.toMatch(/\bfont-extrabold\b/)
    expect(html).toMatch(/font-medium|font-semibold/)
  })

  it('auto-scrolls to bottom when a new message arrives and the user was at-bottom', async () => {
    const wrapper = mountChat({ messages: baseMessages })
    const ul = wrapper.find<HTMLUListElement>('ul').element

    // Pin scroll metrics so isAtBottom() reads "at bottom" before the
    // length watcher fires.
    Object.defineProperty(ul, 'scrollHeight', { configurable: true, value: 200 })
    Object.defineProperty(ul, 'clientHeight', { configurable: true, value: 100 })
    ul.scrollTop = 100 // at bottom (200 - 100 - 100 === 0 < 50px tolerance)

    // Add a 4th message — watcher should fire.
    await wrapper.setProps({
      messages: [...baseMessages, makeMsg('m4', 'alice-uuid', 'Alice', 'new')],
    })

    // After nextTick, scrollTop should equal scrollHeight (post-DOM-update value).
    Object.defineProperty(ul, 'scrollHeight', { configurable: true, value: 280 })
    await nextTick()
    await flushPromises()
    expect(ul.scrollTop).toBe(280)
  })

  it('does NOT auto-scroll if the user has scrolled up beyond the 50px tolerance', async () => {
    const wrapper = mountChat({ messages: baseMessages })
    const ul = wrapper.find<HTMLUListElement>('ul').element

    Object.defineProperty(ul, 'scrollHeight', { configurable: true, value: 500 })
    Object.defineProperty(ul, 'clientHeight', { configurable: true, value: 100 })
    ul.scrollTop = 100 // 500 - 100 - 100 = 300 px from bottom → NOT at bottom

    await wrapper.setProps({
      messages: [...baseMessages, makeMsg('m4', 'alice-uuid', 'Alice', 'new')],
    })
    await nextTick()
    await flushPromises()

    // scrollTop should remain at the user's chosen position.
    expect(ul.scrollTop).toBe(100)
  })

  it('styles own-message bubble with bg-primary/10 and others with bg-foreground/5', () => {
    const wrapper = mountChat({ messages: baseMessages })
    const items = wrapper.findAll('li')
    // m2 is Bob, user_id=self-uuid → own bubble.
    expect(items[1].html()).toContain('bg-primary/10')
    // m1 + m3 are Alice → others bubble.
    expect(items[0].html()).toContain('bg-foreground/5')
    expect(items[2].html()).toContain('bg-foreground/5')
  })
})
