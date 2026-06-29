# aePlayer — RAW/DUB model, combo/URL respect, top-3 fallback, subtitle defaults

**Date:** 2026-06-29
**Status:** Approved (design) — owner confirmed all four decisions + translatable labels
**Scope:** `frontend/web/` aePlayer (OurEnglish) only. No backend changes.

## Problem

Owner-reported bugs in the aePlayer:

1. Player always opens **SUB EN**, ignoring the saved watch combo and URL params.
2. The audio/language sliders are wrong. Wanted: a top **RAW / DUB** slider; a second **RU / EN** slider that appears **only under DUB**.
3. Selecting **Japanese** shows nothing; and when every provider is degraded/down the user **cannot pick any source without hacker mode**.
4. Subtitle settings reset on episode change; aePlayer subtitles are **on by default** (should be off); the CC icon doesn't reflect whether subs are enabled.
5. (Added) The **RAW / DUB** labels must be translatable → **Оригинал / Озвучка**.

## Locked decisions

- **RAW model = "original voices".** RAW merges today's `sub` + pure raw‑JP (everything that is NOT a dub). No language slider under RAW; subtitle language is chosen in the Subtitles menu (off by default). DUB → RU/EN.
- **Subtitle default = always OFF, no exceptions** — including pure raw‑JP. This deliberately reverses the prior "raw/JP auto-enables Jimaku" rule (memory `project_aeplayer_subdub_hardsub_autoselect`); update memory after ship.
- **Top‑3 fallback = pad-to-3.** When fewer than 3 selectable (active) rows exist for the current facet, pad with the highest‑ranked degraded/recovering rows up to 3 and make those selectable **without** hacker mode. When ≥3 active exist, behavior is unchanged.
- **Labels translatable** in all three locales (en/ru/ja — the i18n parity gate blocks redeploy otherwise).

## Internal model (no backend change)

`Combo.audio` stays `'sub' | 'dub'` and `Combo.lang` stays `'en' | 'ru' | 'ja'`. The redesign is a **UI relabel + filter‑semantics change**, not a type change:

| UI facet | `combo.audio` | language slider | `combo.lang` |
|----------|---------------|-----------------|--------------|
| **RAW** (Оригинал) | `'sub'` | hidden | derived — follows the selected provider's group; not user-set |
| **DUB** (Озвучка) | `'dub'` | **RU / EN** shown | user-chosen (`en`/`ru`) |

### Invariants

- **I1 — RAW relevance ignores language and matches original-audio kinds.** Under RAW, a provider row is relevant iff its caps `audios ∩ {sub, raw} ≠ ∅` and its content matches (hentai rule unchanged). The lang gate is dropped, so EN‑sub (`gogoanime`), RU‑sub (`kodik`), and pure‑JP (`raw`) all appear in one list. Under DUB, relevance is `audios.includes('dub') && GROUP_LANGS[group].includes(lang) && content` (lang ∈ {en, ru}).
- **I2 — `raw` caps surface as `sub` rows.** `toRow` maps cap audio `'raw' → 'sub'` (and `'sub' → 'sub'`, `'dub' → 'dub'`) so a raw‑only provider becomes a selectable RAW row and contributes a `watch_type:'sub'` combo to availability.
- **I3 — RAW lang changes are inert.** Because RAW ignores lang in filtering, and the facet-change watcher treats a lang change as a re-pick trigger **only when `audio === 'dub'`**, syncing `combo.lang` to the chosen provider under RAW never churns the row list nor re-picks a provider.
- **I4 — `combo.lang` under RAW follows the provider.** When a provider is selected while `audio === 'sub'`, set `combo.lang` to that provider's group primary lang (`en→en`, `ru→ru`, `jp→ja`, `firstparty→`current-if-in-group-else `ja`, `adult→en`) via a **team-preserving** setter (must NOT reset `combo.team`, unlike `setLang`). This keeps persistence (`comboToWatchCombo` reads `combo.lang`) and subtitle logic correct without a user-facing lang slider.
- **I5 — DUB clamps lang.** Switching RAW→DUB while `combo.lang === 'ja'` clamps to a DUB-valid lang (prefer existing `en`/`ru`, else `en`). The DUB language slider only offers RU/EN.

## The eight fixes

### F1 — RAW/DUB + conditional language slider (`SourcePanel.vue`)
- `audioOptions` → `[{ value:'sub', label:t('player.aePlayer.source.audio.raw') }, { value:'dub', label:t('...audio.dub') }]`.
- Language slider block wrapped in `v-if="audio === 'dub'"`; `langOptions` reduced to `en`/`ru` (2‑col thumb, drop `ja`). Keep autonym labels (`English`, `Русский`) — only RAW/DUB are translatable per request.
- Header `Sources`/`Audio`/`Language`/`Provider`/`Team`/`Server` copy: route through existing/new i18n keys where cheap (RAW/DUB are the required ones).
- The control-bar `audioLabel` (`AePlayer.vue:861`, shown as `{{ providerName }} · {{ audioLabel }}`) maps `'dub'→Озвучка`, `'sub'→Оригинал` via the same i18n keys.

### F2 — JA reachable / RAW shows all original sources (`useProviderFeed.ts`)
- Rewrite `relevant()` per **I1**; `toRow` per **I2**.
- `RowFilter` keeps `{audio, lang, content}`; relevance branches on `f.audio`.

### F3 — Respect saved watch combo + URL (`AePlayer.vue`)
Root cause: `buildAvailable()` iterates `rows.value` (already facet-filtered to sub/en), so the availability list passed to preference resolution only ever contains EN‑sub combos → any saved `dub`/`ru`/`ja` pref can't match → resolver collapses to SUB EN.
- Rebuild `buildAvailable()` from the **full capability report** (every non-`no_content` provider across all families × each audio it serves, mapping `raw→sub` × each lang in `GROUP_LANGS[group]`), independent of the current facet.
- Add **read** support for `?audio` / `?lang` URL params (today only `?provider/?team/?episode` exist): new `initialAudio`/`initialLang` props on `AePlayer`, parsed in `Anime.vue` from `route.query`. Apply precedence in `resolvePreference().finally()`: **URL audio/lang > saved combo (`applyResolvedCombo`) > smart default**, then `applyInitialProvider()` (which clamps to a `?provider`), then `preferenceSettled = true`. `?audio` accepts `raw|sub` (→`'sub'`) and `dub`; `?lang` accepts `en|ru|ja`.
- **Write-back of `?audio`/`?lang` is OUT OF SCOPE** for this change — the existing `urlSyncState` two-way sync (`{provider,team,episode}`, `Anime.vue onUrlSync`) is left untouched to avoid changing its contract. Reading the params is the requirement; a shared `?provider=` link already implies its facet via the deep-link clamp. (Defer write-back to a follow-up if shareable facet links are wanted.)

### F4 — Top‑3 fallback (`SourcePanel.vue` + `ProviderChip.vue`)
- `collapsedRows`: after taking up to `TOP_N` active rows, if `< TOP_N`, pad from `sortedRows` (already state→order sorted) with the next degraded/recovering rows until length `TOP_N` (still pin the selected provider).
- Compute `forcedSelectableIds` = the padded (non-active) ids now shown. Pass `:forced="forcedSelectableIds.has(r.id)"` to `ProviderChip`; chip selectability becomes `row.selectable && (!row.hackerOnly || hackerMode || forced)`.
- **Dead-player guard:** when `activeRows` is empty, the smart-default / `repickProviderForFacet` path auto-picks the **top forced row** (highest `order` among the padded set) so the player still attempts playback instead of showing "no source". (smartDefault stays pure — the fallback lives in the caller.)

### F5 — Subtitles OFF by default (`pickDefaultSubtitle.ts` + `AePlayer.vue`)
- `pickAutoSubtitle` → always returns `null` (remove the `bundled[0]` and `lang==='ja'` auto-enable). Simplest: delete `autoSelectSubtitle()`'s effect — keep the function as a no-op or remove it and its two watchers (lines ~2147‑2156). `state.subLang` stays `'off'` until the user picks a track.
- `hardsubNote` keeps working (it already keys on no chosen track + EN/RU + no bundled). Keep the `pickAutoSubtitle` export only if a spec/consumer still needs it; otherwise update/remove its spec.

### F6 — Subtitles persist across episode change (`AePlayer.vue`)
- Replace the hard reset `watch(subEpisode, () => { subUserDecided=false; chosenSub.value=null })`.
- New behavior on episode change: keep `subUserDecided` and `state.subLang`; clear only the now-stale `chosenSub` URL, then **re-resolve** a track for the persisted `state.subLang` from the new episode's tracks once they load (`if subLang !== 'off' → chosenSub = pickBestForLang(tracks, subLang)`; else stays null/off). Wire via the existing `watch(subtitleTracks, …)`. Net: if the user enabled RU subs on ep 1, ep 2 re-enables the best RU track; if they left/turned subs off, ep 2 stays off.
- Size/bg/offset already persist (they live in `usePlayerState`, untouched on episode change).

### F7 — CC icon reflects enabled state (`PlayerControlBar.vue` + `AePlayer.vue`)
- New prop `:subs-on` (boolean) = `state.subLang.value !== 'off' && !!chosenSubUrl`, passed to `PlayerControlBar`.
- CC `PlayerIconButton`: keep `:active="openMenu==='subs'"` for the menu-open highlight, and add a distinct **enabled** affordance when `subsOn` (e.g. `text-brand-cyan` + a small dot, or swap `Captions ↔ CaptionsOff` from lucide). Enabled-state must be visible when the menu is closed.

### F8 — i18n keys (en / ru / ja)
Add under `player.aePlayer.source` (or nearest existing namespace) in `en.json`, `ru.json`, `ja.json`:

| key | en | ru | ja |
|-----|----|----|----|
| `audio.raw` | `Original` | `Оригинал` | `オリジナル` |
| `audio.dub` | `Dub` | `Озвучка` | `吹き替え` |

(Parity test `frontend/web/src/locales/__tests__/*` must stay green.)

## Edge cases

- **Saved `sub/ru` (Kodik) under RAW:** restored as RAW; smart default picks the globally top‑ranked original-audio provider, but **biased to the saved language group when a provider exists there** (don't yank a RU‑sub watcher onto an EN source if a RU original source is available). Implement as: among active RAW rows, prefer the highest `order` whose group primary lang == saved `combo.lang`; fall back to global top. Honors `feedback_watch_preferences` "never cross language" softly while keeping the slider‑free UI.
- **RAW with only a dub-capable provider:** none — dub-only providers are filtered out of RAW by I1.
- **Switch DUB→RAW:** lang gate drops; provider list widens; if the current DUB provider also serves original audio it stays, else `repickProviderForFacet` picks the best RAW source.
- **Hentai titles:** adult group still always visible (unchanged hentai branch in `relevant`).

## Testing

- `pickDefaultSubtitle.spec.ts` — `pickAutoSubtitle` now always null (update existing cases incl. the JP/raw case).
- `useProviderFeed.spec.ts` — RAW shows en+ru+jp original sources & hides dub-only; DUB keeps lang gate; `raw→sub` mapping.
- `SourcePanel.spec.ts` — language slider hidden under RAW / shown under DUB (RU/EN only); pad-to-3 with degraded rows; `forced` selectability.
- `AePlayer.urlsync.spec.ts` / a new init spec — saved `dub/ru` and `?audio/?lang` restored (no collapse to SUB EN); `buildAvailable` enumerates cross-facet combos.
- `AePlayer.subtitles.spec.ts` — default off; episode change re-resolves persisted lang; CC `subsOn`.
- Gate: `/frontend-verify` (DS-lint, i18n en/ru/ja, real `bun run build`, vue-tsc), then `/animeenigma-after-update`.

## Out of scope

- No backend `/capabilities` changes (top‑3 is FE-side; the BE feed stays honest about state/selectable/hacker_only).
- No new persistence of subtitle prefs to server (session-scoped, as today).
- Furigana / JP sub Phase 4 untouched.
