# English Scraper Reconnect Implementation Plan

> **SUPERSEDED 2026-05-19** — content absorbed into v3.1 Phase 24/25/26 plan sketches per `.planning/milestones/v3.1-REOPENING.md`. Authoritative planning surface is `.planning/milestones/v3.1-phases/`. Phase 0 (provider verification) maps to v3.1 Phase 24 Wave 0 (SCRAPER-HEAL-20); Phase A.1 (player + tab) maps to v3.1 Phase 24 Waves 1-3 (SCRAPER-HEAL-17..19); Phase A.2 (browse filter + has_english) moved entirely to v3.1 Phase 26 Wave 1 alongside the AllAnime lift per Phase 24 D4 (filter is only useful once 3+ providers are populating has_english). Phase A.3 (health-aware tab hiding) deferred indefinitely per Phase 24 D5. This file remains in `docs/superpowers/plans/` as a historical record. Implementation also pivoted on Phase 24 D1: restore EnglishPlayer.vue from git commit `8424e99` instead of writing from scratch.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore an English-source streaming tab in the player by wiring the orphaned `services/scraper` microservice (gogoanime + animepahe live, animekai escape-hatched) to a new `EnglishPlayer.vue` component, plus the small backend additions to make the browse filter usable.

**Architecture:** Frontend-only for Phase A.1 (new player component + tab in `Anime.vue`, consuming the already-public `/api/anime/{id}/scraper/*` routes through `scraperApi`). Phase A.2 adds a `has_english` column to the `Anime` GORM model and activates the browse filter. Phase A.3 adds health-aware tab hiding. No `services/scraper` changes.

**Tech Stack:** Vue 3 + TypeScript + hls.js (already in `package.json` at `^1.6.15`), vue-i18n, GORM/PostgreSQL (catalog), bun + bunx for all CLI tooling (Playwright, TSC, ESLint).

**Spec reference:** `docs/superpowers/specs/2026-05-19-english-scraper-reconnect-design.md` (commit `17a7c86`).

---

## Phase 0: Verify every scraper provider end-to-end

Before touching frontend code, prove the backend actually serves what the spec assumes. The user explicitly asked: "don't forget to test each provider." A.1 is blocked until each task in Phase 0 has a green checkbox.

### Task 0.1: Confirm scraper container is up and reachable

**Files:** none (commands only)

- [ ] **Step 1: Check container is running**

Run:
```bash
docker ps --filter "name=animeenigma-scraper" --format '{{.Names}} {{.Status}}'
```
Expected: a line like `animeenigma-scraper Up X hours (healthy)`. If missing or unhealthy, run `make redeploy-scraper` and re-check.

- [ ] **Step 2: Check basic liveness endpoint**

Run:
```bash
curl -sf http://localhost:8088/health | jq .
```
Expected: `{"status":"ok"}` (HTTP 200).

- [ ] **Step 3: Check orchestrator-aware health endpoint**

Run:
```bash
curl -sf http://localhost:8088/scraper/health | jq .
```
Expected: a JSON object with at least one `providers[]` entry. Note which providers are registered — should be `gogoanime`, `animepahe`, and (only if `SCRAPER_ANIMEKAI_ENABLED=true` in `docker/.env`) `animekai`.

- [ ] **Step 4: Confirm gateway → catalog → scraper path is reachable as anonymous user**

Run:
```bash
curl -sf -o /dev/null -w "%{http_code}\n" http://localhost:8000/api/anime/_/scraper/health
```
Expected: `200`. If `401`/`403`, the gateway is enforcing auth where the spec assumed public — surface this finding and stop until resolved.

### Task 0.2: Verify gogoanime (Anitaku) end-to-end against a known-popular anime

**Files:** none (commands only)

The chosen test anime is **Frieren: Beyond Journey's End** (MAL 52991). It's a high-traffic ongoing as of 2026-05 and is mirrored across every English provider that's still alive.

- [ ] **Step 1: Resolve the anime UUID from local DB**

Run:
```bash
docker compose -f docker/docker-compose.yml exec -T postgres \
  psql -U postgres -d animeenigma \
  -t -c "SELECT id FROM animes WHERE shikimori_id = '52991' LIMIT 1;" \
  | tr -d ' '
```
Expected: a UUID like `c4a8b2e0-...`. Save it to a shell variable:
```bash
ANIME_ID=$(docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -t -c "SELECT id FROM animes WHERE shikimori_id = '52991' LIMIT 1;" | tr -d ' \n')
echo "ANIME_ID=$ANIME_ID"
```
If empty, the anime isn't in the local DB yet — open `https://animeenigma.ru/anime/frieren-beyond-journey-s-end` in a browser once to trigger on-demand population, then re-run.

- [ ] **Step 2: Episodes via gogoanime (prefer=gogoanime)**

Run:
```bash
curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/episodes?prefer=gogoanime" \
  | jq '{count: (.data.episodes | length), first: .data.episodes[0], tried: .data.meta.tried}'
```
Expected: `count >= 1`, `first` has `id`, `number`, `title`. `tried[0]` should be `"gogoanime"` (no fallback used).

- [ ] **Step 3: Servers for episode 1**

Run:
```bash
EP_ID=$(curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/episodes?prefer=gogoanime" | jq -r '.data.episodes[0].id')
echo "EP_ID=$EP_ID"
curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/servers?episode=$EP_ID&prefer=gogoanime" \
  | jq '{count: (.data.servers | length), names: [.data.servers[].name]}'
```
Expected: `count >= 1`. Capture one server ID:
```bash
SRV_ID=$(curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/servers?episode=$EP_ID&prefer=gogoanime" | jq -r '.data.servers[0].id')
```

- [ ] **Step 4: Stream for that server**

Run:
```bash
curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/stream?episode=$EP_ID&server=$SRV_ID&category=sub&prefer=gogoanime" \
  | jq '{sources: [.data.sources[] | {type, url: (.url[:80] + "...")}], tracks: (.data.tracks | length), intro: .data.intro, outro: .data.outro}'
```
Expected: at least one source. If `sources[0].type == "hls"`, the URL ends in `.m3u8` (truncated). If empty, capture the response and stop — gogoanime is broken for this anime.

- [ ] **Step 5: Smoke-fetch the HLS playlist**

Run:
```bash
URL=$(curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/stream?episode=$EP_ID&server=$SRV_ID&category=sub&prefer=gogoanime" | jq -r '.data.sources[0].url')
curl -sf -o /dev/null -w "playlist %{http_code} %{size_download} bytes\n" "$URL"
```
Expected: `playlist 200 NNNN bytes` with a non-zero size. A 403/404 indicates the extractor returned an expired or wrong URL.

### Task 0.3: Verify animepahe end-to-end (same anime, prefer=animepahe)

**Files:** none (commands only)

- [ ] **Step 1: Episodes via animepahe**

Run:
```bash
curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/episodes?prefer=animepahe" \
  | jq '{count: (.data.episodes | length), tried: .data.meta.tried}'
```
Expected: `count >= 1`, `tried[0] == "animepahe"`.

- [ ] **Step 2: Servers + stream (one-liner since the shape is identical)**

Run:
```bash
PE_EP_ID=$(curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/episodes?prefer=animepahe" | jq -r '.data.episodes[0].id')
PE_SRV_ID=$(curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/servers?episode=$PE_EP_ID&prefer=animepahe" | jq -r '.data.servers[0].id')
curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/stream?episode=$PE_EP_ID&server=$PE_SRV_ID&category=sub&prefer=animepahe" \
  | jq '.data.sources[0].type, (.data.sources[0].url[:80])'
```
Expected: a type (`"hls"` or `"mp4"`) and a URL prefix. AnimePahe streams come from kwik.cx — URL should host-match `kwik`.

- [ ] **Step 3: Smoke-fetch animepahe stream URL**

Run:
```bash
PE_URL=$(curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/stream?episode=$PE_EP_ID&server=$PE_SRV_ID&category=sub&prefer=animepahe" | jq -r '.data.sources[0].url')
curl -sf -o /dev/null -w "pahe %{http_code} %{size_download} bytes\n" "$PE_URL"
```
Expected: `pahe 200 NNNN bytes`. AnimePahe sometimes returns the playlist with a `Referer` requirement; if `200 0`, try with `-H "Referer: https://kwik.cx/"` to confirm the URL is valid but Referer-gated (proxy will handle this in-browser).

### Task 0.4: Verify animekai escape-hatch behaves correctly (only if enabled)

**Files:** none (commands only)

The spec says AnimeKai is escape-hatched: every Provider method returns `domain.ErrProviderDown`. We need to confirm the orchestrator handles this — picking AnimeKai must transparently fall through to the next provider, not surface an error.

- [ ] **Step 1: Check whether AnimeKai is enabled in this env**

Run:
```bash
grep "SCRAPER_ANIMEKAI_ENABLED" docker/.env 2>/dev/null || echo "not set, defaulting to false"
```
If unset or `false`, AnimeKai is not registered with the orchestrator. **Skip the rest of Task 0.4 with a note in the verification log.** This is the production-normal state.

- [ ] **Step 2: (Only if enabled) Confirm fall-through behavior**

Run:
```bash
curl -sf "http://localhost:8000/api/anime/$ANIME_ID/scraper/episodes?prefer=animekai" \
  | jq '{count: (.data.episodes | length), tried: .data.meta.tried}'
```
Expected: `count >= 1` AND `tried` contains `"animekai"` followed by `"gogoanime"` (or `"animepahe"`). The orchestrator should try AnimeKai first (per `prefer`), see `ErrProviderDown`, log the fallback, and serve gogoanime data. If `tried == ["animekai"]` and count is 0, the orchestrator isn't falling through correctly — surface this and stop A.1.

### Task 0.5: Record the verification result and commit it

**Files:**
- Create: `docs/issues/scraper-provider-verification-2026-05-19.md`

- [ ] **Step 1: Write a one-page verification log**

Create the file with this exact structure (replace the bracketed values with the real curl outputs):

```markdown
# Scraper Provider Verification — 2026-05-19

**Date:** 2026-05-19
**Operator:** [your handle]
**Purpose:** Pre-A.1 gate per `docs/superpowers/plans/2026-05-19-english-scraper-reconnect.md`.

## Test anime
- Title: Frieren: Beyond Journey's End
- MAL ID: 52991
- Local UUID: [from Task 0.1 Step 1]

## Provider results

| Provider | Episodes | Servers (ep1) | Stream URL fetched | Notes |
|----------|----------|---------------|--------------------|-------|
| gogoanime | [count] | [count] | 200 / [bytes] | [any notes] |
| animepahe | [count] | [count] | 200 / [bytes] | Referer-gated: y/n |
| animekai | n/a (escape-hatched) | n/a | n/a | Fall-through verified: y/n/disabled |

## Verdict

- A.1 unblocked: **YES** / **NO**
- Blocking issues (if any):
```

- [ ] **Step 2: Commit**

```bash
git add docs/issues/scraper-provider-verification-2026-05-19.md
git commit -m "$(cat <<'EOF'
docs(scraper): provider verification log for English reconnect Phase 0

Pre-A.1 gate per plan 2026-05-19-english-scraper-reconnect.md.
Verified gogoanime + animepahe episodes/servers/stream against
Frieren (MAL 52991). AnimeKai fall-through verified [or noted disabled].

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Phase A.1: EnglishPlayer.vue + EN tab

### Task A1.1: Add i18n keys for the EN tab + player

**Files:**
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ru.json`
- Modify: `frontend/web/src/locales/ja.json`

- [ ] **Step 1: Add `videoTab.english` to all three locales**

In each file, find the `"videoTab"` object (it already contains `"ru"`, `"raw"`, etc. — locate by searching for `"videoTab"`). Add `"english"` key after `"ru"`:

`en.json`:
```json
    "english": "English",
```
`ru.json`:
```json
    "english": "Английский",
```
`ja.json`:
```json
    "english": "英語",
```

- [ ] **Step 2: Add `player.english*` keys**

In each file, find the `"player"` object. Add these keys (alphabetize within the object as the file convention dictates):

`en.json`:
```json
    "englishEmpty": "No English episodes available — try Kodik or AnimeLib.",
    "englishUnavailable": "English sources temporarily unavailable. Try Kodik or AnimeLib.",
    "serverPicker": "Server",
    "categorySub": "Sub",
    "categoryDub": "Dub",
```
`ru.json`:
```json
    "englishEmpty": "Английских серий нет — попробуйте Kodik или AnimeLib.",
    "englishUnavailable": "Английские источники временно недоступны. Попробуйте Kodik или AnimeLib.",
    "serverPicker": "Сервер",
    "categorySub": "Сабы",
    "categoryDub": "Дубляж",
```
`ja.json`:
```json
    "englishEmpty": "英語のエピソードがありません — Kodik か AnimeLib をお試しください。",
    "englishUnavailable": "英語のソースは一時的に利用できません。Kodik か AnimeLib をお試しください。",
    "serverPicker": "サーバー",
    "categorySub": "字幕",
    "categoryDub": "吹替",
```

- [ ] **Step 3: Run the i18n key lint to confirm parity**

Run:
```bash
cd frontend/web && bun run lint:i18n 2>&1 | tail -10
```
Expected: `Missing keys: 0`. If non-zero, the three locales drifted — re-add the missing key in the relevant file.

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "$(cat <<'EOF'
feat(i18n): keys for EN tab + EnglishPlayer (A.1)

videoTab.english, player.englishEmpty, player.englishUnavailable,
player.serverPicker, player.categorySub, player.categoryDub.
Parity verified via bun run lint:i18n.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A1.2: Create the EnglishPlayer skeleton (episodes list only)

**Files:**
- Create: `frontend/web/src/components/player/EnglishPlayer.vue`

- [ ] **Step 1: Write the skeleton with episode fetching**

Full file content (this is the starting point; Tasks A1.3–A1.7 extend it):

```vue
<template>
  <div class="english-player english-player-wrapper">
    <!-- Loading episodes -->
    <div v-if="loadingEpisodes" class="flex items-center justify-center py-20">
      <div class="w-10 h-10 border-2 border-purple-400 border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- Empty / unavailable states -->
    <div
      v-else-if="fatalError"
      class="text-center py-20 text-pink-400"
      role="alert"
    >
      {{ fatalError }}
    </div>
    <div
      v-else-if="episodes.length === 0"
      class="text-center py-20 text-white/60"
    >
      <svg class="w-12 h-12 mx-auto mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
      </svg>
      {{ $t('player.englishEmpty') }}
    </div>

    <!-- Main two-column layout: video + episode sidebar -->
    <div v-else class="flex flex-col lg:flex-row gap-4">
      <div class="flex-1 min-w-0">
        <div class="relative aspect-video bg-black rounded-xl overflow-hidden flex items-center justify-center text-white/40">
          <!-- video element + overlays land here in Task A1.5 -->
          Episode {{ selectedEpisode?.number }} selected
        </div>
      </div>

      <aside class="lg:w-72 shrink-0">
        <ul class="max-h-[60vh] overflow-y-auto pr-1 space-y-1">
          <li v-for="ep in episodes" :key="ep.id">
            <button
              type="button"
              class="w-full text-left px-3 py-2 rounded-md text-sm transition-colors"
              :class="selectedEpisode?.id === ep.id
                ? 'bg-purple-500/20 text-white'
                : 'text-white/70 hover:bg-white/5'"
              @click="selectEpisode(ep)"
            >
              {{ $t('player.episode', { n: ep.number }) }}
              <span v-if="ep.title" class="text-white/40">— {{ ep.title }}</span>
            </button>
          </li>
        </ul>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { scraperApi } from '@/api/client'

interface ScraperEpisode {
  id: string
  number: number
  title?: string
}

interface Props {
  animeId: string
  animeName: string
  totalEpisodes?: number
  initialEpisode?: number
}

const props = defineProps<Props>()

const loadingEpisodes = ref(true)
const fatalError = ref<string | null>(null)
const episodes = ref<ScraperEpisode[]>([])
const selectedEpisode = ref<ScraperEpisode | null>(null)

async function loadEpisodes() {
  loadingEpisodes.value = true
  fatalError.value = null
  try {
    const resp = await scraperApi.getEpisodes(props.animeId)
    episodes.value = resp.data?.data?.episodes ?? []
    if (episodes.value.length > 0) {
      const startNum = props.initialEpisode ?? 1
      selectedEpisode.value =
        episodes.value.find((e) => e.number === startNum) ?? episodes.value[0]
    }
  } catch (err: unknown) {
    const status = (err as { response?: { status?: number } })?.response?.status
    if (status === 503) {
      // Translated below via $t in template; the ref holds the resolved string.
      // Imported locally so the component is i18n-aware without prop drilling.
      const { t } = await import('vue-i18n').then((m) => m.useI18n())
      fatalError.value = t('player.englishUnavailable')
    } else {
      fatalError.value = String((err as { message?: string })?.message ?? err)
    }
    episodes.value = []
  } finally {
    loadingEpisodes.value = false
  }
}

function selectEpisode(ep: ScraperEpisode) {
  selectedEpisode.value = ep
}

onMounted(loadEpisodes)
</script>

<style scoped>
.english-player-wrapper {
  width: 100%;
}
</style>
```

- [ ] **Step 2: Type-check**

Run:
```bash
cd frontend/web && bunx tsc --noEmit 2>&1 | tail -30
```
Expected: no errors mentioning `EnglishPlayer.vue`. The dynamic `vue-i18n` import in the catch block is awkward — replace it with the top-level `useI18n()` once the test passes. (See Step 3.)

- [ ] **Step 3: Move `useI18n()` to setup top-level**

Edit the script block — replace the dynamic import inside `loadEpisodes` with a top-level call:

Replace:
```ts
import { ref, onMounted } from 'vue'
import { scraperApi } from '@/api/client'
```
with:
```ts
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { scraperApi } from '@/api/client'
```

Add right under `const props = defineProps<Props>()`:
```ts
const { t } = useI18n()
```

Replace the catch block's dynamic import:
```ts
    if (status === 503) {
      const { t } = await import('vue-i18n').then((m) => m.useI18n())
      fatalError.value = t('player.englishUnavailable')
    } else {
```
with:
```ts
    if (status === 503) {
      fatalError.value = t('player.englishUnavailable')
    } else {
```

Re-run `bunx tsc --noEmit` — expect zero errors.

- [ ] **Step 4: Commit (skeleton only, NOT yet wired into Anime.vue)**

```bash
git add frontend/web/src/components/player/EnglishPlayer.vue
git commit -m "$(cat <<'EOF'
feat(player): EnglishPlayer.vue skeleton with episode list (A.1)

Renders episodes from scraperApi.getEpisodes. Empty/error states
wired. Video element + server picker land in subsequent commits.
Not yet mounted in Anime.vue — that's the next task.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A1.3: Wire EnglishPlayer into Anime.vue (type unions, tab, player mount)

**Files:**
- Modify: `frontend/web/src/views/Anime.vue`

- [ ] **Step 1: Widen the language and provider whitelists**

Find this block (lines 1117–1130 from the recent localStorage-sanitization fix; search for `VALID_LANGUAGES`):

```ts
const VALID_LANGUAGES = ['ru', '18+', 'raw'] as const
type VideoLanguage = (typeof VALID_LANGUAGES)[number]
const _savedLang = localStorage.getItem('preferred_video_language')
const videoLanguage = ref<VideoLanguage>(
  (VALID_LANGUAGES as readonly string[]).includes(_savedLang ?? '') ? (_savedLang as VideoLanguage) : 'ru'
)
const VALID_PROVIDERS = ['kodik', 'animelib', 'hanime', 'raw'] as const
```

Replace with:

```ts
const VALID_LANGUAGES = ['ru', 'en', '18+', 'raw'] as const
type VideoLanguage = (typeof VALID_LANGUAGES)[number]
const _savedLang = localStorage.getItem('preferred_video_language')
const videoLanguage = ref<VideoLanguage>(
  (VALID_LANGUAGES as readonly string[]).includes(_savedLang ?? '') ? (_savedLang as VideoLanguage) : 'ru'
)
const VALID_PROVIDERS = ['kodik', 'animelib', 'english', 'hanime', 'raw'] as const
```

- [ ] **Step 2: Import the new component**

Find the existing `import KodikPlayer from '...'` block (search for `from '@/components/player/KodikPlayer.vue'`). Add right after the imports for the other players:

```ts
import EnglishPlayer from '@/components/player/EnglishPlayer.vue'
```

- [ ] **Step 3: Add the EN tab button**

Find the RU tab button block (search for `switchLanguage('ru')` — should be around line 358). Immediately after the RU `</button>` closing tag and before the `v-if="isHentai"` 18+ button, insert:

```vue
              <button
                @click="switchLanguage('en')"
                :aria-pressed="videoLanguage === 'en'"
                class="px-3 py-1.5 rounded-md text-sm font-medium transition-all"
                :class="videoLanguage === 'en'
                  ? 'bg-white/15 text-white'
                  : 'text-white/50 hover:text-white/70'"
              >
                {{ $t('videoTab.english') }}
              </button>
```

- [ ] **Step 4: Add the EnglishPlayer mount branch**

Find the player v-if/v-else-if chain (around line 477; search for `videoProvider === 'kodik'`). Add a new branch after the AnimeLib block and before the Hanime block:

Find:
```vue
            <!-- Hanime Player -->
            <HanimePlayer
              v-else-if="videoProvider === 'hanime'"
```

Insert immediately above:
```vue
            <!-- English Player (scraper microservice — gogoanime/animepahe) -->
            <EnglishPlayer
              v-else-if="videoProvider === 'english'"
              :anime-id="anime.id"
              :anime-name="anime.title"
              :total-episodes="anime.totalEpisodes"
              :initial-episode="resumeStartEpisode"
            />
```

- [ ] **Step 5: Update `switchLanguage` to handle 'en'**

Find the `function switchLanguage` definition (search for `function switchLanguage`). Locate the body that sets `videoProvider.value` based on the chosen language. Add an `'en'` branch:

Find the chain (it currently handles `'ru'`, `'18+'`, `'raw'`):
```ts
function switchLanguage(lang: VideoLanguage) {
  videoLanguage.value = lang
  // ... existing branches
}
```

Wherever the function picks a default provider per language, add:
```ts
  } else if (lang === 'en') {
    const savedEn = localStorage.getItem('preferred_en_provider')
    videoProvider.value = savedEn === 'english' ? 'english' : 'english'
  }
```

(There's only one EN provider in A.1, so the saved-value check is forward-compatible but always lands on `'english'`. This pattern matches how the function handles single-provider languages today.)

- [ ] **Step 6: Update the videoProvider save watcher for 'en'**

Find the watcher (search for `watch(videoProvider`):

```ts
watch(videoProvider, (newProvider) => {
  localStorage.setItem('preferred_video_provider', newProvider)
  if (videoLanguage.value === 'ru') {
    localStorage.setItem('preferred_ru_provider', newProvider)
  } else if (videoLanguage.value === 'raw') {
    localStorage.setItem('preferred_raw_provider', newProvider)
  }
})
```

Add an `'en'` branch:
```ts
watch(videoProvider, (newProvider) => {
  localStorage.setItem('preferred_video_provider', newProvider)
  if (videoLanguage.value === 'ru') {
    localStorage.setItem('preferred_ru_provider', newProvider)
  } else if (videoLanguage.value === 'raw') {
    localStorage.setItem('preferred_raw_provider', newProvider)
  } else if (videoLanguage.value === 'en') {
    localStorage.setItem('preferred_en_provider', newProvider)
  }
})
```

- [ ] **Step 7: Update the stale-localStorage comment at line ~1118**

Find:
```ts
// EN player ('english' / 'hianime' / 'consumet') would otherwise hit a value
```

Replace the whole comment with:
```ts
// Sanitize localStorage values against the current whitelists — the 2026-05-18
// cleanup narrowed the unions, and stale strings from older builds (or future
// renames) need to fall back to the default rather than crash the v-if chain.
```

- [ ] **Step 8: Type-check**

Run:
```bash
cd frontend/web && bunx tsc --noEmit 2>&1 | tail -30
```
Expected: zero errors.

- [ ] **Step 9: ESLint**

Run:
```bash
cd frontend/web && bunx eslint src/views/Anime.vue src/components/player/EnglishPlayer.vue 2>&1 | tail -20
```
Expected: zero errors. Warnings are OK.

- [ ] **Step 10: Commit**

```bash
git add frontend/web/src/views/Anime.vue
git commit -m "$(cat <<'EOF'
feat(anime): mount EnglishPlayer + wire EN tab (A.1)

Widens videoLanguage/videoProvider whitelists, adds EN tab button
between RU and 18+, adds v-else-if branch mounting EnglishPlayer.
switchLanguage and the videoProvider save watcher learn about 'en'.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A1.4: Add server picker + stream loading + hls.js attachment

**Files:**
- Modify: `frontend/web/src/components/player/EnglishPlayer.vue`

- [ ] **Step 1: Extend the script block with server/stream state**

Find the existing script block in `EnglishPlayer.vue`. Add these interfaces above the existing `ScraperEpisode` interface:

```ts
interface ScraperServer {
  id: string
  name: string
  type: 'sub' | 'dub' | 'raw'
}

interface ScraperSubtitle {
  url: string
  lang: string
  default?: boolean
}

interface ScraperSource {
  url: string
  type: 'hls' | 'mp4'
  quality?: string
}

interface ScraperStream {
  sources: ScraperSource[]
  tracks?: ScraperSubtitle[]
  intro?: { start: number; end: number }
  outro?: { start: number; end: number }
}
```

- [ ] **Step 2: Add server/stream refs**

Below the existing `selectedEpisode` ref, add:

```ts
const loadingServers = ref(false)
const loadingStream = ref(false)
const streamError = ref<string | null>(null)
const servers = ref<ScraperServer[]>([])
const selectedServer = ref<ScraperServer | null>(null)
const category = ref<'sub' | 'dub'>('sub')
const stream = ref<ScraperStream | null>(null)
const videoRef = ref<HTMLVideoElement | null>(null)
let hlsInstance: import('hls.js').default | null = null
```

- [ ] **Step 3: Add server + stream loaders**

Below the `selectEpisode` function, add:

```ts
async function loadServers(ep: ScraperEpisode) {
  loadingServers.value = true
  streamError.value = null
  servers.value = []
  selectedServer.value = null
  try {
    const resp = await scraperApi.getServers(props.animeId, ep.id)
    servers.value = resp.data?.data?.servers ?? []
    if (servers.value.length > 0) {
      selectedServer.value = servers.value[0]
    }
  } catch (err) {
    streamError.value = String((err as { message?: string })?.message ?? err)
  } finally {
    loadingServers.value = false
  }
}

async function loadStream(ep: ScraperEpisode, srv: ScraperServer, cat: 'sub' | 'dub', isRetry = false) {
  loadingStream.value = true
  streamError.value = null
  destroyHls()
  stream.value = null
  try {
    const resp = await scraperApi.getStream(props.animeId, ep.id, srv.id, cat)
    stream.value = resp.data?.data ?? null
    await attachHls()
  } catch (err) {
    if (!isRetry) {
      // One-shot retry for expired stream URLs (TTL ~5 min server-side).
      await loadStream(ep, srv, cat, true)
      return
    }
    streamError.value = String((err as { message?: string })?.message ?? err)
  } finally {
    loadingStream.value = false
  }
}

function destroyHls() {
  if (hlsInstance) {
    hlsInstance.destroy()
    hlsInstance = null
  }
}

async function attachHls() {
  const src = stream.value?.sources?.[0]
  if (!src || !videoRef.value) return
  if (src.type === 'mp4') {
    videoRef.value.src = src.url
    return
  }
  // HLS branch
  if (videoRef.value.canPlayType('application/vnd.apple.mpegurl')) {
    // Safari native HLS
    videoRef.value.src = src.url
    return
  }
  const Hls = (await import('hls.js')).default
  if (!Hls.isSupported()) {
    streamError.value = 'HLS not supported in this browser'
    return
  }
  hlsInstance = new Hls()
  hlsInstance.loadSource(src.url)
  hlsInstance.attachMedia(videoRef.value)
}
```

- [ ] **Step 4: Wire selectEpisode + watchers**

Replace the existing `selectEpisode` function with:

```ts
function selectEpisode(ep: ScraperEpisode) {
  selectedEpisode.value = ep
  void loadServers(ep)
}
```

Add watchers below the function:

```ts
import { watch } from 'vue'  // add to the existing 'vue' import line at top

watch(selectedServer, (srv) => {
  if (srv && selectedEpisode.value) {
    void loadStream(selectedEpisode.value, srv, category.value)
  }
})

watch(category, (cat) => {
  if (selectedServer.value && selectedEpisode.value) {
    void loadStream(selectedEpisode.value, selectedServer.value, cat)
  }
})
```

Also extend the existing `onMounted(loadEpisodes)` — after `loadEpisodes` completes and an initial `selectedEpisode` is set, trigger `loadServers`. Replace `onMounted(loadEpisodes)` with:

```ts
onMounted(async () => {
  await loadEpisodes()
  if (selectedEpisode.value) {
    await loadServers(selectedEpisode.value)
  }
})
```

Add cleanup:
```ts
import { onBeforeUnmount } from 'vue'  // add to existing 'vue' import

onBeforeUnmount(destroyHls)
```

- [ ] **Step 5: Replace the placeholder video area in the template**

Find this placeholder block in the template:
```vue
        <div class="relative aspect-video bg-black rounded-xl overflow-hidden flex items-center justify-center text-white/40">
          <!-- video element + overlays land here in Task A1.5 -->
          Episode {{ selectedEpisode?.number }} selected
        </div>
```

Replace with:

```vue
        <div class="relative aspect-video bg-black rounded-xl overflow-hidden">
          <!-- Loading overlay -->
          <div
            v-if="loadingStream || loadingServers"
            class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
          >
            <div class="text-center">
              <div class="w-10 h-10 border-2 border-purple-400 border-t-transparent rounded-full animate-spin mx-auto mb-3" />
              <p class="text-white/60 text-sm">{{ $t('player.loadingEpisode', { n: selectedEpisode?.number }) }}</p>
            </div>
          </div>

          <!-- Inline stream error -->
          <div
            v-if="streamError && !loadingStream"
            class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
            role="alert"
          >
            <div class="text-center text-pink-400 px-4">
              <p>{{ streamError }}</p>
            </div>
          </div>

          <!-- Video element -->
          <video
            v-if="stream && !streamError"
            ref="videoRef"
            class="absolute inset-0 w-full h-full"
            controls
            playsinline
            autoplay
            @error="onVideoError"
          />
        </div>

        <!-- Server picker + sub/dub category -->
        <div v-if="servers.length > 0" class="mt-3 flex flex-wrap items-center gap-2">
          <label class="text-sm text-white/60">{{ $t('player.serverPicker') }}:</label>
          <select
            v-model="selectedServer"
            class="bg-white/5 border border-white/10 rounded-md text-white text-sm px-2 py-1"
          >
            <option v-for="srv in servers" :key="srv.id" :value="srv">
              {{ srv.name }}
            </option>
          </select>
          <div class="ml-2 inline-flex rounded-md border border-white/10 overflow-hidden">
            <button
              type="button"
              class="px-2 py-1 text-xs"
              :class="category === 'sub' ? 'bg-purple-500/30 text-white' : 'text-white/60'"
              @click="category = 'sub'"
            >{{ $t('player.categorySub') }}</button>
            <button
              type="button"
              class="px-2 py-1 text-xs"
              :class="category === 'dub' ? 'bg-purple-500/30 text-white' : 'text-white/60'"
              @click="category = 'dub'"
            >{{ $t('player.categoryDub') }}</button>
          </div>
        </div>
```

- [ ] **Step 6: Add the `onVideoError` handler**

Add to the script block, near the other handlers:

```ts
function onVideoError() {
  if (!selectedEpisode.value || !selectedServer.value) return
  // The retry-once policy is implemented inside loadStream — re-invoking is
  // safe and self-limiting.
  void loadStream(selectedEpisode.value, selectedServer.value, category.value)
}
```

- [ ] **Step 7: Type-check and lint**

Run:
```bash
cd frontend/web && bunx tsc --noEmit 2>&1 | tail -30 && bunx eslint src/components/player/EnglishPlayer.vue 2>&1 | tail -10
```
Expected: zero errors.

- [ ] **Step 8: Commit**

```bash
git add frontend/web/src/components/player/EnglishPlayer.vue
git commit -m "$(cat <<'EOF'
feat(player): EnglishPlayer server picker + hls.js streaming (A.1)

Adds server/category picker, fetches stream via scraperApi.getStream,
attaches hls.js for HLS sources (with native Safari fallback). Implements
the one-shot retry policy from the spec for expired stream URLs.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A1.5: Wire SubtitleOverlay + ReportButton + progress tracking

**Files:**
- Modify: `frontend/web/src/components/player/EnglishPlayer.vue`

- [ ] **Step 1: Import the shared components**

Add to the script block imports:

```ts
import SubtitleOverlay from '@/components/player/SubtitleOverlay.vue'
import OtherSubsPanel from '@/components/player/OtherSubsPanel.vue'
import ReportButton from '@/components/player/ReportButton.vue'
```

- [ ] **Step 2: Add emit declarations**

Below `const props = defineProps<Props>()` add:

```ts
const emit = defineEmits<{
  progress: [time: number, duration: number]
  'episode-change': [episodeNumber: number]
  ended: []
}>()
```

- [ ] **Step 3: Wire `@timeupdate`, `@pause`, `@ended` on the `<video>` element**

Find the `<video>` element in the template. Replace its event handlers:

```vue
          <video
            v-if="stream && !streamError"
            ref="videoRef"
            class="absolute inset-0 w-full h-full"
            controls
            playsinline
            autoplay
            @timeupdate="onTimeUpdate"
            @ended="onEnded"
            @error="onVideoError"
          />
```

Add the handlers in the script block:

```ts
function onTimeUpdate() {
  const v = videoRef.value
  if (!v) return
  emit('progress', v.currentTime, v.duration || 0)
}

function onEnded() {
  emit('ended')
}
```

Also update `selectEpisode` to emit:

```ts
function selectEpisode(ep: ScraperEpisode) {
  selectedEpisode.value = ep
  emit('episode-change', ep.number)
  void loadServers(ep)
}
```

- [ ] **Step 4: Add SubtitleOverlay + OtherSubsPanel + ReportButton to template**

Inside the main `<div v-else class="flex flex-col lg:flex-row gap-4">` block, inside the left `<div class="flex-1 min-w-0">`, after the server-picker block, add:

```vue
        <SubtitleOverlay
          v-if="videoRef && selectedEpisode"
          :video-element="videoRef"
          :anime-id="props.animeId"
          :episode-number="selectedEpisode.number"
        />

        <OtherSubsPanel
          v-if="selectedEpisode"
          :anime-id="props.animeId"
          :episode-number="selectedEpisode.number"
          class="mt-3"
        />

        <div class="mt-3 flex justify-end">
          <ReportButton
            :anime-id="props.animeId"
            :anime-name="props.animeName"
            :episode-number="selectedEpisode?.number ?? 0"
            player-type="english"
            accent-color="#a855f7"
          />
        </div>
```

(The ReportButton's `player-type` is the canonical identifier the player service uses to bucket reports — `"english"` is consistent with the type unions widened in `frontend/web/src/types/preference.ts` and the validated value set in `services/player/internal/handler/report.go`. If that handler's `allowedPlayerTypes` map does not yet contain `"english"`, Task A1.6 will add it.)

- [ ] **Step 5: Type-check and lint**

Run:
```bash
cd frontend/web && bunx tsc --noEmit 2>&1 | tail -30 && bunx eslint src/components/player/EnglishPlayer.vue 2>&1 | tail -10
```
Expected: zero errors. The `SubtitleOverlay` / `OtherSubsPanel` props are inferred from those components — if either complains about a missing prop, open the relevant `.vue` file and match its `defineProps` exactly.

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/player/EnglishPlayer.vue
git commit -m "$(cat <<'EOF'
feat(player): SubtitleOverlay + OtherSubsPanel + ReportButton in EnglishPlayer (A.1)

Wires the shared JP subtitle overlay, the Jimaku subtitle panel, and
the ReportButton. Adds progress/episode-change/ended emits matching
the other players' contract for watch-history tracking.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A1.6: Backend allow-list for `english` reports

**Files:**
- Modify: `services/player/internal/handler/report.go`
- Modify: `services/player/internal/domain/preference.go`

- [ ] **Step 1: Add `"english"` to the report handler's allowed player types**

Open `services/player/internal/handler/report.go`. Search for `allowedPlayerTypes`:

```go
var allowedPlayerTypes = map[string]bool{
  "kodik":    true,
  "animelib": true,
}
```

Add the new entry:
```go
var allowedPlayerTypes = map[string]bool{
  "kodik":    true,
  "animelib": true,
  "english":  true,
}
```

- [ ] **Step 2: Add `"english"` to the player preference allow-list**

Open `services/player/internal/domain/preference.go`. Search for `ValidPlayers`:

```go
var ValidPlayers = map[string]bool{
  "kodik":    true,
  "animelib": true,
}
```

Replace with:
```go
var ValidPlayers = map[string]bool{
  "kodik":    true,
  "animelib": true,
  "english":  true,
}
```

Also update the `Player` field comment on the struct (search for `// kodik, animelib`):
```go
Player string // kodik, animelib, english
```

- [ ] **Step 3: Build and test the player service**

Run:
```bash
cd services/player && go build ./... && go test ./... 2>&1 | tail -20
```
Expected: build success, all tests pass.

- [ ] **Step 4: Redeploy player**

Run:
```bash
make redeploy-player
```
Wait for health.

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/handler/report.go services/player/internal/domain/preference.go
git commit -m "$(cat <<'EOF'
feat(player): allow 'english' player type for reports + preferences (A.1)

ValidPlayers and allowedPlayerTypes both grow an 'english' entry so
EnglishPlayer.vue's ReportButton + the preference-save path don't 422.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A1.7: Playwright e2e for the EN tab

**Files:**
- Create: `frontend/web/tests/e2e/english-player.spec.ts`

- [ ] **Step 1: Write the test**

Full file content:

```ts
import { test, expect } from '@playwright/test'

const TEST_ANIME_SLUG = 'frieren-beyond-journey-s-end'

test.describe('English player', () => {
  test('EN tab loads episodes and plays episode 1', async ({ page }) => {
    await page.goto(`/anime/${TEST_ANIME_SLUG}`)
    await page.waitForLoadState('networkidle')

    const enTab = page.locator('button[aria-pressed]', { hasText: /English|Английский|英語/ })
    await expect(enTab).toBeVisible({ timeout: 10_000 })

    await enTab.click()
    await expect(enTab).toHaveAttribute('aria-pressed', 'true')

    const episodeList = page.locator('.english-player aside ul li button').first()
    await expect(episodeList).toBeVisible({ timeout: 15_000 })

    await episodeList.click()

    const video = page.locator('.english-player video')
    await expect(video).toBeVisible({ timeout: 20_000 })

    await page.waitForFunction(
      () => {
        const v = document.querySelector('.english-player video') as HTMLVideoElement | null
        return v != null && v.readyState >= 2
      },
      { timeout: 20_000 }
    )
  })

  test('EN tab shows empty-state copy when episodes are unavailable', async ({ page }) => {
    await page.route('**/api/anime/*/scraper/episodes**', (route) => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ data: { episodes: [], meta: { tried: ['gogoanime', 'animepahe'] } } }),
      })
    })

    await page.goto(`/anime/${TEST_ANIME_SLUG}`)
    await page.waitForLoadState('networkidle')

    const enTab = page.locator('button[aria-pressed]', { hasText: /English|Английский|英語/ })
    await enTab.click()

    const empty = page.locator('.english-player').getByText(/No English episodes|Английских серий|英語のエピソード/)
    await expect(empty).toBeVisible({ timeout: 10_000 })
  })
})
```

- [ ] **Step 2: Run against the locally deployed stack**

Run:
```bash
cd frontend/web && bunx playwright test english-player --reporter=list 2>&1 | tail -40
```
Expected: both tests pass. If the first test fails because the Frieren UUID isn't in the local DB, open the page once in a browser to trigger on-demand population, then re-run.

- [ ] **Step 3: Commit**

```bash
git add frontend/web/tests/e2e/english-player.spec.ts
git commit -m "$(cat <<'EOF'
test(e2e): Playwright spec for EN tab + EnglishPlayer (A.1)

Covers the happy path (episode list loads, episode 1 plays) and the
empty-state branch (route-mocked empty episodes array).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A1.8: Redeploy + manual smoke + after-update

**Files:** none (orchestration only)

- [ ] **Step 1: Redeploy web**

Run:
```bash
make redeploy-web
```
Wait for the deploy to finish; verify with `make health` that web is healthy.

- [ ] **Step 2: Manual smoke (logged-out)**

In a private browser window, navigate to `https://animeenigma.ru/anime/frieren-beyond-journey-s-end`. Verify:
- EN tab visible between RU and 18+.
- Click EN → episode list renders within 5 seconds.
- Click episode 1 → video element appears and starts playing within 15 seconds.
- Open browser DevTools network tab; confirm no `4xx`/`5xx` from `/api/anime/*/scraper/*`.

- [ ] **Step 3: Manual smoke (logged-in as `ui_audit_bot`)**

Reuse `ui_audit_bot` per `CLAUDE.md`. Confirm:
- EN tab visible.
- Watch progress saves to history (open Profile → History → verify the episode appears).
- ReportButton opens the modal and the report submission succeeds (network tab shows `POST /api/players/report` returning `200`).

- [ ] **Step 4: Run `/animeenigma-after-update`**

Per `CLAUDE.md`: invoke this skill. It will run lint/build, redeploy any leftover services, update `frontend/web/public/changelog.json` with a user-facing entry (informative + enthusiastic + emojis), commit, and push.

A.1 is complete when the after-update skill exits cleanly and `git status` shows `Your branch is up to date with 'origin/main'`.

---

## Phase A.2: `has_english` column + browse filter

### Task A2.1: Add `HasEnglish` to the Anime GORM model

**Files:**
- Modify: `services/catalog/internal/domain/anime.go`

- [ ] **Step 1: Locate the Anime struct and add the field**

Search for `HasKodik` or `HasAnimelib`. They look like:
```go
HasKodik    bool `gorm:"default:false" json:"has_kodik"`
HasAnimelib bool `gorm:"default:false" json:"has_animelib"`
```

Add immediately below:
```go
HasEnglish  bool `gorm:"default:false" json:"has_english"`
```

- [ ] **Step 2: Build to confirm**

Run:
```bash
cd services/catalog && go build ./...
```
Expected: no errors. The GORM AutoMigrate on next service start adds the column with default `false`.

- [ ] **Step 3: Commit**

```bash
git add services/catalog/internal/domain/anime.go
git commit -m "$(cat <<'EOF'
feat(catalog): add HasEnglish to Anime model (A.2)

GORM AutoMigrate adds the has_english column on next service restart.
Populated opportunistically by Task A2.4 when the scraper resolves
English episodes for the first time.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A2.2: Add `SetHasEnglish` repo method + provider-map entry

**Files:**
- Modify: `services/catalog/internal/repo/anime.go`

- [ ] **Step 1: Add the provider-map entry**

Search for the provider-to-column map (similar to `"kodik": "has_kodik", "animelib": "has_animelib"`). Add:
```go
"english": "has_english",
```

- [ ] **Step 2: Add the setter method**

Search for `func (r *AnimeRepo) SetHasKodik` to find the pattern. Add an analogous method:

```go
func (r *AnimeRepo) SetHasEnglish(ctx context.Context, animeID string, has bool) error {
  return r.db.WithContext(ctx).Model(&domain.Anime{}).
    Where("id = ?", animeID).
    Update("has_english", has).
    Error
}
```

- [ ] **Step 3: Build + test**

Run:
```bash
cd services/catalog && go build ./... && go test ./internal/repo/... 2>&1 | tail -20
```
Expected: build success, tests pass.

- [ ] **Step 4: Commit**

```bash
git add services/catalog/internal/repo/anime.go
git commit -m "$(cat <<'EOF'
feat(catalog): SetHasEnglish repo method + provider-map entry (A.2)

Mirrors SetHasKodik / SetHasAnimelib. Called opportunistically from
the catalog service when the scraper successfully resolves English
episodes for an anime.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A2.3: Add `"english"` to the providers filter switch case

**Files:**
- Modify: `services/catalog/internal/handler/catalog.go`

- [ ] **Step 1: Edit the switch case**

Search for `case "kodik", "animelib":`. Replace with:

```go
case "kodik", "animelib", "english":
```

- [ ] **Step 2: Build + test**

Run:
```bash
cd services/catalog && go build ./... && go test ./internal/handler/... 2>&1 | tail -20
```
Expected: build success, tests pass.

- [ ] **Step 3: Commit**

```bash
git add services/catalog/internal/handler/catalog.go
git commit -m "$(cat <<'EOF'
feat(catalog): accept 'english' in /anime providers filter (A.2)

Browse filter row activates as soon as has_english starts getting
populated by the opportunistic setter in Task A2.4.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A2.4: Opportunistic `has_english` setter on scraper-episodes success

**Files:**
- Modify: `services/catalog/internal/service/catalog.go`

- [ ] **Step 1: Locate the scraper proxy method**

Search for `GetScraperEpisodes` in `services/catalog/internal/service/catalog.go` (the catalog-side method that proxies the request to the scraper microservice).

- [ ] **Step 2: Add the setter call on success**

Inside that method, after the scraper response is parsed and the episode count is known to be > 0 and BEFORE the response is returned, add:

```go
if len(episodes) > 0 {
  // Opportunistic flag — fire-and-forget so a DB blip doesn't break the
  // user-facing scraper response. The setter is idempotent.
  go func(id string) {
    bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    if err := s.animeRepo.SetHasEnglish(bgCtx, id, true); err != nil {
      s.log.Warnw("failed to set has_english", "anime_id", id, "error", err)
    }
  }(animeID)
}
```

Imports may already exist; if not, add `"context"` and `"time"` to the import block.

- [ ] **Step 3: Build + test**

Run:
```bash
cd services/catalog && go build ./... && go test ./... 2>&1 | tail -20
```
Expected: success.

- [ ] **Step 4: Redeploy catalog**

Run:
```bash
make redeploy-catalog
```

- [ ] **Step 5: Verify backfill happens for the test anime**

Run:
```bash
# After hitting the EN tab on Frieren once, confirm the column flipped:
docker compose -f docker/docker-compose.yml exec -T postgres \
  psql -U postgres -d animeenigma \
  -c "SELECT id, name, has_english FROM animes WHERE shikimori_id = '52991';"
```
Expected: `has_english | t` after at least one EN-tab open.

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/service/catalog.go
git commit -m "$(cat <<'EOF'
feat(catalog): opportunistic has_english setter on scraper success (A.2)

When GetScraperEpisodes returns a non-empty episode list, the catalog
fire-and-forgets a SetHasEnglish(true) so the browse filter starts
matching real rows without needing a separate backfill job.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A2.5: Activate the BrowseSidebar provider row + composable + i18n

**Files:**
- Modify: `frontend/web/src/composables/useBrowseFilters.ts`
- Modify: `frontend/web/src/components/browse/BrowseSidebar.vue`
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ru.json`
- Modify: `frontend/web/src/locales/ja.json`

- [ ] **Step 1: Widen `Provider` union and `PROVIDER_VALUES`**

Open `frontend/web/src/composables/useBrowseFilters.ts`. Find the `Provider` type:

```ts
export type Provider = 'kodik' | 'animelib'
```

Replace with:
```ts
export type Provider = 'kodik' | 'animelib' | 'english'
```

Find the `PROVIDER_VALUES` constant. It looks like:
```ts
const PROVIDER_VALUES = ['kodik', 'animelib'] as const
```
Replace with:
```ts
const PROVIDER_VALUES = ['kodik', 'animelib', 'english'] as const
```

- [ ] **Step 2: Add the new provider row to BrowseSidebar**

Open `frontend/web/src/components/browse/BrowseSidebar.vue`. Find the `providerOptions` computed (lines 247–258). Currently:

```ts
const providerOptions = computed<{ value: Provider; label: string; accent: string }[]>(() => [
  {
    value: 'kodik',
    label: t('browse.filters.provider.kodik'),
    accent: 'text-cyan-500 focus:ring-cyan-500',
  },
  {
    value: 'animelib',
    label: t('browse.filters.provider.animelib'),
    accent: 'text-orange-500 focus:ring-orange-500',
  },
])
```

Add a third entry:
```ts
  {
    value: 'english',
    label: t('browse.filters.provider.english'),
    accent: 'text-purple-500 focus:ring-purple-500',
  },
```

- [ ] **Step 3: Add `browse.filters.provider.english` to all three locales**

Find `"provider"` inside `"browse.filters"` in each file. Add:

`en.json`:
```json
        "english": "English",
```
`ru.json`:
```json
        "english": "Английский",
```
`ja.json`:
```json
        "english": "英語",
```

- [ ] **Step 4: Lint + type-check + i18n parity**

Run:
```bash
cd frontend/web && bun run lint:i18n && bunx tsc --noEmit 2>&1 | tail -10 && bunx eslint src/composables/useBrowseFilters.ts src/components/browse/BrowseSidebar.vue 2>&1 | tail -10
```
Expected: `Missing keys: 0`, zero tsc errors, zero eslint errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/useBrowseFilters.ts \
  frontend/web/src/components/browse/BrowseSidebar.vue \
  frontend/web/src/locales/en.json \
  frontend/web/src/locales/ru.json \
  frontend/web/src/locales/ja.json
git commit -m "$(cat <<'EOF'
feat(browse): English provider filter row + Provider union widening (A.2)

Provider union grows 'english', BrowseSidebar renders a purple-accent
row, i18n key added to all three locales. Filter activates against the
has_english column populated by the catalog setter in the previous task.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A2.6: A.2 deployment + after-update

**Files:** none

- [ ] **Step 1: Redeploy web (catalog already redeployed in A2.4)**

Run:
```bash
make redeploy-web && make health
```

- [ ] **Step 2: Manual smoke**

Open `https://animeenigma.ru/browse`. Confirm the new "English" provider checkbox appears in the sidebar with a purple accent. Tick it; confirm the result grid narrows to anime where `has_english=true`. (Initially this may be a small set — only anime users have opened the EN tab for. That's expected behavior.)

- [ ] **Step 3: Run `/animeenigma-after-update`**

A.2 is complete when the skill exits cleanly.

---

## Phase A.3: Health-aware tab hiding

### Task A3.1: Health-check call + 60s cache in Anime.vue

**Files:**
- Modify: `frontend/web/src/views/Anime.vue`

- [ ] **Step 1: Add the health-fetching helper at module scope**

Inside `Anime.vue`'s script setup, near the top (above the `videoLanguage` ref), add:

```ts
import { computed as _computed } from 'vue'  // reuse existing 'vue' import — drop _ alias

// 60s in-process cache for /scraper/health so we don't hammer the endpoint
// when the user opens many anime pages.
let cachedHealth: { providers: Array<{ provider: string; stages: Record<string, { status: string }> }> } | null = null
let cachedAt = 0
const HEALTH_TTL_MS = 60_000

async function fetchScraperHealth() {
  if (cachedHealth && Date.now() - cachedAt < HEALTH_TTL_MS) {
    return cachedHealth
  }
  try {
    const { scraperApi } = await import('@/api/client')
    const resp = await scraperApi.getHealth()
    cachedHealth = resp.data?.data ?? null
    cachedAt = Date.now()
    return cachedHealth
  } catch {
    return null  // fail-open: show the EN tab anyway
  }
}
```

- [ ] **Step 2: Add a reactive `enTabAvailable` computed**

Add a `ref<boolean>(true)` and an onMounted hook to populate it:

```ts
const enTabAvailable = ref(true)

onMounted(async () => {
  const health = await fetchScraperHealth()
  if (!health) return  // fail-open
  const anyUp = health.providers.some((p) =>
    Object.values(p.stages).some((s) => s.status === 'UP')
  )
  enTabAvailable.value = anyUp
})
```

(If `onMounted` is already imported and used elsewhere in `Anime.vue`, append to or merge with the existing hook rather than declaring a second one.)

- [ ] **Step 3: Gate the EN tab button on `enTabAvailable`**

Find the EN tab button added in Task A1.3 Step 3. Add `v-if="enTabAvailable"` to it:

```vue
              <button
                v-if="enTabAvailable"
                @click="switchLanguage('en')"
                ...
```

- [ ] **Step 4: Type-check + lint**

Run:
```bash
cd frontend/web && bunx tsc --noEmit 2>&1 | tail -10 && bunx eslint src/views/Anime.vue 2>&1 | tail -10
```
Expected: zero errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/views/Anime.vue
git commit -m "$(cat <<'EOF'
feat(anime): hide EN tab when all scraper providers DOWN (A.3)

scraperApi.getHealth() is called once on mount with a 60s in-process
cache. The EN tab v-if's on enTabAvailable; fail-open behavior on
health-endpoint errors keeps the tab visible.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

### Task A3.2: A.3 deployment + after-update

**Files:** none

- [ ] **Step 1: Redeploy + smoke**

Run:
```bash
make redeploy-web
```

Open any anime page. Confirm the EN tab still shows (because both providers are UP). To verify the hide path actually works, temporarily set `SCRAPER_DEGRADED_PROVIDERS=gogoanime,animepahe` in `docker/.env`, restart the scraper, hard-reload an anime page — EN tab should disappear within the 60s cache window. **Revert the env change after testing.**

- [ ] **Step 2: Run `/animeenigma-after-update`**

A.3 (and Project A) is complete when the skill exits cleanly.

---

## Self-Review (post-write)

Done as part of writing this plan:

**1. Spec coverage:**
- Spec §"Components" 1–6: covered by Tasks A1.2, A1.3, A1.4, A1.5, A2.5, A3.1.
- Spec §"Phased rollout" A.1/A.2/A.3: covered by the three Phase sections.
- Spec §"Error handling" matrix: covered inside Tasks A1.4 (stream retry, 503), A1.5 (ReportButton).
- Spec §"Testing strategy" — Playwright e2e: Task A1.7. Manual smoke: Task A1.8 Steps 2–3. Backend verification: Phase 0 (and the user explicitly asked for this).
- Spec §"Done definition" 1–5: A1.8 Step 4 (`/animeenigma-after-update`) covers items 4–5; item 1 by A1.8 Step 2; item 2 by A3.2 Step 1; item 3 by A2.6 Step 2.

**2. Placeholder scan:** no TBD/TODO/"implement later"/"add appropriate error handling" patterns. Every code step ships actual code.

**3. Type consistency:**
- `ScraperEpisode`, `ScraperServer`, `ScraperStream`, `ScraperSource`, `ScraperSubtitle` defined once in Task A1.2 / A1.4 and re-used unchanged.
- `Provider` union changes from A.2 happen entirely inside Task A2.5; consistent across composable + sidebar.
- `'english'` literal appears in: `VALID_PROVIDERS` (A1.3), `videoProvider` v-else-if (A1.3), `ValidPlayers` map (A1.6), `allowedPlayerTypes` map (A1.6), `Provider` type (A2.5), `providers` switch case (A2.3), `providerOptions` list (A2.5). All match.

No gaps.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-19-english-scraper-reconnect.md`. Two execution options:

**1. Subagent-Driven (recommended)** — fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
