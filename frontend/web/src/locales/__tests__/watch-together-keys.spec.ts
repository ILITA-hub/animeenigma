import { describe, it, expect } from 'vitest'
import en from '../en.json'
import ru from '../ru.json'

// NOTE: ja.json is intentionally NOT imported here — the watch_together
// namespace is en + ru only for v1.0 (Watch Together ships in en/ru first).
// Japanese locale parity will land in v1.1 alongside other ja additions.

// Recursively collect every leaf key path in an object, e.g.
// { a: { b: 'x', c: 'y' } } -> ['a.b', 'a.c']
function leafPaths(obj: unknown, prefix = ''): string[] {
  if (obj === null || typeof obj !== 'object') return [prefix]
  return Object.entries(obj as Record<string, unknown>).flatMap(([k, v]) => {
    const next = prefix ? `${prefix}.${k}` : k
    return leafPaths(v, next)
  })
}

// Walk to a value via dot-path. Returns undefined if any segment missing.
function get(obj: unknown, path: string): unknown {
  return path.split('.').reduce<unknown>((acc, seg) => {
    if (acc == null || typeof acc !== 'object') return undefined
    return (acc as Record<string, unknown>)[seg]
  }, obj)
}

describe('watch_together i18n parity', () => {
  const enWT = (en as Record<string, unknown>).watch_together
  const ruWT = (ru as Record<string, unknown>).watch_together

  it('en.json has a top-level watch_together object', () => {
    expect(enWT).toBeTypeOf('object')
    expect(enWT).not.toBeNull()
  })

  it('ru.json has a top-level watch_together object', () => {
    expect(ruWT).toBeTypeOf('object')
    expect(ruWT).not.toBeNull()
  })

  it('en and ru watch_together key sets are identical', () => {
    const enKeys = leafPaths(enWT).sort()
    const ruKeys = leafPaths(ruWT).sort()
    expect(ruKeys).toEqual(enKeys)
  })

  it('every watch_together.* leaf value is a non-empty string in en.json', () => {
    const paths = leafPaths(enWT)
    for (const p of paths) {
      const v = get(enWT, p)
      expect(typeof v, `en watch_together.${p}`).toBe('string')
      expect((v as string).trim().length, `en watch_together.${p} non-empty`).toBeGreaterThan(0)
    }
  })

  it('every watch_together.* leaf value is a non-empty string in ru.json', () => {
    const paths = leafPaths(ruWT)
    for (const p of paths) {
      const v = get(ruWT, p)
      expect(typeof v, `ru watch_together.${p}`).toBe('string')
      expect((v as string).trim().length, `ru watch_together.${p} non-empty`).toBeGreaterThan(0)
    }
  })

  // Explicit expected keys — a deletion of any of these (e.g. by a future
  // refactor) is surfaced individually rather than as a generic parity diff.
  // List matches the required keys enumerated in CONTEXT.md §i18n plus the
  // additional Wave 3+ component-needs keys decided in plan 02.2 Task 1.
  const expectedKeys = [
    'title',
    'subtitle',
    'members_heading',
    'host_badge',
    'you_badge',
    'empty_chat',
    'chat_input_placeholder',
    'chat_input_aria',
    'chat_char_count',
    'send_button',
    'reaction_palette_title',
    'reaction_palette_aria',
    'invite_button_label',
    'invite_copied_toast',
    'invite_copy_manual',
    'kodik_sync_unavailable',
    'room_ended_title',
    'room_ended_back_button',
    'reconnecting_indicator',
    'reconnect_failed_title',
    'reconnect_failed_button',
    'capacity_full_title',
    'capacity_full_back_button',
    'auth_expired_error',
    'loading',
    'member_joined',
    'member_left',
    'room_id_label',
    'sync_toast_played',
    'sync_toast_paused',
    'sync_toast_seeked',
    'connection_status_closed',
    // Phase 04 (state-switching) Plan 04.4 — PlayerTabBar tab labels.
    'player_tab_kodik',
    'player_tab_animelib',
    'player_tab_ourenglish',
    'player_tab_hanime',
    'player_tab_raw',
    // Phase 04 (state-switching) Plan 04.4 — sender-only state-error toasts.
    'state_change_episode_unavailable',
    'state_change_player_unavailable',
    'state_change_translation_unavailable',
    // Phase 05 (polish) Plan 05.4 — mobile bottom-sheet tab labels (WT-POLISH-03).
    'bottom_sheet_tab_chat',
    'bottom_sheet_tab_reactions',
  ] as const

  it.each(expectedKeys)('en.json has watch_together.%s as a string', (key) => {
    expect(typeof (enWT as Record<string, unknown>)[key]).toBe('string')
  })

  it.each(expectedKeys)('ru.json has watch_together.%s as a string', (key) => {
    expect(typeof (ruWT as Record<string, unknown>)[key]).toBe('string')
  })

  // Interpolation-token preservation. vue-i18n templates use `{name}` syntax;
  // a translator might "translate" the placeholder by mistake which would
  // render literal `{n}` or drop the parameter silently. Lock the contract.

  it('watch_together.chat_char_count preserves {n} interpolation in both locales', () => {
    expect((enWT as Record<string, string>).chat_char_count).toContain('{n}')
    expect((ruWT as Record<string, string>).chat_char_count).toContain('{n}')
  })

  it('watch_together.member_joined preserves {username} interpolation in both locales', () => {
    expect((enWT as Record<string, string>).member_joined).toContain('{username}')
    expect((ruWT as Record<string, string>).member_joined).toContain('{username}')
  })

  it('watch_together.member_left preserves {username} interpolation in both locales', () => {
    expect((enWT as Record<string, string>).member_left).toContain('{username}')
    expect((ruWT as Record<string, string>).member_left).toContain('{username}')
  })

  it('watch_together.sync_toast_played preserves {username} interpolation in both locales', () => {
    expect((enWT as Record<string, string>).sync_toast_played).toContain('{username}')
    expect((ruWT as Record<string, string>).sync_toast_played).toContain('{username}')
  })

  it('watch_together.sync_toast_paused preserves {username} interpolation in both locales', () => {
    expect((enWT as Record<string, string>).sync_toast_paused).toContain('{username}')
    expect((ruWT as Record<string, string>).sync_toast_paused).toContain('{username}')
  })

  it('watch_together.sync_toast_seeked preserves {username} and {time} interpolation in both locales', () => {
    expect((enWT as Record<string, string>).sync_toast_seeked).toContain('{username}')
    expect((enWT as Record<string, string>).sync_toast_seeked).toContain('{time}')
    expect((ruWT as Record<string, string>).sync_toast_seeked).toContain('{username}')
    expect((ruWT as Record<string, string>).sync_toast_seeked).toContain('{time}')
  })
})
