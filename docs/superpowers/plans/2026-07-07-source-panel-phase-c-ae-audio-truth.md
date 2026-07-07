# Source Panel — Phase C: `ae` reports real audio / language / quality — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the first-party `ae` source report its **real** audio kind, language, and quality per title so a self-hosted English dub (e.g. Black Lagoon) surfaces under the **DUB / EN** slider instead of the fabricated "SUB · 1080p" trait, then label + ingest the Black Lagoon franchise (S1 backfill, S2 + OVA ingest) as English dub.

**Architecture:** The library records the audio language the ingest already knows (`library-batchingest -audio-lang` → new `audio_lang` + `quality` columns + `Track=dub`), surfaces it through the existing library episodes endpoint → catalog `AeTitleInfo`, and `aeFamily` builds real capability variants from it (JP→`sub`/original, EN→`dub`+`lang:en`, RU→`dub`+`lang:ru`) with a new per-cap `lang`. The frontend chip already renders sub/dub + quality; combo routing reads `cap.lang` so ae dub routes under DUB/EN. RAW autocache is untouched (stays `Track=raw` → original/sub).

**Tech Stack:** Go (library + catalog services, GORM), ffmpeg/ffprobe, Vue 3 + TypeScript (aePlayer), Vitest, `bun`/`bunx`, Docker Compose (prod deploy + batchingest ops).

## Global Constraints

- Work in the existing worktree `source-panel-truth-and-ranking` (synced to `origin/main`). Never edit the base tree at `/data/animeenigma`.
- **RAW/DUB label mapping (authoritative):** an episode's `audio_lang` → capability: original/Japanese audio (`jpn`, `jp`, empty) → `Track=raw` → surfaces as `audio:sub` (original); localized audio (`eng`→`dub`+`lang:en`, `rus`→`dub`+`lang:ru`) → `Track=dub`. There is no JP dub. Only `en`/`ru` are dub languages.
- **Capture is ingest-known, not ffprobed.** `library-batchingest` already receives the language via `-audio-lang`; persist THAT. The RAW autocache/encoder-worker path (`Transcode` wrapper) is JP-original and MUST stay `Track=raw` (behavior-preserving — do not touch it).
- **Never-worse-than-today fallback:** when a title has no `audio_lang`/`Track` info (older rows, autocache), `aeFamily` falls back to today's `variantsFromTraits(row)`. `ProviderCap.lang` is set ONLY for the `aeProvider` standalone; `en`/`ru`/`adult` groups keep deriving language from `group`.
- Frontend gate: run `frontend-verify` before finishing any `frontend/web/` change.
- Go tests for touched services: `cd services/<svc> && go test ./... -count=1`.
- Deploy is FROM a clean `origin/main` worktree (never the shared dirty tree), `docker/.env` auto-copied by `redeploy.sh`'s ISS-030 guard. Deploy order for this phase: **library → catalog → web** (surfacing data must exist before the feed reads it; FE is backward-tolerant).
- batchingest ops: run **detached** (`docker compose run -d`), honor `LIBRARY_ENCODE_THREADS`/`_NICE`, `-dry-run` first, idempotent (skips existing rows). Heavy CPU — nice 15 yields to video serving.
- Commit trailers on every commit:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```

---

### Task 1: Library schema + domain — `audio_lang`, `quality`, output height

**Files:**
- Modify: `services/library/internal/domain/episode.go` (add `AudioLang`, `Quality` fields)
- Modify: `services/library/internal/ffmpeg/transcoder.go` (surface probed output height on `Result`)
- Test: `services/library/internal/ffmpeg/transcoder_test.go` (Result carries height)

**Interfaces:**
- Produces: `Episode.AudioLang string` (gorm `column:audio_lang`), `Episode.Quality string` (gorm `column:quality`); `ffmpeg.Result.Height int`.

- [ ] **Step 1: Add the columns to the domain struct** (GORM `AutoMigrate` adds nullable-safe columns on boot — no manual SQL):

```go
// in Episode struct, after Track:
AudioLang string `gorm:"type:text;not null;default:'';column:audio_lang" json:"audio_lang,omitempty"`
Quality   string `gorm:"type:text;not null;default:'';column:quality" json:"quality,omitempty"`
```

- [ ] **Step 2: Write a failing test that `Result` exposes the encoded height**

In `transcoder_test.go`, add a test asserting `Transcode`/`TranscodeWithOpts` returns `Result.Height` equal to the probed source height for a fixture (reuse the existing probe fixture pattern in this file):

```go
func TestTranscode_ResultCarriesHeight(t *testing.T) {
	// (mirror the existing transcoder fixture setup in this file)
	res, err := tr.Transcode(context.Background(), fixturePath)
	if err != nil { t.Fatalf("transcode: %v", err) }
	if res.Height == 0 { t.Fatalf("Result.Height not populated") }
}
```

- [ ] **Step 3: Run it — expect FAIL**

Run: `cd services/library && go test ./internal/ffmpeg/ -run TestTranscode_ResultCarriesHeight`
Expected: FAIL (`Result` has no `Height` field / it is 0).

- [ ] **Step 4: Add `Height` to `Result` and populate it from the existing `probe()`**

In `transcoder.go`: add `Height int` to `Result`, and in `TranscodeWithOpts` set it from the value `probe()` already returns (the function currently returns `(w, h)` around line 248). Wire the probed `h` into the returned `Result`.

- [ ] **Step 5: Run tests — expect PASS**

Run: `cd services/library && go test ./internal/ffmpeg/ -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/library/internal/domain/episode.go services/library/internal/ffmpeg/transcoder.go services/library/internal/ffmpeg/transcoder_test.go
git commit -m "$(cat <<'EOF'
feat(library): add audio_lang + quality episode columns; surface encode height

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

### Task 2: batchingest writes the audio label at row creation

**Files:**
- Modify: `services/library/cmd/library-batchingest/main.go` (row-creation block ~line 274; add a pure `langToTrack` + `normalizeLang` helper)
- Test: `services/library/cmd/library-batchingest/main_test.go` (create if absent) — `langToTrack` / `normalizeLang`

**Interfaces:**
- Consumes: `Episode.AudioLang`, `Episode.Quality` (Task 1); `ffmpeg.Result.Height` (Task 1).
- Produces: `langToTrack(audioLang string) domain.EpisodeTrack`; `normalizeLang(string) string`.

- [ ] **Step 1: Write failing tests for the pure helpers**

```go
func TestLangToTrack(t *testing.T) {
	cases := map[string]domain.EpisodeTrack{
		"": domain.EpisodeTrackRaw, "jpn": domain.EpisodeTrackRaw, "jp": domain.EpisodeTrackRaw, "ja": domain.EpisodeTrackRaw,
		"eng": domain.EpisodeTrackDub, "en": domain.EpisodeTrackDub, "rus": domain.EpisodeTrackDub, "ru": domain.EpisodeTrackDub,
	}
	for in, want := range cases {
		if got := langToTrack(in); got != want {
			t.Errorf("langToTrack(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd services/library && go test ./cmd/library-batchingest/ -run TestLangToTrack`
Expected: FAIL (`langToTrack` undefined).

- [ ] **Step 3: Implement the helpers + set the fields on the Episode row**

Add the pure helpers:

```go
// normalizeLang lowercases + maps ISO-639-1 aliases to the -2 form we compare on.
func normalizeLang(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "en", "eng": return "eng"
	case "ru", "rus": return "rus"
	case "ja", "jp", "jpn": return "jpn"
	default: return strings.ToLower(strings.TrimSpace(s))
	}
}

// langToTrack maps an ingest audio language to the stored Track. Localized
// languages (eng/rus) are dubs; original/Japanese/empty stays raw (original audio).
func langToTrack(audioLang string) domain.EpisodeTrack {
	switch normalizeLang(audioLang) {
	case "eng", "rus": return domain.EpisodeTrackDub
	default: return domain.EpisodeTrackRaw
	}
}
```

In the row-creation block (~line 274), set the new fields on the `&domain.Episode{...}` literal:

```go
ep := &domain.Episode{
	ShikimoriID:   j.shikimoriID,
	EpisodeNumber: j.episode,
	MinioPath:     prefix,
	DurationSec:   &dur,
	SizeBytes:     &size,
	Track:         langToTrack(audioLang),
	AudioLang:     normalizeLang(audioLang),
	Quality:       formatHeight(result.Height), // e.g. 1080 -> "1080p"; "" when 0
}
```

Add a tiny `formatHeight(h int) string` (returns `""` for 0, else `fmt.Sprintf("%dp", h)`) local to this package.

- [ ] **Step 4: Run — expect PASS**

Run: `cd services/library && go test ./cmd/library-batchingest/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/library/cmd/library-batchingest/main.go services/library/cmd/library-batchingest/main_test.go
git commit -m "$(cat <<'EOF'
feat(library): batchingest persists audio_lang + Track(dub) + quality on ingest

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

### Task 3: Surface the audio info through the library → catalog data path

**Files:**
- Modify: `services/library/internal/handler/episodes.go` (add `track`/`audio_lang`/`quality` to the per-episode response item)
- Modify: `services/catalog/internal/service/raw_resolver.go` (`RawEpisode` gains `Track`/`AudioLang`/`Quality`; add `AeTitleInfo`-shaped aggregation)
- Modify: `services/catalog/cmd/catalog-api/main.go` (`aeLibraryAdapter` gains `AeTitleInfo`)
- Modify: `services/catalog/internal/service/capability/families_firstparty.go` (`LibrarySource` interface: add `AeTitleInfo`)
- Test: `services/catalog/internal/service/raw_resolver_test.go` (response carries fields), `families_firstparty_test.go` (fake adapter)

**Interfaces:**
- Produces: `type AeInfo struct { Present bool; AudioLang, Track, Quality string }`; `LibrarySource.AeTitleInfo(ctx, animeID) (AeInfo, error)`; catalog `RawEpisode` gains `Track string`, `AudioLang string`, `Quality string` (json `track`/`audio_lang`/`quality`).

- [ ] **Step 1: Write failing test — library episodes response carries the new fields**

In `raw_resolver_test.go`, extend the happy-path fake library server response body to include `"track":"dub","audio_lang":"eng","quality":"1080p"` on an episode, and assert `GetLibraryEpisodes` surfaces them on `RawEpisode`. (Follow the existing `TestRawResolver_GetLibraryEpisodes_HappyPath` shape at line 169.)

- [ ] **Step 2: Run — expect FAIL**

Run: `cd services/catalog && go test ./internal/service/ -run TestRawResolver_GetLibraryEpisodes_HappyPath`
Expected: FAIL (fields not decoded).

- [ ] **Step 3: Add the fields + the AeTitleInfo aggregation**

- `episodes.go`: on the per-episode response item struct (the one built around lines 90-102 and 176-187), add `Track string \`json:"track,omitempty"\``, `AudioLang string \`json:"audio_lang,omitempty"\``, `Quality string \`json:"quality,omitempty"\`` and populate them from the `ep` row.
- `raw_resolver.go`: add the three fields to `RawEpisode`. Add a method on `RawResolver`:

```go
// AeTitleInfo aggregates the self-hosted audio facts for a title: whether any
// episode is a localized dub (and which language) vs original, plus the modal
// quality. Best-effort — an empty/absent library yields Present=false.
func (r *RawResolver) AeTitleInfo(ctx context.Context, animeID string) (AeInfo, error) {
	resp, err := r.GetLibraryEpisodes(ctx, animeID)
	if err != nil || resp == nil || !resp.Available || len(resp.Episodes) == 0 {
		return AeInfo{}, err
	}
	info := AeInfo{Present: true}
	for _, ep := range resp.Episodes {
		if ep.Track == string(/* dub */ "dub") && info.AudioLang == "" {
			info.AudioLang, info.Track = ep.AudioLang, ep.Track
		}
		if info.Quality == "" && ep.Quality != "" { info.Quality = ep.Quality }
	}
	if info.Track == "" { info.Track = "raw" } // original/sub when no dub episode
	return info, nil
}
```

Define `AeInfo` in the `service` package (or `capability`, matching where `LibrarySource` lives).

- `families_firstparty.go`: change the `LibrarySource` interface method from `HasLibraryTitle(ctx, animeID) (bool, error)` to `AeTitleInfo(ctx, animeID) (AeInfo, error)` (keep `HasLibraryTitle` as a thin wrapper if other callers need it — grep first).
- `main.go`: implement `aeLibraryAdapter.AeTitleInfo` delegating to `r.AeTitleInfo`.

- [ ] **Step 4: Run — expect PASS**

Run: `cd services/catalog && go test ./internal/service/ ./internal/service/capability/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/library/internal/handler/episodes.go services/catalog/internal/service/raw_resolver.go services/catalog/internal/service/raw_resolver_test.go services/catalog/cmd/catalog-api/main.go services/catalog/internal/service/capability/families_firstparty.go services/catalog/internal/service/capability/families_firstparty_test.go
git commit -m "$(cat <<'EOF'
feat(catalog): surface self-hosted (ae) per-title audio_lang/track/quality

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

### Task 4: `aeFamily` builds real variants + FE combo routing

**Files:**
- Modify: `services/catalog/internal/service/capability/families_firstparty.go` (`aeFamily` real variants)
- Modify: `services/catalog/internal/domain/capability.go` (`ProviderCap.Lang string`)
- Modify: `frontend/web/src/types/capabilities.ts` (`ProviderCap.lang?`)
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (combo language derivation reads `cap.lang`)
- Test: `families_firstparty_test.go` (jpn→sub, eng→dub/en, empty→trait); FE `useProviderFeed.spec.ts` or AePlayer combo spec

**Interfaces:**
- Consumes: `AeInfo` (Task 3).
- Produces: `domain.ProviderCap.Lang string` (json `lang,omitempty`); ae cap sets `Audios`/`Variants`/`Lang` from real content.

- [ ] **Step 1: Write failing `aeFamily` tests**

In `families_firstparty_test.go`, cases (fake `AeTitleInfo`): EN dub → `Audios == ["dub"]`, variant `Category == "dub"`, `cap.Lang == "en"`, quality from info; JP original → `Audios == ["sub"]`, `Category == "sub"`; empty/`Present:false` → falls back to `variantsFromTraits` (today's `sub`/1080p trait).

- [ ] **Step 2: Run — expect FAIL**

Run: `cd services/catalog && go test ./internal/service/capability/ -run TestAeFamily`
Expected: FAIL.

- [ ] **Step 3: Implement**

- `capability.go`: add `Lang string \`json:"lang,omitempty"\`` to `ProviderCap`.
- `families_firstparty.go` `aeFamily`: fetch `AeTitleInfo`; when `Present`, build variants + audios + `Lang` per the RAW/DUB mapping (jpn→`sub`, eng→`dub`+`en`, rus→`dub`+`ru`), `QualitySource:"probed"`, quality from `AeInfo.Quality` (fallback to trait ceiling when empty); when not present, keep `variantsFromTraits(row)` and leave `Lang` empty. `no_content` (not self-hosted) path unchanged.

- [ ] **Step 4: FE — combo routing reads `cap.lang`**

- `capabilities.ts`: add `lang?: 'en' | 'ru' | 'ja'` to `ProviderCap`.
- `AePlayer.vue` `buildAvailable` (the `GROUP_LANGS[cap.group]` line ~977): derive langs as `cap.lang ? [cap.lang] : GROUP_LANGS[cap.group]` so an ae dub cap (group `firstparty`) routes under its real language instead of the firstparty default. Add/adjust a `useProviderFeed.spec.ts` or combo assertion covering an ae `dub`/`lang:en` cap surfacing under DUB/EN.

- [ ] **Step 5: Run — expect PASS**

Run: `cd services/catalog && go test ./internal/service/capability/ -count=1` and `cd frontend/web && bunx vitest run src/composables/aePlayer/ && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 6: `frontend-verify`**

Run: `/frontend-verify`
Expected: DS-lint + i18n parity + build all green.

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/service/capability/families_firstparty.go services/catalog/internal/service/capability/families_firstparty_test.go services/catalog/internal/domain/capability.go frontend/web/src/types/capabilities.ts frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/composables/aePlayer/useProviderFeed.spec.ts
git commit -m "$(cat <<'EOF'
feat(player): ae surfaces real audio/lang/quality; dub routes under DUB slider

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

### Task 5: Deploy + data ops (backfill S1, ingest S2 + OVA)

Operational task — no unit tests; verification is DB rows + playback. Push Tasks 1–4 to `main` first, then build/deploy from a clean `origin/main` worktree, then run batchingest.

- [ ] **Step 1: Land Tasks 1–4 on `main`**

```bash
cd /data/animeenigma/.claude/worktrees/source-panel-truth-and-ranking
git fetch -q origin && git rebase -q origin/main && git push -q origin HEAD:main
```

- [ ] **Step 2: Build + deploy from a clean worktree (order: library → catalog → web)**

```bash
git worktree add /tmp/ae-deploy-c origin/main
cd /tmp/ae-deploy-c
cp /data/animeenigma/docker/.env docker/.env
# library (episodes handler + batchingest image), then catalog (aeFamily), then web
./deploy/scripts/redeploy.sh library || true   # if 'library' not in redeploy.sh roster, build directly:
docker compose -f docker/docker-compose.yml build library && docker compose -f docker/docker-compose.yml up -d --force-recreate --no-deps library
./deploy/scripts/redeploy.sh catalog
cd /data/animeenigma && make redeploy-web    # run web gates from a tree with node_modules, OR build image directly from /tmp/ae-deploy-c
```

(If `make redeploy-web`'s host-side gates need node_modules the clean worktree lacks, build the web image directly: `docker compose -f docker/docker-compose.yml build web && docker compose -f docker/docker-compose.yml up -d --no-deps web` from `/tmp/ae-deploy-c`.) Then `make health`.

- [ ] **Step 3: Backfill S1 (889) — already ingested as EN dub but labeled raw**

```bash
docker exec -i $(docker ps -qf name=postgres) psql -U postgres -d library -c \
  "UPDATE library_episodes SET track='dub', audio_lang='eng' WHERE shikimori_id='889' AND track='raw';"
```
Verify: `... -c "SELECT episode_number, track, audio_lang, quality FROM library_episodes WHERE shikimori_id='889' ORDER BY episode_number;"` → all `dub`/`eng`.

- [ ] **Step 4: Ingest S2 (1519) + OVA (4901) English packs (detached, dry-run first)**

Judas batch on disk: `/var/lib/docker/volumes/docker_library_torrents/_data/<infohash>/[Judas] Black Lagoon.../` — `S2 - The Second Barrage`→1519, `OVAs - Roberta's Blood Trail` (S03Exx)→4901.

```bash
cd /data/animeenigma/docker
# DRY RUN first for each (confirms file→episode mapping), then drop -dry-run and add -d:
docker compose run --rm --no-deps --entrypoint /app/library-batchingest library \
  -shikimori 1519 -pattern 'S02E(\d{2})' -eps-per-season 24 -audio-lang eng -dry-run -dir '/torrents/<ih>/[Judas] .../S2 - The Second Barrage'
docker compose run --rm --no-deps --entrypoint /app/library-batchingest library \
  -shikimori 4901 -pattern 'S03E(\d{2})' -eps-per-season 5 -audio-lang eng -dry-run -dir '/torrents/<ih>/[Judas] .../OVAs - Roberta's Blood Trail'
```
Confirm episode counts/mapping from dry-run output (adjust `-eps-per-season`/`-only` to the actual episode counts), then run detached without `-dry-run` (add `-e LIBRARY_ENCODE_THREADS=6` if the box is idle). Poll `library_episodes` for `shikimori_id IN ('1519','4901')` until complete.

- [ ] **Step 5: Clean up the deploy worktree**

```bash
git worktree remove /tmp/ae-deploy-c --force
```

---

### Task 6: End-to-end verification

- [ ] **Step 1: Capability feed shows ae as DUB/EN for Black Lagoon**

```bash
curl -s http://localhost:8081/api/anime/<blacklagoon-uuid>/capabilities | \
  python3 -c "import sys,json; d=json.load(sys.stdin)['data']; [print(p['provider'],p.get('audios'),p.get('lang'),[v.get('category') for v in p.get('variants',[])]) for f in d['families'] for p in f['providers'] if p['provider']=='ae']"
```
Expected: `ae ['dub'] en ['dub']` (not `['sub']`). Map the Black Lagoon shikimori 889/1519/4901 to their catalog UUIDs first.

- [ ] **Step 2: In-player check** — open the Black Lagoon anime page, confirm the `ae` chip shows **DUB · EN · <quality>**, sits under the DUB slider (EN), and plays English audio.

- [ ] **Step 3: Regression** — confirm a JP-original self-hosted title still shows under the original/sub slider (fallback intact), and a non-self-hosted title still shows `ae` as `no_content`.

- [ ] **Step 4: `/animeenigma-after-update`** — changelog (Trump-mode, Russian; this IS user-facing: "Black Lagoon на АНГЛИЙСКОМ дубляже — и теперь ЧЕСТНО помечен DUB"), then commit + push any remaining changes.

---

## Self-Review

**Spec coverage (Phase C section, as re-scoped 2026-07-07):** capture via persisted ingest-known language → Tasks 1–2; surfacing (library→catalog→aeFamily) → Tasks 3–4; `ProviderCap.lang` + combo routing → Task 4; backfill S1 + ingest S2/OVA → Task 5; e2e → Task 6. RAW/DUB mapping + never-worse fallback → Global Constraints + Task 4 empty-info case.

**Placeholder scan:** the only `<...>` are runtime values that cannot be known until deploy time — the Black Lagoon catalog UUIDs (Task 6) and the torrent infohash directory (Task 5, given in [[project_library_batchingest_dub_audio_lang]] as `daebcaec…`); the executor resolves them via the commands shown. `-eps-per-season`/`-only` are confirmed from the `-dry-run` output before the real run.

**Type consistency:** `Episode.AudioLang/Quality`, `Result.Height`, `langToTrack`/`normalizeLang`/`formatHeight`, `RawEpisode.Track/AudioLang/Quality`, `AeInfo{Present,AudioLang,Track,Quality}`, `LibrarySource.AeTitleInfo`, `ProviderCap.Lang`/`lang` are used consistently across tasks.

## Notes for execution

- **Deploy is real this phase** (unlike Phase D): library + catalog + web must ship together for the label to surface. Deploy order library→catalog→web; FE is backward-tolerant.
- **`HasLibraryTitle` callers:** Task 3 changes the `LibrarySource` interface — grep for all `HasLibraryTitle` callers and either keep a wrapper or migrate them; the capability `aeFamily` is the primary caller.
- Confirm the Black Lagoon franchise's catalog UUIDs (search `Black Lagoon`) and that shikimori 889/1519/4901 map to distinct catalog anime before Task 6.
