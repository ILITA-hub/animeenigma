# Curated Spotlight Card Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `curated` spotlight card ("Curator Recommends" / «Куратор рекомендует») that features one hand-picked, currently-airing anime on the home hero carousel, weighted (priority 1.5) to open more often and self-expiring when the show stops airing.

**Architecture:** New card type via the standard 5-anchor spotlight recipe (resolver → Data type → DI → SFC → dispatch/i18n/types), plus one cross-cutting `priority float64` field added to every `Card`. Priority is normalized to `1.0` for existing cards in the aggregator and consumed by the frontend as the **weight in the carousel's random opening-slide pick** — no forced ordering. The "active this season only" behaviour is a pure airing-status gate in the resolver (`status == ongoing`); no dates.

**Tech Stack:** Go (catalog service, `spotlight` package), Vue 3 + TypeScript (`frontend/web`), Redis cache, GORM/Postgres, Vitest, vue-tsc, project DS-lint.

## Global Constraints

- **Work only in the worktree** `/data/ae-curated-spotlight` (branch `feat/spotlight-curated-card`). NEVER edit `/data/animeenigma` (base tree). Use worktree-relative or `/data/ae-curated-spotlight/...` paths for every edit (absolute base-tree paths silently edit the wrong tree).
- **Featured anime**: `shikimori_id 63403` (Yani Neko / Табакошка). Env override `SPOTLIGHT_CURATED_SHIKIMORI_ID`, default `"63403"`, empty ⇒ card off.
- **Priority values**: default cards = `1.0`; curated = `1.5`. Field JSON name `priority`.
- **Airing gate**: eligible only while `anime.Status == domain.StatusOngoing`; otherwise resolver returns `(nil, nil)` and does NOT write cache (manual-cache discipline — no caching of ineligible states).
- **Kicker copy** (exact): en `Curator Recommends` · ru `Куратор рекомендует` · ja `キュレーターのおすすめ`.
- **Accent**: `cyan` (content-core triad slot). **Icon**: `star` (differentiates from featured's `sparkles`; both cyan). No 4th hue — DS-lint fails the build on off-palette classes.
- **i18n parity**: every `spotlight.curated.*` key must exist in `en.json`, `ru.json`, AND `ja.json` (CLAUDE.md 3-locale gate; `spotlight-keys.spec.ts` enforces en/ru).
- **Cache**: new `priority` field changes payload shape ⇒ same-day flush of `spotlight:*` card keys AND `spotlight:snapshot:*` after deploy.
- **Commit co-authors** on every commit:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```

All commands below assume `cd /data/ae-curated-spotlight` first.

---

### Task 1: Backend — `Card.Priority` field + aggregator normalization

**Files:**
- Modify: `services/catalog/internal/service/spotlight/types.go` (Card struct)
- Modify: `services/catalog/internal/service/spotlight/aggregator.go` (constants + collection loop)
- Test: `services/catalog/internal/service/spotlight/priority_test.go` (new)

**Interfaces:**
- Produces: `Card{ Type string; Priority float64; Data any }` — later tasks and the FE read `priority`. `defaultCardPriority = 1.0` (package const).

- [ ] **Step 1: Write the failing test** — `services/catalog/internal/service/spotlight/priority_test.go`

```go
package spotlight

import (
	"context"
	"testing"
)

// A resolver that returns a card with priority 0 must be normalized to 1.0;
// a resolver that sets its own priority (e.g. curated=1.5) is left untouched.
func TestAggregator_NormalizesCardPriority(t *testing.T) {
	resolvers := []Resolver{
		&fakeResolver{typ: "normal", card: &Card{Type: "normal"}},          // Priority 0 → 1.0
		&fakeResolver{typ: "weighted", card: &Card{Type: "weighted", Priority: 1.5}},
	}
	agg := NewAggregator(newFakeCache(), testLogger(t), resolvers)

	resp, err := agg.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	got := map[string]float64{}
	for _, c := range resp.Cards {
		got[c.Type] = c.Priority
	}
	if got["normal"] != 1.0 {
		t.Errorf("normal card priority = %v, want 1.0", got["normal"])
	}
	if got["weighted"] != 1.5 {
		t.Errorf("weighted card priority = %v, want 1.5", got["weighted"])
	}
}
```

- [ ] **Step 2: Run it — verify it fails**

Run: `cd services/catalog && go test ./internal/service/spotlight/ -run TestAggregator_NormalizesCardPriority`
Expected: FAIL — `Card` has no field `Priority` (compile error).

- [ ] **Step 3: Add the field** — `types.go`, replace the `Card` struct (around line 37):

```go
// Card is the outer discriminated-union envelope. Each resolver produces
// a Card with its own Type discriminator (e.g. "featured") and a
// per-type Data struct embedded as `any`. The TypeScript side narrows on
// the `type` field. Priority is a display weight (default 1.0, normalized
// by the aggregator); the frontend uses it as the weight in the carousel's
// random opening-slide pick.
type Card struct {
	Type     string  `json:"type"`
	Priority float64 `json:"priority"`
	Data     any     `json:"data"`
}
```

- [ ] **Step 4: Normalize in the aggregator** — `aggregator.go`.

Add to the `const (...)` block (after `snapshotTTL`):

```go
	// defaultCardPriority is assigned to any card whose resolver left
	// Priority at its zero value — so only the curated resolver (1.5) needs
	// to set a non-default weight; the other 8 resolvers stay untouched.
	defaultCardPriority = 1.0
```

In `Resolve`, change the append in the collector loop (currently `cards = append(cards, *res.card)`):

```go
		c := *res.card
		if c.Priority == 0 {
			c.Priority = defaultCardPriority
		}
		cards = append(cards, c)
```

- [ ] **Step 5: Run the test — verify it passes**

Run: `cd services/catalog && go test ./internal/service/spotlight/ -run TestAggregator_NormalizesCardPriority`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/service/spotlight/types.go \
        services/catalog/internal/service/spotlight/aggregator.go \
        services/catalog/internal/service/spotlight/priority_test.go
git commit -m "feat(spotlight): add numeric Card.priority + aggregator normalization" # + co-authors
```

---

### Task 2: Backend — `CuratedData` payload type

**Files:**
- Modify: `services/catalog/internal/service/spotlight/types.go`
- Test: `services/catalog/internal/service/spotlight/types_test.go`

**Interfaces:**
- Produces: `CuratedData{ Anime domain.Anime }` — consumed by the curated resolver (Task 3) and the FE type (Task 5). Wire shape: `{"anime": {...}}`.

- [ ] **Step 1: Write the failing test** — append to `types_test.go`:

```go
func TestCuratedData_RoundTrip(t *testing.T) {
	t.Parallel()
	in := Card{Type: "curated", Priority: 1.5, Data: CuratedData{Anime: domain.Anime{Name: "Yani Neko"}}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"priority":1.5`) {
		t.Errorf("expected priority in JSON, got %s", b)
	}
	if !strings.Contains(string(b), `"anime"`) {
		t.Errorf("expected anime key in JSON, got %s", b)
	}
}
```

- [ ] **Step 2: Run it — verify it fails**

Run: `cd services/catalog && go test ./internal/service/spotlight/ -run TestCuratedData_RoundTrip`
Expected: FAIL — `CuratedData` undefined.

- [ ] **Step 3: Add the type** — `types.go`, after `FeaturedData`:

```go
// CuratedData is the payload for Card{Type: "curated"} — a single
// hand-picked anime surfaced ONLY while it is currently airing.
type CuratedData struct {
	Anime domain.Anime `json:"anime"`
}
```

- [ ] **Step 4: Run the test — verify it passes**

Run: `cd services/catalog && go test ./internal/service/spotlight/ -run TestCuratedData_RoundTrip`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/spotlight/types.go \
        services/catalog/internal/service/spotlight/types_test.go
git commit -m "feat(spotlight): add CuratedData card payload type" # + co-authors
```

---

### Task 3: Backend — `CuratedResolver`

**Files:**
- Create: `services/catalog/internal/service/spotlight/cards/curated.go`
- Test: `services/catalog/internal/service/spotlight/cards/curated_test.go`

**Interfaces:**
- Consumes: `spotlight.CuratedData` (Task 2), `cache.Cache`, `*logger.Logger`, `domain.Anime`, `domain.StatusOngoing`, package consts `cardTTL` (already in `featured.go`).
- Produces: `NewCuratedResolver(repo animeGetter, c cache.Cache, log *logger.Logger, shikimoriID string) *CuratedResolver` — wired by Task 4. `animeGetter` interface: `GetByShikimoriID(ctx, string) (*domain.Anime, error)` (satisfied by `*repo.AnimeRepository`). `curatedPriority = 1.5`.

- [ ] **Step 1: Write the failing test** — `curated_test.go`:

```go
package cards

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// fakeAnimeGetter implements animeGetter with a canned anime + call counter.
type fakeAnimeGetter struct {
	anime *domain.Anime
	err   error
	calls int
}

func (f *fakeAnimeGetter) GetByShikimoriID(_ context.Context, _ string) (*domain.Anime, error) {
	f.calls++
	return f.anime, f.err
}

func TestCuratedResolver_Type(t *testing.T) {
	r := &CuratedResolver{}
	if got := r.Type(); got != "curated" {
		t.Errorf("Type() = %q, want curated", got)
	}
}

func TestCuratedResolver_OngoingAnime_ReturnsCard(t *testing.T) {
	repo := &fakeAnimeGetter{anime: &domain.Anime{ID: "u1", Name: "Yani Neko", Status: domain.StatusOngoing}}
	r := NewCuratedResolver(repo, newFakeCache(), testLogger(), "63403")

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected a card for an ongoing anime")
	}
	if card.Type != "curated" {
		t.Errorf("Card.Type = %q, want curated", card.Type)
	}
	if card.Priority != 1.5 {
		t.Errorf("Card.Priority = %v, want 1.5", card.Priority)
	}
	if _, ok := card.Data.(spotlight.CuratedData); !ok {
		t.Fatalf("Card.Data is not CuratedData: %T", card.Data)
	}
}

func TestCuratedResolver_ReleasedAnime_DropsCard(t *testing.T) {
	repo := &fakeAnimeGetter{anime: &domain.Anime{ID: "u1", Status: domain.StatusReleased}}
	r := NewCuratedResolver(repo, newFakeCache(), testLogger(), "63403")

	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card != nil {
		t.Fatalf("expected (nil,nil) for released anime, got card=%v err=%v", card, err)
	}
}

func TestCuratedResolver_EmptyID_Disabled(t *testing.T) {
	repo := &fakeAnimeGetter{anime: &domain.Anime{Status: domain.StatusOngoing}}
	r := NewCuratedResolver(repo, newFakeCache(), testLogger(), "")

	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card != nil {
		t.Fatalf("expected (nil,nil) when disabled, got card=%v err=%v", card, err)
	}
	if repo.calls != 0 {
		t.Errorf("repo should not be queried when disabled, got %d calls", repo.calls)
	}
}

func TestCuratedResolver_NotFound_DropsCard(t *testing.T) {
	repo := &fakeAnimeGetter{anime: nil} // GetByShikimoriID returns (nil,nil) on not-found
	r := NewCuratedResolver(repo, newFakeCache(), testLogger(), "63403")

	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card != nil {
		t.Fatalf("expected (nil,nil) for missing anime, got card=%v err=%v", card, err)
	}
}
```

- [ ] **Step 2: Run it — verify it fails**

Run: `cd services/catalog && go test ./internal/service/spotlight/cards/ -run TestCuratedResolver`
Expected: FAIL — `CuratedResolver` / `NewCuratedResolver` undefined.

- [ ] **Step 3: Implement** — `curated.go`:

```go
package cards

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// curatedPriority is the display weight the curated card carries. The
// aggregator leaves any non-zero priority untouched; the frontend uses it
// as the weight in the carousel's random opening-slide pick (1.5× the
// default-1.0 cards).
const curatedPriority = 1.5

// animeGetter is the subset of repo.AnimeRepository the curated resolver
// needs — a single fetch by Shikimori ID (the catalog's primary key).
type animeGetter interface {
	GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error)
}

// CuratedResolver implements spotlight.Resolver for the `curated` card — a
// single hand-picked anime surfaced ONLY while it is currently airing.
type CuratedResolver struct {
	repo        animeGetter
	cache       cache.Cache
	log         *logger.Logger
	shikimoriID string
}

// NewCuratedResolver constructs the resolver. shikimoriID is the pinned
// Shikimori ID (env SPOTLIGHT_CURATED_SHIKIMORI_ID); an empty string
// disables the card entirely.
func NewCuratedResolver(repo animeGetter, c cache.Cache, log *logger.Logger, shikimoriID string) *CuratedResolver {
	return &CuratedResolver{repo: repo, cache: c, log: log, shikimoriID: shikimoriID}
}

// Type returns the card discriminator consumed by the frontend union.
func (r *CuratedResolver) Type() string { return "curated" }

// Resolve returns the curated card. userID is ignored — the pick is global.
//
// Eligibility (each returns (nil, nil) WITHOUT caching, so an upstream
// recovery is retried on the next request — the workstream's manual-cache
// discipline):
//   - shikimoriID empty (card disabled);
//   - the anime is not in the catalog yet;
//   - the anime is not currently airing (status != ongoing) — this is the
//     entire "active this season only" self-expiry.
func (r *CuratedResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	if r.shikimoriID == "" {
		return nil, nil
	}

	key := "spotlight:curated:" + spotlight.DateKeyUTC(time.Now())

	var cached spotlight.CuratedData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Priority: curatedPriority, Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	anime, err := r.repo.GetByShikimoriID(ctx, r.shikimoriID)
	if err != nil {
		return nil, err
	}
	if anime == nil {
		return nil, nil // not populated yet — do NOT cache
	}
	if anime.Status != domain.StatusOngoing {
		return nil, nil // finished / announced — self-expire, do NOT cache
	}

	data := spotlight.CuratedData{Anime: *anime}
	if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Priority: curatedPriority, Data: data}, nil
}
```

- [ ] **Step 4: Run the tests — verify they pass**

Run: `cd services/catalog && go test ./internal/service/spotlight/cards/ -run TestCuratedResolver -v`
Expected: PASS (5 subtests).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/spotlight/cards/curated.go \
        services/catalog/internal/service/spotlight/cards/curated_test.go
git commit -m "feat(spotlight): curated card resolver (airing-gated, priority 1.5)" # + co-authors
```

---

### Task 4: Backend — config + DI wiring + env docs

**Files:**
- Modify: `services/catalog/internal/config/config.go` (struct field + loader)
- Modify: `services/catalog/cmd/catalog-api/main.go` (resolver slice)
- Modify: `docs/environment-variables.md` (document the new var)

**Interfaces:**
- Consumes: `cfg.SpotlightCuratedShikimoriID`, `cards.NewCuratedResolver` (Task 3), existing `animeRepo`, `redisCache`, `log`.

- [ ] **Step 1: Add the config field** — `config.go`, in the config struct next to `SpotlightEnabled` (~line 39):

```go
	// SpotlightCuratedShikimoriID pins the anime featured by the `curated`
	// spotlight card (env SPOTLIGHT_CURATED_SHIKIMORI_ID). Empty disables it.
	SpotlightCuratedShikimoriID string
```

- [ ] **Step 2: Load it** — `config.go`, in the loader next to `SpotlightEnabled: getEnvBool(...)` (~line 211):

```go
		SpotlightCuratedShikimoriID: getEnv("SPOTLIGHT_CURATED_SHIKIMORI_ID", "63403"),
```

- [ ] **Step 3: Wire the resolver** — `main.go`, append inside the `spotlightResolvers := []spotlight.Resolver{ ... }` slice (after the `NewContinueWatchingNewResolver` line):

```go
		// Curated «Куратор рекомендует» — one env-pinned, airing-gated anime.
		cards.NewCuratedResolver(animeRepo, redisCache, log, cfg.SpotlightCuratedShikimoriID),
```

- [ ] **Step 4: Document the var** — `docs/environment-variables.md`, in the Catalog section, add a row:

```
| `SPOTLIGHT_CURATED_SHIKIMORI_ID` | `63403` | Shikimori ID featured by the `curated` spotlight card ("Curator Recommends"); empty string disables the card. Card shows only while the anime is `ongoing`. |
```

- [ ] **Step 5: Build — verify it compiles**

Run: `cd services/catalog && go build ./... && go vet ./internal/service/spotlight/...`
Expected: no output (success).

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/config/config.go \
        services/catalog/cmd/catalog-api/main.go \
        docs/environment-variables.md
git commit -m "feat(spotlight): wire curated resolver via SPOTLIGHT_CURATED_SHIKIMORI_ID" # + co-authors
```

---

### Task 5: Frontend — types (`CuratedData` + union member + `priority`)

**Files:**
- Modify: `frontend/web/src/types/spotlight.ts`

**Interfaces:**
- Produces: `CuratedData{ anime: SpotlightAnime }`; `SpotlightCard` union gains `{ type:'curated'; data:CuratedData }` and a shared optional `priority?: number` (via intersection — preserves discriminated-union narrowing for the v-if dispatch chain and vue-tsc).

- [ ] **Step 1: Add the `CuratedData` interface** — after the `FeaturedData` interface:

```ts
export interface CuratedData {
  anime: SpotlightAnime
}
```

- [ ] **Step 2: Extend the union** — replace the `export type SpotlightCard = ...` block:

```ts
export type SpotlightCard = (
  | { type: 'featured'; data: FeaturedData }
  | { type: 'random_tail'; data: RandomTailData }
  | { type: 'latest_news'; data: LatestNewsData }
  | { type: 'platform_stats'; data: PlatformStatsData }
  | { type: 'personal_pick'; data: PersonalPickData }
  | { type: 'telegram_news'; data: TelegramNewsData }
  | { type: 'now_watching'; data: NowWatchingData }
  | { type: 'not_time_yet'; data: NotTimeYetData }
  | { type: 'continue_watching_new'; data: ContinueWatchingNewData }
  | { type: 'curated'; data: CuratedData }
) & { priority?: number }
```

- [ ] **Step 3: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: PASS (no new errors). NOTE — this alone will make `tokens.spec.ts` fail (Task 7 adds the token) and the DS-lint hook may run on save; that's expected until Tasks 6–7 land.

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/types/spotlight.ts
git commit -m "feat(spotlight): CuratedData type + curated union member + priority" # + co-authors
```

---

### Task 6: Frontend — i18n `spotlight.curated.*` (en / ru / ja)

**Files:**
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ru.json`
- Modify: `frontend/web/src/locales/ja.json`
- Test: `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` (existing — must stay green)

**Interfaces:**
- Produces keys `spotlight.curated.{title,watchEpisode,addCta,airedLabel,episodesLabel}` used by `CuratedCard.vue` (Task 9) and `tokens.ts` (Task 7 — `title`).

- [ ] **Step 1: Add the block to `en.json`** — inside the top-level `"spotlight"` object (e.g. right after the `"featured"` block):

```json
    "curated": {
      "title": "Curator Recommends",
      "watchEpisode": "Watch · ep. {n}",
      "addCta": "Add to list",
      "airedLabel": "ep. {n} aired",
      "episodesLabel": "{n} ep."
    },
```

- [ ] **Step 2: Add the block to `ru.json`**:

```json
    "curated": {
      "title": "Куратор рекомендует",
      "watchEpisode": "Смотреть · эп. {n}",
      "addCta": "В список",
      "airedLabel": "эп. {n} вышел",
      "episodesLabel": "{n} эп."
    },
```

- [ ] **Step 3: Add the block to `ja.json`**:

```json
    "curated": {
      "title": "キュレーターのおすすめ",
      "watchEpisode": "視聴 · 第{n}話",
      "addCta": "リストに追加",
      "airedLabel": "第{n}話配信中",
      "episodesLabel": "{n}話"
    },
```

- [ ] **Step 4: Run the parity test — verify it passes**

Run: `cd frontend/web && bunx vitest run src/locales/__tests__/spotlight-keys.spec.ts`
Expected: PASS (en/ru key sets identical; all leaves non-empty strings).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "i18n(spotlight): curated card strings (en/ru/ja)" # + co-authors
```

---

### Task 7: Frontend — `tokens.ts` curated entry

**Files:**
- Modify: `frontend/web/src/components/home/spotlight/tokens.ts`
- Test: `frontend/web/src/components/home/spotlight/tokens.spec.ts` (existing parity test — must go green)

**Interfaces:**
- Consumes: the `curated` union member (Task 5), `spotlight.curated.title` key (Task 6).

- [ ] **Step 1: Add the token** — in `cardTokens`, add an entry (keep the aligned style):

```ts
  curated:               { accent: 'cyan',   kickerKey: 'spotlight.curated.title',             icon: 'star'     },
```

- [ ] **Step 2: Run the parity test — verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/home/spotlight/tokens.spec.ts`
Expected: PASS — every `SpotlightCard['type']` (now including `curated`) resolves to a token.

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/components/home/spotlight/tokens.ts
git commit -m "feat(spotlight): curated card token (cyan + star icon)" # + co-authors
```

---

### Task 8: Frontend — `weightedRandomIndex` util (weighted opening-slide pick)

**Files:**
- Create: `frontend/web/src/components/home/spotlight/weightedRandom.ts`
- Test: `frontend/web/src/components/home/spotlight/weightedRandom.spec.ts`

**Interfaces:**
- Produces: `weightedRandomIndex(cards: SpotlightCard[], rng?: () => number): number` — used by `HeroSpotlightBlock.vue` (Task 10). Injectable `rng` (default `Math.random`) for deterministic tests.

- [ ] **Step 1: Write the failing test** — `weightedRandom.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { weightedRandomIndex } from './weightedRandom'
import type { SpotlightCard } from '@/types/spotlight'

const card = (type: string, priority?: number) =>
  ({ type, priority, data: {} }) as unknown as SpotlightCard

describe('weightedRandomIndex', () => {
  it('returns 0 for an empty array', () => {
    expect(weightedRandomIndex([])).toBe(0)
  })

  it('is uniform when every weight is the default (missing priority)', () => {
    const cards = [card('a'), card('b'), card('c')]
    expect(weightedRandomIndex(cards, () => 0)).toBe(0)
    expect(weightedRandomIndex(cards, () => 0.999)).toBe(2)
  })

  it('biases toward the higher-priority card (weights 1,1.5,1 → total 3.5)', () => {
    const cards = [card('a', 1), card('b', 1.5), card('c', 1)]
    // rng*3.5: 0→0 (idx0), 0.5→1.75 (idx1, the 1.0–2.5 bucket), 0.8→2.8 (idx2)
    expect(weightedRandomIndex(cards, () => 0)).toBe(0)
    expect(weightedRandomIndex(cards, () => 0.5)).toBe(1)
    expect(weightedRandomIndex(cards, () => 0.8)).toBe(2)
  })
})
```

- [ ] **Step 2: Run it — verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/home/spotlight/weightedRandom.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement** — `weightedRandom.ts`:

```ts
import type { SpotlightCard } from '@/types/spotlight'

/**
 * Picks a card index at random, biased by each card's `priority` weight
 * (default 1). A priority-1.5 card is 1.5× likelier to be chosen than a
 * default-1.0 card. When every weight is equal this is a plain uniform pick,
 * preserving the carousel's original random-start-for-variety behaviour.
 *
 * `rng` is injectable so tests are deterministic; production passes the
 * default Math.random. Returns 0 for an empty array (callers guard n>0).
 */
export function weightedRandomIndex(
  cards: SpotlightCard[],
  rng: () => number = Math.random,
): number {
  if (cards.length === 0) return 0
  const weights = cards.map((c) => {
    const w = c.priority ?? 1
    return w > 0 ? w : 0
  })
  const total = weights.reduce((a, w) => a + w, 0)
  if (total <= 0) return Math.floor(rng() * cards.length)
  let r = rng() * total
  for (let i = 0; i < weights.length; i++) {
    r -= weights[i]
    if (r < 0) return i
  }
  return cards.length - 1
}
```

- [ ] **Step 4: Run the test — verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/home/spotlight/weightedRandom.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/home/spotlight/weightedRandom.ts \
        frontend/web/src/components/home/spotlight/weightedRandom.spec.ts
git commit -m "feat(spotlight): weightedRandomIndex util for priority-weighted start" # + co-authors
```

---

### Task 9: Frontend — `CuratedCard.vue`

**Files:**
- Create: `frontend/web/src/components/home/spotlight/cards/CuratedCard.vue`
- Test: `frontend/web/src/components/home/spotlight/cards/CuratedCard.spec.ts`

**Interfaces:**
- Consumes: `CuratedData` (Task 5), `SpotlightCardShell`, `spotlight.curated.*` (Task 6). Props: `{ data: CuratedData }`.

- [ ] **Step 1: Write the failing spec** — `CuratedCard.spec.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (name?: string, nameRu?: string, nameJp?: string) =>
    name || nameRu || nameJp || '',
}))

import CuratedCard from './CuratedCard.vue'

function mountCard(props: Record<string, unknown>) {
  return mount(CuratedCard, {
    props: props as unknown as InstanceType<typeof CuratedCard>['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const data = {
  anime: {
    id: 'a1',
    name: 'Yani Neko',
    name_ru: 'Табакошка',
    name_jp: 'ヤニねこ',
    status: 'ongoing',
    episodes_aired: 2,
    score: 7.0,
    year: 2026,
    episodes_count: 12,
    description: undefined as string | undefined,
    genres: [] as { id: string; name?: string; russian?: string }[],
  },
}

describe('CuratedCard', () => {
  it('renders the curated kicker title', () => {
    expect(mountCard({ data }).text()).toContain('spotlight.curated.title')
  })

  it('renders the watch CTA with next episode = aired + 1', () => {
    expect(mountCard({ data }).text()).toContain('"n":3')
  })

  it('renders the add-to-list CTA', () => {
    expect(mountCard({ data }).text()).toContain('spotlight.curated.addCta')
  })

  it('renders JP subtitle and score', () => {
    const text = mountCard({ data }).text()
    expect(text).toContain('ヤニねこ')
    expect(text).toContain('7.0')
  })

  it('links the primary CTA to the watch route', () => {
    const links = mountCard({ data }).findAllComponents(RouterLinkStub)
    const watch = links.find((l) => {
      const to = l.props('to') as string
      return typeof to === 'string' && to.includes('/anime/a1') && to.includes('/watch')
    })
    expect(watch).toBeDefined()
  })

  it('has a single root <article> (SpotlightCardShell) — no fragment root', () => {
    expect(mountCard({ data }).element.tagName).toBe('ARTICLE')
  })
})
```

- [ ] **Step 2: Run it — verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/home/spotlight/cards/CuratedCard.spec.ts`
Expected: FAIL — component file does not exist.

- [ ] **Step 3: Implement** — `CuratedCard.vue`:

```vue
<template>
  <SpotlightCardShell
    accent="cyan"
    backdrop="none"
    justify="end"
    :kicker="t('spotlight.curated.title')"
    content-class="max-w-[720px]"
  >
    <!--
      Curated «Куратор рекомендует» hero card (type: 'curated'). One
      env-pinned, airing-gated anime. Mirrors FeaturedCard's cinematic hero
      but with a fixed curated kicker (star lead) and a single ongoing CTA —
      the resolver only ever surfaces an `ongoing` anime, so no status switch.
      DS: SpotlightCardShell anatomy, Button-variant CTAs, overlay Badges,
      font-medium/semibold only.
    -->
    <template #background>
      <div class="curated-bg" aria-hidden="true">
        <div v-if="posterSrc && !bgLoaded" class="absolute inset-0 skeleton-shimmer" />
        <img
          v-if="posterSrc"
          :src="posterSrc"
          alt=""
          decoding="async"
          class="transition-opacity duration-300"
          :class="bgLoaded ? 'opacity-100' : 'opacity-0'"
          @load="onBgLoad"
          @error="onBgLoad"
        />
      </div>
    </template>

    <template #kicker-lead>
      <Star class="size-3" fill="currentColor" aria-hidden="true" />
    </template>
    <template #kicker-extra>
      <template v-if="data.anime.season"
        ><span class="opacity-50">·</span>{{ data.anime.season }}</template
      >
    </template>

    <h3 class="curated-title font-display font-semibold">
      <span class="main">{{ getLocalizedTitle(data.anime.name, data.anime.name_ru, data.anime.name_jp) }}</span>
      <span v-if="data.anime.name_jp" class="jp font-medium">{{ data.anime.name_jp }}</span>
    </h3>

    <div class="flex items-center gap-2 flex-wrap text-[13px] text-muted-foreground">
      <Badge v-if="data.anime.score" variant="warning" size="sm" overlay>
        <template #icon>
          <Star class="size-3" fill="currentColor" aria-hidden="true" />
        </template>
        {{ data.anime.score.toFixed(1) }}
      </Badge>
      <Badge v-if="data.anime.episodes_aired" variant="success" size="sm" overlay>
        {{ t('spotlight.curated.airedLabel', { n: data.anime.episodes_aired }) }}
      </Badge>
      <span v-if="data.anime.year">{{ data.anime.year }}</span>
      <span v-if="data.anime.episodes_count" class="opacity-40" aria-hidden="true">·</span>
      <span v-if="data.anime.episodes_count">{{
        t('spotlight.curated.episodesLabel', { n: data.anime.episodes_count })
      }}</span>
      <Badge
        v-for="g in (data.anime.genres || []).slice(0, 3)"
        :key="g.id"
        size="sm"
        overlay
      >
        {{ locale === 'ru' ? (g.russian || g.name) : (g.name || g.russian) }}
      </Badge>
    </div>

    <!-- eslint-disable-next-line vue/no-v-html -->
    <p
      v-if="data.anime.description"
      data-testid="curated-desc"
      class="text-[15px] leading-relaxed text-white/70 max-w-[540px] line-clamp-2 [text-wrap:pretty]"
      v-html="parsedDescription"
    />

    <template #cta>
      <router-link :to="watchTo" :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']">
        <Play class="w-4 h-4" fill="currentColor" aria-hidden="true" />
        {{ t('spotlight.curated.watchEpisode', { n: (data.anime.episodes_aired || 0) + 1 }) }}
      </router-link>
      <router-link
        :to="`/anime/${data.anime.id}`"
        :class="[buttonVariants({ variant: 'ghost', size: 'md' }), 'text-sm']"
      >
        {{ t('spotlight.curated.addCta') }}
      </router-link>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Play, Star } from 'lucide-vue-next'
import { getLocalizedTitle } from '@/utils/title'
import { parseDescription } from '@/utils/description-parser'
import { cardPosterUrl } from '@/composables/useImageProxy'
import { isImageWarm, markImageWarm } from '@/utils/preload-image'
import type { CuratedData } from '@/types/spotlight'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'

const props = defineProps<{ data: CuratedData }>()
const { t, locale: i18nLocale } = useI18n()

const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

const posterSrc = computed(() =>
  props.data.anime.poster_url ? cardPosterUrl(props.data.anime.poster_url, 640) : '',
)
const bgLoaded = ref(isImageWarm(posterSrc.value))
function onBgLoad(): void {
  bgLoaded.value = true
  markImageWarm(posterSrc.value)
}

const parsedDescription = computed(() =>
  props.data.anime.description ? parseDescription(props.data.anime.description) : '',
)

const watchTo = computed(() => `/anime/${props.data.anime.id}/watch`)
</script>

<style scoped>
.curated-bg { position: absolute; inset: 0; }
.curated-bg img { width: 100%; height: 100%; object-fit: cover; object-position: center 30%; filter: saturate(105%); }
.curated-bg::after {
  content: ""; position: absolute; inset: 0;
  background:
    linear-gradient(90deg, var(--scrim-bg-strong) 0%, var(--scrim-bg-strong) 35%, var(--scrim-bg-soft) 65%, transparent 100%),
    linear-gradient(180deg, transparent 50%, var(--scrim-bg-strong) 100%);
}
.curated-title { font-size: clamp(28px, 2.6vw, 34px); line-height: 1.1; letter-spacing: -.02em; text-wrap: balance; }
.curated-title .main { display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.curated-title .jp { display: -webkit-box; -webkit-line-clamp: 1; line-clamp: 1; -webkit-box-orient: vertical; overflow: hidden; font-family: var(--font-jp); font-size: .42em; letter-spacing: .02em; color: var(--muted-foreground); margin-top: 8px; }
</style>
```

- [ ] **Step 4: Run the spec — verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/home/spotlight/cards/CuratedCard.spec.ts`
Expected: PASS (6 assertions).

- [ ] **Step 5: DS-lint the new SFC**

Run: `cd frontend/web && bash scripts/design-system-lint.sh`
Expected: `ERRORS: 0` (mirrors FeaturedCard's DS-compliant patterns).

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/home/spotlight/cards/CuratedCard.vue \
        frontend/web/src/components/home/spotlight/cards/CuratedCard.spec.ts
git commit -m "feat(spotlight): CuratedCard.vue hero component" # + co-authors
```

---

### Task 10: Frontend — HeroSpotlightBlock wiring (dispatch + weighted start + title + prefetch)

**Files:**
- Modify: `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue`

**Interfaces:**
- Consumes: `CuratedCard` (Task 9), `weightedRandomIndex` (Task 8), the `curated` union member (Task 5).

- [ ] **Step 1: Import the component and the util** — in the `<script setup>` import block (near the other `./cards/*` imports, ~line 175):

```ts
import CuratedCard from './cards/CuratedCard.vue'
import { weightedRandomIndex } from './weightedRandom'
```

- [ ] **Step 2: Add the dispatch branch** — in the template, after the `continue_watching_new` branch (~line 133):

```html
          <CuratedCard
            v-else-if="active.type === 'curated'"
            :key="`curated:${currentIndex}`"
            :data="active.data"
          />
```

- [ ] **Step 3: Use the weighted pick for the opening slide** — replace the uniform random init (currently `currentIndex.value = Math.floor(Math.random() * n)`, ~line 430):

```ts
    currentIndex.value = weightedRandomIndex(cards.value)
```

- [ ] **Step 4: Add the `cardTitle` case** — in the `cardTitle()` switch, add `curated` to the anime-title group (alongside `featured`/`random_tail`):

```ts
    case 'curated':
      return getLocalizedTitle(
        card.data.anime.name,
        card.data.anime.name_ru,
        card.data.anime.name_jp,
      )
```

- [ ] **Step 5: Register the prefetch poster** — in `cardImageUrls()`, add before `default:`:

```ts
    case 'curated':
      return card.data.anime.poster_url
        ? [cardPosterUrl(card.data.anime.poster_url, 640)]
        : []
```

- [ ] **Step 6: Verify — build, type-check, and run the full spotlight suite**

Run:
```bash
cd frontend/web && bunx tsc --noEmit \
  && bunx vitest run src/components/home/spotlight/ src/locales/__tests__/spotlight-keys.spec.ts
```
Expected: PASS (tsc clean; all spotlight + parity + tokens specs green; `HeroSpotlightBlock` existing specs unaffected — weighted pick is uniform when only default-1.0 cards are present).

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue
git commit -m "feat(spotlight): dispatch curated card + priority-weighted opening slide" # + co-authors
```

---

### Task 11: Full verification + rollout (build, DS-lint, cache flush, live smoke)

**Files:** none (verification + deploy).

- [ ] **Step 1: Backend full test + build**

Run: `cd services/catalog && go test ./internal/service/spotlight/... -count=1 -race && go build ./...`
Expected: PASS.

- [ ] **Step 2: Frontend gate** — run `/frontend-verify` (DS-lint + i18n en/ru/ja parity + real `bun run build` + TS traps). Expected: all green.

- [ ] **Step 3: Deploy** — via `/animeenigma-after-update` (runs /simplify on the diff, redeploys `catalog` + `web`, health-checks, updates the changelog in Russian Trump-mode, commits + pushes). Confirm `make redeploy-catalog` and `make redeploy-web` succeed and `make health` is green.

- [ ] **Step 4: Flush stale spotlight cache** — the new `priority` field changed the payload shape, so old-shape cached JSON must be purged (guidelines §5). After the catalog redeploy:

```bash
# confirm the redis container name first: docker ps | grep redis
docker exec animeenigma-redis sh -c "redis-cli --scan --pattern 'spotlight:*' | xargs -r redis-cli DEL"
```
Expected: deleted keys count ≥ 0 (both `spotlight:<card>:<date>` and `spotlight:snapshot:*` cleared).

- [ ] **Step 5: Live smoke** — confirm the curated card is present and priorities are correct:

```bash
curl -s http://localhost:8000/api/home/spotlight | jq '.cards[] | {type, priority}'
```
Expected: a `{"type":"curated","priority":1.5}` entry appears (Табакошка is `ongoing`), and every other card shows `"priority":1`. If `curated` is absent, verify `SPOTLIGHT_CURATED_SHIKIMORI_ID=63403` reached the catalog container and that shikimori_id 63403 is still `ongoing` in the DB.

- [ ] **Step 6: Post-merge cleanup** — after `/animeenigma-after-update` pushes and health is green, remove the worktree from the base tree:

```bash
cd /data/animeenigma && git worktree remove /data/ae-curated-spotlight && git worktree prune
```

---

## Self-Review

**Spec coverage:**
- Card type `curated` via 5-anchor recipe → Tasks 3 (resolver), 2 (Data type), 4 (DI), 9 (SFC), 5+10 (dispatch/types) + 6 (i18n) + 7 (token). ✅
- Priority 1.5 real numeric field → Task 1 (field + normalize), Task 3 (resolver sets 1.5), Task 8 + 10 (weighted start). ✅
- Airing-status season gate → Task 3 (`status != ongoing` ⇒ nil). ✅
- Env-configurable pin, empty = off → Task 3 + 4. ✅
- Kicker "Curator Recommends"/ru/ja → Task 6; star icon → Task 7. ✅
- Cache flush + live smoke → Task 11. ✅
- Weighted-random collapses to uniform when all 1.0 (backward-compat) → Task 8 test + Task 10 note. ✅

**Placeholder scan:** No TBD/TODO; every code step carries full code. ✅

**Type consistency:** `CuratedData{Anime}` (BE) ↔ `CuratedData{anime}` (FE) match their language's JSON casing; `curated` discriminator identical across resolver `Type()`, union, token key, dispatch branch, `cardTitle`, `cardImageUrls`; `weightedRandomIndex(cards, rng?)` signature identical between Task 8 def and Task 10 use; `anime.Status != domain.StatusOngoing` uses the correct `AnimeStatus` type. ✅
