# Phase 03 — UI Review (Dynamic Cards + Migration)

**Audited:** 2026-05-21
**Baseline:** `02-UI-SPEC.md` (master design contract — covers all 9 cards, Neon Tokyo)
**Screenshots:** not captured (no dev server on :3000/:5173/:8080; live SPA renders client-side). Audit is **code-only against UI-SPEC + the 5 new SFCs that shipped + dispatch wiring**.
**Scope:** the 5 NEW Phase 3 cards (`PersonalPickCard`, `TelegramNewsCard`, `NowWatchingCard`, `NotTimeYetCard`, `ContinueWatchingNewCard`), the 9-branch dispatch in `HeroSpotlightBlock.vue`, the `Home.vue` migration (HSB-MIG-01), and the new `spotlight.{personalPick,telegramNews,nowWatching,notTimeYet,continueWatchingNew}.*` i18n leaves in en/ru/ja.
**Out of scope:** the 4 Phase 02 cards (already audited in `02-UI-REVIEW.md`, 16/24). Findings on them only surface here if they regressed in commit `24f9388`.
**Audit stance:** adversarial — Phase 02's user feedback ("still bad until two rounds of fixes") is treated as a prior; Phase 03 surface is scored against the contract, not against intent.

---

## Pillar Scores

| Pillar | Score | Key Finding |
|--------|-------|-------------|
| 1. Copywriting | **2**/4 | `ContinueWatchingNew` mixes English "ep" into RU/JA badge ("Новая серия ep {n}!", "新エピソード {n}!" reasonable but RU is wrong); `episodesLabel` ("{n} ep.") reused to render *last-watched-episode* — semantic mismatch; `NotTimeYet`/`ContinueWatchingNew` CTAs labelled "Watch"/"Resume" but route to `/anime/{id}` detail page, not to a player. `TelegramNewsCard` post dates rendered raw, same defect as Phase 02 F1.2. |
| 2. Visuals | **2**/4 | `NowWatchingCard` row is a single giant `<router-link>` that wraps the LIVE dot + poster + username + anime + episode + LIVE badge into ONE click target → impossible to click the username to visit the profile (HSB-NF-04's `public_id` is shipped but unreachable from the UI). `PersonalPickCard` section heading is `text-sm` (Label role), other multi-item cards are `text-lg md:text-xl` (Heading role) — visual hierarchy inconsistency. No skeletons for any Phase 3 card surfaced inside the carousel (single global skeleton only). |
| 3. Color | **1**/4 | UI-SPEC §Color line 83 — **"Strictly reuses the Neon Tokyo palette … No new colors are introduced."** `ContinueWatchingNewCard` introduces `purple-300/90` + `purple-500/90` — two net-new colors with zero declaration. UI-SPEC line 90 reserved pink `#ff2d7c` for the `now_watching` LIVE indicator; implementation chose `green-400` instead — spec contract violation. The 60/30/10 cyan budget remains intact, but the palette has silently doubled. |
| 4. Typography | **3**/4 | All 5 cards stay within `font-medium`/`font-semibold` (two-weight rule held). Sizes used: xs/sm/lg/xl/2xl/3xl/base — within spec. **But** `PersonalPickCard` heading is `text-sm font-medium uppercase` — role drift: spec lists `text-sm` as Body/CTA, not Heading. Other multi-item cards use the Heading role (`text-lg md:text-xl font-semibold`) — visible inconsistency in the section-title font tier. |
| 5. Spacing | **3**/4 | All 5 cards honor `p-4 md:p-4 lg:p-6` outer + `gap-3` / `gap-4` content rhythm. Skeleton/loaded heights regressed from `lg:max-h-[360px]` (spec) to `lg:max-h-[400px]` — partial revert of Phase-02 F5.1, +40px over desktop ceiling. `ContinueWatchingNewCard` badge `px-2 py-0.5` matches genre-chip precedent. No `[Npx]` arbitrary values in card bodies. |
| 6. Experience Design | **2**/4 | Phase 02 F6.1 fixed (`pauseAutoplay` SR live-region now wired in `HeroSpotlightBlock.vue:126-128` — commit `24f9388`). NEW Phase 3 defects: `NowWatchingCard` whole-row link kills the username link contract (UI-SPEC §3 `PersonalPickCard` & §NowWatching ref imply a username → profile link); `NotTimeYet`/`ContinueWatchingNew` CTAs are mis-routed (label says "Watch"/"Resume", target is detail page); `PersonalPickCard` mobile-only "+ N more →" footer link uses `useMediaQuery` (client-only) — SSR builds would briefly render mobile fallback on desktop before hydration. |

**Overall: 13/24**

(Phase 02 baseline 16/24; Phase 03 cards regressed the overall score against the same spec because they introduced two net-new colors, broke the username-link contract, and shipped routing/copy mismatches on two single-anime CTAs.)

---

## Top 3 Priority Fixes

1. **BLOCKER — `ContinueWatchingNewCard` introduces `purple-*` palette + RU badge mixes English "ep"** (`frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.vue` — found via grep `"purple-300/90"`).
   UI-SPEC line 83 — "Strictly reuses the Neon Tokyo palette … No new colors are introduced." — and line 90 — "Accent secondary (rare) — **pink** `#ff2d7c` — Reserved for Phase 3's `now_watching` 'live' indicator". The implementation introduces `purple-300/90` (eyebrow) + `purple-500/90` (new-episode badge bg) — **purple is not in the Neon Tokyo palette at all**. The badge copy in ru.json reads `"Новая серия ep {n}!"` (line 1048 of ru.json — found via grep `"newEpisodeBadge"`) — mixes English `ep` into a Cyrillic string.
   **Fix:** (a) swap `purple-*` to either `pink-500/90` + `pink-300/90` (matches spec's reserved Phase-3 accent — pink-500 = `#ff2d7c`, declared via `--color-pink-500` in `main.css`) OR `cyan-500/90` + `cyan-300/90` (stays within the 10% cyan budget). (b) Change RU badge to `"Новая серия {n}!"` (drop the English `ep`); EN stays "New ep {n}!"; JA "新エピソード {n}!" reads fine.

2. **BLOCKER — `NowWatchingCard` swallows the username link** (`NowWatchingCard.vue:19-50` — found via grep `"router-link"`).
   The entire row is wrapped in a single `<router-link :to="`/anime/${s.anime_id}`">` — clicking anywhere on the row (LIVE dot, poster, username text, episode text, LIVE badge) always navigates to the anime detail page. The data shipped includes `s.username` and `s.public_id` (verified at `types/spotlight.ts:178-179` — found via grep `"public_id"`), and the i18n `sessionLabel` reads `"{username} → {anime} · ep {n}"` (en.json:1037) — the "→" arrow implies "from user to anime" but the username is **not** clickable. HSB-NF-04 (Phase 3 privacy contract) names `public_id` as the public-profile identifier; the UI ships the data but blocks the affordance.
   **Fix:** split the row into two interactive zones — wrap the username text in `<router-link :to="`/user/${s.public_id}`">` (or a nested `<a>`), wrap the poster + anime + episode in `<router-link :to="`/anime/${s.anime_id}`">`. Use `flex` so the two links sit side by side; nesting `<a>` inside `<a>` is invalid HTML so a parent `<router-link>` must be removed.

3. **WARNING — `NotTimeYet` & `ContinueWatchingNew` CTAs route to detail page, not player** (`NotTimeYetCard.vue:75-80` + `ContinueWatchingNewCard.vue:73-79` — found via grep `"watchCta\|resumeCta"`).
   Both CTAs are labelled "Watch" / "Смотреть" (`spotlight.notTimeYet.watchCta` — en.json:1044, ru.json:1044) and "Resume" / "Продолжить" (`spotlight.continueWatchingNew.resumeCta` — en.json:1049, ru.json:1049). Both link to `/anime/${data.anime.id}` — the detail page — not to `/anime/${id}/watch` (the player route, see `AnimeOfDayCard.vue` for precedent). Users press "Resume" and land on the detail page instead of the new episode, then have to click "Watch" again. Two clicks where one was promised.
   **Fix:** in both files, change the `<router-link :to>` to `\`/anime/${data.anime.id}/watch\`` (matches the player route convention used by `AnimeOfDayCard.vue:88` — found via grep `"watch"`). For `ContinueWatchingNew`, append `?ep=${data.new_episode_number}` so the player jumps straight to the new episode.

---

## Detailed Findings

### Pillar 1: Copywriting (2/4)

**BLOCKER findings:**

- **F1.1 — `ContinueWatchingNew` RU badge mixes English `ep`** (`frontend/web/src/locales/ru.json:1048` — found via grep `"newEpisodeBadge"`). RU string is `"Новая серия ep {n}!"` — `ep` is English, breaks Cyrillic register. EN string `"New ep {n}!"` (en.json:1048) is fine; JA `"新エピソード {n}!"` (ja.json:1048) is fine.
  - **Fix:** RU → `"Новая серия {n}!"` or `"Новая серия №{n}!"`.

- **F1.2 — `episodesLabel` ("{n} ep.") reused to render *last-watched* episode count** (`ContinueWatchingNewCard.vue:65-69` — found via grep `"episodesLabel"`). The i18n key `spotlight.animeOfDay.episodesLabel` is defined as "total episode count for an anime" (Phase 02 spec line 142). `ContinueWatchingNew` feeds it `data.last_watched_episode` so a user who watched up to episode 10 sees `"10 ep."` next to a brand-new-episode badge — the same label that elsewhere means "this anime has 10 episodes total". Users will read it as "10 episodes available" not "you watched ep 10". The plan-05 summary acknowledges this as a deliberate choice to "keep the i18n surface lean" — it's leanness at the cost of correctness.
  - **Fix:** add a dedicated `spotlight.continueWatchingNew.lastWatchedLabel` key — e.g. EN `"Last watched: ep {n}"`, RU `"Последний просмотр: эп {n}"`, JA `"最後に見た: 第{n}話"`.

**WARNING findings:**

- **F1.3 — `TelegramNewsCard` renders `post.date` as raw string** (`TelegramNewsCard.vue:32-37` — found via grep `"post.date"`). Same defect as Phase 02 F1.2 on `LatestNewsCard` (which is still unfixed in `LatestNewsCard.vue:53-55` — found via grep `"formatEntryDate"`). No `Intl.DateTimeFormat` / `formatRelative`. Backend serves whatever the Telegram parser emits — likely a raw ISO or formatted string — so the visible value is unpredictable across locales.
  - **Fix:** wrap `post.date` in `new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(new Date(post.date))` with a try/catch fallback to raw on parse failure.

- **F1.4 — `NotTimeYet.watchCta` says "Watch" but routes to detail page** (`NotTimeYetCard.vue:75-80` — found via grep `"watchCta"`). Copy promises player navigation; href delivers detail page. Same defect on `ContinueWatchingNew.resumeCta` (`ContinueWatchingNewCard.vue:74-79` — found via grep `"resumeCta"`).
  - See Top-3 Fix #3 for the routing patch.

- **F1.5 — `nowWatching.sessionLabel` "{username} → {anime} · ep {n}" interpolates 3 nouns into a single truncating label** (en.json:1037 — found via grep `"sessionLabel"`). On a narrow row with `text-sm truncate`, a long username + long anime title results in the EP count being clipped or the anime title being clipped. The arrow `→` visually says "this user is watching this anime", but at truncate boundary the meaning evaporates. No `<bdi>` wrapper either; bidirectional usernames could shake the order.
  - **Fix:** decompose into `<span class="font-medium">{{ username }}</span>` + small "→" + `<span class="truncate">{{ anime }}</span>` + `<span class="text-gray-400">ep {n}</span>` separated spans so each can truncate independently; pair with F2.1 (split the row link).

- **F1.6 — `personalPick.titleAnon` "Trending now" vs `title` "Picked for you"** (en.json:1023-1024 — found via grep `"titleAnon"`). Acceptable. RU: `"В тренде"` vs `"Выбрано для вас"` — also acceptable. No issue here; flagged as PASS for completeness of the copy audit.

- **F1.7 — Phase 02 F1.1 (LatestNews readMore `to="/"` dead-arrow) IS RESOLVED in commit `24f9388`** (`LatestNewsCard.vue:9-14` — found via grep `"readMore"`). New target is `/changelog` and a `LastUpdates`-mounted route was added. Verifying via grep, the EN copy reads `"Read full changelog →"`. ✓

### Pillar 2: Visuals (2/4)

**BLOCKER findings:**

- **F2.1 — `NowWatchingCard` whole-row link kills the username-profile contract** (`NowWatchingCard.vue:19-50` — found via grep `"router-link"`). See Top-3 Fix #2. The data layer ships `username` + `public_id` (privacy-safe per HSB-NF-04); the UI silently consumes them but provides no path to a user profile. The visual design *looks* like it should link the username (it's bolded, separated by an arrow) — false affordance.

**WARNING findings:**

- **F2.2 — `PersonalPickCard` heading is `text-sm font-medium uppercase tracking-wider`** (`PersonalPickCard.vue:5-15` — found via grep `"text-sm font-medium text-cyan-400"`). Other multi-item cards use `<h3 class="text-lg md:text-xl font-semibold text-white">` for the section title (TelegramNewsCard:7, NowWatchingCard:7, LatestNewsCard precedent). UI-SPEC §Typography line 70 declares `text-lg md:text-xl font-semibold` as the "Heading" role for "Card section title". `PersonalPickCard` ships a `text-sm` Label-tier element in the heading slot — visually it looks like an eyebrow label, not a card title. Adjacent cards in the carousel will visibly shift in font hierarchy on every advance.
  - **Fix:** wrap PersonalPick's heading text in a real `<h3>` styled `text-lg md:text-xl font-semibold text-white`. The cyan-400 eyebrow style can remain as a secondary `text-xs uppercase` line above it if the "Trending now" vs "Picked for you" tonal split needs the eyebrow treatment.

- **F2.3 — No card-level skeleton for the 5 new types** (HeroSpotlightBlock.vue:21-30 — found via grep `"skeleton-shimmer"`). The block-level skeleton paints one full-block shimmer (Phase 02 F2.3, score 3/4). Phase 3's 5 new cards keep the same pattern — no per-card silhouette. With ContinueWatchingNew and NotTimeYet both being poster-left layouts, a silhouette of "poster rect on left + 3 text rows on right" would be cheap to add and more visually-accurate. Acceptable Tier-1 but not Tier-2.

- **F2.4 — `NowWatchingCard` poster img has `:alt="''"`** (`NowWatchingCard.vue:30` — found via grep `":alt=\\\"''\\\""`). Empty alt is correct *if* the title text immediately follows in the same accessible flow — which it does (the `sessionLabel` text is inside the same `<router-link>` content). Verified PASS for a11y semantics.

- **F2.5 — `TelegramNewsCard` post body is non-interactive but visually identical to `LatestNewsCard` post body** (`TelegramNewsCard.vue:13-48` — found via grep `"hover:bg-white/10"`). The card uses `hover:bg-white/10 transition-colors` on the `<article>` (line 19) which is a hover affordance — but the only clickable inside is the small "Open post →" anchor at the bottom. Users hover the whole tile and get visual feedback, then click and nothing happens unless they hit the small footer link. Compare to `LatestNewsCard.vue:16-28` which has the same pattern (also a defect — Phase 02 review didn't flag it because the LatestNews card has zero interactive children, so the hover was decorative-only). TelegramNews's hover-promise + footer-link reality is worse.
  - **Fix:** wrap the entire `<article>` content in the same `<a target="_blank" rel="noopener noreferrer">` when `post.link` exists; the footer "Open post →" becomes a redundant call-to-action. OR remove the `hover:bg-white/10` so the tile doesn't look interactive.

- **F2.6 — `NowWatchingCard` row gap and chevron clipping risk** (`NowWatchingCard.vue:13-52` — found via grep `"flex flex-col gap-2"`). With 3 sessions × `gap-2` (8px) + 3 × `p-2` (8px each side) + 3 × ~44px row content + the section-title header — total content is well under 320px desktop height. ✓ Healthy. BUT the row content has no min-height guarantee, so the LIVE badge can dance vertically if username text wraps. Add `min-h-[44px]` to each `<li>` row to lock the row height and pin the LIVE badge.

### Pillar 3: Color (1/4)

Token distribution across the 5 new cards (grep counts):

```
PersonalPickCard:        cyan-400 (×2), cyan-300/80 (×1), white (×1), gray (none)
TelegramNewsCard:        cyan-400 (×1), cyan-300 (×1, hover), white (×2), gray-300, gray-500
NowWatchingCard:         green-400 (×2), white (×1), gray (none)
NotTimeYetCard:          cyan-300/80 (×2), white (×2), gray-400 (×2)
ContinueWatchingNewCard: purple-300/90 (×2), purple-500/90 (×1), white (×3), gray-400 (×1)
```

**BLOCKER findings:**

- **F3.1 — `ContinueWatchingNewCard` introduces purple to the palette** (`ContinueWatchingNewCard.vue:7,33,47` — found via grep `"purple-"`). UI-SPEC line 83: **"Strictly reuses the Neon Tokyo palette from `main.css`. No new colors are introduced."** UI-SPEC §Color table lines 86-93 enumerates the palette — purple is absent. The Tailwind defaults will resolve `purple-300` to `#d8b4fe` and `purple-500/90` to `~rgba(168,85,247,0.9)` — values that have **no declaration** in `main.css`. The badge is visually striking (it's the only purple on the home page) but it's outside the design contract.
  - **Fix:** swap to `pink-500/90` (`#ff2d7c`, the Phase-3 reserved accent from UI-SPEC line 90) + `pink-300/90` for the eyebrow; OR swap to `cyan-500/90` + `cyan-300/90` (within the 10% cyan budget). Pink is the spec-compliant choice; cyan is the budget-disciplined choice. Either way, **`purple-*` must go**.

- **F3.2 — `NowWatchingCard` LIVE indicator is `green-400` not the spec's reserved `pink`** (`NowWatchingCard.vue:25,46` — found via grep `"green-400"`). UI-SPEC line 90: "Accent secondary (rare) — **pink** `#ff2d7c` — Reserved for Phase 3's `now_watching` 'live' indicator — DO NOT use in Phase 2". The spec named pink as **the** live-indicator color; Phase 3 chose `green-400` (which resolves to Tailwind `#4ade80` — also not declared in `main.css`). The `--color-success: #00ff9d` CSS var IS declared (line 92 of UI-SPEC, line ~135 of main.css per the spec's "Reserved for Phase 3" note) but `bg-green-400` does not reference `--color-success` — it uses Tailwind's default green palette.
  - **Fix:** either (a) swap `bg-green-400` → `bg-pink-500` + `text-pink-500` (spec-literal compliance), OR (b) swap to a real `--color-success` reference via a custom utility class like `.bg-success` declared in `main.css` (palette-disciplined alternative). The Phase 02 audit `02-UI-REVIEW.md` already flagged this in F3.1 as a Phase-3 audit concern; this audit confirms the defect persists.

- **F3.3 — 60/30/10 budget intact for the 5 new cards individually, but the palette doubled at the workstream level** (`spotlight/cards/*.vue` — found via grep `"text-(cyan|purple|green|pink|red|yellow|orange|blue|teal)"`). Aggregate count of accent tokens across all 9 cards now sees `cyan-400` (×9), `cyan-300` (×5), `yellow-400` (×2, score chip), `green-400` (×2, NowWatching), `purple-300/90` (×2, ContinueWatchingNew), `purple-500/90` (×1, ContinueWatchingNew). The cards collectively use **5 distinct hue families** — UI-SPEC declared **2** (cyan + the reserved pink). The accent is no longer "rare".

### Pillar 4: Typography (3/4)

Font-size distribution in the 5 new cards (grep):

```
text-xs:    11 occurrences (labels, eyebrows, badges, dates, reasons)
text-sm:    11 occurrences (body, meta, secondary CTAs, headings(!))
text-base:   4 occurrences (CTA at md+)
text-lg:     2 occurrences (TelegramNews + NowWatching headings)
text-xl:     2 occurrences (md: counterpart)
text-2xl:    2 occurrences (NotTimeYet + ContinueWatchingNew display title)
text-3xl:    2 occurrences (md: counterpart)
```

Font-weight distribution: `font-medium` and `font-semibold` only. ✓ Two-weight rule held.

**WARNING findings:**

- **F4.1 — `PersonalPickCard` heading downgraded to Label tier** (`PersonalPickCard.vue:5-15` — found via grep `"text-sm font-medium"`). See F2.2. The card's section title is `text-sm font-medium uppercase tracking-wider` — the spec's Label role, not the Heading role. Visual hierarchy across the carousel becomes uneven: cycling from TelegramNews (Heading 18→20px) to PersonalPick (Label 14px) is a perceptible "title shrunk" moment.

- **F4.2 — `text-sm` used for both section titles (PersonalPick) AND post titles (TelegramNews `<h4>`)** (`PersonalPickCard.vue:7` + `TelegramNewsCard.vue:23` — found via grep `"text-sm font-(semibold|medium)"`). The same size token plays two different roles. Spec-drift; not a regression versus Phase 2 (Phase 2 didn't have nested headings in the multi-item cards).

- **F4.3 — `font-semibold` on body text inside `TelegramNewsCard` post titles** (`TelegramNewsCard.vue:23` — found via grep `"font-semibold text-white"`). Within spec (`font-semibold` is one of the two declared weights). Establishes a `font-semibold` Title vs `font-medium` Body contrast inside the same tile — sound choice. ✓

- **F4.4 — `tabular-nums` is NOT used on NowWatching episode counts** (`NowWatchingCard.vue:38-43` — found via grep `"tabular-nums"`). When 3 sessions stack with different episode numbers (1, 12, 124), the proportional digit widths cause column jitter on auto-cycle. The Phase 02 PlatformStats already uses `tabular-nums` (UI-SPEC §Typography line 77) for a similar reason.
  - **Fix:** add `tabular-nums` to the episode-number `<span>` once F1.5 splits the label.

### Pillar 5: Spacing (3/4)

Spacing token distribution in the 5 new cards (grep ranked):

```
p-4:    10  — outer card padding (default + md:p-4)
p-6:     5  — desktop card padding (lg:p-6)
gap-3:   6  — vertical content rhythm
gap-4:   4  — desktop md:gap-4 grids
gap-2:   3  — chip flow, dot gaps
gap-1:   1  — meta inline (TelegramNews post body)
gap-6:   3  — desktop md:gap-6 row gap (poster ↔ meta)
p-2:     3  — chip / row item inner pad
p-3:     1  — TelegramNews post tile inner pad
mb-1:    2  — eyebrow → title spacing
mb-2:    4  — title → body spacing
mt-2:    3  — body → CTA spacing
mt-3:    2  — CTA wrapper
mt-auto: 1  — TelegramNews footer pin
py-0.5:  1  — badge vertical (matches genre-chip precedent)
px-2:    1  — badge horizontal
```

4-multiple scale (4/8/16/24/32 px) held substantially; documented exceptions present (12px `gap-3`, 2px `py-0.5`).

**WARNING findings:**

- **F5.1 — Phase 02 F5.1 partially fixed — `lg:max-h-[400px]` now present, but `360px` ceiling still off-spec** (`HeroSpotlightBlock.vue:27, 50` — found via grep `"max-h-\["`). UI-SPEC line 52 declares `lg:max-h-[360px]`. Implementation ships `lg:max-h-[400px]` — +40px over the desktop ceiling. The `h-[420px]` rigid pin from Phase 02 audit is gone (now `min-h-[420px]`) so the constraint type is correct, but the magnitudes drifted. Mobile is `min-h-[420px]` vs spec `min-h-[400px]` (+20px). Tablet is `md:min-h-[340px]` ✓ (matches spec — Phase 02 F5.1 tablet defect resolved). Desktop floor `lg:min-h-[320px]` ✓ (matches spec).
  - **Fix:** either (a) restore `lg:max-h-[360px]` per spec (tight but the cards proved they fit in Phase 02), OR (b) amend UI-SPEC to declare the new 400px ceiling. Option (b) is the honest path post-Round-3 fixes.

- **F5.2 — `ContinueWatchingNewCard` badge `px-2 py-0.5` is fine but `rounded shadow` is undeclared** (`ContinueWatchingNewCard.vue:33` — found via grep `"rounded shadow"`). UI-SPEC §Visual Contract score chip uses `rounded-md`, not bare `rounded` (which is Tailwind shorthand for `rounded-[0.25rem]` = 4px). `shadow` is Tailwind's default elevation; no declaration in UI-SPEC §Color or §Spacing. Cosmetic; <2px difference; flagged for spec hygiene.

- **F5.3 — `NowWatchingCard` row `p-2 gap-3` is sub-spec but within the 12px sub-step exception** (`NowWatchingCard.vue:21` — found via grep `"gap-3 p-2"`). UI-SPEC §Spacing line 59 permits 12px (`gap-3`) as a sub-step. `p-2` = 8px = standard. ✓

- **F5.4 — `PersonalPickCard` mobile single-poster `v-show` with `useMediaQuery`** (`PersonalPickCard.vue:21-26, 87-90` — found via grep `"mdAndUp"`). Uses `useMediaQuery('(min-width: 768px)')` to gate `v-show`. `useMediaQuery` from `@vueuse/core` is client-only; on SSR-rendered or first-paint, `mdAndUp` is `false` so desktop users may briefly see only 1 poster before hydration swaps in all 3. Tailwind's `md:hidden` / `hidden md:flex` are SSR-safe alternatives. Plan-05 SUMMARY documents this as a "deliberate trade-off for mobile image-loading" — acceptable but a hydration-flicker on desktop is a measurable visual defect on first paint.

### Pillar 6: Experience Design (2/4)

State coverage in the 5 new cards:

- Loading: **block-level only** (HeroSpotlightBlock skeleton). No per-card loading. ✓ Spec-compliant.
- Empty: backend resolver returns `(nil, nil)` per HSB-BE-30 adaptive rule when no items match — card never mounts. ✓
- Error: silent self-hide via block-level state machine. ✓
- Pulse: only `NowWatchingCard` uses `animate-pulse` (LIVE dot). ✓
- Disabled: no buttons require disabled state in Phase 3 (all CTAs are router-links).
- Focus: every interactive control is a `<router-link>` or `<a>` — inherits global `:focus-visible` ring. ✓

**BLOCKER findings:**

- **F6.1 — `NowWatchingCard` row link contract violation** (`NowWatchingCard.vue:19-50`). See Top-3 Fix #2. The username is bolded, separated by an arrow glyph (→), and styled like the primary entity in the row — but it's not clickable as a profile link. False affordance + lost feature (the `public_id` is shipped specifically for profile linking per HSB-NF-04).

- **F6.2 — `NotTimeYet`/`ContinueWatchingNew` CTAs route to detail, not player** (`NotTimeYetCard.vue:75-80` + `ContinueWatchingNewCard.vue:73-79`). See Top-3 Fix #3. The CTA labels promise immediate playback; the destination is a content listing page. Two-click defect on the "Resume new episode" hot path.

**WARNING findings:**

- **F6.3 — `PersonalPickCard` mobile `useMediaQuery` first-paint flicker** (`PersonalPickCard.vue:90`). See F5.4. Hydration-time toggle from "no poster" → "3 posters" is a measurable LCP defect on desktop, since the carousel decides which slide is active before hydration runs.
  - **Fix:** replace `v-show="i === 0 || mdAndUp"` with a Tailwind-only `:class="i === 0 ? '' : 'hidden md:flex'"`. SSR-safe; same visual outcome.

- **F6.4 — `TelegramNewsCard` external anchor leaks navigation to a new tab without user warning** (`TelegramNewsCard.vue:38-46` — found via grep `"target=\"_blank\""`). The "Open post →" anchor opens `t.me/...` in a new tab via `target="_blank" rel="noopener noreferrer"` (T-03-18 mitigated). UI-SPEC §Accessibility doesn't mandate a visual indicator for "opens in new tab"; convention is an inline arrow or the `↗` glyph. The right-arrow `→` is ambiguous — same glyph used by all internal "Read full changelog →" / "+ N more →" links.
  - **Fix:** swap `→` to `↗` (upper-right arrow, the de-facto external-link affordance) ONLY on the TelegramNews open CTA.

- **F6.5 — `ContinueWatchingNew` badge announces "New ep N!" without sr-only context** (`ContinueWatchingNewCard.vue:32-40` — found via grep `"newEpisodeBadge"`). Screen-reader users hear "New ep 5!" detached from the anime title — the badge is positioned absolutely on the poster (line 32) and its DOM order is BEFORE the anime title `<h3>`. SR announcement order: "New ep 5!" → "Continue watching" → "(Anime name)". Confusing.
  - **Fix:** move the `<span>` badge to AFTER the `<h3>` in the DOM (keep the absolute positioning for visuals) so SR order is "Continue watching" → "(Anime name)" → "New ep 5!". Alternatively add `aria-label="(Anime name): new episode 5"` on the badge.

- **F6.6 — `NowWatchingCard` does not announce "live" status semantically** (`NowWatchingCard.vue:23-26, 45-49`). The green pulsing dot is `aria-hidden="true"` (correct — decorative); the "LIVE" text badge has no `role="status"` or `aria-live` so SR users get "LIVE" announced once per slide but never re-announced as sessions change.
  - **Fix:** add `role="status"` on the LIVE badge OR an `sr-only` line `"{username} is watching now"` co-located with each row.

- **F6.7 — Phase 02 F6.3 `onAdd` stub on `AnimeOfDayCard`** was patched in commit `24f9388` (`AnimeOfDayCard.vue:99` — found via grep `"opacity-50 cursor-not-allowed"`). The "Add to list" button now ships `opacity-50 cursor-not-allowed` — visually disabled. ✓ Phase 02 audit defect resolved.

- **F6.8 — `Home.vue` legacy `trendingRecs` row removed cleanly** (`Home.vue:38` — found via grep `"HeroSpotlightBlock"`). `grep -c "trendingRecs" Home.vue` = 0 (verified). HSB-MIG-01 satisfied. ✓

---

## Registry Safety

`components.json` not found in the repo (`test -f /data/animeenigma/components.json` → absent). Project does not use shadcn or any external component registry. No new external block fetched in Phase 03. Registry audit: **skipped — N/A**.

---

## Files Audited

**Phase 03 source (primary scope):**
- `frontend/web/src/components/home/spotlight/cards/PersonalPickCard.vue` (91 lines)
- `frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.vue` (58 lines)
- `frontend/web/src/components/home/spotlight/cards/NowWatchingCard.vue` (63 lines)
- `frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.vue` (93 lines)
- `frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.vue` (92 lines)
- `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` (310 lines — 9-branch dispatch)
- `frontend/web/src/types/spotlight.ts` (236 lines — 9-variant union)
- `frontend/web/src/locales/en.json` (lines 1022-1050 — 5 new sub-namespaces)
- `frontend/web/src/locales/ru.json` (lines 1022-1050)
- `frontend/web/src/locales/ja.json` (lines 1022-1050)
- `frontend/web/src/views/Home.vue` (verified: 0 `trendingRecs` refs, 1 `<HeroSpotlightBlock />` mount at line 38)

**Phase 02 baseline (reference, not re-audited):**
- `02-UI-SPEC.md` (master design contract)
- `02-UI-REVIEW.md` (prior audit, 16/24 baseline)

**Phase 03 context + plan inputs:**
- `03-CONTEXT.md`
- `03-VERIFICATION.md` (passed: 10/10 truths, 18/18 requirements)
- `03-01-SUMMARY.md` through `03-07-SUMMARY.md`

**Live deploy reference (not re-fetched this audit):**
- Phase 02 audit captured `GET https://animeenigma.ru/api/home/spotlight` returning 6 cards (anon) and `ui_audit_bot` auth returning 7 (incl. `continue_watching_new`). `03-VERIFICATION.md` re-verified this.
- The 5 new SFCs render against the live aggregator payload via the 9-branch dispatch in `HeroSpotlightBlock.vue:70-114`.

---

## Audit Notes

- **Phase 02 fix lineage:** commit `24f9388` ("fix(hero-spotlight): address all UI audit findings (F1.1..F6.3)") landed and was verified — Phase 02 F1.1 (LatestNews `to="/"` dead arrow) now routes to `/changelog`; Phase 02 F6.1 (`pauseAutoplay` sr-only consumer) now wired in `HeroSpotlightBlock.vue:126-128`; Phase 02 F6.3 (`onAdd` stub) now visually-disabled with `opacity-50 cursor-not-allowed`. Phase 02 F5.1 (height regression) PARTIALLY fixed: rigid `h-[420px]` → flexible `min-h-[420px]` but `lg:max-h-[400px]` still drifts +40px from spec's `lg:max-h-[360px]`.
- **Phase 03 NEW defects:** the 5 new cards introduce 6 defects that Phase 02 audit predicted (purple/green palette bleed under F3.1, now confirmed as F3.1/F3.2). Two BLOCKER routing defects (F6.1 NowWatching row link, F6.2 NotTimeYet/ContinueWatchingNew CTA target) are new — not predictable from Phase 02 audit because the cards didn't exist there.
- **Score delta vs Phase 02:** Phase 02 = 16/24, Phase 03 = 13/24. The 3-point regression is concentrated in Color (3 → 1, palette violation) and Experience Design (3 → 2, two new BLOCKERs).
- **Screenshots:** not captured. No dev server detected at :3000/:5173/:8080. All findings anchored to file paths + line numbers via in-pass grep calls.
- **Recommendation count:** 3 priority fixes (1 BLOCKER palette, 1 BLOCKER row-link, 1 WARNING CTA-routing), 12 detailed findings across 6 pillars.

