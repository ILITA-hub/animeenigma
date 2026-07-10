# Fanfic Engine v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add «Continue this story» (one-click append-in-place of a next part) and «Canon continuation» (a generate-form toggle that preloads the anime's real synopsis) to the shipped fanfic engine.

**Architecture:** Extend `services/fanfic/` in place — 2 additive `fanfics` columns, one new SSE route `POST /api/fanfic/{id}/continue`, a `canon` flag on `/generate`, and a fail-soft server-to-server catalog client for the synopsis. Gateway adds one route to the existing `FeatureGate("fanfic")` group. Frontend extends the existing `/fanfics` view (canon `Switch`, library-reader «Продолжить» button).

**Tech Stack:** Go (chi, GORM, google/uuid, prometheus via `libs/metrics`), Groq OpenAI-compatible SSE, Vue 3 + TS + Pinia, vitest, Neon-Tokyo DS (shadcn-vue).

**Spec:** `docs/superpowers/specs/2026-07-10-fanfic-engine-v2-design.md`.

## Global Constraints

- **Worktree:** all work happens in `/tmp/ae-fanfic-v2` (branch `feat/fanfic-v2`). Never edit `/data/animeenigma` paths — absolute paths under the base tree edit the WRONG tree. No push until the land step.
- **Go module:** `github.com/ILITA-hub/animeenigma`; multi-module via `go.work`. Run Go commands from `services/fanfic` (or repo root with the module path).
- **Commit co-authors** (every commit, verbatim):
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **SSE:** fanfic HTTP server keeps `WriteTimeout: 0` (already set). Gateway uses `ProxyToFanficStream` (`proxyStreamFlush`) for streaming routes.
- **GORM:** AutoMigrate only ADDS columns/tables. `PartCount` is set explicitly to `1` in code on generate (do not rely on the `default:` tag for the value) — the "GORM omits zero-value fields" trap.
- **Repo tests:** sqlite in-memory (project convention — NO testcontainers). External HTTP is faked with `net/http/httptest`. No real Groq / catalog in tests.
- **Effort metrics:** never use days/hours. Use UXΔ / CDI / MVQ if a metric is needed.
- **Frontend:** Neon-Tokyo DS tokens only; brand hues `cyan pink orange rose indigo teal lime` are EXEMPT (not off-palette). No raw hex/rgba. i18n keys must exist in **all three** locales en/ru/ja (parity-gated). Run `/frontend-verify` before finishing FE work. Use `bun`, not npm.
- **Reader XSS rule:** `FanficReader.vue` renders TEXT nodes only (`{{ b.text }}`), never `v-html`.
- **gofmt landmine:** never run `gofmt -w` / `make fmt` (smart-quote landmine). Write already-formatted Go.

---

### Task 1: Domain fields + canon request validation

**Files:**
- Modify: `services/fanfic/internal/domain/fanfic.go` (add `Canon`, `PartCount`)
- Modify: `services/fanfic/internal/domain/request.go` (add `Canon` field + canon validation)
- Test: `services/fanfic/internal/domain/request_test.go`

**Interfaces:**
- Produces: `domain.Fanfic.Canon bool`, `domain.Fanfic.PartCount int`; `domain.GenerateRequest.Canon bool`.

- [ ] **Step 1: Add the failing validation test**

Append to `services/fanfic/internal/domain/request_test.go`:

```go
func TestValidate_CanonRequiresAnimeIdentity(t *testing.T) {
	base := GenerateRequest{
		Anime:    AnimeRef{Title: "Frieren"}, // title set, but no id / shikimori_id
		Length:   "oneshot",
		POV:      "third",
		Rating:   "teen",
		Language: "ru",
		Canon:    true,
	}
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: canon without anime id/shikimori_id must be rejected")
	}

	withID := base
	withID.Anime.ID = "11111111-1111-1111-1111-111111111111"
	if err := withID.Validate(); err != nil {
		t.Fatalf("canon with anime.id should pass, got %v", err)
	}

	withShiki := base
	withShiki.Anime.ShikimoriID = "52991"
	if err := withShiki.Validate(); err != nil {
		t.Fatalf("canon with shikimori_id should pass, got %v", err)
	}

	// Non-canon with no identity stays valid (unchanged behavior).
	nonCanon := base
	nonCanon.Canon = false
	if err := nonCanon.Validate(); err != nil {
		t.Fatalf("non-canon without identity should pass, got %v", err)
	}
}
```

- [ ] **Step 2: Run it, verify it fails**

Run: `cd services/fanfic && go test ./internal/domain/ -run TestValidate_CanonRequiresAnimeIdentity -v`
Expected: FAIL (compile error — `Canon` field does not exist yet).

- [ ] **Step 3: Add the domain fields**

In `services/fanfic/internal/domain/fanfic.go`, add these two fields to the `Fanfic` struct, immediately after the `Prompt` field:

```go
	Canon            bool           `gorm:"default:false" json:"canon"`
	PartCount        int            `gorm:"default:1" json:"part_count"`
```

In `services/fanfic/internal/domain/request.go`, add the `Canon` field to `GenerateRequest` (after `Prompt`):

```go
	Prompt     string         `json:"prompt"`
	Canon      bool           `json:"canon"`
```

And add this block to `GenerateRequest.Validate()`, just before the final `return nil`:

```go
	if r.Canon && strings.TrimSpace(r.Anime.ID) == "" && strings.TrimSpace(r.Anime.ShikimoriID) == "" {
		return fmt.Errorf("canon mode requires an anime id or shikimori_id")
	}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `cd services/fanfic && go test ./internal/domain/ -v`
Expected: PASS (all domain tests, including the new one).

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add services/fanfic/internal/domain/
git commit -F - <<'EOF'
feat(fanfic): canon + part_count fields, canon request validation

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 2: Repository `AppendPart`

**Files:**
- Modify: `services/fanfic/internal/repo/fanfic.go`
- Test: `services/fanfic/internal/repo/fanfic_test.go`

**Interfaces:**
- Produces: `func (r *Repository) AppendPart(ctx context.Context, userID, id, appended string, addedUsage, newPartCount int) error` — atomic `content ||` append, sets `part_count`, adds `token_usage`, owner-scoped; 0 rows → `NotFound`.

- [ ] **Step 1: Write the failing test**

Append to `services/fanfic/internal/repo/fanfic_test.go` (reuse the existing test harness that opens the in-memory sqlite DB — mirror an existing test's setup call):

```go
func TestAppendPart(t *testing.T) {
	repo := newTestRepo(t) // existing helper in this file; if named differently, match it
	ctx := context.Background()

	f := &domain.Fanfic{
		UserID: "u1", AnimeTitle: "Frieren", Status: domain.StatusComplete,
		Content: "первая часть", TokenUsage: 100, PartCount: 1,
	}
	if err := repo.Create(ctx, f); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.AppendPart(ctx, "u1", f.ID, "\n\n---\n\n## Часть 2\n\nвторая часть", 55, 2); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := repo.Get(ctx, "u1", f.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PartCount != 2 {
		t.Errorf("part_count = %d, want 2", got.PartCount)
	}
	if got.TokenUsage != 155 {
		t.Errorf("token_usage = %d, want 155", got.TokenUsage)
	}
	if !strings.Contains(got.Content, "первая часть") || !strings.Contains(got.Content, "вторая часть") {
		t.Errorf("content missing a part: %q", got.Content)
	}

	// Non-owner append affects zero rows -> NotFound.
	if err := repo.AppendPart(ctx, "someone-else", f.ID, "x", 1, 3); err == nil {
		t.Error("expected NotFound for non-owner append")
	}
}
```

Ensure `"strings"` and `"context"` are imported in the test file.

- [ ] **Step 2: Run it, verify it fails**

Run: `cd services/fanfic && go test ./internal/repo/ -run TestAppendPart -v`
Expected: FAIL (`repo.AppendPart` undefined).

- [ ] **Step 3: Implement `AppendPart`**

Add to `services/fanfic/internal/repo/fanfic.go` (after `MarkFailed`):

```go
// AppendPart atomically appends `appended` to content, sets part_count, and
// adds addedUsage to token_usage — owner-scoped. The `content || ?` SQL
// expression avoids a read-modify-write race. Zero rows affected (missing or
// non-owner) returns NotFound.
func (r *Repository) AppendPart(ctx context.Context, userID, id, appended string, addedUsage, newPartCount int) error {
	res := r.db.WithContext(ctx).Model(&domain.Fanfic{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]interface{}{
			"content":     gorm.Expr("content || ?", appended),
			"part_count":  newPartCount,
			"token_usage": gorm.Expr("token_usage + ?", addedUsage),
		})
	if res.Error != nil {
		return liberrors.Wrap(res.Error, liberrors.CodeInternal, "append fanfic part")
	}
	if res.RowsAffected == 0 {
		return liberrors.NotFound("fanfic")
	}
	return nil
}
```

`gorm` and `liberrors` are already imported in this file.

- [ ] **Step 4: Run the test, verify it passes**

Run: `cd services/fanfic && go test ./internal/repo/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add services/fanfic/internal/repo/
git commit -F - <<'EOF'
feat(fanfic): repo.AppendPart — atomic owner-scoped part append

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 3: Catalog synopsis client

**Files:**
- Create: `services/fanfic/internal/catalog/client.go`
- Test: `services/fanfic/internal/catalog/client_test.go`

**Interfaces:**
- Produces: `catalog.NewClient(baseURL string, timeout time.Duration, log *logger.Logger) *Client` and `func (c *Client) FetchSynopsis(ctx context.Context, animeID, shikimoriID string) (title, synopsis string, err error)`.

- [ ] **Step 1: Write the failing test**

Create `services/fanfic/internal/catalog/client_test.go`:

```go
package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchSynopsis_ByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/anime/abc" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"name":"Frieren","description":"A mage journeys..."}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 2*time.Second, nil)
	title, synopsis, err := c.FetchSynopsis(context.Background(), "abc", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if title != "Frieren" || synopsis != "A mage journeys..." {
		t.Fatalf("got title=%q synopsis=%q", title, synopsis)
	}
}

func TestFetchSynopsis_ShikimoriFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/anime/shikimori/52991" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"name":"Frieren","description":"desc"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 2*time.Second, nil)
	_, synopsis, err := c.FetchSynopsis(context.Background(), "", "52991")
	if err != nil || synopsis != "desc" {
		t.Fatalf("fallback failed: synopsis=%q err=%v", synopsis, err)
	}
}

func TestFetchSynopsis_ErrorIsGraceful(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 2*time.Second, nil)
	_, synopsis, err := c.FetchSynopsis(context.Background(), "abc", "")
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if synopsis != "" {
		t.Fatalf("synopsis should be empty on error, got %q", synopsis)
	}
}
```

- [ ] **Step 2: Run it, verify it fails**

Run: `cd services/fanfic && go test ./internal/catalog/ -v`
Expected: FAIL (package/`NewClient` undefined).

- [ ] **Step 3: Implement the client**

Create `services/fanfic/internal/catalog/client.go`:

```go
// Package catalog is a thin, fail-soft client for the catalog service's public
// anime endpoint, used to preload an anime's real synopsis for canon-mode
// generation. Mirrors services/anidle/internal/service/poolclient.go.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

type Client struct {
	baseURL string
	client  *http.Client
	log     *logger.Logger
}

func NewClient(baseURL string, timeout time.Duration, log *logger.Logger) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
		log:     log,
	}
}

type animeEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Name        string `json:"name"`
		NameRU      string `json:"name_ru"`
		Description string `json:"description"`
	} `json:"data"`
}

// FetchSynopsis returns (canonicalTitle, synopsis). Prefers the catalog uuid;
// falls back to the shikimori resolve route when only a shikimori id is given.
// Any transport/decoding failure returns a non-nil error and empty strings so
// the caller can degrade gracefully (canon gen proceeds without the preload).
func (c *Client) FetchSynopsis(ctx context.Context, animeID, shikimoriID string) (string, string, error) {
	var endpoint string
	switch {
	case strings.TrimSpace(animeID) != "":
		endpoint = c.baseURL + "/api/anime/" + url.PathEscape(animeID)
	case strings.TrimSpace(shikimoriID) != "":
		endpoint = c.baseURL + "/api/anime/shikimori/" + url.PathEscape(shikimoriID)
	default:
		return "", "", fmt.Errorf("no anime id or shikimori_id")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", fmt.Errorf("build anime request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("anime request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("anime endpoint returned %d", resp.StatusCode)
	}

	var env animeEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return "", "", fmt.Errorf("decode anime envelope: %w", err)
	}
	title := env.Data.Name
	if title == "" {
		title = env.Data.NameRU
	}
	return title, env.Data.Description, nil
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `cd services/fanfic && go test ./internal/catalog/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add services/fanfic/internal/catalog/
git commit -F - <<'EOF'
feat(fanfic): fail-soft catalog synopsis client for canon mode

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 4: Prompt — canon injection + continue-mode messages

**Files:**
- Modify: `services/fanfic/internal/service/prompt.go`
- Test: `services/fanfic/internal/service/prompt_test.go`

**Interfaces:**
- Changes: `BuildMessages(req domain.GenerateRequest, synopsis string) (system, user string)` (new `synopsis` param).
- Produces: `BuildContinueMessages(f domain.Fanfic, prior string) (system, user string)`; `TailRunes(s string, max int) string`.

- [ ] **Step 1: Write the failing tests**

Append to `services/fanfic/internal/service/prompt_test.go`:

```go
func TestBuildMessages_CanonInjectsSynopsis(t *testing.T) {
	req := domain.GenerateRequest{
		Anime:    domain.AnimeRef{Title: "Frieren", Japanese: "葬送のフリーレン"},
		Length:   "oneshot", POV: "third", Rating: "teen", Language: "ru", Canon: true,
		Prompt:   "куда дальше",
	}
	sys, usr := BuildMessages(req, "Фрирен путешествует после смерти Химмеля.")
	if !strings.Contains(usr, "Фрирен путешествует") {
		t.Errorf("synopsis not injected into user prompt: %q", usr)
	}
	if !strings.Contains(sys, "канон") && !strings.Contains(sys, "РЕАЛЬНЫЙ") {
		t.Errorf("canon instruction missing from system prompt: %q", sys)
	}
}

func TestBuildMessages_NonCanonUnchanged(t *testing.T) {
	req := domain.GenerateRequest{
		Anime: domain.AnimeRef{Title: "Frieren"}, Length: "drabble",
		POV: "first", Rating: "teen", Language: "en",
	}
	sys, _ := BuildMessages(req, "")
	if !strings.Contains(sys, "# Title") {
		t.Errorf("non-canon system prompt should keep the title instruction: %q", sys)
	}
}

func TestBuildContinueMessages(t *testing.T) {
	f := domain.Fanfic{
		AnimeTitle: "Frieren", Length: "oneshot", POV: "third",
		Rating: "teen", Language: "ru",
	}
	sys, usr := BuildContinueMessages(f, "конец первой части")
	if strings.Contains(sys, "# Заголовок") || strings.Contains(sys, "# Title") {
		t.Errorf("continue system prompt must NOT instruct a title: %q", sys)
	}
	if !strings.Contains(usr, "конец первой части") {
		t.Errorf("prior context missing from continue user prompt: %q", usr)
	}
}

func TestTailRunes(t *testing.T) {
	if got := TailRunes("abcdef", 3); got != "def" {
		t.Errorf("TailRunes = %q, want def", got)
	}
	if got := TailRunes("ab", 5); got != "ab" {
		t.Errorf("TailRunes short = %q, want ab", got)
	}
	// Multibyte-safe: 5 Cyrillic runes, keep last 2.
	if got := TailRunes("абвгд", 2); got != "гд" {
		t.Errorf("TailRunes cyrillic = %q, want гд", got)
	}
}
```

Update the EXISTING `BuildMessages` calls in this file to pass `""` as the new second arg (they currently call `BuildMessages(req)`), e.g. `sys, usr := BuildMessages(req, "")`.

- [ ] **Step 2: Run it, verify it fails**

Run: `cd services/fanfic && go test ./internal/service/ -run 'TestBuildMessages_Canon|TestBuildContinueMessages|TestTailRunes' -v`
Expected: FAIL (signature mismatch / undefined funcs).

- [ ] **Step 3: Implement the prompt changes**

In `services/fanfic/internal/service/prompt.go`:

(a) Change the signature and add the canon system line + synopsis injection. Replace the `BuildMessages` signature line and the system-prompt title instruction:

```go
func BuildMessages(req domain.GenerateRequest, synopsis string) (string, string) {
```

In the RU system-prompt block, replace the title-instruction line with a canon-aware version:

```go
		if req.Canon {
			fmt.Fprintf(&sys, "Это ПРОДОЛЖЕНИЕ КАНОНА: продолжи РЕАЛЬНЫЙ сюжет аниме за пределами финала, оставаясь верным канону и характерам.\n")
		}
		fmt.Fprintf(&sys, "Ответ начни СТРОГО со строки «# Заголовок», затем с новой строки — текст истории в Markdown.\n")
```

And in the EN system-prompt block:

```go
		if req.Canon {
			fmt.Fprintf(&sys, "This is a CANON CONTINUATION: continue the anime's ACTUAL plot past its finale, staying faithful to canon and characterization.\n")
		}
		fmt.Fprintf(&sys, "Begin your reply STRICTLY with a line '# Title', then on a new line the story in Markdown.\n")
```

(b) Inject the synopsis into the user prompt. In the RU user block, before "Задание автора", add:

```go
	if strings.TrimSpace(synopsis) != "" {
		if ru {
			fmt.Fprintf(&usr, "Официальный синопсис: %s\n", strings.TrimSpace(synopsis))
		} else {
			fmt.Fprintf(&usr, "Official synopsis: %s\n", strings.TrimSpace(synopsis))
		}
	}
```

Place this block AFTER the `Теги:`/`Tags:` line and BEFORE the `Задание автора:`/`Author brief:` line (it must run for both language branches — put it between the language `if/else` that writes fandom/characters/tags and the author-brief line; restructure so the author-brief line is written after this block in both branches). The simplest correct restructure: write fandom/characters/tags inside the `if ru {…} else {…}`, then this synopsis block, then a final `if ru { author brief } else { author brief }`.

(c) Add the continue builder and tail helper at the end of the file:

```go
// BuildContinueMessages builds the system+user prompts to generate the NEXT
// part of an existing fanfic. It reuses the stored rating/POV/language shape
// but omits the title instruction (we're mid-document) and feeds the prior
// text back as context.
func BuildContinueMessages(f domain.Fanfic, prior string) (string, string) {
	ru := f.Language == "ru"
	povWord := "third"
	if f.POV == "first" {
		povWord = "first"
	}
	langName := "ENGLISH"
	if ru {
		langName = "РУССКИЙ"
		povWord = "третьего"
		if f.POV == "first" {
			povWord = "первого"
		}
	}

	var sys strings.Builder
	if ru {
		fmt.Fprintf(&sys, "Ты — талантливый автор фанфиков, пишущий живую художественную прозу.\n")
		fmt.Fprintf(&sys, "Язык вывода строго: %s.\n", langName)
		fmt.Fprintf(&sys, "%s\n", ratingRuleRU(f.Rating))
		fmt.Fprintf(&sys, "Все персонажи — совершеннолетние (18+).\n")
		fmt.Fprintf(&sys, "Повествование от %s лица.\n", povWord)
		if f.Canon {
			fmt.Fprintf(&sys, "Это продолжение канона — оставайся верным сюжету и характерам аниме.\n")
		}
		fmt.Fprintf(&sys, "Это ПРОДОЛЖЕНИЕ уже начатой истории. НЕ пиши заголовок и НЕ повторяй предыдущее — продолжи следующей частью, логично развивая сюжет.")
	} else {
		fmt.Fprintf(&sys, "You are a talented fanfiction author writing vivid literary prose.\n")
		fmt.Fprintf(&sys, "Output language strictly: %s.\n", langName)
		fmt.Fprintf(&sys, "%s\n", ratingRuleEN(f.Rating))
		fmt.Fprintf(&sys, "Portray all characters as adults (18+).\n")
		fmt.Fprintf(&sys, "Write in the %s person.\n", povWord)
		if f.Canon {
			fmt.Fprintf(&sys, "This is a canon continuation — stay faithful to the anime's plot and characters.\n")
		}
		fmt.Fprintf(&sys, "This CONTINUES an in-progress story. Do NOT write a title and do NOT repeat prior text — write the next part, advancing the plot logically.")
	}

	var usr strings.Builder
	if ru {
		fmt.Fprintf(&usr, "Фандом: %s\n", f.AnimeTitle)
		fmt.Fprintf(&usr, "Предыдущие части истории:\n%s\n\nНапиши следующую часть.", prior)
	} else {
		fmt.Fprintf(&usr, "Fandom: %s\n", f.AnimeTitle)
		fmt.Fprintf(&usr, "Story so far:\n%s\n\nWrite the next part.", prior)
	}
	return sys.String(), usr.String()
}

// TailRunes returns the last `max` runes of s (all of s if shorter). Used to
// bound the prior-content context fed into a continuation.
func TailRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[len(r)-max:])
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `cd services/fanfic && go test ./internal/service/ -run 'TestBuildMessages|TestBuildContinueMessages|TestTailRunes' -v`
Expected: PASS. (The generate/quota service tests will still fail to COMPILE until Task 5 updates `NewGenerator` callers — that's expected; this step scopes to the prompt tests via `-run`.)

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add services/fanfic/internal/service/prompt.go services/fanfic/internal/service/prompt_test.go
git commit -F - <<'EOF'
feat(fanfic): canon synopsis injection + continue-mode prompts

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 5: Service — canon preload in Generate + new Continue

**Files:**
- Modify: `services/fanfic/internal/service/generate.go` (Generator deps, canon preload, set PartCount=1)
- Create: `services/fanfic/internal/service/continue.go`
- Test: `services/fanfic/internal/service/generate_test.go` (update `NewGenerator` calls), `services/fanfic/internal/service/continue_test.go`

**Interfaces:**
- Consumes: `domain.Fanfic{Canon,PartCount}` (Task 1), `store.Get`/`store.AppendPart` (Task 2), `catalog.FetchSynopsis` shape (Task 3), `BuildMessages(req, synopsis)` / `BuildContinueMessages` / `TailRunes` (Task 4).
- Changes: `NewGenerator(groq streamer, store fanficStore, quota quota, catalog synopsisFetcher, model string, contextRunes int, log *logger.Logger) *Generator`. `fanficStore` gains `Get` + `AppendPart`.
- Produces: `func (g *Generator) Continue(ctx context.Context, userID, id string, emit Emit) error`.

- [ ] **Step 1: Write the failing continue test**

Create `services/fanfic/internal/service/continue_test.go`. Mirror the fakes already used by `generate_test.go` (a fake streamer, a fake store, a fake quota). Reuse those fakes if the existing test file exposes them; otherwise define minimal ones here:

```go
package service

import (
	"context"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"gorm.io/datatypes"
)

// fakeContinueStore records Get + AppendPart.
type fakeContinueStore struct {
	get       *domain.Fanfic
	appended  string
	addedUse  int
	newPart   int
	appendErr error
}

func (s *fakeContinueStore) Create(context.Context, *domain.Fanfic) error            { return nil }
func (s *fakeContinueStore) UpdateResult(context.Context, string, string, string, int) error { return nil }
func (s *fakeContinueStore) MarkFailed(context.Context, string, string) error        { return nil }
func (s *fakeContinueStore) Get(ctx context.Context, userID, id string) (*domain.Fanfic, error) {
	return s.get, nil
}
func (s *fakeContinueStore) AppendPart(ctx context.Context, userID, id, appended string, addedUsage, newPart int) error {
	s.appended, s.addedUse, s.newPart = appended, addedUsage, newPart
	return s.appendErr
}

func TestContinue_AppendsSectionedPart(t *testing.T) {
	store := &fakeContinueStore{get: &domain.Fanfic{
		ID: "f1", UserID: "u1", AnimeTitle: "Frieren",
		Length: "oneshot", POV: "third", Rating: "teen", Language: "ru",
		Status: domain.StatusComplete, Content: "первая часть", PartCount: 1,
		Characters: datatypes.JSON([]byte(`[]`)), Tags: datatypes.JSON([]byte(`[]`)),
	}}
	// fakeStreamer returns fixed text + usage; reuse the one from generate_test.go.
	g := NewGenerator(&fakeStreamer{text: "# Ignored\nследующая часть", usage: 55}, store,
		&fakeQuota{}, nil, "test-model", 24000, nil)

	var events []string
	emit := func(event string, data any) error { events = append(events, event); return nil }
	if err := g.Continue(context.Background(), "u1", "f1", emit); err != nil {
		t.Fatalf("continue: %v", err)
	}
	if store.newPart != 2 {
		t.Errorf("newPart = %d, want 2", store.newPart)
	}
	if !strings.Contains(store.appended, "## Часть 2") || !strings.Contains(store.appended, "---") {
		t.Errorf("appended not sectioned: %q", store.appended)
	}
	if strings.Contains(store.appended, "# Ignored") {
		t.Errorf("stray title H1 should be stripped: %q", store.appended)
	}
	if store.addedUse != 55 {
		t.Errorf("addedUse = %d, want 55", store.addedUse)
	}
}

func TestContinue_RejectsNonComplete(t *testing.T) {
	store := &fakeContinueStore{get: &domain.Fanfic{
		ID: "f1", UserID: "u1", Status: domain.StatusGenerating,
		Characters: datatypes.JSON([]byte(`[]`)), Tags: datatypes.JSON([]byte(`[]`)),
	}}
	g := NewGenerator(&fakeStreamer{}, store, &fakeQuota{}, nil, "m", 24000, nil)
	err := g.Continue(context.Background(), "u1", "f1", func(string, any) error { return nil })
	if err == nil {
		t.Fatal("expected error continuing a non-complete fanfic")
	}
}
```

> If `fakeStreamer`/`fakeQuota` in `generate_test.go` have different field names, match them. If they are unexported and incompatible, define local equivalents in this file under different names.

- [ ] **Step 2: Run it, verify it fails**

Run: `cd services/fanfic && go test ./internal/service/ -run TestContinue -v`
Expected: FAIL (compile — `Continue`, new `NewGenerator` arity, `Get`/`AppendPart` on the store interface).

- [ ] **Step 3: Extend the Generator (generate.go)**

In `services/fanfic/internal/service/generate.go`:

Add to the `fanficStore` interface:

```go
	Get(ctx context.Context, userID, id string) (*domain.Fanfic, error)
	AppendPart(ctx context.Context, userID, id, appended string, addedUsage, newPartCount int) error
```

Add the synopsis-fetcher interface and extend the struct + constructor:

```go
// synopsisFetcher preloads an anime's real synopsis for canon mode. Nil-safe:
// a nil fetcher (or a non-canon request) skips the preload.
type synopsisFetcher interface {
	FetchSynopsis(ctx context.Context, animeID, shikimoriID string) (title, synopsis string, err error)
}

type Generator struct {
	groq         streamer
	store        fanficStore
	quota        quota
	catalog      synopsisFetcher
	model        string
	contextRunes int
	log          *logger.Logger
}

func NewGenerator(groq streamer, store fanficStore, quota quota, catalog synopsisFetcher, model string, contextRunes int, log *logger.Logger) *Generator {
	if contextRunes <= 0 {
		contextRunes = 24000
	}
	return &Generator{groq: groq, store: store, quota: quota, catalog: catalog, model: model, contextRunes: contextRunes, log: log}
}
```

In `Generate`, set `PartCount: 1` on the new `f` (add the field to the struct literal), and add the canon preload before `BuildMessages`:

```go
	f := &domain.Fanfic{
		// ...existing fields...
		Canon:            req.Canon,
		PartCount:        1,
		Status:           domain.StatusGenerating,
	}
	if err := g.store.Create(ctx, f); err != nil {
		return err
	}
	g.safeEmit(emit, "meta", map[string]any{"id": f.ID, "model": g.model})

	synopsis := ""
	if req.Canon && g.catalog != nil {
		if _, syn, err := g.catalog.FetchSynopsis(ctx, req.Anime.ID, req.Anime.ShikimoriID); err != nil {
			if g.log != nil {
				g.log.Warnw("canon synopsis preload failed; continuing without it", "anime_id", req.Anime.ID, "error", err)
			}
		} else {
			synopsis = syn
		}
	}

	system, user := BuildMessages(req, synopsis)
```

(Replace the existing `system, user := BuildMessages(req)` line.)

- [ ] **Step 4: Implement Continue (continue.go)**

Create `services/fanfic/internal/service/continue.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"fmt"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

// Continue generates the next part of an existing, complete fanfic and appends
// it to that fanfic's content, sectioned by a divider + «Часть N» heading. It
// reuses every stored parameter (length/POV/rating/language/canon); the prior
// content is fed back as context, bounded to contextRunes.
func (g *Generator) Continue(ctx context.Context, userID, id string, emit Emit) error {
	f, err := g.store.Get(ctx, userID, id)
	if err != nil {
		g.safeEmit(emit, "error", map[string]any{"message": err.Error()})
		return err
	}
	if f.Status != domain.StatusComplete {
		e := liberrors.New(liberrors.CodeConflict, "fanfic is not complete")
		g.safeEmit(emit, "error", map[string]any{"message": e.Error()})
		return e
	}

	release, err := g.quota.Acquire(ctx, userID)
	if err != nil {
		g.safeEmit(emit, "error", map[string]any{"message": err.Error()})
		return err
	}
	defer release()

	part := f.PartCount + 1
	heading := headingWord(f.Language)
	prefix := fmt.Sprintf("\n\n---\n\n## %s %d\n\n", heading, part)

	g.safeEmit(emit, "meta", map[string]any{"id": f.ID, "model": g.model, "part": part})
	// Emit the divider+heading first so the live reader matches the stored form.
	g.safeEmit(emit, "delta", map[string]any{"text": prefix})

	prior := TailRunes(f.Content, g.contextRunes)
	system, user := BuildContinueMessages(*f, prior)

	text, usage, err := g.groq.Stream(ctx, system, user, MaxTokensFor(f.Length), 0.9, func(delta string) {
		g.safeEmit(emit, "delta", map[string]any{"text": delta})
	})
	if err != nil {
		g.safeEmit(emit, "error", map[string]any{"message": err.Error()})
		return err
	}

	// Strip any stray leading title the model emitted; keep the body.
	_, body := SplitTitle(text)
	if body == "" {
		body = text
	}
	appended := prefix + body
	if err := g.store.AppendPart(ctx, userID, id, appended, usage, part); err != nil {
		if g.log != nil {
			g.log.Errorw("failed to append fanfic part", "id", id, "error", err)
		}
	}
	if g.log != nil {
		g.log.Infow("fanfic continued", "user_id", userID, "fanfic_id", id, "action", "continue",
			"canon", f.Canon, "part", part, "token_usage", usage, "status", "complete")
	}
	g.safeEmit(emit, "done", map[string]any{"id": f.ID, "part": part, "token_usage": usage})
	return nil
}

// headingWord returns the localized «part» heading word.
func headingWord(language string) string {
	if language == "ru" {
		return "Часть"
	}
	return "Part"
}

// ensure encoding/json stays referenced if future edits drop it; the reserved
// import keeps parity with generate.go's characters marshalling helpers.
var _ = json.Marshal
```

> If the linter flags the unused `encoding/json` import, drop both the import and the `var _ = json.Marshal` line — it is only a guard.

- [ ] **Step 5: Update `generate_test.go` NewGenerator calls**

Every `NewGenerator(...)` call in `generate_test.go` gains the two new args. Add `nil` (no catalog) as the 4th arg and `24000` as the 6th, e.g.:

```go
g := NewGenerator(fakeGroq, fakeStore, fakeQuota, nil, "test-model", 24000, nil)
```

If any existing test's fake store does not implement `Get`/`AppendPart`, add no-op methods to it (return `nil, nil` / `nil`).

- [ ] **Step 6: Run the service tests, verify pass**

Run: `cd services/fanfic && go test ./internal/service/ -v`
Expected: PASS (generate, prompt, quota, continue).

- [ ] **Step 7: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add services/fanfic/internal/service/
git commit -F - <<'EOF'
feat(fanfic): canon synopsis preload + Continue (append next part)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 6: Handler + router — `POST /{id}/continue`

**Files:**
- Modify: `services/fanfic/internal/handler/fanfic.go` (add `Continue`; extend `generator` interface)
- Modify: `services/fanfic/internal/transport/router.go` (register route)
- Test: `services/fanfic/internal/handler/fanfic_test.go`

**Interfaces:**
- Consumes: `service.Generator.Continue` (Task 5), `libraryStore.Get` (existing).
- Produces: `Handler.Continue(w, r)` mounted at `POST /api/fanfic/{id}/continue`.

- [ ] **Step 1: Write the failing handler tests**

Append to `services/fanfic/internal/handler/fanfic_test.go` (mirror the existing handler-test harness: a fake generator + fake libraryStore + an authed request-context helper already present in this file):

```go
func TestContinue_409OnNonComplete(t *testing.T) {
	repo := &fakeLibraryStore{get: &domain.Fanfic{ID: "f1", UserID: "u1", Status: domain.StatusGenerating}}
	h := NewHandler(&fakeGenerator{}, repo, nil)

	req := authedRequest(t, http.MethodPost, "/api/fanfic/f1/continue", "u1") // existing helper
	req = withURLParam(req, "id", "f1")                                       // existing chi-param helper
	rec := httptest.NewRecorder()
	h.Continue(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestContinue_StreamsWhenComplete(t *testing.T) {
	repo := &fakeLibraryStore{get: &domain.Fanfic{ID: "f1", UserID: "u1", Status: domain.StatusComplete}}
	gen := &fakeGenerator{continueEvents: []string{"meta", "delta", "done"}}
	h := NewHandler(gen, repo, nil)

	req := authedRequest(t, http.MethodPost, "/api/fanfic/f1/continue", "u1")
	req = withURLParam(req, "id", "f1")
	rec := httptest.NewRecorder()
	h.Continue(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}
	if !gen.continueCalled {
		t.Fatal("expected gen.Continue to be called")
	}
}
```

> Match the exact names of the existing fakes/helpers in this test file. Add a `Continue` method + `continueCalled`/`continueEvents` fields to the existing `fakeGenerator`, and a `get` field + `Get` method to the existing fake libraryStore if not already present. If the fake libraryStore's `Get` returns an error for missing rows, model the 404 case similarly.

- [ ] **Step 2: Run it, verify it fails**

Run: `cd services/fanfic && go test ./internal/handler/ -run TestContinue -v`
Expected: FAIL (`Handler.Continue` undefined; `generator` interface lacks `Continue`).

- [ ] **Step 3: Extend the generator interface + add the handler**

In `services/fanfic/internal/handler/fanfic.go`, add to the `generator` interface:

```go
	Continue(ctx context.Context, userID, id string, emit service.Emit) error
```

Add the handler method (place after `Generate`):

```go
// Continue streams the next part of an existing fanfic as SSE and appends it
// on completion. Ownership + complete-status are checked BEFORE switching to
// SSE so a rejected request returns a real 404/409 (not an SSE error frame).
func (h *Handler) Continue(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}
	id := chi.URLParam(r, "id")

	f, err := h.repo.Get(r.Context(), id, userID) // NOTE: match libraryStore.Get(ctx, userID, id) arg order
	if err != nil {
		httputil.Error(w, err) // owner-scoped NotFound -> 404
		return
	}
	if f.Status != domain.StatusComplete {
		httputil.Error(w, liberrors.New(liberrors.CodeConflict, "fanfic is not complete"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	emit := func(event string, data any) error {
		payload, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
			return err
		}
		return rc.Flush()
	}

	ctx := context.WithoutCancel(r.Context())
	if err := h.gen.Continue(ctx, userID, id, emit); err != nil && h.log != nil {
		h.log.Warnw("fanfic continue ended with error", "user_id", userID, "fanfic_id", id, "error", err)
	}
}
```

Add imports to `fanfic.go` if missing: `"github.com/ILITA-hub/animeenigma/libs/errors"` aliased as `liberrors` and `"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"` (domain is already imported). Fix the `h.repo.Get` argument order to match the actual `libraryStore.Get(ctx, userID, id)` signature.

- [ ] **Step 4: Register the route**

In `services/fanfic/internal/transport/router.go`, inside the `/api/fanfic` group, add after the `/generate` line:

```go
		r.Post("/{id}/continue", h.Continue)
```

- [ ] **Step 5: Run tests, verify pass**

Run: `cd services/fanfic && go test ./... -v`
Expected: PASS (whole service).

- [ ] **Step 6: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add services/fanfic/internal/handler/ services/fanfic/internal/transport/
git commit -F - <<'EOF'
feat(fanfic): POST /{id}/continue SSE handler + route

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 7: Config + main wiring + compose

**Files:**
- Modify: `services/fanfic/internal/config/config.go`
- Modify: `services/fanfic/cmd/fanfic-api/main.go`
- Modify: `docker/docker-compose.yml`

**Interfaces:**
- Consumes: `catalog.NewClient` (Task 3), the new `NewGenerator` arity (Task 5).
- Produces: `cfg.CatalogURL string`, `cfg.CatalogTimeout time.Duration`, `cfg.ContinueContextRunes int`.

- [ ] **Step 1: Add config fields**

In `services/fanfic/internal/config/config.go`, add to `Config`:

```go
	CatalogURL           string
	CatalogTimeout       time.Duration
	ContinueContextRunes int
```

And in `Load()`'s returned struct:

```go
		CatalogURL:           getEnv("CATALOG_URL", "http://catalog:8081"),
		CatalogTimeout:       getEnvDuration("FANFIC_CATALOG_TIMEOUT", 5*time.Second),
		ContinueContextRunes: getEnvInt("FANFIC_CONTINUE_CONTEXT_RUNES", 24000),
```

- [ ] **Step 2: Wire the catalog client + new Generator args in main.go**

In `services/fanfic/cmd/fanfic-api/main.go`:

Add the import:

```go
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/catalog"
```

Replace the generator construction:

```go
	catalogClient := catalog.NewClient(cfg.CatalogURL, cfg.CatalogTimeout, log)
	generator := service.NewGenerator(groqClient, fanficRepo, quota, catalogClient, cfg.Groq.Model, cfg.ContinueContextRunes, log)
```

- [ ] **Step 3: Build the service**

Run: `cd services/fanfic && go build ./... && go vet ./...`
Expected: no errors.

- [ ] **Step 4: Add compose env**

In `docker/docker-compose.yml`, find the `fanfic:` service block. Add to its `environment:` map:

```yaml
      CATALOG_URL: http://catalog:8081
      FANFIC_CATALOG_TIMEOUT: 5s
      FANFIC_CONTINUE_CONTEXT_RUNES: "24000"
```

And add `catalog` to the `fanfic` service's `depends_on:` list (ordering only; the client is fail-soft). Match the existing YAML indentation/style exactly.

- [ ] **Step 5: Validate compose**

Run: `cd docker && docker compose config >/dev/null && echo OK`
Expected: `OK` (no YAML errors).

- [ ] **Step 6: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add services/fanfic/internal/config/ services/fanfic/cmd/ docker/docker-compose.yml
git commit -F - <<'EOF'
feat(fanfic): CATALOG_URL + continue-context config; wire catalog client

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 8: Gateway route

**Files:**
- Modify: `services/gateway/internal/transport/router.go`

**Interfaces:**
- Consumes: existing `proxyHandler.ProxyToFanficStream` (flushing SSE proxy).

- [ ] **Step 1: Add the continue route**

In `services/gateway/internal/transport/router.go`, in the `r.Route("/fanfic", …)` block, add immediately after the `/generate` line:

```go
			r.Post("/{id}/continue", proxyHandler.ProxyToFanficStream)
```

(It sits inside the same group, so it inherits `FeatureGate("fanfic")` + JWT + guest-block. Use the streaming proxy so continue deltas flush live.)

- [ ] **Step 2: Build the gateway**

Run: `cd services/gateway && go build ./... && go vet ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add services/gateway/internal/transport/router.go
git commit -F - <<'EOF'
feat(gateway): route POST /api/fanfic/{id}/continue to the SSE proxy

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 9: Frontend types + `continueStory` API

**Files:**
- Modify: `frontend/web/src/types/fanfic.ts`
- Modify: `frontend/web/src/api/fanfic.ts`
- Test: `frontend/web/src/api/__tests__/fanfic.spec.ts`

**Interfaces:**
- Produces: `GenerateInput.canon?: boolean`; `Fanfic.canon: boolean` + `Fanfic.part_count: number`; `StreamHandlers.onMeta?(id, model, part?)` / `onDone?(id, title, tokenUsage, part?)`; `fanficApi.continueStory(id, handlers, signal?)`.

- [ ] **Step 1: Write the failing test**

Append to `frontend/web/src/api/__tests__/fanfic.spec.ts` a test for the SSE `part` field parsing (mirror the existing `parseSSEBuffer`/`handleSSEEvent` tests in this file):

```ts
it('handleSSEEvent surfaces the part number on meta/done', () => {
  const parts: number[] = []
  handleSSEEvent(
    { event: 'meta', data: { id: 'f1', model: 'm', part: 2 } },
    { onMeta: (_id, _model, part) => part !== undefined && parts.push(part) },
  )
  handleSSEEvent(
    { event: 'done', data: { id: 'f1', title: '', token_usage: 5, part: 2 } },
    { onDone: (_id, _t, _u, part) => part !== undefined && parts.push(part) },
  )
  expect(parts).toEqual([2, 2])
})
```

- [ ] **Step 2: Run it, verify it fails**

Run: `cd frontend/web && bunx vitest run src/api/__tests__/fanfic.spec.ts`
Expected: FAIL (onMeta/onDone don't pass `part`).

- [ ] **Step 3: Update types**

In `frontend/web/src/types/fanfic.ts`:
- Add `canon?: boolean` to `GenerateInput`.
- Add `canon: boolean` and `part_count: number` to `Fanfic`.
- Update `StreamHandlers`:

```ts
export interface StreamHandlers {
  onMeta?: (id: string, model: string, part?: number) => void
  onDelta?: (text: string) => void
  onDone?: (id: string, title: string, tokenUsage: number, part?: number) => void
  onError?: (message: string) => void
}
```

- [ ] **Step 4: Update the api client**

In `frontend/web/src/api/fanfic.ts`:

(a) In `handleSSEEvent`, pass `part` through:

```ts
    case 'meta': {
      const d = evt.data as { id: string; model: string; part?: number }
      h.onMeta?.(d.id, d.model, d.part)
      break
    }
    ...
    case 'done': {
      const d = evt.data as { id: string; title: string; token_usage: number; part?: number }
      h.onDone?.(d.id, d.title, d.token_usage, d.part)
      break
    }
```

(b) DRY the SSE plumbing: extract the shared reader used by `generate` into a private `streamSSE(path, body, handlers, signal)` and have BOTH `generate` and the new `continueStory` call it. `streamSSE` contains everything from the `attempt()`/401-refresh/`getReader()` loop currently inside `generate`; `generate` becomes `streamSSE('/fanfic/generate', input, handlers, signal)`. Add:

```ts
  /** Stream a continuation of a saved fanfic (empty body — params reused server-side). */
  async continueStory(id: string, handlers: StreamHandlers, signal?: AbortSignal): Promise<void> {
    return streamSSE(`/fanfic/${encodeURIComponent(id)}/continue`, undefined, handlers, signal)
  },
```

`streamSSE` posts `JSON.stringify(body ?? {})`. Keep the exact 401-refresh-retry and AbortError handling already in `generate`.

- [ ] **Step 5: Run tests, verify pass**

Run: `cd frontend/web && bunx vitest run src/api/__tests__/fanfic.spec.ts`
Expected: PASS.

- [ ] **Step 6: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit -p tsconfig.app.json 2>&1 | head -20`
Expected: no new errors in `fanfic.ts` / `types/fanfic.ts`.

- [ ] **Step 7: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add frontend/web/src/types/fanfic.ts frontend/web/src/api/fanfic.ts frontend/web/src/api/__tests__/fanfic.spec.ts
git commit -F - <<'EOF'
feat(fanfic-web): canon/part_count types + continueStory SSE client

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 10: Reader `hr` block

**Files:**
- Modify: `frontend/web/src/components/fanfic/renderFanfic.ts`
- Modify: `frontend/web/src/components/fanfic/FanficReader.vue`
- Test: `frontend/web/src/components/fanfic/__tests__/renderFanfic.spec.ts`

**Interfaces:**
- Changes: `FanficBlock` type gains `'hr'`; `renderFanfic` emits `{ type: 'hr', text: '' }` for a `---`/`***`/`___` line.

- [ ] **Step 1: Write the failing test**

Append to `frontend/web/src/components/fanfic/__tests__/renderFanfic.spec.ts`:

```ts
it('renders a horizontal rule for --- / *** / ___', () => {
  const blocks = renderFanfic('первая часть\n\n---\n\n## Часть 2\n\nвторая часть')
  expect(blocks.some((b) => b.type === 'hr')).toBe(true)
  // The divider must NOT leak as literal paragraph text.
  expect(blocks.some((b) => b.type === 'p' && b.text.trim() === '---')).toBe(false)
  // The heading still renders as h3 (## maps to h3).
  expect(blocks.some((b) => b.type === 'h3' && b.text === 'Часть 2')).toBe(true)
})
```

- [ ] **Step 2: Run it, verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/fanfic/__tests__/renderFanfic.spec.ts`
Expected: FAIL (no `hr` block; `---` becomes a paragraph).

- [ ] **Step 3: Implement**

In `frontend/web/src/components/fanfic/renderFanfic.ts`, update the type and the loop:

```ts
export type FanficBlock = { type: 'h2' | 'h3' | 'p' | 'hr'; text: string }

export function renderFanfic(md: string): FanficBlock[] {
  const blocks: FanficBlock[] = []
  for (const raw of md.split(/\n{2,}/)) {
    const chunk = raw.trim()
    if (!chunk) continue
    if (/^([-*_])\1{2,}$/.test(chunk)) blocks.push({ type: 'hr', text: '' })
    else if (chunk.startsWith('## ')) blocks.push({ type: 'h3', text: chunk.slice(3).trim() })
    else if (chunk.startsWith('# ')) blocks.push({ type: 'h2', text: chunk.slice(2).trim() })
    else blocks.push({ type: 'p', text: chunk.replace(/\n/g, ' ') })
  }
  return blocks
}
```

In `frontend/web/src/components/fanfic/FanficReader.vue`, add an `hr` branch in the block `<template v-for>` (before the `<p>` fallback):

```vue
      <hr v-else-if="b.type === 'hr'" class="my-6 border-border" />
```

- [ ] **Step 4: Run tests, verify pass**

Run: `cd frontend/web && bunx vitest run src/components/fanfic/__tests__/renderFanfic.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add frontend/web/src/components/fanfic/renderFanfic.ts frontend/web/src/components/fanfic/FanficReader.vue frontend/web/src/components/fanfic/__tests__/renderFanfic.spec.ts
git commit -F - <<'EOF'
feat(fanfic-web): render --- as an <hr> divider in the reader

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 11: GenerateForm canon toggle

**Files:**
- Modify: `frontend/web/src/components/fanfic/GenerateForm.vue`
- Test: `frontend/web/src/components/fanfic/__tests__/GenerateForm.spec.ts`

**Interfaces:**
- Consumes: `Switch` from `@/components/ui`; `GenerateInput.canon` (Task 9).
- Produces: `buildInput()` includes `canon`; when canon on, the prompt is optional in `canGenerate`.

- [ ] **Step 1: Write the failing test**

Append to `frontend/web/src/components/fanfic/__tests__/GenerateForm.spec.ts` (mirror the existing mount + `defineExpose` access pattern used by the other tests in this file):

```ts
it('canon mode makes the prompt optional and is emitted in the input', async () => {
  const wrapper = mountForm() // existing helper/mount used elsewhere in this file
  const vm = wrapper.vm as any
  vm.selectedAnime = { id: 'a1', title: 'Frieren' }
  vm.prompt = ''
  vm.canon = true
  await wrapper.vm.$nextTick()
  expect(vm.canGenerate).toBe(true) // empty prompt is OK in canon mode
  expect(vm.buildInput().canon).toBe(true)
})
```

If the file has no shared `mountForm`, mount `GenerateForm` directly with the i18n global plugin used by the other specs.

- [ ] **Step 2: Run it, verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/fanfic/__tests__/GenerateForm.spec.ts`
Expected: FAIL (`canon` not exposed / prompt still required).

- [ ] **Step 3: Implement**

In `frontend/web/src/components/fanfic/GenerateForm.vue`:
- Import `Switch`: add to the `@/components/ui` import → `import { Input, Select, Chip, Button, Switch } from '@/components/ui'`.
- Add state: `const canon = ref(false)`.
- Relax `canGenerate` so canon drops the prompt requirement:

```ts
const canGenerate = computed(
  () =>
    !!selectedAnime.value &&
    (canon.value || prompt.value.trim().length > 0) &&
    !promptOverLimit.value &&
    !props.disabled,
)
```

- Include `canon` in `buildInput()`:

```ts
    prompt: prompt.value.trim(),
    canon: canon.value,
```

- Add `canon` to `defineExpose({ … })`.
- In the template, add a canon toggle row above the Prompt block:

```vue
    <!-- Canon continuation -->
    <div class="flex items-center justify-between rounded-xl border border-border bg-card p-3">
      <div>
        <p class="text-sm font-medium text-white/80">{{ t('fanfic.canon.label') }}</p>
        <p class="text-xs text-muted-foreground">{{ t('fanfic.canon.hint') }}</p>
      </div>
      <Switch v-model="canon" :aria-label="t('fanfic.canon.label')" />
    </div>
```

- Make the prompt label/placeholder canon-aware:

```vue
      <label class="block text-sm font-medium text-white/70 mb-2">
        {{ canon ? t('fanfic.canon.directionLabel') : t('fanfic.form.prompt') }}
      </label>
```
and `:placeholder="canon ? t('fanfic.canon.directionPlaceholder') : t('fanfic.form.promptPlaceholder')"` on the textarea.

> Verify `Switch` uses `v-model` (check `frontend/web/src/components/ui/Switch.vue` — if it emits `update:modelValue` via a different prop, bind accordingly). Do not hardcode colors; the toggle uses DS tokens already.

- [ ] **Step 4: Run tests, verify pass**

Run: `cd frontend/web && bunx vitest run src/components/fanfic/__tests__/GenerateForm.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add frontend/web/src/components/fanfic/GenerateForm.vue frontend/web/src/components/fanfic/__tests__/GenerateForm.spec.ts
git commit -F - <<'EOF'
feat(fanfic-web): canon-continuation toggle in the generate form

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 12: Library «Продолжить» button + canon badge

**Files:**
- Modify: `frontend/web/src/views/FanficsView.vue` (continue wiring + Modal footer button)
- Modify: `frontend/web/src/components/fanfic/LibraryGrid.vue` (canon badge)

**Interfaces:**
- Consumes: `fanficApi.continueStory` (Task 9), `Fanfic.canon`/`part_count` (Task 9).

- [ ] **Step 1: Add continue wiring in FanficsView.vue**

In `frontend/web/src/views/FanficsView.vue` `<script setup>`, add continue state + handler (after the reader-dialog state):

```ts
const continuing = ref(false)
let continueAbort: AbortController | null = null

async function onContinueFanfic(): Promise<void> {
  const f = readerFanfic.value
  if (!f || continuing.value) return
  continuing.value = true
  continueAbort?.abort()
  continueAbort = new AbortController()
  await fanficApi.continueStory(
    f.id,
    {
      onDelta: (text) => {
        if (readerFanfic.value) readerFanfic.value.content += text
      },
      onDone: (_id, _title, _usage, part) => {
        if (readerFanfic.value && part) readerFanfic.value.part_count = part
        continuing.value = false
        libraryGridRef.value?.refresh()
      },
      onError: () => {
        continuing.value = false
      },
    },
    continueAbort.signal,
  )
  continuing.value = false
}
```

Add `continuing` and `onContinueFanfic` to `defineExpose`, and abort on unmount by extending the existing `onBeforeUnmount` to also call `continueAbort?.abort()`.

- [ ] **Step 2: Add the Modal footer button**

In the reader `Modal` in `FanficsView.vue`, add a `#footer` slot with the continue button:

```vue
    <Modal v-model="readerOpen" :title="readerFanfic?.title" size="xl">
      <FanficReader v-if="readerFanfic" :title="readerFanfic.title" :content="readerFanfic.content" :streaming="continuing" />
      <template v-if="readerFanfic && readerFanfic.status === 'complete'" #footer>
        <Button :loading="continuing" @click="onContinueFanfic">{{ t('fanfic.reader.continue') }}</Button>
      </template>
    </Modal>
```

(`Button` is already imported in this view.)

- [ ] **Step 3: Add the canon badge in LibraryGrid.vue**

In `frontend/web/src/components/fanfic/LibraryGrid.vue`, in the badges row, add a canon badge (use the brand-hue `Badge`; `cyan` is DS-exempt). After the rating `Badge`:

```vue
              <Badge v-if="f.canon" variant="default" size="sm" class="text-cyan-400">{{ t('fanfic.library.canonBadge') }}</Badge>
```

> If `Badge` supports a semantic `variant` that reads as an accent, prefer that over the utility class. Keep it DS-compliant (no hex; `cyan` is an allowlisted brand hue).

- [ ] **Step 4: Type-check + run the view/grid specs**

Run: `cd frontend/web && bunx vue-tsc --noEmit -p tsconfig.app.json 2>&1 | head -20`
Then: `cd frontend/web && bunx vitest run src/views/__tests__/FanficsView.spec.ts src/components/fanfic/`
Expected: no new type errors; specs pass (update `FanficsView.spec.ts` `defineExpose` expectations if it asserts the exposed key set).

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add frontend/web/src/views/FanficsView.vue frontend/web/src/components/fanfic/LibraryGrid.vue frontend/web/src/views/__tests__/FanficsView.spec.ts
git commit -F - <<'EOF'
feat(fanfic-web): library «Продолжить» button + canon badge

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 13: i18n keys (en/ru/ja parity)

**Files:**
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`

**Interfaces:**
- Produces the keys referenced by Tasks 11–12: `fanfic.canon.label`, `fanfic.canon.hint`, `fanfic.canon.directionLabel`, `fanfic.canon.directionPlaceholder`, `fanfic.reader.continue`, `fanfic.library.canonBadge`.

- [ ] **Step 1: Add keys to all three locales**

Under the existing `fanfic` object in each locale, add a `canon` block and the two new leaf keys, keeping each locale's own translations. Values:

**en.json** (`fanfic.canon` + `fanfic.reader.continue` + `fanfic.library.canonBadge`):
```json
"canon": {
  "label": "Canon continuation",
  "hint": "Continue the anime's real plot, past where it ended.",
  "directionLabel": "Where should the plot go? (optional)",
  "directionPlaceholder": "e.g. Frieren finds an old letter from Himmel…"
}
```
`fanfic.reader.continue`: `"Continue"` · `fanfic.library.canonBadge`: `"canon"`

**ru.json:**
```json
"canon": {
  "label": "Продолжение канона",
  "hint": "Продолжить реальный сюжет аниме — за пределами финала.",
  "directionLabel": "Куда двигаться сюжету? (необязательно)",
  "directionPlaceholder": "напр. Фрирен находит старое письмо Химмеля…"
}
```
`fanfic.reader.continue`: `"Продолжить"` · `fanfic.library.canonBadge`: `"канон"`

**ja.json:**
```json
"canon": {
  "label": "原作の続き",
  "hint": "アニメの本編のその先を描きます。",
  "directionLabel": "物語をどう進める？（任意）",
  "directionPlaceholder": "例：フリーレンがヒンメルの古い手紙を見つける…"
}
```
`fanfic.reader.continue`: `"続きを書く"` · `fanfic.library.canonBadge`: `"原作"`

Place `reader.continue` inside the existing `fanfic.reader` object and `library.canonBadge` inside `fanfic.library` in each file.

- [ ] **Step 2: Verify i18n parity + JSON validity**

Run: `cd frontend/web && node -e "for(const l of ['en','ru','ja']){JSON.parse(require('fs').readFileSync('src/locales/'+l+'.json','utf8'))};console.log('json ok')"`
Then run the repo's i18n parity check if present (e.g. part of `/frontend-verify`).
Expected: valid JSON; en/ru/ja have identical key sets for the new keys.

- [ ] **Step 3: Commit**

```bash
cd /tmp/ae-fanfic-v2
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -F - <<'EOF'
feat(fanfic-web): i18n keys for canon toggle + continue (en/ru/ja)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 14: Full verification + `/frontend-verify`

**Files:** none (verification only)

- [ ] **Step 1: Whole-service Go tests + vet**

Run: `cd services/fanfic && go test ./... && go vet ./...`
Then gateway: `cd services/gateway && go build ./... && go vet ./...`
Expected: all PASS / no vet errors.

- [ ] **Step 2: Frontend suite**

Run: `cd frontend/web && bunx vitest run src/components/fanfic/ src/api/__tests__/fanfic.spec.ts src/views/__tests__/FanficsView.spec.ts`
Expected: PASS.

- [ ] **Step 3: `/frontend-verify`**

Invoke the `/frontend-verify` skill (DS-lint + i18n en/ru/ja parity + real `bun run build` + TS/lucide/Tailwind traps) over the `frontend/web` changes. Fix any violations it reports (DS-lint, i18n parity, build).
Expected: green.

- [ ] **Step 4: Confirm no base-tree contamination**

Run: `cd /tmp/ae-fanfic-v2 && git status --porcelain && git log --oneline origin/main..HEAD`
Expected: clean tree; the commit list shows Tasks 1–13 (+ the spec/plan docs).

This task has no commit of its own — it gates the land/deploy step (`/animeenigma-after-update`), which runs after the plan is complete.

---

## Self-Review

**Spec coverage:**
- §3 data model (Canon, PartCount) → Task 1. ✓
- §4.1 canon request + validation → Task 1. ✓
- §4.2 `/{id}/continue` SSE, 404/409, live divider → Tasks 5 (service), 6 (handler status guard + SSE). ✓
- §5.1 canon preload + fail-soft → Task 5 (Generate). ✓
- §5.2 continue-mode prompt, no-title, tail budget, prefix-as-first-delta → Tasks 4 (prompt), 5 (Continue). ✓
- §5.3 prompt templates → Task 4. ✓
- §6 AppendPart atomic → Task 2. ✓
- §7 catalog client → Task 3. ✓
- §8 gateway route → Task 8. ✓
- §9 config/compose → Task 7. ✓
- §10 FE types/api/form/reader/badge/i18n → Tasks 9–13. ✓
- §11 logging (action/canon/part) → Task 5 (`log.Infow` in Continue; canon Generate keeps existing logging). ✓
- §12 tests → embedded per task + Task 14. ✓

**Placeholder scan:** No TBD/TODO; each code step carries full code. The only "match the existing fake/helper names" notes (Tasks 5–6, 11) are unavoidable — they point at real, already-present test harnesses the implementer must read; the test bodies themselves are complete.

**Type consistency:** `AppendPart(ctx, userID, id, appended, addedUsage, newPartCount)` identical across repo (Task 2), service `fanficStore` (Task 5), and fake (Task 5). `FetchSynopsis(ctx, animeID, shikimoriID) (title, synopsis, err)` identical across client (Task 3), `synopsisFetcher` (Task 5), and tests. `BuildMessages(req, synopsis)` updated at definition (Task 4) and both call sites (Task 4 tests, Task 5 Generate). `Continue(ctx, userID, id, emit)` identical across service (Task 5), `generator` interface + handler (Task 6), fakes (Tasks 5–6). SSE `part` field threaded through Go (Tasks 5–6) and TS `onMeta`/`onDone` (Task 9). ⚠ handler `h.repo.Get` arg order flagged in Task 6 to match `libraryStore.Get(ctx, userID, id)`.
