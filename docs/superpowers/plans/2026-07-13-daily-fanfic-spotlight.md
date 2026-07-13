# Daily Fanfic Spotlight — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a daily-rotated «Фанфик дня» spotlight card that showcases one user fanfic (or an auto-generated bot fanfic when none exists in 24h), where the auto-generate path doubles as a Groq API-key health probe that alerts on 401; and flip the fanfic feature fully public.

**Architecture:** Fanfic service (`:8097`) owns the pick/generation/alert and exposes `GET /internal/fanfic/daily` (compact, spotlight), `GET /api/fanfic/daily` (public reader), `POST /internal/fanfic/ensure-daily` (generate + key-probe + alert). A scheduler cron drives ensure-daily daily. The catalog spotlight aggregator reads the compact DTO under its 800ms deadline via a new HTTP client + resolver (the standard 5-anchor spotlight recipe). The Vue frontend adds one card SFC + an author opt-in checkbox.

**Tech Stack:** Go (chi, GORM, robfig/cron v3), Vue 3 + TypeScript + Tailwind v4, Redis (spotlight cache), Groq (OpenAI-compatible), Telegram Bot API (alerts).

## Global Constraints

- **NO time-effort units.** Score plans/changelog with UXΔ / CDI / MVQ per `.planning/CONVENTIONS.md`.
- **Work only in the worktree** `/data/animeenigma/.claude/worktrees/daily-fanfic-spotlight` — NEVER edit `/data/animeenigma/<path>` directly (absolute base-tree paths silently bypass the worktree). Exception: `docker/.env` (host-only, git-ignored) is edited in the base tree.
- **Land via `bin/ae-land.sh`** (run from the worktree): `printf '<subject>\n\n<body>' | bash bin/ae-land.sh <file …>` — appends the 3 standard co-authors, rebases onto origin/main, pushes HEAD:main. Never `git add -A` (sweeps untracked worktrees/binaries).
- **Co-authors** (ae-land adds them): `Claude Code <noreply@anthropic.com>`, `0neymik0 <0neymik0@gmail.com>`, `NANDIorg <super.egor.mamonov@yandex.ru>`.
- **JSON is snake_case end-to-end** (no FE transform step) — every new payload field uses snake_case in Go tags AND the TS interface.
- **Spotlight resolvers must never block** — 800ms/card deadline; the resolver only does a fast HTTP read (700ms client timeout), never generation.
- **GORM `default:false` on a bool omits the field on false inserts** — set the three new bool columns explicitly in every insert path.
- **Go build:** each service is its own module in `go.work`; run `go build ./...` + `go test ./...` from the service dir. **Never** `go work sync` / `gofmt -w` / `make fmt`.
- **FE gates:** run `bin/ae-fe-verify.sh <files>` (DS-lint + eslint + `bun run build` + touched vitest) before landing FE; add i18n keys to en/ru/ja.
- **Fanfic model** (`services/fanfic/internal/domain/fanfic.go`): `Fanfic{ID,UserID,AnimeID,AnimeShikimoriID,AnimeTitle,AnimeJapanese,AnimePoster,Characters,Tags,Length,POV,Rating,Language,Prompt,Canon,PartCount,Title,Content,Model,TokenUsage,Status,ErrorMsg,CreatedAt,UpdatedAt,DeletedAt}`; statuses `generating|complete|failed`; `TableName()="fanfics"`; `BeforeCreate` sets a UUID.

---

## Phase 1 — Fanfic service: data + pick + probe/alert (independently testable via `go test` + curl)

### Task 1: Schema columns + eligible-list repo query

**Files:**
- Modify: `services/fanfic/internal/domain/fanfic.go` (add 3 fields)
- Modify: `services/fanfic/internal/repo/fanfic.go` (add `ListEligibleSince`, `DailyBotExists`)
- Test: `services/fanfic/internal/repo/fanfic_test.go` (extend)

**Interfaces:**
- Produces: `domain.Fanfic{AuthorUsername string, SpotlightCredit bool, AIGenerated bool}`; `Repository.ListEligibleSince(ctx context.Context, since time.Time) ([]domain.Fanfic, error)` (all users, `status='complete'`, `created_at > since`, ordered `created_at ASC, id ASC`); `Repository.DailyBotExists(ctx context.Context, since time.Time) (bool, error)`.

- [ ] **Step 1: Write the failing test** — append to `fanfic_test.go`:

```go
func TestListEligibleSince(t *testing.T) {
	r := newTestRepo(t) // existing sqlite in-mem helper in this file
	ctx := context.Background()
	old := &domain.Fanfic{UserID: "u1", AnimeTitle: "A", Status: domain.StatusComplete, Content: "x", CreatedAt: time.Now().Add(-48 * time.Hour)}
	recent := &domain.Fanfic{UserID: "u1", AnimeTitle: "B", Status: domain.StatusComplete, Content: "y"}
	failed := &domain.Fanfic{UserID: "u1", AnimeTitle: "C", Status: domain.StatusFailed}
	for _, f := range []*domain.Fanfic{old, recent, failed} {
		if err := r.Create(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	// force old CreatedAt (Create stamps now)
	r.db.Model(&domain.Fanfic{}).Where("id = ?", old.ID).Update("created_at", time.Now().Add(-48*time.Hour))
	got, err := r.ListEligibleSince(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].AnimeTitle != "B" {
		t.Fatalf("want only recent complete B, got %+v", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `cd services/fanfic && go test ./internal/repo/ -run TestListEligibleSince -v` → FAIL (method undefined).

- [ ] **Step 3: Add the fields** to `domain/fanfic.go` (after `Language`, keep tags):

```go
	AuthorUsername  string `gorm:"size:64" json:"author_username,omitempty"`
	SpotlightCredit bool   `gorm:"default:false" json:"spotlight_credit"`
	AIGenerated     bool   `gorm:"default:false;index" json:"ai_generated"`
```

- [ ] **Step 4: Add repo methods** to `repo/fanfic.go`:

```go
// ListEligibleSince returns completed fanfics (any user) created after `since`,
// oldest-first for a stable daily pick. Not user-scoped — this feeds the public
// «Фанфик дня» pick.
func (r *Repository) ListEligibleSince(ctx context.Context, since time.Time) ([]domain.Fanfic, error) {
	var out []domain.Fanfic
	err := r.db.WithContext(ctx).
		Where("status = ? AND created_at > ?", domain.StatusComplete, since).
		Order("created_at ASC, id ASC").
		Find(&out).Error
	return out, err
}

// DailyBotExists reports whether an AI-generated fanfic already exists since `since`
// (idempotency guard for ensure-daily).
func (r *Repository) DailyBotExists(ctx context.Context, since time.Time) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&domain.Fanfic{}).
		Where("ai_generated = ? AND status = ? AND created_at > ?", true, domain.StatusComplete, since).
		Count(&n).Error
	return n > 0, err
}
```

- [ ] **Step 5: Run to verify it passes** — `cd services/fanfic && go test ./internal/repo/ -run TestListEligibleSince -v` → PASS.

- [ ] **Step 6: Commit** — from worktree: `printf 'feat(fanfic): eligible-fanfic query + spotlight columns' | bash bin/ae-land.sh services/fanfic/internal/domain/fanfic.go services/fanfic/internal/repo/fanfic.go services/fanfic/internal/repo/fanfic_test.go`

---

### Task 2: Typed Groq status error (401 detection)

**Files:**
- Modify: `services/fanfic/internal/groq/client.go`
- Test: `services/fanfic/internal/groq/client_test.go` (create)

**Interfaces:**
- Produces: `groq.StatusError{Code int, Body string}` implementing `error`; `Client.Stream` returns `*StatusError` for any non-200 (message unchanged: `"groq status %d: %s"`).

- [ ] **Step 1: Write the failing test** (`client_test.go`):

```go
package groq

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStream_401_ReturnsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_api_key"}}`))
	}))
	defer srv.Close()
	c := New("bad", srv.URL, "m", 5*time.Second)
	_, _, err := c.Stream(context.Background(), "s", "u", 10, 0.9, func(string) {})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusUnauthorized {
		t.Fatalf("want *StatusError{401}, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `cd services/fanfic && go test ./internal/groq/ -run TestStream_401 -v` → FAIL (`StatusError` undefined).

- [ ] **Step 3: Add the type + return it.** Add to `client.go`:

```go
// StatusError is a non-200 response from Groq. It preserves the HTTP status so
// callers (ensure-daily) can distinguish auth failures (401/403) from transient
// errors without string-matching.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("groq status %d: %s", e.Code, e.Body)
}
```

Replace the existing non-200 return (currently `return "", 0, fmt.Errorf("groq status %d: %s", ...)`):

```go
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", 0, &StatusError{Code: resp.StatusCode, Body: strings.TrimSpace(string(snippet))}
	}
```

- [ ] **Step 4: Run to verify it passes** — `cd services/fanfic && go test ./internal/groq/ -v` → PASS.

- [ ] **Step 5: Commit** — `printf 'feat(fanfic): typed groq StatusError for 401 detection' | bash bin/ae-land.sh services/fanfic/internal/groq/client.go services/fanfic/internal/groq/client_test.go`

---

### Task 3: Daily pick + DTO shaping (pure logic)

**Files:**
- Create: `services/fanfic/internal/service/daily.go`
- Test: `services/fanfic/internal/service/daily_test.go`

**Interfaces:**
- Consumes: `domain.Fanfic`; `Repository.ListEligibleSince`.
- Produces:
  - `func PickDaily(eligible []domain.Fanfic, seed int) *domain.Fanfic` — prefers non-AI (user) fanfics; falls back to AI; deterministic `pool[seed%len(pool)]`; nil when empty.
  - `func DailySeed(t time.Time) int` — day-of-epoch style seed (reuse formula `y*100*32+m*32+d`).
  - `func BuildExcerpt(content string, maxRunes int) string` — strips leading `# H1` / `## ` / `---`, first paragraph, clamped on a word boundary, plain text.
  - `type DailyDTO struct{...}` with json tags (see spec §5) + `func ToDTO(f *domain.Fanfic) DailyDTO` (explicit → empty excerpt, `explicit=true`; author_username only when `SpotlightCredit`).

- [ ] **Step 1: Write the failing tests** (`daily_test.go`):

```go
package service

import (
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

func TestPickDaily_PrefersUserOverBot_Deterministic(t *testing.T) {
	bot := domain.Fanfic{ID: "bot", AIGenerated: true, Status: domain.StatusComplete}
	u1 := domain.Fanfic{ID: "u1", AIGenerated: false, Status: domain.StatusComplete}
	u2 := domain.Fanfic{ID: "u2", AIGenerated: false, Status: domain.StatusComplete}
	got := PickDaily([]domain.Fanfic{bot, u1, u2}, 3) // 3 % 2 == 1 -> u2
	if got == nil || got.ID != "u2" {
		t.Fatalf("want u2, got %v", got)
	}
	// same seed -> same pick
	if PickDaily([]domain.Fanfic{bot, u1, u2}, 3).ID != "u2" {
		t.Fatal("nondeterministic")
	}
}

func TestPickDaily_FallsBackToBot(t *testing.T) {
	bot := domain.Fanfic{ID: "bot", AIGenerated: true, Status: domain.StatusComplete}
	if PickDaily([]domain.Fanfic{bot}, 5).ID != "bot" {
		t.Fatal("want bot fallback")
	}
	if PickDaily(nil, 5) != nil {
		t.Fatal("want nil on empty")
	}
}

func TestToDTO_ExplicitHidesExcerpt(t *testing.T) {
	f := &domain.Fanfic{ID: "x", Title: "T", AnimeTitle: "A", Content: "Long body here.", Rating: "explicit", SpotlightCredit: true, AuthorUsername: "neo"}
	d := ToDTO(f)
	if d.Excerpt != "" || !d.Explicit {
		t.Fatalf("explicit must hide excerpt: %+v", d)
	}
	if d.AuthorUsername != "neo" || !d.Credited {
		t.Fatalf("credited author expected: %+v", d)
	}
}

func TestToDTO_AnonWhenNotCredited(t *testing.T) {
	f := &domain.Fanfic{ID: "x", Content: "Body.", Rating: "teen", SpotlightCredit: false, AuthorUsername: "neo"}
	d := ToDTO(f)
	if d.AuthorUsername != "" || d.Credited {
		t.Fatalf("must anonymize: %+v", d)
	}
	if d.Excerpt == "" {
		t.Fatal("teen must have excerpt")
	}
}

func TestBuildExcerpt_StripsHeadingAndClamps(t *testing.T) {
	in := "# Title\n\n## Часть 1\n\nОна открыла дверь и замерла на пороге, не в силах произнести ни слова."
	got := BuildExcerpt(in, 30)
	if got == "" || len([]rune(got)) > 31 || got[0] == '#' {
		t.Fatalf("bad excerpt: %q", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `cd services/fanfic && go test ./internal/service/ -run 'TestPickDaily|TestToDTO|TestBuildExcerpt' -v` → FAIL.

- [ ] **Step 3: Implement `daily.go`:**

```go
package service

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

// DailySeed is a UTC day-stable integer (same formula as catalog spotlight's
// DateSeedUTC) so the pick rolls over at UTC midnight.
func DailySeed(t time.Time) int {
	u := t.UTC()
	return u.Year()*100*32 + int(u.Month())*32 + u.Day()
}

// PickDaily deterministically selects one fanfic for the day. User-authored
// fanfics are preferred; AI-generated ones are the fallback pool. Returns nil
// when nothing is eligible. `eligible` is assumed pre-sorted (created_at,id).
func PickDaily(eligible []domain.Fanfic, seed int) *domain.Fanfic {
	var users, bots []domain.Fanfic
	for _, f := range eligible {
		if f.AIGenerated {
			bots = append(bots, f)
		} else {
			users = append(users, f)
		}
	}
	pool := users
	if len(pool) == 0 {
		pool = bots
	}
	if len(pool) == 0 {
		return nil
	}
	idx := seed % len(pool)
	if idx < 0 {
		idx += len(pool)
	}
	return &pool[idx]
}

// DailyDTO is the wire shape shared by GET /internal/fanfic/daily (spotlight,
// excerpt only) and the metadata half of GET /api/fanfic/daily (public reader).
type DailyDTO struct {
	ID             string    `json:"id"`
	FanficTitle    string    `json:"fanfic_title"`
	AnimeTitle     string    `json:"anime_title"`
	AnimeJapanese  string    `json:"anime_japanese"`
	AnimePoster    string    `json:"anime_poster"`
	Excerpt        string    `json:"excerpt"`
	Rating         string    `json:"rating"`
	Language       string    `json:"language"`
	Explicit       bool      `json:"explicit"`
	AuthorUsername string    `json:"author_username"`
	Credited       bool      `json:"credited"`
	AIGenerated    bool      `json:"ai_generated"`
	PartCount      int       `json:"part_count"`
	CreatedAt      time.Time `json:"created_at"`
}

const excerptRunes = 240

// ToDTO shapes a fanfic into the compact DTO. Explicit content carries NO
// excerpt (so nothing explicit enters the globally-cached spotlight payload);
// the author name is included only when the author opted in (SpotlightCredit).
func ToDTO(f *domain.Fanfic) DailyDTO {
	explicit := f.Rating == "explicit"
	d := DailyDTO{
		ID:            f.ID,
		FanficTitle:   f.Title,
		AnimeTitle:    f.AnimeTitle,
		AnimeJapanese: f.AnimeJapanese,
		AnimePoster:   f.AnimePoster,
		Rating:        f.Rating,
		Language:      f.Language,
		Explicit:      explicit,
		AIGenerated:   f.AIGenerated,
		PartCount:     f.PartCount,
		CreatedAt:     f.CreatedAt,
	}
	if !explicit {
		d.Excerpt = BuildExcerpt(f.Content, excerptRunes)
	}
	if f.SpotlightCredit && f.AuthorUsername != "" {
		d.AuthorUsername = f.AuthorUsername
		d.Credited = true
	}
	return d
}

// BuildExcerpt returns a clean plain-text teaser: drops leading markdown
// headings / horizontal rules, takes the first non-empty paragraph, and clamps
// to maxRunes on a word boundary.
func BuildExcerpt(content string, maxRunes int) string {
	for _, para := range strings.Split(content, "\n\n") {
		p := strings.TrimSpace(para)
		if p == "" || strings.HasPrefix(p, "#") || p == "---" {
			continue
		}
		p = strings.ReplaceAll(p, "\n", " ")
		if utf8.RuneCountInString(p) <= maxRunes {
			return p
		}
		runes := []rune(p)[:maxRunes]
		cut := string(runes)
		if i := strings.LastIndex(cut, " "); i > 0 {
			cut = cut[:i]
		}
		return strings.TrimSpace(cut) + "…"
	}
	return ""
}
```

- [ ] **Step 4: Run to verify it passes** — `cd services/fanfic && go test ./internal/service/ -run 'TestPickDaily|TestToDTO|TestBuildExcerpt' -v` → PASS.

- [ ] **Step 5: Commit** — `printf 'feat(fanfic): daily pick + DTO shaping (explicit-safe excerpt)' | bash bin/ae-land.sh services/fanfic/internal/service/daily.go services/fanfic/internal/service/daily_test.go`

---

### Task 4: Telegram alerter (401 → maintenance chat)

**Files:**
- Create: `services/fanfic/internal/alert/telegram.go`
- Test: `services/fanfic/internal/alert/telegram_test.go`

**Interfaces:**
- Produces: `type Alerter interface { Send(ctx context.Context, text string) error }`; `type Telegram struct{...}`; `func NewTelegram(botToken, chatID, baseURL string, hc *http.Client) *Telegram` (empty `baseURL`→`https://api.telegram.org`); `func NewNoop() Alerter` (used when token/chat unset — fail-open). `Send` POSTs `{baseURL}/bot{token}/sendMessage` form `chat_id`+`text`; non-2xx → error; NEVER logs the token.

- [ ] **Step 1: Write the failing test** (`telegram_test.go`):

```go
package alert

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegram_Send_PostsSendMessage(t *testing.T) {
	var gotPath, gotChat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotChat = r.Form.Get("chat_id")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	tg := NewTelegram("SECRET", "-100123", srv.URL, srv.Client())
	if err := tg.Send(context.Background(), "boom"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/botSECRET/sendMessage") || gotChat != "-100123" {
		t.Fatalf("path=%s chat=%s", gotPath, gotChat)
	}
}

func TestNoop_Send_NoError(t *testing.T) {
	if err := NewNoop().Send(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `cd services/fanfic && go test ./internal/alert/ -v` → FAIL (package/type undefined).

- [ ] **Step 3: Implement `telegram.go`:**

```go
// Package alert sends operational alerts to the maintenance Telegram chat via
// the ALERTS bot. The auth bot token is NOT usable here (not a chat member).
package alert

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Alerter interface {
	Send(ctx context.Context, text string) error
}

type noop struct{}

func NewNoop() Alerter                              { return noop{} }
func (noop) Send(context.Context, string) error     { return nil }

type Telegram struct {
	token   string
	chatID  string
	baseURL string
	http    *http.Client
}

func NewTelegram(botToken, chatID, baseURL string, hc *http.Client) *Telegram {
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Telegram{token: botToken, chatID: chatID, baseURL: strings.TrimRight(baseURL, "/"), http: hc}
}

func (t *Telegram) Send(ctx context.Context, text string) error {
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", t.baseURL, t.token)
	form := url.Values{"chat_id": {t.chatID}, "text": {text}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram send: %w", err) // never wrap the token
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram send: status %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 4: Run to verify it passes** — `cd services/fanfic && go test ./internal/alert/ -v` → PASS.

- [ ] **Step 5: Commit** — `printf 'feat(fanfic): telegram alerter (ALERTS bot, noop fallback)' | bash bin/ae-land.sh services/fanfic/internal/alert/telegram.go services/fanfic/internal/alert/telegram_test.go`

---

### Task 5: Extend fanfic catalog client to return poster + japanese

**Files:**
- Modify: `services/fanfic/internal/catalog/client.go`
- Test: `services/fanfic/internal/catalog/client_test.go` (extend)

**Interfaces:**
- Produces: `type AnimeMeta struct{ Title, Japanese, Poster, Synopsis string }`; `Client.FetchMeta(ctx, animeID, shikimoriID string) (AnimeMeta, error)`. Keep existing `FetchSynopsis` (used by the generator). `FetchMeta` decodes `data.{name,name_ru,japanese,poster_url|poster,description}` from the same anime envelope.

- [ ] **Step 1: Write the failing test** — extend `client_test.go` with a stub server returning `{"data":{"name":"Naruto","japanese":"ナルト","poster_url":"http://p/x.jpg","description":"d"}}` and assert `FetchMeta` maps `Poster` + `Japanese`.

```go
func TestFetchMeta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"name":"Naruto","japanese":"ナルト","poster_url":"http://p/x.jpg","description":"d"}}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, time.Second, nil)
	m, err := c.FetchMeta(context.Background(), "abc", "")
	if err != nil || m.Poster != "http://p/x.jpg" || m.Japanese != "ナルト" || m.Title != "Naruto" {
		t.Fatalf("meta=%+v err=%v", m, err)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `cd services/fanfic && go test ./internal/catalog/ -run TestFetchMeta -v` → FAIL.

- [ ] **Step 3: Implement** — inspect the existing `animeEnvelope` struct in `client.go`; extend it with `Japanese string json:"japanese"` and `PosterURL string json:"poster_url"` (add `Poster string json:"poster"` fallback if the API uses that — verify by reading the struct). Add:

```go
type AnimeMeta struct {
	Title, Japanese, Poster, Synopsis string
}

// FetchMeta returns title/japanese/poster/synopsis for bot fanfic generation.
// Fail-soft: transport/decode errors return a zero AnimeMeta + error.
func (c *Client) FetchMeta(ctx context.Context, animeID, shikimoriID string) (AnimeMeta, error) {
	// reuse the same request path as FetchSynopsis (animeID first, else shikimori)
	env, err := c.fetchEnvelope(ctx, animeID, shikimoriID) // extract the shared fetch+decode into fetchEnvelope
	if err != nil {
		return AnimeMeta{}, err
	}
	title := env.Data.Name
	if title == "" {
		title = env.Data.NameRU
	}
	return AnimeMeta{Title: title, Japanese: env.Data.Japanese, Poster: env.Data.PosterURL, Synopsis: env.Data.Description}, nil
}
```

Refactor the existing `FetchSynopsis` body into a shared `fetchEnvelope(ctx, animeID, shikimoriID) (animeEnvelope, error)` and have both call it (DRY).

- [ ] **Step 4: Run to verify it passes** — `cd services/fanfic && go test ./internal/catalog/ -v` → PASS (both old + new tests).

- [ ] **Step 5: Commit** — `printf 'feat(fanfic): catalog client FetchMeta (poster+japanese) for bot gen' | bash bin/ae-land.sh services/fanfic/internal/catalog/client.go services/fanfic/internal/catalog/client_test.go`

---

### Task 6: EnsureDaily service (generate + probe + alert + idempotency)

**Files:**
- Create: `services/fanfic/internal/service/ensure_daily.go`
- Test: `services/fanfic/internal/service/ensure_daily_test.go`

**Interfaces:**
- Consumes: `streamer` (groq), `Repository` (`ListEligibleSince`, `DailyBotExists`, `Create`), catalog `FetchMeta`, `alert.Alerter`, `domain.CuratedTags`, `BuildMessages`, `MaxTokensFor`, `SplitTitle`, `groq.StatusError`.
- Produces:
  - `const FanficBotUserID = "00000000-0000-0000-0000-0000000000b0"` and `const FanficBotUsername = "AnimeEnigma"`.
  - `type animeMetaFetcher interface { FetchMeta(ctx, animeID, shikimoriID string) (catalog.AnimeMeta, error) }`
  - `type DailyService struct{...}` + `NewDailyService(groq streamer, repo dailyRepo, meta animeMetaFetcher, alerter alert.Alerter, model string, animePool []string, lang string, now func() time.Time, log *logger.Logger) *DailyService`
  - `func (s *DailyService) EnsureDaily(ctx context.Context) (EnsureResult, error)` — `EnsureResult{Generated bool, Reason string, FanficID string}`.
  - `func (s *DailyService) DailyPick(ctx context.Context) (*domain.Fanfic, error)` (used by the daily handlers).
  - `dailyRepo` interface = `{ ListEligibleSince; DailyBotExists; Create }`.

- [ ] **Step 1: Write the failing tests** (`ensure_daily_test.go`) — fakes only (no testify):

```go
package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/catalog"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/groq"
)

type fakeRepo struct {
	eligible []domain.Fanfic
	botDaily bool
	created  *domain.Fanfic
}

func (f *fakeRepo) ListEligibleSince(context.Context, time.Time) ([]domain.Fanfic, error) { return f.eligible, nil }
func (f *fakeRepo) DailyBotExists(context.Context, time.Time) (bool, error)               { return f.botDaily, nil }
func (f *fakeRepo) Create(_ context.Context, ff *domain.Fanfic) error                     { ff.ID = "new"; f.created = ff; return nil }

type fakeMeta struct{}

func (fakeMeta) FetchMeta(context.Context, string, string) (catalog.AnimeMeta, error) {
	return catalog.AnimeMeta{Title: "Naruto", Poster: "http://p/x.jpg"}, nil
}

type fakeStream struct{ err error; text string }

func (f fakeStream) Stream(_ context.Context, _, _ string, _ int, _ float64, on func(string)) (string, int, error) {
	if f.err != nil {
		return "", 0, f.err
	}
	return f.text, 42, nil
}

type fakeAlerter struct{ sent []string }

func (a *fakeAlerter) Send(_ context.Context, s string) error { a.sent = append(a.sent, s); return nil }

func newDaily(repo dailyRepo, stream streamer, al *fakeAlerter) *DailyService {
	return NewDailyService(stream, repo, fakeMeta{}, al, "m", []string{"20"}, "ru", func() time.Time { return time.Unix(1700000000, 0) }, nil)
}

func TestEnsureDaily_UserFanficExists_NoOp(t *testing.T) {
	repo := &fakeRepo{eligible: []domain.Fanfic{{ID: "u", AIGenerated: false, Status: domain.StatusComplete}}}
	al := &fakeAlerter{}
	res, err := newDaily(repo, fakeStream{text: "# T\n\nBody"}, al).EnsureDaily(context.Background())
	if err != nil || res.Generated || res.Reason != "user_exists" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if repo.created != nil {
		t.Fatal("must not generate when a user fanfic exists")
	}
}

func TestEnsureDaily_GeneratesBotFanfic(t *testing.T) {
	repo := &fakeRepo{}
	al := &fakeAlerter{}
	res, err := newDaily(repo, fakeStream{text: "# Тайна\n\nОна вошла."}, al).EnsureDaily(context.Background())
	if err != nil || !res.Generated {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if repo.created == nil || !repo.created.AIGenerated || repo.created.Rating != "teen" ||
		repo.created.AuthorUsername != FanficBotUsername || !repo.created.SpotlightCredit ||
		repo.created.Status != domain.StatusComplete || repo.created.Title != "Тайна" {
		t.Fatalf("bad bot row: %+v", repo.created)
	}
}

func TestEnsureDaily_401_Alerts(t *testing.T) {
	repo := &fakeRepo{}
	al := &fakeAlerter{}
	stream := fakeStream{err: &groq.StatusError{Code: http.StatusUnauthorized, Body: "invalid_api_key"}}
	res, err := newDaily(repo, stream, al).EnsureDaily(context.Background())
	if err == nil || res.Generated {
		t.Fatalf("want error, res=%+v", res)
	}
	if len(al.sent) != 1 {
		t.Fatalf("want 1 alert, got %d", len(al.sent))
	}
}

func TestEnsureDaily_TransientError_NoAlert(t *testing.T) {
	repo := &fakeRepo{}
	al := &fakeAlerter{}
	_, err := newDaily(repo, fakeStream{err: errors.New("timeout")}, al).EnsureDaily(context.Background())
	if err == nil || len(al.sent) != 0 {
		t.Fatalf("transient must not alert; err=%v sent=%d", err, len(al.sent))
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `cd services/fanfic && go test ./internal/service/ -run TestEnsureDaily -v` → FAIL.

- [ ] **Step 3: Implement `ensure_daily.go`** — random params from `DailySeed` (deterministic per day, avoids `math/rand` global), forced teen/first-or-third/oneshot, RU:

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/alert"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/catalog"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/groq"
)

const (
	FanficBotUserID   = "00000000-0000-0000-0000-0000000000b0"
	FanficBotUsername = "AnimeEnigma"
	eligibleWindow    = 24 * time.Hour
)

type dailyRepo interface {
	ListEligibleSince(ctx context.Context, since time.Time) ([]domain.Fanfic, error)
	DailyBotExists(ctx context.Context, since time.Time) (bool, error)
	Create(ctx context.Context, f *domain.Fanfic) error
}

type animeMetaFetcher interface {
	FetchMeta(ctx context.Context, animeID, shikimoriID string) (catalog.AnimeMeta, error)
}

type EnsureResult struct {
	Generated bool
	Reason    string
	FanficID  string
}

type DailyService struct {
	groq      streamer
	repo      dailyRepo
	meta      animeMetaFetcher
	alerter   alert.Alerter
	model     string
	animePool []string
	lang      string
	now       func() time.Time
	log       *logger.Logger
}

func NewDailyService(g streamer, repo dailyRepo, meta animeMetaFetcher, alerter alert.Alerter, model string, animePool []string, lang string, now func() time.Time, log *logger.Logger) *DailyService {
	if now == nil {
		now = time.Now
	}
	if lang == "" {
		lang = "ru"
	}
	return &DailyService{groq: g, repo: repo, meta: meta, alerter: alerter, model: model, animePool: animePool, lang: lang, now: now, log: log}
}

// DailyPick returns the day's fanfic (or nil). Shared by both daily handlers.
func (s *DailyService) DailyPick(ctx context.Context) (*domain.Fanfic, error) {
	eligible, err := s.repo.ListEligibleSince(ctx, s.now().Add(-eligibleWindow))
	if err != nil {
		return nil, err
	}
	return PickDaily(eligible, DailySeed(s.now())), nil
}

// EnsureDaily generates a bot fanfic when no eligible user fanfic exists in the
// window. The Groq call doubles as the daily key-health probe: a 401/403 fires a
// Telegram alert. Idempotent — skips if a bot fanfic already exists today.
func (s *DailyService) EnsureDaily(ctx context.Context) (EnsureResult, error) {
	since := s.now().Add(-eligibleWindow)
	eligible, err := s.repo.ListEligibleSince(ctx, since)
	if err != nil {
		return EnsureResult{}, err
	}
	for _, f := range eligible {
		if !f.AIGenerated {
			return EnsureResult{Generated: false, Reason: "user_exists"}, nil
		}
	}
	if exists, err := s.repo.DailyBotExists(ctx, since); err == nil && exists {
		return EnsureResult{Generated: false, Reason: "bot_exists"}, nil
	}

	req := s.randomRequest(ctx)
	system, user := BuildMessages(req, "")
	text, usage, err := s.groq.Stream(ctx, system, user, MaxTokensFor(req.Length), 0.9, func(string) {})
	if err != nil {
		var se *groq.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
			msg := fmt.Sprintf("🚨 Fanfic daily generation FAILED: Groq rejected the API key (status %d). Model=%s. Fix FANFIC_GROQ_API_KEY.", se.Code, s.model)
			_ = s.alerter.Send(ctx, msg)
			if s.log != nil {
				s.log.Errorw("fanfic.daily.groq_auth_failed", "status", se.Code)
			}
		} else if s.log != nil {
			s.log.Warnw("fanfic.daily.groq_failed", "error", err)
		}
		return EnsureResult{}, fmt.Errorf("ensure-daily: groq: %w", err)
	}

	title, _ := SplitTitle(text)
	f := &domain.Fanfic{
		UserID:          FanficBotUserID,
		AuthorUsername:  FanficBotUsername,
		SpotlightCredit: true,
		AIGenerated:     true,
		AnimeID:         req.Anime.ID,
		AnimeShikimoriID: req.Anime.ShikimoriID,
		AnimeTitle:      req.Anime.Title,
		AnimeJapanese:   req.Anime.Japanese,
		AnimePoster:     req.Anime.Poster,
		Length:          req.Length,
		POV:             req.POV,
		Rating:          req.Rating,
		Language:        req.Language,
		PartCount:       1,
		Title:           title,
		Content:         text,
		Model:           s.model,
		TokenUsage:      usage,
		Status:          domain.StatusComplete,
	}
	if err := s.repo.Create(ctx, f); err != nil {
		return EnsureResult{}, fmt.Errorf("ensure-daily: persist: %w", err)
	}
	return EnsureResult{Generated: true, Reason: "generated", FanficID: f.ID}, nil
}

// randomRequest builds deterministic-per-day random params (teen, RU) and fetches
// anime metadata (fail-soft — generation proceeds with whatever title is available).
func (s *DailyService) randomRequest(ctx context.Context) domain.GenerateRequest {
	seed := DailySeed(s.now())
	shiki := ""
	if len(s.animePool) > 0 {
		shiki = s.animePool[seed%len(s.animePool)]
	}
	anime := domain.AnimeRef{ShikimoriID: shiki, Title: "аниме"}
	if m, err := s.meta.FetchMeta(ctx, "", shiki); err == nil && m.Title != "" {
		anime.Title, anime.Japanese, anime.Poster = m.Title, m.Japanese, m.Poster
	}
	tags := domain.CuratedTags
	t1 := tags[seed%len(tags)].Slug
	t2 := tags[(seed/7)%len(tags)].Slug
	povs := []string{"first", "third"}
	return domain.GenerateRequest{
		Anime:    anime,
		Tags:     []string{t1, t2},
		Length:   "oneshot",
		POV:      povs[seed%2],
		Rating:   "teen",
		Language: s.lang,
	}
}
```

- [ ] **Step 4: Run to verify it passes** — `cd services/fanfic && go test ./internal/service/ -run TestEnsureDaily -v` → PASS.

- [ ] **Step 5: Commit** — `printf 'feat(fanfic): EnsureDaily bot generation + groq key probe + alert' | bash bin/ae-land.sh services/fanfic/internal/service/ensure_daily.go services/fanfic/internal/service/ensure_daily_test.go`

---

### Task 7: Daily + ensure-daily handlers + routes

**Files:**
- Create: `services/fanfic/internal/handler/daily.go`
- Modify: `services/fanfic/internal/transport/router.go` (add public `/api/fanfic/daily` OUTSIDE the JWT group; add internal `/internal/*` routes)
- Test: `services/fanfic/internal/handler/daily_test.go`

**Interfaces:**
- Consumes: `DailyService.DailyPick`, `DailyService.EnsureDaily`, `service.ToDTO`.
- Produces: `DailyHandler{ daily dailyProvider }` where `dailyProvider interface { DailyPick(ctx)(*domain.Fanfic,error); EnsureDaily(ctx)(service.EnsureResult,error) }`; handlers:
  - `Internal(w,r)` → `GET /internal/fanfic/daily` → `service.ToDTO(pick)` (compact) or 404.
  - `Public(w,r)` → `GET /api/fanfic/daily` → full: DTO fields + `content` (empty+`gated` when explicit; `gate_reason="login"` if no JWT else `"adult_setting"`) or 404.
  - `Ensure(w,r)` → `POST /internal/fanfic/ensure-daily` → JSON `EnsureResult`; auth-failure returns 200 with `{generated:false,error:"groq_auth"}` (the alert already fired) so the scheduler records success (alert is the signal); other errors → 500.

- [ ] **Step 1: Write failing handler tests** — httptest against the three handlers with a fake `dailyProvider`. Assert: internal returns compact DTO (200) / 404 when nil; explicit pick → `content==""` + `gated==true`; ensure returns `{generated:true}`.

- [ ] **Step 2: Run to verify it fails** — `cd services/fanfic && go test ./internal/handler/ -run TestDaily -v` → FAIL.

- [ ] **Step 3: Implement `daily.go`** (public struct embeds `service.DailyDTO` + `Content string json:"content"` + `Gated bool json:"gated"` + `GateReason string json:"gate_reason,omitempty"`). For `Public`, read auth via `authz.UserIDFromContext(r.Context())` (empty = anon → `gate_reason="login"`). Use `httputil` JSON writers consistent with existing handlers.

- [ ] **Step 4: Wire routes in `router.go`** — add BEFORE the `r.Route("/api/fanfic", …)` JWT group:

```go
	// Public daily reader (optional-JWT — no auth middleware; handler reads
	// claims if present). Must be registered so it is NOT under the JWT group.
	r.Get("/api/fanfic/daily", dh.Public)
	// Internal (docker-network only; gateway does not proxy /internal/*).
	r.Get("/internal/fanfic/daily", dh.Internal)
	r.Post("/internal/fanfic/ensure-daily", dh.Ensure)
```

Extend `NewRouter` signature to accept `dh *handler.DailyHandler`. NOTE: the public `/api/fanfic/daily` needs claims-on-context when a JWT IS present — add a lightweight optional-auth: parse the bearer if present via `authz.NewJWTManager(jwtConfig).ValidateAccessToken` and attach claims, else continue. Implement as a tiny inline middleware wrapping only that one route (do NOT reuse the mandatory `AuthMiddleware`, which 401s anon).

- [ ] **Step 5: Run to verify it passes** — `cd services/fanfic && go test ./internal/handler/ ./internal/transport/ -v` → PASS.

- [ ] **Step 6: Commit** — `printf 'feat(fanfic): daily + ensure-daily HTTP handlers and routes' | bash bin/ae-land.sh services/fanfic/internal/handler/daily.go services/fanfic/internal/handler/daily_test.go services/fanfic/internal/transport/router.go`

---

### Task 8: User generate path — thread AuthorUsername + SpotlightCredit

**Files:**
- Modify: `services/fanfic/internal/domain/request.go` (add `SpotlightCredit`)
- Modify: `services/fanfic/internal/service/generate.go` (set the 3 fields on the created row; add `username` param)
- Modify: `services/fanfic/internal/handler/fanfic.go` (pass `claims.Username`)
- Test: extend `generate_test.go`, `fanfic_test.go` (handler)

**Interfaces:**
- Produces: `GenerateRequest.SpotlightCredit bool json:"spotlight_credit"`; `Generator.Generate(ctx, userID, username string, req, emit)` — the created `Fanfic` sets `AuthorUsername=username`, `SpotlightCredit=req.SpotlightCredit`, `AIGenerated=false`.
- Consumes (handler): `authz.ClaimsFromContext(r.Context())` → `claims.Username`.

- [ ] **Step 1: Write the failing test** — in `generate_test.go`, assert the created row carries `AuthorUsername` + `SpotlightCredit`; update the `generator` fake in the handler test to the new 5-arg signature and assert the handler forwards the username from claims.

- [ ] **Step 2: Run to verify it fails** — `cd services/fanfic && go test ./internal/service/ ./internal/handler/ -v` → FAIL (signature mismatch).

- [ ] **Step 3: Implement** — add the field; change `Generate` signature + the row-build (find where the `domain.Fanfic` is constructed inside `Generate` and set the three fields); update `handler/fanfic.go` `Generate` to read claims:

```go
	claims, _ := authz.ClaimsFromContext(r.Context())
	username := ""
	if claims != nil {
		username = claims.Username
	}
	// ... existing userID check ...
	err := h.gen.Generate(r.Context(), userID, username, req, emit)
```

Update the handler's `generator` interface to the new signature, and `continue.go` is unaffected (no username needed).

- [ ] **Step 4: Run to verify it passes** — `cd services/fanfic && go test ./... -count=1` → PASS.

- [ ] **Step 5: Commit** — `printf 'feat(fanfic): persist author username + spotlight_credit on generate' | bash bin/ae-land.sh services/fanfic/internal/domain/request.go services/fanfic/internal/service/generate.go services/fanfic/internal/handler/fanfic.go services/fanfic/internal/service/generate_test.go services/fanfic/internal/handler/fanfic_test.go`

---

### Task 9: Config + main.go wiring (fanfic)

**Files:**
- Modify: `services/fanfic/internal/config/config.go`
- Modify: `services/fanfic/cmd/fanfic-api/main.go`

**Interfaces:**
- Produces config fields: `AlertsBotToken`, `AlertsChatID` (from `TELEGRAM_ALERTS_BOT_TOKEN` / `TELEGRAM_ADMIN_CHAT_ID`), `DailyAnimePool []string` (`FANFIC_DAILY_ANIME_POOL` CSV, default a handful of popular shikimori IDs e.g. `"20,21,1735,52991,16498,5114"`), `BotLanguage` (`FANFIC_BOT_LANGUAGE`, default `ru`).

- [ ] **Step 1: Add config fields + Load()** entries (mirror existing `getEnv`/`getEnvInt`; add a `getEnvCSV` helper or split in main). Default pool = a few well-known shikimori IDs.

- [ ] **Step 2: Wire in `main.go`** after the existing generator wiring:

```go
	var alerter alert.Alerter = alert.NewNoop()
	if cfg.AlertsBotToken != "" && cfg.AlertsChatID != "" {
		alerter = alert.NewTelegram(cfg.AlertsBotToken, cfg.AlertsChatID, "", nil)
	}
	dailyService := service.NewDailyService(groqClient, fanficRepo, catalogClient, alerter, cfg.Groq.Model, cfg.DailyAnimePool, cfg.BotLanguage, time.Now, log)
	dailyHandler := handler.NewDailyHandler(dailyService, log)
	// router now takes dailyHandler:
	router := transport.NewRouter(h, dailyHandler, cfg.JWT, log, mc)
```

Ensure `db.AutoMigrate(&domain.Fanfic{})` (already present at main.go:60) adds the 3 new columns on boot.

- [ ] **Step 3: Build** — `cd services/fanfic && go build ./... && go test ./... -count=1` → PASS.

- [ ] **Step 4: Commit** — `printf 'feat(fanfic): wire DailyService, alerter, config env' | bash bin/ae-land.sh services/fanfic/internal/config/config.go services/fanfic/cmd/fanfic-api/main.go`

- [ ] **Step 5: Deploy + runtime-smoke the probe** — copy `.env` into worktree if deploying from it (`project_worktree_deploy_needs_dotenv`); then from the base tree after ae-land: `bash bin/ae-deploy.sh fanfic`. Then: `curl -s -XPOST http://localhost:8097/internal/fanfic/ensure-daily` → expect `{"generated":true,...}`; `curl -s http://localhost:8097/internal/fanfic/daily` → compact DTO. (The real Groq key is live — confirmed working today.)

---

## Phase 2 — Catalog spotlight resolver (independently testable via `go test`)

### Task 10: `DailyFanficData` type + round-trip test

**Files:**
- Modify: `services/catalog/internal/service/spotlight/types.go`
- Test: `services/catalog/internal/service/spotlight/types_test.go`

**Interfaces:**
- Produces: `spotlight.DailyFanficData` — mirrors the fanfic `DailyDTO` fields (snake_case json). It is assigned to `Card.Data` by the resolver.

- [ ] **Step 1: Write the failing round-trip test** — mirror the existing `PersonalPickData` subtest in `types_test.go`: marshal/unmarshal `DailyFanficData{ID:"x",FanficTitle:"T",AnimeTitle:"A",Rating:"teen",Explicit:false,AuthorUsername:"neo",Credited:true}` and assert fields survive.

- [ ] **Step 2: Run to verify it fails** — `cd services/catalog && go test ./internal/service/spotlight/ -run RoundTrip -v` → FAIL.

- [ ] **Step 3: Add the struct** to `types.go` (after the other `*Data` structs):

```go
// DailyFanficData is the «Фанфик дня» card payload (snake_case, no transform).
// Mirrors services/fanfic DailyDTO. Explicit picks carry an empty excerpt.
type DailyFanficData struct {
	ID             string `json:"id"`
	FanficTitle    string `json:"fanfic_title"`
	AnimeTitle     string `json:"anime_title"`
	AnimeJapanese  string `json:"anime_japanese"`
	AnimePoster    string `json:"anime_poster"`
	Excerpt        string `json:"excerpt"`
	Rating         string `json:"rating"`
	Language       string `json:"language"`
	Explicit       bool   `json:"explicit"`
	AuthorUsername string `json:"author_username"`
	Credited       bool   `json:"credited"`
	AIGenerated    bool   `json:"ai_generated"`
	PartCount      int    `json:"part_count"`
	CreatedAt      string `json:"created_at"`
}
```

- [ ] **Step 4: Run to verify it passes** — `cd services/catalog && go test ./internal/service/spotlight/ -run RoundTrip -v` → PASS.

- [ ] **Step 5: Commit** — `printf 'feat(catalog): DailyFanficData spotlight card type' | bash bin/ae-land.sh services/catalog/internal/service/spotlight/types.go services/catalog/internal/service/spotlight/types_test.go`

---

### Task 11: Fanfic client + DailyFanficResolver

**Files:**
- Create: `services/catalog/internal/service/spotlight/client/fanfic_client.go`
- Create: `services/catalog/internal/service/spotlight/cards/daily_fanfic.go`
- Test: `services/catalog/internal/service/spotlight/cards/daily_fanfic_test.go`

**Interfaces:**
- Produces:
  - `client.FanficClient` + `client.NewFanficClient(baseURL string, hc *http.Client, log *logger.Logger) *FanficClient` (empty→`http://fanfic:8097`, nil→700ms client). `FetchDaily(ctx) (*spotlight.DailyFanficData, error)` → `GET {base}/internal/fanfic/daily`; **404 → `(nil, nil)`**; other non-200/transport → error.
  - `cards.DailyFanficResolver` + `cards.NewDailyFanficResolver(fc dailyFanficSource, c cache.Cache, log *logger.Logger)`; `Type()=="daily_fanfic"`; `Resolve(ctx, _ *string)` caches `spotlight:daily_fanfic:<DateKeyUTC>` (24h `cardTTL`), no-cache-on-empty, `(nil,nil)` when the client returns nil.
  - `dailyFanficSource interface { FetchDaily(ctx context.Context) (*spotlight.DailyFanficData, error) }` (local to cards, for a handwritten fake).

- [ ] **Step 1: Write the failing resolver test** (`daily_fanfic_test.go`) — handwritten fake source (returns a `*DailyFanficData` or nil) + a fake `cache.Cache` (or the repo's existing in-mem cache fake). Assert: data → `Card{Type:"daily_fanfic"}` cached; nil → `(nil,nil)` and NOT cached; cache hit path returns without calling the source. Mirror `featured_test.go` structure.

- [ ] **Step 2: Run to verify it fails** — `cd services/catalog && go test ./internal/service/spotlight/cards/ -run DailyFanfic -v` → FAIL.

- [ ] **Step 3: Implement the client** (mirror `player_client.go` exactly — same logger discipline, 700ms timeout, `LimitReader` on error). `FetchDaily` treats `http.StatusNotFound` as `(nil,nil)`.

- [ ] **Step 4: Implement the resolver** (mirror `featured.go` cache get/set discipline; `cardTTL` is already declared in the `cards` package):

```go
func (r *DailyFanficResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	key := "spotlight:daily_fanfic:" + spotlight.DateKeyUTC(time.Now())
	var cached spotlight.DailyFanficData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}
	data, err := r.src.FetchDaily(ctx)
	if err != nil {
		return nil, fmt.Errorf("daily_fanfic: fetch: %w", err)
	}
	if data == nil {
		return nil, nil // ineligible — do NOT cache (Pitfall 5)
	}
	if err := r.cache.Set(ctx, key, *data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: *data}, nil
}
```

- [ ] **Step 5: Run to verify it passes** — `cd services/catalog && go test ./internal/service/spotlight/... -count=1 -race` → PASS.

- [ ] **Step 6: Commit** — `printf 'feat(catalog): daily-fanfic spotlight client + resolver' | bash bin/ae-land.sh services/catalog/internal/service/spotlight/client/fanfic_client.go services/catalog/internal/service/spotlight/cards/daily_fanfic.go services/catalog/internal/service/spotlight/cards/daily_fanfic_test.go`

---

### Task 12: DI + deploy catalog

**Files:**
- Modify: `services/catalog/cmd/catalog-api/main.go`

- [ ] **Step 1: Construct + append** — at the spotlight wiring block (`main.go:639-663`): add `spotlightFanficClient := client.NewFanficClient("", nil, log)` next to the other client constructors, and append `cards.NewDailyFanficResolver(spotlightFanficClient, redisCache, log),` to the `spotlightResolvers` slice.

- [ ] **Step 2: Build** — `cd services/catalog && go build ./... && go test ./internal/service/spotlight/... -count=1 -race` → PASS.

- [ ] **Step 3: Commit** — `printf 'feat(catalog): register daily-fanfic resolver in spotlight DI' | bash bin/ae-land.sh services/catalog/cmd/catalog-api/main.go`

- [ ] **Step 4: Deploy + flush snapshot** — after ae-land, from base tree: `bash bin/ae-deploy.sh catalog`; then flush stale per-user snapshots: `docker exec animeenigma-redis redis-cli --scan --pattern 'spotlight:snapshot:*' | xargs -r docker exec -i animeenigma-redis redis-cli DEL`. Smoke: hit the live spotlight endpoint and confirm a `daily_fanfic` card appears (a bot fanfic was seeded in Task 9).

---

## Phase 3 — Scheduler cron (independently testable via `go test` + build)

### Task 13: fanfic_daily job + config + wiring

**Files:**
- Create: `services/scheduler/internal/jobs/fanfic_daily.go`
- Modify: `services/scheduler/internal/config/config.go`
- Modify: `services/scheduler/internal/service/job.go`
- Modify: `services/scheduler/cmd/scheduler-api/main.go`
- Test: `services/scheduler/internal/jobs/fanfic_daily_test.go`

**Interfaces:**
- Produces: `jobs.FanficDailyJob` + `jobs.NewFanficDailyJob(cfg *config.JobsConfig, log *logger.Logger)`; `Run(ctx)` POSTs `cfg.FanficServiceURL + "/internal/fanfic/ensure-daily"`, non-2xx → error. Config: `FanficDailyCron` (`FANFIC_DAILY_CRON`, default `"30 4 * * *"`), `FanficServiceURL` (`FANFIC_SERVICE_URL`, default `"http://fanfic:8097"`). Client timeout `fanficDailyReqTimeout = 3 * time.Minute` (> fanfic's 120s Groq timeout).

- [ ] **Step 1: Write the failing test** — httptest server asserting the job POSTs `/internal/fanfic/ensure-daily` and treats non-2xx as error (mirror any existing job test; if none, a minimal one).

- [ ] **Step 2: Run to verify it fails** — `cd services/scheduler && go test ./internal/jobs/ -run FanficDaily -v` → FAIL.

- [ ] **Step 3: Implement `fanfic_daily.go`** — copy `subtitle_probe_trigger.go` verbatim; rename to `FanficDailyJob`/`NewFanficDailyJob`; `fanficDailyReqTimeout = 3 * time.Minute`; `url := j.config.FanficServiceURL + "/internal/fanfic/ensure-daily"`; log strings "fanfic daily".

- [ ] **Step 4: Config** — add `FanficDailyCron`/`FanficServiceURL` fields to `JobsConfig` + `Load()`: `getEnv("FANFIC_DAILY_CRON", "30 4 * * *")`, `getEnv("FANFIC_SERVICE_URL", "http://fanfic:8097")`.

- [ ] **Step 5: Wire** — in `main.go`: `fanficDailyJob := jobs.NewFanficDailyJob(&cfg.Jobs, log)`, add to `NewJobService(...)` args and `jobService.Start(..., cfg.Jobs.FanficDailyCron)`. In `job.go`: add `fanficDailyJob *jobs.FanficDailyJob` field + `NewJobService`/`Start` params + a nil-guarded `s.cron.AddFunc(fanficDailyCron, func(){...})` block modeled on the provider_ranking block (metric label `"fanfic_daily"`).

- [ ] **Step 6: Build + test** — `cd services/scheduler && go build ./... && go test ./... -count=1` → PASS.

- [ ] **Step 7: Commit** — `printf 'feat(scheduler): daily fanfic ensure-daily cron' | bash bin/ae-land.sh services/scheduler/internal/jobs/fanfic_daily.go services/scheduler/internal/jobs/fanfic_daily_test.go services/scheduler/internal/config/config.go services/scheduler/internal/service/job.go services/scheduler/cmd/scheduler-api/main.go`

---

## Phase 4 — Frontend card (run `bin/ae-fe-verify.sh` before landing)

> All FE files are under `frontend/web/`. Work in the worktree. The DS-lint PostToolUse hook runs on each `.vue`/`.ts` save.

### Task 14: Types + tokens + dispatch plumbing (makes vue-tsc pass)

**Files:**
- Modify: `frontend/web/src/types/spotlight.ts`
- Modify: `frontend/web/src/components/home/spotlight/tokens.ts`
- Modify: `frontend/web/src/components/home/spotlight/tokens.spec.ts`
- Modify: `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue`

**Interfaces:**
- Produces: TS `DailyFanficData` interface + union member `{ type:'daily_fanfic'; data: DailyFanficData }`; `cardTokens.daily_fanfic`.

- [ ] **Step 1: Add the interface + union member** in `types.ts` — inside the parenthesized union (before the closing `) & { priority?: number }`):

```ts
export interface DailyFanficData {
  id: string
  fanfic_title: string
  anime_title: string
  anime_japanese: string
  anime_poster: string
  excerpt: string
  rating: string
  language: string
  explicit: boolean
  author_username: string
  credited: boolean
  ai_generated: boolean
  part_count: number
  created_at: string
}
```
and `| { type: 'daily_fanfic'; data: DailyFanficData }`.

- [ ] **Step 2: Add the token** in `tokens.ts`: `daily_fanfic: { accent: 'pink', kickerKey: 'spotlight.dailyFanfic.title', icon: 'sparkles' },` (reuse `sparkles` — no new SVG needed). In `tokens.spec.ts`: add `'daily_fanfic'` to `EXPECTED_TYPES` and bump `toHaveLength(10)` → `11`.

- [ ] **Step 3: Wire the dispatch** in `HeroSpotlightBlock.vue`: (a) import `DailyFanficCard from './cards/DailyFanficCard.vue'` (Task 15 creates it — create a stub now: a single-root `<div/>` so tsc compiles, fleshed out in Task 15); (b) add the `v-else-if` branch before `</transition>`; (c) add a `case 'daily_fanfic': return t('spotlight.dailyFanfic.title')` to `cardTitle()` (REQUIRED — non-exhaustive switch breaks tsc); (d) add `case 'daily_fanfic': return card.data.anime_poster ? [cardPosterUrl(card.data.anime_poster, 640)] : []` to `cardImageUrls()`.

- [ ] **Step 4: Type-check** — `cd frontend/web && bunx vue-tsc --noEmit -p tsconfig.app.json 2>&1 | head` → no new errors. Run `bunx vitest run src/components/home/spotlight/tokens.spec.ts` → PASS.

- [ ] **Step 5: Commit** — `printf 'feat(web): daily-fanfic spotlight types + dispatch plumbing' | bash bin/ae-land.sh frontend/web/src/types/spotlight.ts frontend/web/src/components/home/spotlight/tokens.ts frontend/web/src/components/home/spotlight/tokens.spec.ts frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue frontend/web/src/components/home/spotlight/cards/DailyFanficCard.vue`

---

### Task 15: DailyFanficCard.vue

**Files:**
- Create/replace: `frontend/web/src/components/home/spotlight/cards/DailyFanficCard.vue`
- Test: `frontend/web/src/components/home/spotlight/cards/DailyFanficCard.spec.ts`

**Interfaces:**
- Consumes: `DailyFanficData`, `SpotlightCardShell`, `Badge`, `buttonVariants`, `cardPosterUrl`, `useAuthStore().isAuthenticated`, `useI18n`.

Mirror `CuratedCard.vue` structure (poster-blur `#background`, warm-registry `isImageWarm`/`markImageWarm`, `#cta` router-links with `buttonVariants`). Card spec:
- Accent **pink**, backdrop via `#background` = `cardPosterUrl(data.anime_poster, 640)`.
- Kicker `t('spotlight.dailyFanfic.title')`; `#kicker-lead` a lucide `BookOpen` icon; `#kicker-extra` = `«✨ ИИ»` badge `v-if="data.ai_generated"`.
- Body: `<h3>` fanfic title + anime title subline; author line — `v-if="data.credited"` → `«{{ data.author_username }}»` as a `router-link` to `/u/{{ data.author_username }}` (verify the profile route prefix; else plain text), `v-else` → `t('spotlight.dailyFanfic.anonAuthor')`.
- Excerpt: `v-if="!data.explicit && data.excerpt"` → `<p class="line-clamp-3 [text-wrap:pretty]">{{ data.excerpt }}</p>` (TEXT node, never v-html). `v-else-if="data.explicit"` → gate line: `auth.isAuthenticated ? t('spotlight.dailyFanfic.explicitReader') : t('spotlight.dailyFanfic.explicitLogin')` + an `18+` `Badge variant="destructive"`.
- Rating badge (`data.rating`), part-count badge `v-if="data.part_count > 1"`.
- `#cta`: primary router-link `to="/fanfics?daily=1"` = `t('spotlight.dailyFanfic.read')`; secondary router-link `to="/fanfics"` = `t('spotlight.dailyFanfic.writeOwn')`.
- Styling: `font-medium`/`font-semibold` only, Tailwind-utility + scoped CSS like CuratedCard, single-root shell.

- [ ] **Step 1: Write the failing spec** (`.spec.ts`, ≥5 asserts): (1) credited author renders username; (2) `credited:false` renders anon i18n key; (3) `ai_generated:true` renders AI badge; (4) `explicit:true` + not authed renders the login gate, no excerpt text; (5) `explicit:false` renders the excerpt text; (6) CTA hrefs = `/fanfics?daily=1` and `/fanfics`. Mount with a stubbed i18n + a Pinia auth store (mirror an existing spotlight card spec's harness).

- [ ] **Step 2: Run to verify it fails** — `cd frontend/web && bunx vitest run src/components/home/spotlight/cards/DailyFanficCard.spec.ts` → FAIL.

- [ ] **Step 3: Implement the SFC** (mirror CuratedCard; swap data fields; add the explicit gate + auth store).

- [ ] **Step 4: Run to verify it passes + DS-lint** — `bunx vitest run src/components/home/spotlight/cards/DailyFanficCard.spec.ts` → PASS; the save-hook DS-lint must be clean (pink is an EXEMPT brand hue).

- [ ] **Step 5: Commit** — `printf 'feat(web): DailyFanficCard spotlight SFC' | bash bin/ae-land.sh frontend/web/src/components/home/spotlight/cards/DailyFanficCard.vue frontend/web/src/components/home/spotlight/cards/DailyFanficCard.spec.ts`

---

### Task 16: i18n keys (en/ru/ja) + parity spec

**Files:**
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`
- Modify: `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts`

**Interfaces:**
- Produces `spotlight.dailyFanfic.{title,anonAuthor,read,writeOwn,explicitReader,explicitLogin,aiBadge,ratingTeen?,...}` in all three locales (identical key sets).

- [ ] **Step 1: Add `'dailyFanfic'`** to `expectedSubNamespaces` in `spotlight-keys.spec.ts`.

- [ ] **Step 2: Add the keys** to en/ru/ja under `spotlight.dailyFanfic` (RU examples): `title:"Фанфик дня"`, `anonAuthor:"участник сообщества"`, `read:"Читать"`, `writeOwn:"Написать свой"`, `explicitReader:"Откройте, чтобы прочитать (18+)"`, `explicitLogin:"18+ — войдите, чтобы прочитать"`, `aiBadge:"✨ Сгенерировано ИИ"`. Keep key sets byte-identical across locales.

- [ ] **Step 3: Run parity** — `cd frontend/web && bunx vitest run src/locales/__tests__/spotlight-keys.spec.ts` → PASS.

- [ ] **Step 4: Commit** — `printf 'i18n(web): daily-fanfic spotlight strings en/ru/ja' | bash bin/ae-land.sh frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json frontend/web/src/locales/__tests__/spotlight-keys.spec.ts`

---

### Task 17: Author opt-in checkbox in GenerateForm

**Files:**
- Modify: `frontend/web/src/types/fanfic.ts` (`GenerateInput.spotlight_credit?`)
- Modify: `frontend/web/src/components/fanfic/GenerateForm.vue`
- Modify: `frontend/web/src/components/fanfic/GenerateForm.spec.ts` (if it asserts `defineExpose`)

- [ ] **Step 1: Add the field** to `GenerateInput` (`spotlight_credit?: boolean`).
- [ ] **Step 2: Add `const spotlightCredit = ref(false)`**, a `<Switch v-model="spotlightCredit">` row (mirror the `canon` Switch at `GenerateForm.vue:466-472`) with label `t('fanfic.spotlightCredit.label')`, include it in `buildInput()` (`spotlight_credit: spotlightCredit.value`), and in `defineExpose`.
- [ ] **Step 3: i18n** — `fanfic.spotlightCredit.label` en/ru/ja ("Показывать моё имя в «Фанфик дня»").
- [ ] **Step 4: Verify** — `bunx vitest run src/components/fanfic/GenerateForm.spec.ts` → PASS; `bunx vue-tsc --noEmit` clean.
- [ ] **Step 5: Commit** — `printf 'feat(web): fanfic author opt-in (spotlight_credit) toggle' | bash bin/ae-land.sh frontend/web/src/types/fanfic.ts frontend/web/src/components/fanfic/GenerateForm.vue frontend/web/src/components/fanfic/GenerateForm.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json`

---

### Task 18: «Читать» CTA → daily reader

**Files:**
- Modify: `frontend/web/src/api/fanfic.ts` (`getDaily()`)
- Modify: `frontend/web/src/views/FanficsView.vue` (open reader on `?daily=1`)

**Interfaces:**
- Produces: `fanficApi.getDaily(): Promise<Fanfic & { gated?: boolean; gate_reason?: string }>` → `GET /api/fanfic/daily` (public; forwards bearer if present). `FanficsView` reads `route.query.daily === '1'` on mount → `fanficApi.getDaily()` → open the reader Modal (or, if `gated`, show a gate message).

- [ ] **Step 1: Add `getDaily()`** to the api client (a plain axios GET to `/fanfic/daily` — public; the axios instance forwards the token when present).
- [ ] **Step 2: On mount in `FanficsView.vue`** — if `route.query.daily === '1'`, `readerFanfic.value = await fanficApi.getDaily(); readerOpen.value = true` (reuse the existing `onOpenFanfic` open path; if `gated` is true, instead surface the gate copy). Guard against the reader opening for `gated` explicit content.
- [ ] **Step 3: Verify** — `bunx vue-tsc --noEmit` clean; `bin/ae-fe-verify.sh` on all touched FE files (DS-lint + eslint + `bun run build` + touched specs) → green.
- [ ] **Step 4: Commit** — `printf 'feat(web): daily-fanfic reader via /fanfics?daily=1' | bash bin/ae-land.sh frontend/web/src/api/fanfic.ts frontend/web/src/views/FanficsView.vue`

- [ ] **Step 5: Deploy web** — from base tree after ae-land: `bash bin/ae-deploy.sh web`.

---

## Phase 5 — Go-public flip + rollout

### Task 19: Flip fanfic public + compose env

**Files:**
- Modify: `docker/docker-compose.yml` (fanfic env: `TELEGRAM_ALERTS_BOT_TOKEN`, `TELEGRAM_ADMIN_CHAT_ID`, `FANFIC_DAILY_ANIME_POOL`, `FANFIC_BOT_LANGUAGE`; scheduler env: `FANFIC_SERVICE_URL`, `FANFIC_DAILY_CRON`)
- Modify: `docker/.env` (BASE TREE — sanctioned exception): `FANFIC_ADMIN_ONLY=false`, `VITE_FANFIC_ADMIN_ONLY=false`
- Verify: RBAC/`policy` default does not re-gate `/fanfics` (check `services/policy` feature-flag seed for `fanfic`).

- [ ] **Step 1: Add fanfic env** to the `fanfic:` compose service — `TELEGRAM_ALERTS_BOT_TOKEN: ${TELEGRAM_ALERTS_BOT_TOKEN:-}`, `TELEGRAM_ADMIN_CHAT_ID: ${TELEGRAM_ADMIN_CHAT_ID:-}`, `FANFIC_DAILY_ANIME_POOL: ${FANFIC_DAILY_ANIME_POOL:-20,21,1735,52991,16498,5114}`, `FANFIC_BOT_LANGUAGE: ${FANFIC_BOT_LANGUAGE:-ru}`.
- [ ] **Step 2: Add scheduler env** to the `scheduler:` service — `FANFIC_SERVICE_URL: http://fanfic:8097`, `FANFIC_DAILY_CRON: ${FANFIC_DAILY_CRON:-30 4 * * *}`.
- [ ] **Step 3: Flip flags** in `docker/.env` (base tree): set `FANFIC_ADMIN_ONLY=false` and `VITE_FANFIC_ADMIN_ONLY=false`. Confirm the RBAC/policy `fanfic` feature default is not admin-only (read `services/policy` seed; if it force-gates, set the public default there too).
- [ ] **Step 4: Land compose** — `printf 'chore(deploy): fanfic daily env + go-public flip' | bash bin/ae-land.sh docker/docker-compose.yml`
- [ ] **Step 5: Recreate** — env changes need a RECREATE, not restart (`feedback_restart_vs_recreate_for_env_changes`): `docker compose -f /data/animeenigma/docker/docker-compose.yml up -d fanfic scheduler` (+ `web` for the VITE flag → `bash bin/ae-deploy.sh web`). Verify: `docker exec animeenigma-fanfic sh -c 'echo $FANFIC_DAILY_ANIME_POOL'` non-empty; `/fanfics` reachable for a non-admin logged-in user.

---

### Task 20: After-update (changelog + deploy verify + push)

- [ ] **Step 1: Invoke `/animeenigma-after-update`** — it runs `/simplify` on the changed code, lints/builds affected services, redeploys changed services (`fanfic`, `catalog`, `scheduler`, `web`), health-checks, writes the Russian Trump-mode changelog entry (via `bin/ae-changelog-add.sh`), and commits+pushes. Feature summary for the changelog: «Фанфик дня» spotlight card + daily AI fallback that health-checks the Groq key.
- [ ] **Step 2: Runtime-smoke** — home page shows the «Фанфик дня» card at 1440px + 390px; «Читать» opens the reader; force-break the key in a scratch env to confirm the 401 alert lands in the maintenance chat (or trust the unit test + a manual `ensure-daily` with a known-bad key on a throwaway container).
- [ ] **Step 3: Worktree cleanup** — only after after-update is green: from base tree `git worktree remove .claude/worktrees/daily-fanfic-spotlight && git worktree prune`.

---

## Self-review notes (author)

- **Spec coverage:** ① fanfic svc → Tasks 1-9; ② catalog resolver → 10-12; ③ scheduler → 13; ④ FE card → 14-18; ⑤ go-public → 19; ⑥ opt-in → 17; alert → 4/6; DTO/explicit-safety → 3/7. Fast-follow ⑦ (account 18+ setting) intentionally absent.
- **Effort & impact:** UXΔ = +2 (Better); CDI = 0.02 * 21; MVQ = Griffin 80%/75% (as spec §8).
- **Open verification for the executor** (confirm against live code, don't assume): (a) exact `animeEnvelope` field names in the fanfic catalog client (Task 5); (b) profile route prefix for the author link (Task 15 — `/u/` vs `/profile/`); (c) the `policy` service `fanfic` feature default (Task 19); (d) whether `ja.json` is required by the broader `locale-parity.spec.ts` (Task 16 — add ja regardless).
