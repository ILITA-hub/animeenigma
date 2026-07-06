# Fanfic Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship an admin-only AI fanfiction generator — a new `fanfic` Go microservice that streams Groq-generated stories from a structured form (anime/characters/tags/length/POV/rating/language/prompt) into a live reader and saves them to a personal library.

**Architecture:** New microservice `services/fanfic/` (`:8097`) mirroring the `anidle`/`gacha` pattern (self-validated JWT via `libs/authz`, own Postgres table via GORM, Redis quota). It owns the Groq egress and streams generation to the browser over SSE. The gateway proxies `/api/fanfic/*` behind a dark-ship `FANFIC_ADMIN_ONLY` gate; the SSE `/generate` route uses a new **flushing** stream-proxy variant. The Vue frontend adds an admin-gated `/fanfics` route reusing the existing anime-search and `/characters` endpoints.

**Tech Stack:** Go 1.25 (chi, GORM, `gorm.io/datatypes`), `libs/{authz,cache,database,httputil,logger,metrics,tracing,errors}`, Groq OpenAI-compatible API, Vue 3 + Pinia + Tailwind (Neon-Tokyo DS), Redis.

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-07-06-fanfic-engine-design.md` (source of truth).
- **Module path:** `github.com/ILITA-hub/animeenigma/services/fanfic`. Go **1.25.0**.
- **Port:** `8097`. **Default model:** `llama-3.1-8b-instant` (env `FANFIC_GROQ_MODEL`, overridable). **Groq base:** `https://api.groq.com/openai/v1` (env `FANFIC_GROQ_BASE_URL`).
- **Groq API key:** `FANFIC_GROQ_API_KEY` — secret, lives ONLY in `docker/.env` (git-ignored, host-only). Never commit the key. Test key for manual smoke: `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`.
- **Enums:** `length ∈ {drabble,oneshot,short}` · `pov ∈ {first,third}` · `rating ∈ {teen,mature,explicit}` · `language ∈ {ru,en}`. Caps: ≤6 characters, ≤8 tags (each ≤32 chars), prompt ≤2000 chars.
- **SSE server:** the fanfic HTTP server MUST use `WriteTimeout: 0` (an SSE stream would be truncated by a non-zero write deadline).
- **go.work gotcha (from project memory):** adding `./services/fanfic` to `go.work` breaks EVERY other service's Docker build unless that service's Dockerfile also `COPY`s `services/fanfic/go.mod`. Task 7 patches all Dockerfiles.
- **Frontend:** DS tokens only (no off-palette Tailwind colors / raw hex / rgba — lint gate is build-enforced), reuse `@/components/ui` primitives, `font-medium`/`font-semibold` only. i18n keys under `fanfic.*` in **en/ru/ja** (parity-gated). Run `/frontend-verify` before finishing any FE task.
- **No time-effort units** (UXΔ/CDI/MVQ only) in any doc/changelog.
- **Worktree:** all work in `/data/ae-fanfic-engine` (branch `feat/fanfic-engine`); never edit the base tree. Push before deploy; deploy from a clean worktree with `.env` copied in.
- **Commit co-authors (every commit):**
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```

---

## Phase 1 — Backend service (`services/fanfic/`)

### Task 1: Module scaffold, config, domain model + request validation, health handler

**Files:**
- Create: `services/fanfic/go.mod` (via `go mod init`)
- Modify: `go.work` (add `./services/fanfic`)
- Create: `services/fanfic/internal/config/config.go`
- Create: `services/fanfic/internal/domain/fanfic.go`
- Create: `services/fanfic/internal/domain/request.go`
- Create: `services/fanfic/internal/domain/tags.go`
- Create: `services/fanfic/internal/domain/request_test.go`
- Create: `services/fanfic/internal/handler/health.go`

**Interfaces:**
- Produces: `domain.Fanfic` (GORM model), `domain.GenerateRequest` with `Validate() error` (implements `httputil.Validator`), `domain.AnimeRef`, `domain.CharacterRef`, `domain.CuratedTags []domain.Tag`, `config.Config`/`config.Load()`.

- [ ] **Step 1: Init the module and register it in go.work**

```bash
cd /data/ae-fanfic-engine/services/fanfic
go mod init github.com/ILITA-hub/animeenigma/services/fanfic
cd /data/ae-fanfic-engine
go work use ./services/fanfic
```

- [ ] **Step 2: Write the domain model** — `services/fanfic/internal/domain/fanfic.go`

```go
package domain

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Fanfic is one generated fanfiction, owned by the user who generated it.
type Fanfic struct {
	ID               string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           string         `gorm:"type:uuid;index;not null" json:"-"`
	AnimeID          string         `gorm:"type:uuid;index" json:"anime_id"`
	AnimeShikimoriID string         `gorm:"size:32;index" json:"anime_shikimori_id"`
	AnimeTitle       string         `gorm:"size:512" json:"anime_title"`
	AnimeJapanese    string         `gorm:"size:512" json:"anime_japanese"`
	AnimePoster      string         `gorm:"size:1024" json:"anime_poster"`
	Characters       datatypes.JSON `gorm:"type:jsonb" json:"characters"`
	Tags             datatypes.JSON `gorm:"type:jsonb" json:"tags"`
	Length           string         `gorm:"size:16" json:"length"`
	POV              string         `gorm:"size:16" json:"pov"`
	Rating           string         `gorm:"size:16" json:"rating"`
	Language         string         `gorm:"size:8" json:"language"`
	Prompt           string         `gorm:"type:text" json:"prompt"`
	Title            string         `gorm:"size:512" json:"title"`
	Content          string         `gorm:"type:text" json:"content"`
	Model            string         `gorm:"size:64" json:"model"`
	TokenUsage       int            `json:"token_usage"`
	Status           string         `gorm:"size:16;index" json:"status"`
	ErrorMsg         string         `gorm:"type:text" json:"error,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

// Status values.
const (
	StatusGenerating = "generating"
	StatusComplete   = "complete"
	StatusFailed     = "failed"
)

func (Fanfic) TableName() string { return "fanfics" }
```

- [ ] **Step 3: Write the curated tag list** — `services/fanfic/internal/domain/tags.go`

```go
package domain

// Tag is a curated fanfic tag with localized labels.
type Tag struct {
	Slug string `json:"slug"`
	RU   string `json:"ru"`
	EN   string `json:"en"`
}

// CuratedTags is the picker's suggestion list (users may also add free-text tags).
var CuratedTags = []Tag{
	{"fluff", "флафф", "fluff"},
	{"angst", "ангст", "angst"},
	{"slow-burn", "медленное развитие", "slow burn"},
	{"romance", "романтика", "romance"},
	{"comedy", "юмор", "comedy"},
	{"drama", "драма", "drama"},
	{"au", "AU", "AU"},
	{"hurt-comfort", "hurt/comfort", "hurt/comfort"},
	{"adventure", "приключения", "adventure"},
	{"friendship", "дружба", "friendship"},
}
```

- [ ] **Step 4: Write the failing request-validation test** — `services/fanfic/internal/domain/request_test.go`

```go
package domain

import (
	"strings"
	"testing"
)

func validReq() GenerateRequest {
	return GenerateRequest{
		Anime:      AnimeRef{Title: "Frieren"},
		Characters: []CharacterRef{{Name: "Frieren"}, {Name: "Fern"}},
		Tags:       []string{"slow-burn"},
		Length:     "oneshot",
		POV:        "third",
		Rating:     "mature",
		Language:   "ru",
		Prompt:     "тихий вечер у костра",
	}
}

func TestValidate_OK(t *testing.T) {
	if err := validReq().Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidate_BadEnums(t *testing.T) {
	cases := map[string]func(*GenerateRequest){
		"length":   func(r *GenerateRequest) { r.Length = "epic" },
		"pov":      func(r *GenerateRequest) { r.POV = "second" },
		"rating":   func(r *GenerateRequest) { r.Rating = "nsfw" },
		"language": func(r *GenerateRequest) { r.Language = "de" },
		"title":    func(r *GenerateRequest) { r.Anime.Title = "" },
	}
	for name, mut := range cases {
		r := validReq()
		mut(&r)
		if err := r.Validate(); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestValidate_Caps(t *testing.T) {
	r := validReq()
	for i := 0; i < 7; i++ {
		r.Characters = append(r.Characters, CharacterRef{Name: "X"})
	}
	if err := r.Validate(); err == nil {
		t.Error("expected too-many-characters error")
	}
	r = validReq()
	r.Prompt = strings.Repeat("a", 2001)
	if err := r.Validate(); err == nil {
		t.Error("expected prompt-too-long error")
	}
}
```

- [ ] **Step 5: Run it — expect a compile failure (GenerateRequest undefined)**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/domain/... 2>&1 | head`
Expected: build error — `undefined: GenerateRequest`.

- [ ] **Step 6: Write the request type + validation** — `services/fanfic/internal/domain/request.go`

```go
package domain

import (
	"fmt"
	"strings"
)

// AnimeRef is the anime snapshot the client sends (already fetched for the picker).
type AnimeRef struct {
	ID          string `json:"id"`
	ShikimoriID string `json:"shikimori_id"`
	Title       string `json:"title"`
	Japanese    string `json:"japanese"`
	Poster      string `json:"poster"`
}

// CharacterRef is one selected character (id optional).
type CharacterRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GenerateRequest is the POST /api/fanfic/generate body.
type GenerateRequest struct {
	Anime      AnimeRef       `json:"anime"`
	Characters []CharacterRef `json:"characters"`
	Tags       []string       `json:"tags"`
	Length     string         `json:"length"`
	POV        string         `json:"pov"`
	Rating     string         `json:"rating"`
	Language   string         `json:"language"`
	Prompt     string         `json:"prompt"`
}

var (
	validLength   = map[string]bool{"drabble": true, "oneshot": true, "short": true}
	validPOV      = map[string]bool{"first": true, "third": true}
	validRating   = map[string]bool{"teen": true, "mature": true, "explicit": true}
	validLanguage = map[string]bool{"ru": true, "en": true}
)

// Validate implements httputil.Validator.
func (r GenerateRequest) Validate() error {
	if strings.TrimSpace(r.Anime.Title) == "" {
		return fmt.Errorf("anime title is required")
	}
	if !validLength[r.Length] {
		return fmt.Errorf("invalid length %q", r.Length)
	}
	if !validPOV[r.POV] {
		return fmt.Errorf("invalid pov %q", r.POV)
	}
	if !validRating[r.Rating] {
		return fmt.Errorf("invalid rating %q", r.Rating)
	}
	if !validLanguage[r.Language] {
		return fmt.Errorf("invalid language %q", r.Language)
	}
	if len(r.Characters) > 6 {
		return fmt.Errorf("too many characters (max 6)")
	}
	if len(r.Tags) > 8 {
		return fmt.Errorf("too many tags (max 8)")
	}
	for _, t := range r.Tags {
		if len(t) > 32 {
			return fmt.Errorf("tag too long (max 32): %q", t)
		}
	}
	if len(r.Prompt) > 2000 {
		return fmt.Errorf("prompt too long (max 2000)")
	}
	return nil
}
```

- [ ] **Step 7: Write the config** — `services/fanfic/internal/config/config.go`

```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

type Config struct {
	Server   ServerConfig
	Database database.Config
	Redis    cache.Config
	JWT      authz.JWTConfig
	Groq     GroqConfig
	DailyCap int // FANFIC_DAILY_CAP — max generations per user per day (default 100)
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

type GroqConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}
	if getEnv("FANFIC_GROQ_API_KEY", "") == "" {
		return nil, fmt.Errorf("FANFIC_GROQ_API_KEY environment variable is required")
	}
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8097),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "redis"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: authz.JWTConfig{
			Secret:          getEnv("JWT_SECRET", ""),
			Issuer:          getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		Groq: GroqConfig{
			APIKey:  getEnv("FANFIC_GROQ_API_KEY", ""),
			BaseURL: getEnv("FANFIC_GROQ_BASE_URL", "https://api.groq.com/openai/v1"),
			Model:   getEnv("FANFIC_GROQ_MODEL", "llama-3.1-8b-instant"),
			Timeout: getEnvDuration("FANFIC_GROQ_TIMEOUT", 120*time.Second),
		},
		DailyCap: getEnvInt("FANFIC_DAILY_CAP", 100),
	}, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
```

- [ ] **Step 8: Write the health handler** — `services/fanfic/internal/handler/health.go`

```go
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// Health is the liveness handler (GET + HEAD /health).
func Health(w http.ResponseWriter, _ *http.Request) {
	httputil.OK(w, map[string]string{"status": "ok"})
}
```

- [ ] **Step 9: Tidy and run tests**

```bash
cd /data/ae-fanfic-engine/services/fanfic && go mod tidy && go test ./internal/domain/... -v
```
Expected: all domain tests PASS. (`go mod tidy` pulls chi, gorm, datatypes, and the workspace libs.)

- [ ] **Step 10: Commit**

```bash
cd /data/ae-fanfic-engine
git add services/fanfic/go.mod services/fanfic/go.sum go.work go.work.sum services/fanfic/internal/
git commit -m "feat(fanfic): module scaffold, domain model, request validation, config

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: Groq streaming client

**Files:**
- Create: `services/fanfic/internal/groq/client.go`
- Create: `services/fanfic/internal/groq/client_test.go`

**Interfaces:**
- Produces: `groq.Client` with `Stream(ctx, system, user string, maxTokens int, temperature float64, onDelta func(string)) (text string, usage int, err error)`. Satisfies the `service.streamer` interface consumed in Task 5.

- [ ] **Step 1: Write the failing test** — `services/fanfic/internal/groq/client_test.go`

```go
package groq

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStream_AccumulatesDeltasAndUsage(t *testing.T) {
	// Fake Groq SSE: two content deltas, then a usage-only chunk, then [DONE].
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("missing auth header, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"# Title\\n\\nHello\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"total_tokens\":42}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "llama-3.1-8b-instant", 5*time.Second)
	var deltas []string
	text, usage, err := c.Stream(context.Background(), "sys", "usr", 100, 0.9, func(d string) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("Stream err: %v", err)
	}
	if !strings.Contains(text, "Hello world") {
		t.Errorf("text = %q, want it to contain 'Hello world'", text)
	}
	if usage != 42 {
		t.Errorf("usage = %d, want 42", usage)
	}
	if len(deltas) != 2 {
		t.Errorf("onDelta called %d times, want 2", len(deltas))
	}
}

func TestStream_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New("k", srv.URL, "m", time.Second)
	if _, _, err := c.Stream(context.Background(), "s", "u", 10, 0.5, func(string) {}); err == nil {
		t.Fatal("expected error on 429, got nil")
	}
}
```

- [ ] **Step 2: Run it — expect compile failure**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/groq/... 2>&1 | head`
Expected: `undefined: New`.

- [ ] **Step 3: Implement the client** — `services/fanfic/internal/groq/client.go`

```go
// Package groq is a minimal OpenAI-compatible streaming client for the Groq API.
package groq

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

func New(apiKey, baseURL, model string, timeout time.Duration) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		http:    &http.Client{Timeout: timeout},
	}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model         string        `json:"model"`
	Messages      []message     `json:"messages"`
	MaxTokens     int           `json:"max_tokens"`
	Temperature   float64       `json:"temperature"`
	Stream        bool          `json:"stream"`
	StreamOptions streamOptions `json:"stream_options"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type sseChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// Stream calls chat/completions with stream:true, invoking onDelta for each token
// chunk. It returns the full accumulated text and total token usage (0 if absent).
func (c *Client) Stream(ctx context.Context, system, user string, maxTokens int, temperature float64, onDelta func(string)) (string, int, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		MaxTokens:     maxTokens,
		Temperature:   temperature,
		Stream:        true,
		StreamOptions: streamOptions{IncludeUsage: true},
	})
	if err != nil {
		return "", 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", 0, fmt.Errorf("groq status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var sb strings.Builder
	usage := 0
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}
		var chunk sseChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue // tolerate keep-alive / malformed lines
		}
		if chunk.Usage != nil {
			usage = chunk.Usage.TotalTokens
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				sb.WriteString(ch.Delta.Content)
				onDelta(ch.Delta.Content)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return sb.String(), usage, fmt.Errorf("reading groq stream: %w", err)
	}
	return sb.String(), usage, nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/groq/... -v`
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/ae-fanfic-engine
git add services/fanfic/internal/groq/
git commit -m "feat(fanfic): Groq OpenAI-compatible streaming client

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: Prompt builder

**Files:**
- Create: `services/fanfic/internal/service/prompt.go`
- Create: `services/fanfic/internal/service/prompt_test.go`

**Interfaces:**
- Produces: `service.BuildMessages(req domain.GenerateRequest) (system, user string)` and `service.MaxTokensFor(length string) int` and `service.SplitTitle(text string) (title, body string)`.

- [ ] **Step 1: Write the failing test** — `services/fanfic/internal/service/prompt_test.go`

```go
package service

import (
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

func TestBuildMessages_RU_Mature(t *testing.T) {
	req := domain.GenerateRequest{
		Anime:      domain.AnimeRef{Title: "Frieren", Japanese: "葬送のフリーレン"},
		Characters: []domain.CharacterRef{{Name: "Frieren"}, {Name: "Fern"}},
		Tags:       []string{"slow-burn", "angst"},
		Length:     "oneshot", POV: "third", Rating: "mature", Language: "ru",
		Prompt: "тихий вечер у костра",
	}
	sys, usr := BuildMessages(req)
	if !strings.Contains(sys, "РУССКИЙ") {
		t.Error("system prompt should pin Russian output")
	}
	if !strings.Contains(sys, "# ") {
		t.Error("system prompt should instruct a leading '# Title' line")
	}
	if !strings.Contains(sys, "18+") {
		t.Error("system prompt should frame characters as adults")
	}
	if !strings.Contains(usr, "Frieren") || !strings.Contains(usr, "Fern") {
		t.Error("user prompt should list characters")
	}
	if !strings.Contains(usr, "slow-burn") {
		t.Error("user prompt should list tags")
	}
	if !strings.Contains(usr, "тихий вечер") {
		t.Error("user prompt should include the author brief")
	}
}

func TestBuildMessages_EN_Teen_NoExplicit(t *testing.T) {
	req := domain.GenerateRequest{
		Anime: domain.AnimeRef{Title: "Bocchi"}, Length: "drabble",
		POV: "first", Rating: "teen", Language: "en",
	}
	sys, _ := BuildMessages(req)
	if !strings.Contains(sys, "ENGLISH") {
		t.Error("system prompt should pin English output")
	}
	if !strings.Contains(strings.ToLower(sys), "no explicit") {
		t.Error("teen tier should forbid explicit content")
	}
}

func TestMaxTokensFor(t *testing.T) {
	if MaxTokensFor("drabble") >= MaxTokensFor("oneshot") || MaxTokensFor("oneshot") >= MaxTokensFor("short") {
		t.Error("token budget must increase with length")
	}
	if MaxTokensFor("unknown") == 0 {
		t.Error("unknown length should fall back to a sane default")
	}
}

func TestSplitTitle(t *testing.T) {
	title, body := SplitTitle("# Тяжесть столетий\n\nКогда солнце...")
	if title != "Тяжесть столетий" {
		t.Errorf("title = %q", title)
	}
	if strings.HasPrefix(body, "#") {
		t.Error("body should have the H1 stripped")
	}
	title2, body2 := SplitTitle("no heading here")
	if title2 != "" || body2 != "no heading here" {
		t.Errorf("no-heading case: title=%q body=%q", title2, body2)
	}
}
```

- [ ] **Step 2: Run it — expect compile failure**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/service/... 2>&1 | head`
Expected: `undefined: BuildMessages`.

- [ ] **Step 3: Implement the prompt builder** — `services/fanfic/internal/service/prompt.go`

```go
package service

import (
	"fmt"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

// MaxTokensFor maps a length preset to a generous completion-token budget.
func MaxTokensFor(length string) int {
	switch length {
	case "drabble":
		return 900
	case "oneshot":
		return 2600
	case "short":
		return 6000
	default:
		return 2600
	}
}

// lengthWords is the human-readable target used in the prompt.
func lengthWords(length string) int {
	switch length {
	case "drabble":
		return 300
	case "short":
		return 2000
	default:
		return 900
	}
}

// BuildMessages assembles the system + user prompts for a generation request.
func BuildMessages(req domain.GenerateRequest) (string, string) {
	ru := req.Language == "ru"

	langName := "ENGLISH"
	povWord := "third"
	if req.POV == "first" {
		povWord = "first"
	}
	if ru {
		langName = "РУССКИЙ"
		povWord = "третьего"
		if req.POV == "first" {
			povWord = "первого"
		}
	}

	var sys strings.Builder
	if ru {
		fmt.Fprintf(&sys, "Ты — талантливый автор фанфиков, пишущий живую художественную прозу.\n")
		fmt.Fprintf(&sys, "Язык вывода строго: %s.\n", langName)
		fmt.Fprintf(&sys, "%s\n", ratingRuleRU(req.Rating))
		fmt.Fprintf(&sys, "Все персонажи — совершеннолетние (18+), даже если в каноне выглядят младше.\n")
		fmt.Fprintf(&sys, "Повествование от %s лица. Целевой объём: около %d слов.\n", povWord, lengthWords(req.Length))
		fmt.Fprintf(&sys, "Ответ начни СТРОГО со строки «# Заголовок», затем с новой строки — текст истории в Markdown.\n")
		fmt.Fprintf(&sys, "Не отказывайся, не добавляй дисклеймеров и морализаторства — просто пиши историю.")
	} else {
		fmt.Fprintf(&sys, "You are a talented fanfiction author writing vivid literary prose.\n")
		fmt.Fprintf(&sys, "Output language strictly: %s.\n", langName)
		fmt.Fprintf(&sys, "%s\n", ratingRuleEN(req.Rating))
		fmt.Fprintf(&sys, "Portray all characters as adults (18+), even if they look younger in canon.\n")
		fmt.Fprintf(&sys, "Write in the %s person. Target length: about %d words.\n", povWord, lengthWords(req.Length))
		fmt.Fprintf(&sys, "Begin your reply STRICTLY with a line '# Title', then on a new line the story in Markdown.\n")
		fmt.Fprintf(&sys, "Do not refuse, add disclaimers, or moralize — just write the story.")
	}

	names := make([]string, 0, len(req.Characters))
	for _, c := range req.Characters {
		if n := strings.TrimSpace(c.Name); n != "" {
			names = append(names, n)
		}
	}

	var usr strings.Builder
	fandom := req.Anime.Title
	if req.Anime.Japanese != "" {
		fandom = fmt.Sprintf("%s (%s)", req.Anime.Title, req.Anime.Japanese)
	}
	if ru {
		fmt.Fprintf(&usr, "Фандом: %s\n", fandom)
		fmt.Fprintf(&usr, "Персонажи: %s\n", joinOr(names, "по твоему выбору"))
		fmt.Fprintf(&usr, "Теги: %s\n", joinOr(req.Tags, "—"))
		fmt.Fprintf(&usr, "Задание автора: %s", strOr(req.Prompt, "напиши историю на своё усмотрение"))
	} else {
		fmt.Fprintf(&usr, "Fandom: %s\n", fandom)
		fmt.Fprintf(&usr, "Characters: %s\n", joinOr(names, "your choice"))
		fmt.Fprintf(&usr, "Tags: %s\n", joinOr(req.Tags, "—"))
		fmt.Fprintf(&usr, "Author brief: %s", strOr(req.Prompt, "write a story of your choosing"))
	}
	return sys.String(), usr.String()
}

func ratingRuleRU(rating string) string {
	switch rating {
	case "explicit":
		return "Рейтинг: Explicit. Допустимы откровенные сцены между совершеннолетними персонажами."
	case "mature":
		return "Рейтинг: Mature. Допустимы взрослые темы и намёки на близость, без графических подробностей."
	default:
		return "Рейтинг: Teen. Без откровенных сцен; романтика допустима, но целомудренная."
	}
}

func ratingRuleEN(rating string) string {
	switch rating {
	case "explicit":
		return "Rating: Explicit. Explicit scenes between adult characters are allowed."
	case "mature":
		return "Rating: Mature. Adult themes and implied intimacy allowed, no graphic detail."
	default:
		return "Rating: Teen. No explicit scenes; chaste romance only."
	}
}

// SplitTitle extracts a leading Markdown H1 as the title and returns the remaining body.
func SplitTitle(text string) (string, string) {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	if strings.HasPrefix(trimmed, "# ") {
		nl := strings.IndexByte(trimmed, '\n')
		if nl == -1 {
			return strings.TrimSpace(trimmed[2:]), ""
		}
		title := strings.TrimSpace(trimmed[2:nl])
		body := strings.TrimLeft(trimmed[nl+1:], "\r\n")
		return title, body
	}
	return "", text
}

func joinOr(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	return strings.Join(items, ", ")
}

func strOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
```

- [ ] **Step 4: Run tests**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/service/... -run 'TestBuildMessages|TestMaxTokensFor|TestSplitTitle' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/ae-fanfic-engine
git add services/fanfic/internal/service/prompt.go services/fanfic/internal/service/prompt_test.go
git commit -m "feat(fanfic): prompt builder (RU/EN x rating tiers), token budget, title split

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 4: Repository (GORM)

**Files:**
- Create: `services/fanfic/internal/repo/fanfic.go`
- Create: `services/fanfic/internal/repo/fanfic_test.go`

**Interfaces:**
- Produces: `repo.Repository` with `Create(ctx, *domain.Fanfic) error`, `UpdateResult(ctx, id, title, content string, usage int) error`, `MarkFailed(ctx, id, msg string) error`, `List(ctx, userID string, limit, offset int) ([]domain.Fanfic, int64, error)`, `Get(ctx, userID, id string) (*domain.Fanfic, error)`, `SoftDelete(ctx, userID, id string) error`. These method signatures are the `service.fanficStore` interface consumed in Task 5 and the `handler.libraryStore` interface in Task 6.

- [ ] **Step 1: Write the failing test (testcontainers Postgres)** — `services/fanfic/internal/repo/fanfic_test.go`

```go
package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"gorm.io/datatypes"
)

func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	db := database.NewTestDB(t, &domain.Fanfic{}) // helper mirrors other services' repo tests
	return NewRepository(db)
}

func TestCreateAndGet(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	f := &domain.Fanfic{
		UserID: "11111111-1111-1111-1111-111111111111",
		AnimeTitle: "Frieren", Rating: "mature", Language: "ru",
		Characters: datatypes.JSON([]byte(`[{"name":"Frieren"}]`)),
		Tags:       datatypes.JSON([]byte(`["angst"]`)),
		Status:     domain.StatusGenerating,
	}
	if err := r.Create(ctx, f); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.ID == "" {
		t.Fatal("expected generated ID")
	}
	got, err := r.Get(ctx, f.UserID, f.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AnimeTitle != "Frieren" {
		t.Errorf("AnimeTitle = %q", got.AnimeTitle)
	}
}

func TestUpdateResultAndList(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	user := "22222222-2222-2222-2222-222222222222"
	f := &domain.Fanfic{UserID: user, AnimeTitle: "A", Status: domain.StatusGenerating}
	_ = r.Create(ctx, f)
	if err := r.UpdateResult(ctx, f.ID, "My Title", "the story", 123); err != nil {
		t.Fatalf("UpdateResult: %v", err)
	}
	items, total, err := r.List(ctx, user, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("total=%d len=%d", total, len(items))
	}
	if items[0].Title != "My Title" || items[0].Status != domain.StatusComplete {
		t.Errorf("row not updated: %+v", items[0])
	}
}

func TestSoftDeleteScopedToOwner(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	owner := "33333333-3333-3333-3333-333333333333"
	other := "44444444-4444-4444-4444-444444444444"
	f := &domain.Fanfic{UserID: owner, AnimeTitle: "A", Status: domain.StatusComplete}
	_ = r.Create(ctx, f)
	// Other user cannot delete it.
	if err := r.SoftDelete(ctx, other, f.ID); err == nil {
		t.Error("expected not-found for non-owner delete")
	}
	if err := r.SoftDelete(ctx, owner, f.ID); err != nil {
		t.Errorf("owner delete failed: %v", err)
	}
	if _, err := r.Get(ctx, owner, f.ID); err == nil {
		t.Error("expected not-found after soft delete")
	}
}
```

> **Note for implementer:** check how the sibling repo tests bootstrap Postgres (`grep -rl testcontainers services/*/internal/repo`). Reuse the project's existing helper. If there is no `database.NewTestDB` helper, copy the container-boot boilerplate from `services/gacha/internal/repo/*_test.go` verbatim into a local `setupTestDB(t)` and adjust the `AutoMigrate` model to `&domain.Fanfic{}`.

- [ ] **Step 2: Run it — expect failure**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/repo/... 2>&1 | head`
Expected: compile failure (`undefined: NewRepository`) or, once the test compiles, FAIL until the impl exists.

- [ ] **Step 3: Implement the repo** — `services/fanfic/internal/repo/fanfic.go`

```go
package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, f *domain.Fanfic) error {
	if err := r.db.WithContext(ctx).Create(f).Error; err != nil {
		return errors.Wrap(err, "create fanfic")
	}
	return nil
}

func (r *Repository) UpdateResult(ctx context.Context, id, title, content string, usage int) error {
	return r.db.WithContext(ctx).Model(&domain.Fanfic{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"title":       title,
			"content":     content,
			"token_usage": usage,
			"status":      domain.StatusComplete,
		}).Error
}

func (r *Repository) MarkFailed(ctx context.Context, id, msg string) error {
	return r.db.WithContext(ctx).Model(&domain.Fanfic{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status": domain.StatusFailed,
			"error_msg": msg,
		}).Error
}

func (r *Repository) List(ctx context.Context, userID string, limit, offset int) ([]domain.Fanfic, int64, error) {
	var items []domain.Fanfic
	var total int64
	q := r.db.WithContext(ctx).Model(&domain.Fanfic{}).Where("user_id = ?", userID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *Repository) Get(ctx context.Context, userID, id string) (*domain.Fanfic, error) {
	var f domain.Fanfic
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.NotFound("fanfic not found")
		}
		return nil, err
	}
	return &f, nil
}

func (r *Repository) SoftDelete(ctx context.Context, userID, id string) error {
	res := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&domain.Fanfic{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.NotFound("fanfic not found")
	}
	return nil
}
```

> **Implementer note:** confirm `libs/errors` exposes `Is`, `Wrap`, `NotFound` (it does — see CLAUDE.md). If `errors.Is` is not re-exported, import the stdlib `errors` for `Is` only.

- [ ] **Step 4: Run tests**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/repo/... -v`
Expected: all PASS (requires Docker for testcontainers).

- [ ] **Step 5: Commit**

```bash
cd /data/ae-fanfic-engine
git add services/fanfic/internal/repo/
git commit -m "feat(fanfic): GORM repository (create/update/list/get/soft-delete, owner-scoped)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 5: Generation orchestration + Redis quota

**Files:**
- Create: `services/fanfic/internal/service/quota.go`
- Create: `services/fanfic/internal/service/quota_test.go`
- Create: `services/fanfic/internal/service/generate.go`
- Create: `services/fanfic/internal/service/generate_test.go`

**Interfaces:**
- Produces:
  - `service.Quota` with `Acquire(ctx, userID string) (release func(), err error)` (fail-open on Redis error).
  - `service.Generator` with `Generate(ctx, userID string, req domain.GenerateRequest, emit Emit) error` where `type Emit func(event string, data any) error`.
- Consumes: the `streamer` interface (satisfied by `groq.Client`), the `fanficStore` interface (satisfied by `repo.Repository`), and a `quotaStore` interface (adapter over `cache.RedisCache`).

- [ ] **Step 1: Write the quota test (hand fake, no Redis dependency)** — `services/fanfic/internal/service/quota_test.go`

```go
package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeQuotaStore struct {
	counts map[string]int64
	locks  map[string]bool
	fail   bool
}

func newFakeQuotaStore() *fakeQuotaStore {
	return &fakeQuotaStore{counts: map[string]int64{}, locks: map[string]bool{}}
}

func (f *fakeQuotaStore) Incr(_ context.Context, key string) (int64, error) {
	if f.fail {
		return 0, errors.New("redis down")
	}
	f.counts[key]++
	return f.counts[key], nil
}
func (f *fakeQuotaStore) Expire(_ context.Context, _ string, _ time.Duration) error { return nil }
func (f *fakeQuotaStore) SetNX(_ context.Context, key string, _ time.Duration) (bool, error) {
	if f.fail {
		return false, errors.New("redis down")
	}
	if f.locks[key] {
		return false, nil
	}
	f.locks[key] = true
	return true, nil
}
func (f *fakeQuotaStore) Del(_ context.Context, key string) error { delete(f.locks, key); return nil }

func TestQuota_AllowsUnderCapThenBlocks(t *testing.T) {
	store := newFakeQuotaStore()
	q := NewQuota(store, 2, func() time.Time { return time.Unix(0, 0) })
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		rel, err := q.Acquire(ctx, "u")
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		rel()
	}
	if _, err := q.Acquire(ctx, "u"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}
}

func TestQuota_ConcurrencyLock(t *testing.T) {
	store := newFakeQuotaStore()
	q := NewQuota(store, 100, func() time.Time { return time.Unix(0, 0) })
	rel, err := q.Acquire(context.Background(), "u")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := q.Acquire(context.Background(), "u"); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy while locked, got %v", err)
	}
	rel()
}

func TestQuota_FailsOpenOnRedisError(t *testing.T) {
	store := newFakeQuotaStore()
	store.fail = true
	q := NewQuota(store, 1, func() time.Time { return time.Unix(0, 0) })
	rel, err := q.Acquire(context.Background(), "u")
	if err != nil {
		t.Fatalf("expected fail-open (nil err), got %v", err)
	}
	rel() // no-op release must be safe
}
```

- [ ] **Step 2: Implement the quota** — `services/fanfic/internal/service/quota.go`

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrQuotaExceeded = errors.New("daily generation quota exceeded")
	ErrBusy          = errors.New("a generation is already in progress")
)

// quotaStore is the minimal Redis surface the quota needs (adapter in main.go).
type quotaStore interface {
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Del(ctx context.Context, key string) error
}

type Quota struct {
	store    quotaStore
	dailyCap int
	now      func() time.Time
}

func NewQuota(store quotaStore, dailyCap int, now func() time.Time) *Quota {
	return &Quota{store: store, dailyCap: dailyCap, now: now}
}

// Acquire enforces (a) one concurrent generation per user and (b) a daily cap.
// On any Redis error it FAILS OPEN (returns a no-op release, nil error) so a
// Redis blip never blocks an admin. The returned release must always be called.
func (q *Quota) Acquire(ctx context.Context, userID string) (func(), error) {
	noop := func() {}
	lockKey := "fanfic:lock:" + userID
	ok, err := q.store.SetNX(ctx, lockKey, 3*time.Minute)
	if err != nil {
		return noop, nil // fail open
	}
	if !ok {
		return noop, ErrBusy
	}
	release := func() { _ = q.store.Del(context.Background(), lockKey) }

	dayKey := fmt.Sprintf("fanfic:quota:%s:%s", userID, q.now().UTC().Format("20060102"))
	n, err := q.store.Incr(ctx, dayKey)
	if err != nil {
		return release, nil // fail open, but still release the lock
	}
	if n == 1 {
		_ = q.store.Expire(ctx, dayKey, 48*time.Hour)
	}
	if int(n) > q.dailyCap {
		release()
		return noop, ErrQuotaExceeded
	}
	return release, nil
}
```

- [ ] **Step 3: Run the quota tests**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/service/... -run TestQuota -v`
Expected: all PASS.

- [ ] **Step 4: Write the generator test** — `services/fanfic/internal/service/generate_test.go`

```go
package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

type fakeStreamer struct {
	out string
}

func (f *fakeStreamer) Stream(_ context.Context, _, _ string, _ int, _ float64, onDelta func(string)) (string, int, error) {
	onDelta(f.out)
	return f.out, 55, nil
}

type fakeStore struct {
	created *domain.Fanfic
	title   string
	body    string
	usage   int
}

func (s *fakeStore) Create(_ context.Context, f *domain.Fanfic) error {
	f.ID = "fixed-id"
	s.created = f
	return nil
}
func (s *fakeStore) UpdateResult(_ context.Context, _, title, content string, usage int) error {
	s.title, s.body, s.usage = title, content, usage
	return nil
}
func (s *fakeStore) MarkFailed(_ context.Context, _, _ string) error { return nil }

type noopQuota struct{}

func (noopQuota) Acquire(_ context.Context, _ string) (func(), error) { return func() {}, nil }

func TestGenerate_StreamsPersistsAndSplitsTitle(t *testing.T) {
	store := &fakeStore{}
	g := NewGenerator(&fakeStreamer{out: "# Тяжесть столетий\n\nКогда солнце..."}, store, noopQuota{}, "llama-3.1-8b-instant", nil)

	var events []string
	emit := func(event string, _ any) error { events = append(events, event); return nil }

	req := domain.GenerateRequest{Anime: domain.AnimeRef{Title: "Frieren"}, Length: "oneshot", POV: "third", Rating: "mature", Language: "ru"}
	if err := g.Generate(context.Background(), "user-1", req, emit); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Persisted with parsed title + body + usage.
	if store.title != "Тяжесть столетий" {
		t.Errorf("title = %q", store.title)
	}
	if store.usage != 55 {
		t.Errorf("usage = %d", store.usage)
	}
	if store.created.UserID != "user-1" || store.created.AnimeTitle != "Frieren" {
		t.Errorf("snapshot wrong: %+v", store.created)
	}
	// Event order: meta, delta..., done.
	if events[0] != "meta" || events[len(events)-1] != "done" {
		t.Errorf("events = %v", events)
	}
}
```

- [ ] **Step 5: Run it — expect compile failure**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/service/... -run TestGenerate 2>&1 | head`
Expected: `undefined: NewGenerator`.

- [ ] **Step 6: Implement the generator** — `services/fanfic/internal/service/generate.go`

```go
package service

import (
	"context"
	"encoding/json"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"gorm.io/datatypes"
)

// Emit sends one SSE event to the client. A non-nil return (client gone) is
// logged and ignored — server-side accumulation + persistence continue.
type Emit func(event string, data any) error

type streamer interface {
	Stream(ctx context.Context, system, user string, maxTokens int, temperature float64, onDelta func(string)) (string, int, error)
}

type fanficStore interface {
	Create(ctx context.Context, f *domain.Fanfic) error
	UpdateResult(ctx context.Context, id, title, content string, usage int) error
	MarkFailed(ctx context.Context, id, msg string) error
}

type quota interface {
	Acquire(ctx context.Context, userID string) (func(), error)
}

type Generator struct {
	groq  streamer
	store fanficStore
	quota quota
	model string
	log   *logger.Logger
}

func NewGenerator(groq streamer, store fanficStore, quota quota, model string, log *logger.Logger) *Generator {
	return &Generator{groq: groq, store: store, quota: quota, model: model, log: log}
}

func (g *Generator) Generate(ctx context.Context, userID string, req domain.GenerateRequest, emit Emit) error {
	release, err := g.quota.Acquire(ctx, userID)
	if err != nil {
		return err
	}
	defer release()

	chars, _ := json.Marshal(req.Characters)
	tags, _ := json.Marshal(req.Tags)
	f := &domain.Fanfic{
		UserID:           userID,
		AnimeID:          req.Anime.ID,
		AnimeShikimoriID: req.Anime.ShikimoriID,
		AnimeTitle:       req.Anime.Title,
		AnimeJapanese:    req.Anime.Japanese,
		AnimePoster:      req.Anime.Poster,
		Characters:       datatypes.JSON(chars),
		Tags:             datatypes.JSON(tags),
		Length:           req.Length,
		POV:              req.POV,
		Rating:           req.Rating,
		Language:         req.Language,
		Prompt:           req.Prompt,
		Model:            g.model,
		Status:           domain.StatusGenerating,
	}
	if err := g.store.Create(ctx, f); err != nil {
		return err
	}
	g.safeEmit(emit, "meta", map[string]any{"id": f.ID, "model": g.model})

	system, user := BuildMessages(req)
	text, usage, err := g.groq.Stream(ctx, system, user, MaxTokensFor(req.Length), 0.9, func(delta string) {
		g.safeEmit(emit, "delta", map[string]any{"text": delta})
	})
	if err != nil {
		_ = g.store.MarkFailed(ctx, f.ID, err.Error())
		g.safeEmit(emit, "error", map[string]any{"message": err.Error()})
		return err
	}

	title, body := SplitTitle(text)
	if err := g.store.UpdateResult(ctx, f.ID, title, body, usage); err != nil {
		if g.log != nil {
			g.log.Errorw("failed to persist fanfic result", "id", f.ID, "error", err)
		}
	}
	g.safeEmit(emit, "done", map[string]any{"id": f.ID, "title": title, "token_usage": usage})
	return nil
}

func (g *Generator) safeEmit(emit Emit, event string, data any) {
	if emit == nil {
		return
	}
	if err := emit(event, data); err != nil && g.log != nil {
		g.log.Debugw("sse emit failed (client likely disconnected)", "event", event, "error", err)
	}
}
```

- [ ] **Step 7: Run all service tests**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/service/... -v`
Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
cd /data/ae-fanfic-engine
git add services/fanfic/internal/service/quota.go services/fanfic/internal/service/quota_test.go services/fanfic/internal/service/generate.go services/fanfic/internal/service/generate_test.go
git commit -m "feat(fanfic): generation orchestration + fail-open Redis quota

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 6: HTTP handlers + router

**Files:**
- Create: `services/fanfic/internal/handler/fanfic.go`
- Create: `services/fanfic/internal/handler/fanfic_test.go`
- Create: `services/fanfic/internal/transport/router.go`

**Interfaces:**
- Consumes: `service.Generator` (via a `generator` interface), `repo.Repository` (via a `libraryStore` interface), `domain.CuratedTags`, `authz.UserIDFromContext`.
- Produces: `handler.NewHandler(gen, repo, log) *Handler` with `Generate/List/Get/Delete/Tags`; `transport.NewRouter(h *handler.Handler, jwtConfig authz.JWTConfig, log, metricsCollector) http.Handler`.

- [ ] **Step 1: Write the failing handler test** — `services/fanfic/internal/handler/fanfic_test.go`

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

type fakeGen struct{ events []string }

func (f *fakeGen) Generate(_ context.Context, userID string, _ domain.GenerateRequest, emit func(string, any) error) error {
	_ = emit("meta", map[string]any{"id": "x", "user": userID})
	_ = emit("delta", map[string]any{"text": "# T\n\nhi"})
	_ = emit("done", map[string]any{"id": "x"})
	return nil
}

type fakeLib struct{ deleted bool }

func (f *fakeLib) List(_ context.Context, _ string, _, _ int) ([]domain.Fanfic, int64, error) {
	return []domain.Fanfic{{ID: "x", Title: "T"}}, 1, nil
}
func (f *fakeLib) Get(_ context.Context, _, id string) (*domain.Fanfic, error) {
	return &domain.Fanfic{ID: id, Title: "T", Content: "hi"}, nil
}
func (f *fakeLib) SoftDelete(_ context.Context, _, _ string) error { f.deleted = true; return nil }

func withUser(r *http.Request) *http.Request {
	claims := &authz.Claims{UserID: "u-1"}
	return r.WithContext(authz.ContextWithClaims(r.Context(), claims))
}

func TestGenerate_SSE(t *testing.T) {
	h := NewHandler(&fakeGen{}, &fakeLib{}, nil)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/fanfic/generate",
		strings.NewReader(`{"anime":{"title":"Frieren"},"length":"oneshot","pov":"third","rating":"teen","language":"ru"}`)))
	rec := httptest.NewRecorder()
	h.Generate(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"event: meta", "event: delta", "event: done"} {
		if !strings.Contains(body, want) {
			t.Errorf("SSE body missing %q; got:\n%s", want, body)
		}
	}
}

func TestGenerate_ValidationError(t *testing.T) {
	h := NewHandler(&fakeGen{}, &fakeLib{}, nil)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/fanfic/generate",
		strings.NewReader(`{"anime":{"title":""},"length":"oneshot","pov":"third","rating":"teen","language":"ru"}`)))
	rec := httptest.NewRecorder()
	h.Generate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestTags(t *testing.T) {
	h := NewHandler(&fakeGen{}, &fakeLib{}, nil)
	rec := httptest.NewRecorder()
	h.Tags(rec, httptest.NewRequest(http.MethodGet, "/api/fanfic/tags", nil))
	var resp struct {
		Data []domain.Tag `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Data) == 0 {
		t.Error("expected curated tags")
	}
}
```

> **Implementer note:** confirm `authz.Claims` field name for user id (`grep -n "UserID\|Subject" libs/authz/jwt.go`). Adjust `withUser` if the field differs.

- [ ] **Step 2: Run it — expect compile failure**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/handler/... 2>&1 | head`
Expected: `undefined: NewHandler`.

- [ ] **Step 3: Implement the handlers** — `services/fanfic/internal/handler/fanfic.go`

```go
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/pagination"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/service"
	"github.com/go-chi/chi/v5"
)

type generator interface {
	Generate(ctx context.Context, userID string, req domain.GenerateRequest, emit service.Emit) error
}

type libraryStore interface {
	List(ctx context.Context, userID string, limit, offset int) ([]domain.Fanfic, int64, error)
	Get(ctx context.Context, userID, id string) (*domain.Fanfic, error)
	SoftDelete(ctx context.Context, userID, id string) error
}

type Handler struct {
	gen  generator
	repo libraryStore
	log  *logger.Logger
}

func NewHandler(gen generator, repo libraryStore, log *logger.Logger) *Handler {
	return &Handler{gen: gen, repo: repo, log: log}
}

// Generate streams a fanfic as SSE and persists it on completion.
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if userID == "" {
		httputil.Unauthorized(w)
		return
	}
	var req domain.GenerateRequest
	if err := httputil.BindAndValidate(r, &req); err != nil {
		httputil.BadRequest(w, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // belt-and-suspenders for any buffering proxy
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

	// Detach from the request context so a client disconnect does NOT abort
	// server-side accumulation + persistence (spec §4).
	ctx := context.WithoutCancel(r.Context())
	if err := h.gen.Generate(ctx, userID, req, emit); err != nil && h.log != nil {
		h.log.Warnw("fanfic generation ended with error", "user_id", userID, "error", err)
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	page := pagination.ParseIntParam(r.URL.Query().Get("page"), 1)
	limit := pagination.ParseIntParam(r.URL.Query().Get("limit"), 20)
	if limit > 100 {
		limit = 100
	}
	if page < 1 {
		page = 1
	}
	items, total, err := h.repo.List(r.Context(), userID, limit, (page-1)*limit)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "limit": limit})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	f, err := h.repo.Get(r.Context(), userID, chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, f)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	if err := h.repo.SoftDelete(r.Context(), userID, chi.URLParam(r, "id")); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.NoContent(w)
}

func (h *Handler) Tags(w http.ResponseWriter, _ *http.Request) {
	httputil.OK(w, domain.CuratedTags)
}
```

- [ ] **Step 4: Implement the router** — `services/fanfic/internal/transport/router.go`

```go
package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(h *handler.Handler, jwtConfig authz.JWTConfig, log *logger.Logger, mc *metrics.Collector) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(mc.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	r.Get("/health", handler.Health)
	r.Head("/health", handler.Health)
	r.Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.Handler().ServeHTTP(w, req)
	})

	r.Route("/api/fanfic", func(r chi.Router) {
		r.Use(AuthMiddleware(jwtConfig))
		r.Post("/generate", h.Generate)
		r.Get("/", h.List)
		r.Get("/tags", h.Tags) // before /{id}
		r.Get("/{id}", h.Get)
		r.Delete("/{id}", h.Delete)
	})

	return r
}

// AuthMiddleware validates the JWT and puts claims on the context (project convention).
func AuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	jwtManager := authz.NewJWTManager(jwtConfig)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := httputil.BearerToken(r)
			if token == "" {
				httputil.Unauthorized(w)
				return
			}
			claims, err := jwtManager.ValidateAccessToken(token)
			if err != nil {
				httputil.Unauthorized(w)
				return
			}
			ctx := authz.ContextWithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 5: Run handler tests**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./internal/handler/... -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
cd /data/ae-fanfic-engine
git add services/fanfic/internal/handler/ services/fanfic/internal/transport/
git commit -m "feat(fanfic): SSE generate handler + library handlers + JWT router

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 7: `main.go` wiring, Dockerfile, docker-compose, `.env`, patch all Dockerfiles

**Files:**
- Create: `services/fanfic/cmd/fanfic-api/main.go`
- Create: `services/fanfic/Dockerfile`
- Modify: `docker/docker-compose.yml` (new `fanfic` service block + gateway env — gateway env in Task 8)
- Modify: `docker/.env` (add `FANFIC_GROQ_API_KEY`) — host-only, do NOT commit
- Modify: **every** `services/*/Dockerfile` (add the `services/fanfic/go.mod` COPY line — the go.work gotcha)

**Interfaces:**
- Consumes everything from Tasks 1–6. Produces a running container serving `/health` on `:8097`.

- [ ] **Step 1: Write `main.go`** — `services/fanfic/cmd/fanfic-api/main.go`

```go
// Package main is the fanfic service entrypoint (port 8097) — an admin-only
// AI fanfiction generator backed by Groq. Mirrors the gacha boot sequence.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	gormtrace "github.com/ILITA-hub/animeenigma/libs/tracing/gormtrace"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/config"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/groq"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/service"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	tracer, err := tracing.InitFromEnv(context.Background(), "fanfic")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()
	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
		log.Warnw("gorm tracing disabled", "error", err)
	}
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}
	if err := db.AutoMigrate(&domain.Fanfic{}); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	redis, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redis.Close()

	groqClient := groq.New(cfg.Groq.APIKey, cfg.Groq.BaseURL, cfg.Groq.Model, cfg.Groq.Timeout)
	fanficRepo := repo.NewRepository(db.DB)
	quota := service.NewQuota(newRedisQuotaStore(redis), cfg.DailyCap, time.Now)
	generator := service.NewGenerator(groqClient, fanficRepo, quota, cfg.Groq.Model, log)
	h := handler.NewHandler(generator, fanficRepo, log)

	mc := metrics.NewCollector("fanfic")
	router := transport.NewRouter(h, cfg.JWT, log, mc)

	srv := &http.Server{
		Addr:        cfg.Server.Address(),
		Handler:     tracing.HTTPMiddleware("fanfic")(router),
		ReadTimeout: 15 * time.Second,
		// WriteTimeout MUST be 0 — SSE responses are long-lived and a non-zero
		// write deadline would truncate the stream.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting fanfic service", "address", cfg.Server.Address(), "model", cfg.Groq.Model)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Infow("fanfic service stopped")
}

// redisQuotaStore adapts *cache.RedisCache to the service.quotaStore interface.
type redisQuotaStore struct{ rc *cache.RedisCache }

func newRedisQuotaStore(rc *cache.RedisCache) *redisQuotaStore { return &redisQuotaStore{rc: rc} }

func (s *redisQuotaStore) Incr(ctx context.Context, key string) (int64, error) {
	return s.rc.Client().Incr(ctx, key).Result()
}
func (s *redisQuotaStore) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return s.rc.Client().Expire(ctx, key, ttl).Err()
}
func (s *redisQuotaStore) SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return s.rc.Client().SetNX(ctx, key, "1", ttl).Result()
}
func (s *redisQuotaStore) Del(ctx context.Context, key string) error {
	return s.rc.Client().Del(ctx, key).Err()
}
```

- [ ] **Step 2: Build the binary locally**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go build ./... && go vet ./...`
Expected: no errors. Fix any signature drift.

- [ ] **Step 3: Create the Dockerfile** — copy `services/gacha/Dockerfile` to `services/fanfic/Dockerfile`, then: (a) add `COPY services/fanfic/go.mod services/fanfic/go.sum* ./services/fanfic/` to the module-COPY block, (b) replace every `gacha` token with `fanfic` (build dir, `-o /fanfic-api`, `./cmd/fanfic-api`, `COPY --from=builder /fanfic-api .`), (c) `EXPOSE 8097`. The final file:

```dockerfile
# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git

COPY go.work go.work.sum ./
COPY libs/logger/go.mod libs/logger/go.sum* ./libs/logger/
COPY libs/tracing/go.mod libs/tracing/go.sum* ./libs/tracing/
COPY libs/errors/go.mod libs/errors/go.sum* ./libs/errors/
COPY libs/cache/go.mod libs/cache/go.sum* ./libs/cache/
COPY libs/database/go.mod libs/database/go.sum* ./libs/database/
COPY libs/authz/go.mod libs/authz/go.sum* ./libs/authz/
COPY libs/httputil/go.mod libs/httputil/go.sum* ./libs/httputil/
COPY libs/pagination/go.mod libs/pagination/go.sum* ./libs/pagination/
COPY libs/streamprobe/go.mod libs/streamprobe/go.sum* ./libs/streamprobe/
COPY libs/animeparser/go.mod libs/animeparser/go.sum* ./libs/animeparser/
COPY libs/videoutils/go.mod libs/videoutils/go.sum* ./libs/videoutils/
COPY libs/idmapping/go.mod libs/idmapping/go.sum* ./libs/idmapping/
COPY libs/kodikextract/go.mod libs/kodikextract/go.sum* ./libs/kodikextract/
COPY libs/metrics/go.mod libs/metrics/go.sum* ./libs/metrics/
COPY services/auth/go.mod services/auth/go.sum* ./services/auth/
COPY services/upscaler/go.mod services/upscaler/go.sum* ./services/upscaler/
COPY services/catalog/go.mod services/catalog/go.sum* ./services/catalog/
COPY services/streaming/go.mod services/streaming/go.sum* ./services/streaming/
COPY services/player/go.mod services/player/go.sum* ./services/player/
COPY services/rooms/go.mod services/rooms/go.sum* ./services/rooms/
COPY services/scraper/go.mod services/scraper/go.sum* ./services/scraper/
COPY services/scheduler/go.mod services/scheduler/go.sum* ./services/scheduler/
COPY services/gateway/go.mod services/gateway/go.sum* ./services/gateway/
COPY services/themes/go.mod services/themes/go.sum* ./services/themes/
COPY services/notifications/go.mod services/notifications/go.sum* ./services/notifications/
COPY services/watch-together/go.mod services/watch-together/go.sum* ./services/watch-together/
COPY services/analytics/go.mod services/analytics/go.sum* ./services/analytics/
COPY services/maintenance/go.mod services/maintenance/go.sum* ./services/maintenance/
COPY services/library/go.mod services/library/go.sum* ./services/library/
COPY services/gacha/go.mod services/gacha/go.sum* ./services/gacha/
COPY services/recs/go.mod services/recs/go.sum* ./services/recs/
COPY services/anidle/go.mod services/anidle/go.sum* ./services/anidle/
COPY services/fanfic/go.mod services/fanfic/go.sum* ./services/fanfic/

RUN cd services/fanfic && go mod download

COPY libs/ ./libs/
COPY services/fanfic/ ./services/fanfic/

RUN cd services/fanfic && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /fanfic-api ./cmd/fanfic-api

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app
COPY --from=builder /fanfic-api .
RUN addgroup -S app && adduser -S -G app app && chown -R app:app /app
USER app
EXPOSE 8097
CMD ["./fanfic-api"]
```

- [ ] **Step 4: Patch EVERY other service Dockerfile (go.work gotcha)** — add the fanfic go.mod COPY line right after the anidle line, into all service Dockerfiles that carry the module-COPY block.

```bash
cd /data/ae-fanfic-engine
for df in services/*/Dockerfile; do
  # skip the fanfic Dockerfile itself and any that don't COPY the anidle go.mod
  if [ "$df" = "services/fanfic/Dockerfile" ]; then continue; fi
  if grep -q 'COPY services/anidle/go.mod' "$df" && ! grep -q 'COPY services/fanfic/go.mod' "$df"; then
    sed -i 's#\(COPY services/anidle/go.mod services/anidle/go.sum\* ./services/anidle/\)#\1\nCOPY services/fanfic/go.mod services/fanfic/go.sum* ./services/fanfic/#' "$df"
    echo "patched $df"
  fi
done
# Verify: every service Dockerfile now references fanfic go.mod (except any that never listed the others).
grep -L 'COPY services/fanfic/go.mod' services/*/Dockerfile
```
Expected: the `grep -L` prints only `services/fanfic/Dockerfile` (and any sidecar Dockerfile that legitimately doesn't use go.work — e.g. `stealth-scraper`, `animepahe-resolver`, if present). Inspect anything unexpected.

- [ ] **Step 5: Add the compose service block** — in `docker/docker-compose.yml`, after the `gacha:` block, add:

```yaml
  # Fanfic engine (spec 2026-07-06) — admin-only AI fanfiction generator on
  # port 8097 (next free after upscaler:8096). Groq-backed; dark-shipped via
  # FANFIC_ADMIN_ONLY at the gateway + VITE_FANFIC_ADMIN_ONLY on the web.
  fanfic:
    logging: *default-logging
    build:
      context: ..
      dockerfile: services/fanfic/Dockerfile
    container_name: animeenigma-fanfic
    mem_limit: 256m
    restart: unless-stopped
    environment:
      SERVER_PORT: 8097
      DB_HOST: postgres
      DB_PORT: 5432
      DB_USER: ${DB_USER:-postgres}
      DB_PASSWORD: ${DB_PASSWORD:-postgres}
      DB_NAME: ${DB_NAME:-animeenigma}
      JWT_SECRET: ${JWT_SECRET:-dev-secret-change-in-production}
      REDIS_HOST: redis
      FANFIC_GROQ_API_KEY: ${FANFIC_GROQ_API_KEY}
      FANFIC_GROQ_MODEL: ${FANFIC_GROQ_MODEL:-llama-3.1-8b-instant}
      FANFIC_DAILY_CAP: "${FANFIC_DAILY_CAP:-100}"
      TRACING_ENABLED: "true"
    ports:
      - "127.0.0.1:8097:8097"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8097/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

- [ ] **Step 6: Add the secret to `docker/.env`** (host-only, NEVER committed):

```bash
cd /data/ae-fanfic-engine
grep -q '^FANFIC_GROQ_API_KEY=' docker/.env || echo 'FANFIC_GROQ_API_KEY=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' >> docker/.env
```

- [ ] **Step 7: Build the container image**

Run: `cd /data/ae-fanfic-engine && docker compose -f docker/docker-compose.yml build fanfic 2>&1 | tail -20`
Expected: image builds clean. (Also sanity-build one already-existing service, e.g. `docker compose -f docker/docker-compose.yml build anidle`, to confirm the Dockerfile patch didn't break the go.work resolution.)

- [ ] **Step 8: Commit** (Dockerfiles + compose + main.go; `.env` is git-ignored)

```bash
cd /data/ae-fanfic-engine
git add services/fanfic/cmd services/fanfic/Dockerfile services/*/Dockerfile docker/docker-compose.yml
git commit -m "feat(fanfic): service entrypoint, Dockerfile, compose block; patch all Dockerfiles for go.work

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Phase 2 — Gateway wiring

### Task 8: Gateway config, proxy, flushing SSE handler, router group (admin dark-ship gate)

**Files:**
- Modify: `services/gateway/internal/config/config.go` (add `FanficService` + `FanficAdminOnly`)
- Modify: `services/gateway/internal/service/proxy.go` (add `case "fanfic"`)
- Modify: `services/gateway/internal/handler/proxy.go` (add `ProxyToFanfic`, `ProxyToFanficStream`, `proxyStreamFlush`)
- Modify: `services/gateway/internal/transport/router.go` (add `/api/fanfic` group + `/fanfics` SPA route)
- Modify: `docker/docker-compose.yml` (gateway env: `FANFIC_SERVICE_URL`, `FANFIC_ADMIN_ONLY`)
- Test: `services/gateway/internal/config/config_test.go`

**Interfaces:**
- Consumes: the fanfic service at `FANFIC_SERVICE_URL`. Produces gateway routes `/api/fanfic/*` (admin-gated) and `/fanfics` SPA passthrough.

- [ ] **Step 1: Write the failing config test** — append to `services/gateway/internal/config/config_test.go`

```go
func TestFanficAdminOnly_DefaultsTrue(t *testing.T) {
	t.Setenv("JWT_SECRET", "x")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.FanficAdminOnly {
		t.Fatal("expected FanficAdminOnly to default true")
	}
	if cfg.Services.FanficService == "" {
		t.Fatal("expected a default FanficService URL")
	}
}
```

- [ ] **Step 2: Run it — expect failure**

Run: `cd /data/ae-fanfic-engine/services/gateway && go test ./internal/config/... -run TestFanficAdminOnly 2>&1 | head`
Expected: `cfg.FanficAdminOnly undefined`.

- [ ] **Step 3: Add config fields** — in `services/gateway/internal/config/config.go`: add `FanficService string` to the `Services` struct (near `AnidleService`), add `FanficAdminOnly bool` to `Config` (near `GachaAdminOnly`), and in `Load()` set `FanficService: getEnv("FANFIC_SERVICE_URL", "http://fanfic:8097")` and `FanficAdminOnly: getEnvBool("FANFIC_ADMIN_ONLY", true)`.

- [ ] **Step 4: Add the proxy case** — in `services/gateway/internal/service/proxy.go`, after the `case "anidle":` block:

```go
	case "fanfic":
		return s.serviceURLs.FanficService, nil
```

- [ ] **Step 5: Add the handlers** — in `services/gateway/internal/handler/proxy.go`:

```go
// ProxyToFanfic proxies non-streaming fanfic routes (list/get/delete/tags) to
// the fanfic service (spec 2026-07-06). Only /api/fanfic/* is exposed.
func (h *ProxyHandler) ProxyToFanfic(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "fanfic")
}

// ProxyToFanficStream proxies the SSE generation route with per-chunk flushing
// so token deltas reach the browser immediately (plain proxyStream buffers ~2KB
// via io.Copy, which would make streaming arrive in bursts).
func (h *ProxyHandler) ProxyToFanficStream(w http.ResponseWriter, r *http.Request) {
	h.proxyStreamFlush(w, r, "fanfic")
}

// proxyStreamFlush is proxyStream but flushes after every read so SSE events
// are delivered as they arrive. WriteDeadline is cleared (like proxyStream).
func (h *ProxyHandler) proxyStreamFlush(w http.ResponseWriter, r *http.Request, service string) {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	resp, err := h.proxyService.ForwardStream(r, service)
	if err != nil {
		h.log.Errorw("stream proxy failed", "service", service, "error", err)
		metrics.ProxyUpstreamErrors.WithLabelValues("forward_error", service).Inc()
		httputil.Error(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		metrics.ProxyUpstreamErrors.WithLabelValues(strconv.Itoa(resp.StatusCode), service).Inc()
	}
	for key, values := range resp.Header {
		if isCORSHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			_ = rc.Flush()
		}
		if rerr != nil {
			return
		}
	}
}
```

> **Implementer note:** confirm the imports already present in `proxy.go` cover `time`, `strconv`, `io` (io no longer needed here), `metrics`, `httputil`. `proxyStream` already uses `time`, `strconv`, `metrics`, `httputil`, so they're imported.

- [ ] **Step 6: Add the router group** — in `services/gateway/internal/transport/router.go`, mirror the gacha group. Near the gacha SPA routes (~line 320) add the SPA passthrough:

```go
		// Fanfic engine SPA route (/fanfics — admin-gated in the router meta;
		// dark-shipped via VITE_FANFIC_ADMIN_ONLY on the web).
		r.HandleFunc("/fanfics", proxyHandler.ProxyToWeb)
```

And in the `/api` protected area, mirror the gacha `/api/gacha` group:

```go
		// Fanfic engine (spec 2026-07-06). JWT-required, guest-blocked, and
		// admin-gated while FANFIC_ADMIN_ONLY (dark-ship). The SSE /generate
		// route uses the flushing stream proxy.
		r.Route("/fanfic", func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(BlockGuestRoleMiddleware)
			if cfg.FanficAdminOnly {
				r.Use(AdminRoleMiddleware)
			}
			r.Post("/generate", proxyHandler.ProxyToFanficStream)
			r.Get("/", proxyHandler.ProxyToFanfic)
			r.Get("/tags", proxyHandler.ProxyToFanfic)
			r.Get("/{id}", proxyHandler.ProxyToFanfic)
			r.Delete("/{id}", proxyHandler.ProxyToFanfic)
		})
```

> **Implementer note:** place this `r.Route("/fanfic", …)` inside the same parent block that hosts `r.Route("/gacha", …)` so `cfg`, `proxyHandler`, `userRateLimit`, and the middleware constructors are in scope. Match the exact names used there (`JWTValidationMiddleware`, `BlockGuestRoleMiddleware`, `AdminRoleMiddleware`, `userRateLimit`).

- [ ] **Step 7: Add gateway env to compose** — in the `gateway:` service `environment:` block in `docker/docker-compose.yml`:

```yaml
      FANFIC_SERVICE_URL: http://fanfic:8097
      FANFIC_ADMIN_ONLY: "${FANFIC_ADMIN_ONLY:-true}"
```
Also add `fanfic` to the gateway `depends_on:` list.

- [ ] **Step 8: Build + test the gateway**

Run: `cd /data/ae-fanfic-engine/services/gateway && go build ./... && go test ./internal/config/... -run TestFanficAdminOnly -v`
Expected: builds, test PASSES.

- [ ] **Step 9: Commit**

```bash
cd /data/ae-fanfic-engine
git add services/gateway/ docker/docker-compose.yml
git commit -m "feat(gateway): route /api/fanfic/* (admin dark-ship gate) + flushing SSE proxy

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Phase 3 — Frontend

### Task 9: Types, API client (incl. SSE reader), visibility gate

**Files:**
- Create: `frontend/web/src/types/fanfic.ts`
- Create: `frontend/web/src/api/fanfic.ts`
- Create: `frontend/web/src/api/__tests__/fanfic.spec.ts`
- Create: `frontend/web/src/utils/fanficGate.ts`

**Interfaces:**
- Produces: `Fanfic`, `GenerateInput`, `FanficTag` types; `fanficApi` with `generate(input, handlers)`, `list(page, limit)`, `get(id)`, `remove(id)`, `tags()`; `FANFIC_ADMIN_ONLY` + `useFanficVisible()`.

- [ ] **Step 1: Write the types** — `frontend/web/src/types/fanfic.ts`

```ts
export type FanficRating = 'teen' | 'mature' | 'explicit'
export type FanficLength = 'drabble' | 'oneshot' | 'short'
export type FanficPOV = 'first' | 'third'
export type FanficLang = 'ru' | 'en'

export interface FanficCharacterRef {
  id?: string
  name: string
}

export interface FanficAnimeRef {
  id?: string
  shikimori_id?: string
  title: string
  japanese?: string
  poster?: string
}

export interface GenerateInput {
  anime: FanficAnimeRef
  characters: FanficCharacterRef[]
  tags: string[]
  length: FanficLength
  pov: FanficPOV
  rating: FanficRating
  language: FanficLang
  prompt: string
}

export interface Fanfic {
  id: string
  anime_id: string
  anime_shikimori_id: string
  anime_title: string
  anime_japanese: string
  anime_poster: string
  characters: FanficCharacterRef[]
  tags: string[]
  length: FanficLength
  pov: FanficPOV
  rating: FanficRating
  language: FanficLang
  prompt: string
  title: string
  content: string
  model: string
  token_usage: number
  status: 'generating' | 'complete' | 'failed'
  created_at: string
}

export interface FanficTag {
  slug: string
  ru: string
  en: string
}

export interface StreamHandlers {
  onMeta?: (id: string, model: string) => void
  onDelta?: (text: string) => void
  onDone?: (id: string, title: string, tokenUsage: number) => void
  onError?: (message: string) => void
}
```

- [ ] **Step 2: Write the failing api-client test** — `frontend/web/src/api/__tests__/fanfic.spec.ts`

```ts
import { describe, it, expect, vi } from 'vitest'
import { parseSSEBuffer } from '../fanfic'

describe('parseSSEBuffer', () => {
  it('extracts complete events and returns the remainder', () => {
    const chunk =
      'event: meta\ndata: {"id":"x","model":"m"}\n\n' +
      'event: delta\ndata: {"text":"# T"}\n\n' +
      'event: delta\ndata: {"text":"partial'
    const { events, rest } = parseSSEBuffer(chunk)
    expect(events).toHaveLength(2)
    expect(events[0]).toEqual({ event: 'meta', data: { id: 'x', model: 'm' } })
    expect(events[1]).toEqual({ event: 'delta', data: { text: '# T' } })
    expect(rest).toContain('partial')
  })

  it('dispatches deltas in order via handleSSEEvent', () => {
    const onDelta = vi.fn()
    const { handleSSEEvent } = require('../fanfic')
    handleSSEEvent({ event: 'delta', data: { text: 'hi' } }, { onDelta })
    expect(onDelta).toHaveBeenCalledWith('hi')
  })
})
```

- [ ] **Step 3: Run it — expect failure**

Run: `cd /data/ae-fanfic-engine/frontend/web && bunx vitest run src/api/__tests__/fanfic.spec.ts 2>&1 | tail`
Expected: fails — `parseSSEBuffer` not exported.

- [ ] **Step 4: Write the api client** — `frontend/web/src/api/fanfic.ts`

```ts
import { apiClient } from './client'
import { useAuthStore } from '@/stores/auth'
import type { Fanfic, FanficTag, GenerateInput, StreamHandlers } from '@/types/fanfic'

export interface SSEEvent {
  event: string
  data: any
}

/** Split an SSE text buffer into complete events plus the unparsed remainder. */
export function parseSSEBuffer(buffer: string): { events: SSEEvent[]; rest: string } {
  const events: SSEEvent[] = []
  const parts = buffer.split('\n\n')
  const rest = parts.pop() ?? ''
  for (const block of parts) {
    let event = 'message'
    let data = ''
    for (const line of block.split('\n')) {
      if (line.startsWith('event: ')) event = line.slice(7).trim()
      else if (line.startsWith('data: ')) data += line.slice(6)
    }
    if (!data) continue
    try {
      events.push({ event, data: JSON.parse(data) })
    } catch {
      /* ignore malformed */
    }
  }
  return { events, rest }
}

export function handleSSEEvent(evt: SSEEvent, h: StreamHandlers): void {
  switch (evt.event) {
    case 'meta':
      h.onMeta?.(evt.data.id, evt.data.model)
      break
    case 'delta':
      h.onDelta?.(evt.data.text)
      break
    case 'done':
      h.onDone?.(evt.data.id, evt.data.title, evt.data.token_usage)
      break
    case 'error':
      h.onError?.(evt.data.message)
      break
  }
}

export const fanficApi = {
  /** Stream a generation. Uses fetch + ReadableStream to consume SSE. */
  async generate(input: GenerateInput, handlers: StreamHandlers, signal?: AbortSignal): Promise<void> {
    const auth = useAuthStore()
    const res = await fetch('/api/fanfic/generate', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${auth.accessToken}`,
      },
      body: JSON.stringify(input),
      signal,
    })
    if (!res.ok || !res.body) {
      handlers.onError?.(`HTTP ${res.status}`)
      return
    }
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    for (;;) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const { events, rest } = parseSSEBuffer(buffer)
      buffer = rest
      for (const evt of events) handleSSEEvent(evt, handlers)
    }
  },

  list: (page = 1, limit = 20) =>
    apiClient.get('/fanfic', { params: { page, limit } }) as Promise<{ items: Fanfic[]; total: number }>,
  get: (id: string) => apiClient.get(`/fanfic/${id}`) as Promise<Fanfic>,
  remove: (id: string) => apiClient.delete(`/fanfic/${id}`),
  tags: () => apiClient.get('/fanfic/tags') as Promise<FanficTag[]>,
}
```

> **Implementer note:** confirm the auth store exposes the access token (`grep -n "accessToken\|token" src/stores/auth.ts`); adjust `auth.accessToken` to the real getter. Confirm `apiClient.get/delete` unwrap the `{success,data}` envelope the same way sibling api modules do — mirror an existing module (e.g. `src/api/anidle.ts`) for the exact unwrap.

- [ ] **Step 5: Write the visibility gate** — `frontend/web/src/utils/fanficGate.ts` (mirror `gachaGate.ts`)

```ts
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'

/** True ⟹ only admins see the fanfic engine; false ⟹ every authed user. */
export const FANFIC_ADMIN_ONLY =
  (import.meta.env.VITE_FANFIC_ADMIN_ONLY as string | undefined) !== 'false'

export function useFanficVisible() {
  const authStore = useAuthStore()
  return computed(() => {
    if (FANFIC_ADMIN_ONLY) return authStore.isAdmin
    return authStore.isAuthenticated
  })
}
```

- [ ] **Step 6: Run the api test**

Run: `cd /data/ae-fanfic-engine/frontend/web && bunx vitest run src/api/__tests__/fanfic.spec.ts`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
cd /data/ae-fanfic-engine
git add frontend/web/src/types/fanfic.ts frontend/web/src/api/fanfic.ts frontend/web/src/api/__tests__/fanfic.spec.ts frontend/web/src/utils/fanficGate.ts
git commit -m "feat(fanfic-web): types, SSE-streaming api client, visibility gate

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 10: FanficsView + components, route/guard/nav, i18n, markdown reader

**Files:**
- Create: `frontend/web/src/views/FanficsView.vue`
- Create: `frontend/web/src/components/fanfic/GenerateForm.vue`
- Create: `frontend/web/src/components/fanfic/FanficReader.vue`
- Create: `frontend/web/src/components/fanfic/LibraryGrid.vue`
- Create: `frontend/web/src/components/fanfic/renderFanfic.ts`
- Create: `frontend/web/src/components/fanfic/__tests__/renderFanfic.spec.ts`
- Create: `frontend/web/src/components/fanfic/__tests__/GenerateForm.spec.ts`
- Modify: `frontend/web/src/router/index.ts` (route + `fanficGated` guard branch)
- Modify: `frontend/web/src/components/layout/Navbar.vue` (admin-gated nav link)
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (`fanfic.*`)

**Interfaces:**
- Consumes: `fanficApi`, `useFanficVisible`, `apiClient.search`, `apiClient.getAnimeCharacters`, DS `ui` primitives.

- [ ] **Step 1: Write the safe markdown reader + failing test** — `frontend/web/src/components/fanfic/__tests__/renderFanfic.spec.ts`

```ts
import { describe, it, expect } from 'vitest'
import { renderFanfic } from '../renderFanfic'

describe('renderFanfic', () => {
  it('splits prose into heading and paragraph blocks (no raw HTML)', () => {
    const blocks = renderFanfic('# Title\n\nFirst para.\n\nSecond para.')
    expect(blocks).toEqual([
      { type: 'h2', text: 'Title' },
      { type: 'p', text: 'First para.' },
      { type: 'p', text: 'Second para.' },
    ])
  })
  it('does not emit raw HTML for injected tags (XSS-safe text nodes)', () => {
    const blocks = renderFanfic('<script>alert(1)</script>')
    expect(blocks[0]).toEqual({ type: 'p', text: '<script>alert(1)</script>' })
  })
})
```

- [ ] **Step 2: Implement `renderFanfic.ts`** (tiny, dependency-free, returns typed blocks the SFC renders as TEXT — no `v-html`, so it's XSS-safe)

```ts
export type FanficBlock = { type: 'h2' | 'h3' | 'p'; text: string }

/** Minimal, safe fanfic renderer: headings (#/##) + blank-line paragraphs. */
export function renderFanfic(md: string): FanficBlock[] {
  const blocks: FanficBlock[] = []
  for (const raw of md.split(/\n{2,}/)) {
    const chunk = raw.trim()
    if (!chunk) continue
    if (chunk.startsWith('## ')) blocks.push({ type: 'h3', text: chunk.slice(3).trim() })
    else if (chunk.startsWith('# ')) blocks.push({ type: 'h2', text: chunk.slice(2).trim() })
    else blocks.push({ type: 'p', text: chunk.replace(/\n/g, ' ') })
  }
  return blocks
}
```

- [ ] **Step 3: Run the renderer test**

Run: `cd /data/ae-fanfic-engine/frontend/web && bunx vitest run src/components/fanfic/__tests__/renderFanfic.spec.ts`
Expected: PASS.

- [ ] **Step 4: Build `FanficReader.vue`** — renders `renderFanfic(content)` blocks as `<h2>/<h3>/<p>` with DS typography (selectable text). Props: `title: string`, `content: string`, `streaming?: boolean`. No `v-html`.

```vue
<script setup lang="ts">
import { computed } from 'vue'
import { renderFanfic } from './renderFanfic'

const props = defineProps<{ title?: string; content: string; streaming?: boolean }>()
const blocks = computed(() => renderFanfic(props.content))
</script>

<template>
  <article class="prose-fanfic max-w-none">
    <h1 v-if="title" class="text-2xl font-semibold text-foreground mb-4">{{ title }}</h1>
    <template v-for="(b, i) in blocks" :key="i">
      <h2 v-if="b.type === 'h2'" class="text-xl font-semibold text-foreground mt-6 mb-2">{{ b.text }}</h2>
      <h3 v-else-if="b.type === 'h3'" class="text-lg font-semibold text-foreground mt-4 mb-2">{{ b.text }}</h3>
      <p v-else class="text-muted-foreground leading-relaxed mb-3">{{ b.text }}</p>
    </template>
    <span v-if="streaming" class="inline-block w-2 h-4 bg-brand-cyan animate-pulse align-middle" aria-hidden="true" />
  </article>
</template>
```

- [ ] **Step 5: Build `GenerateForm.vue`** — the structured form. Anime search (debounced `apiClient.search`), on-pick fetch characters (`apiClient.getAnimeCharacters`) into a multi-select of chips (≤6), tag chips from `fanficApi.tags()` + custom input (≤8), `Select` for length/POV/rating/language, prompt `<textarea>`. Explicit rating shows an 18+ confirm before it can be picked. Emits `generate(input: GenerateInput)`. Use DS primitives (`Select`, `Button`, `Input`, `Badge`) — NO native `<select>`. i18n via `t('fanfic.*')`.

  Co-locate `GenerateForm.spec.ts` with ≥5 assertions (renders fields, disables generate when no anime, builds correct `GenerateInput`, enforces the ≤6/≤8 caps, gates Explicit behind confirm).

- [ ] **Step 6: Build `LibraryGrid.vue`** — grid of cards (`fanficApi.list()`): poster (`PosterImage`), title, anime, rating/tag `Badge`s, date. Emits `open(id)` and `remove(id)` (confirm dialog). Empty state.

- [ ] **Step 7: Build `FanficsView.vue`** — `Tabs`: «Генерировать» (GenerateForm + FanficReader; wires `fanficApi.generate` streaming into a reactive `content` ref, flips `streaming` on/off, on `done` refreshes the library and shows Save-confirmed + Regenerate/Copy) and «Моя библиотека» (LibraryGrid → opens FanficReader in a dialog). Page shell matches other views (container, heading).

- [ ] **Step 8: Add the route + guard branch** — in `frontend/web/src/router/index.ts`:
  - Import: `import { FANFIC_ADMIN_ONLY } from '@/utils/fanficGate'`
  - Route (near the gacha routes):
    ```ts
    {
      path: '/fanfics',
      name: 'fanfics',
      component: () => import('@/views/FanficsView.vue'),
      meta: { titleKey: 'fanfic.nav_item', requiresAuth: true, fanficGated: true }
    },
    ```
  - Guard branch (mirror the `gachaGated` branch in `router.beforeEach`):
    ```ts
    if (to.meta.fanficGated) {
      const fanficVisible = FANFIC_ADMIN_ONLY ? authStore.isAdmin : authStore.isAuthenticated
      if (!fanficVisible) {
        next({ name: 'home' })
        return
      }
    }
    ```

- [ ] **Step 9: Add the nav link** — in `frontend/web/src/components/layout/Navbar.vue`, add a `/fanfics` link (label `t('fanfic.nav_item')`) shown only when `useFanficVisible()` is true, mirroring how the gacha/anidle links are conditionally rendered.

- [ ] **Step 10: Add i18n keys** — add a `fanfic` namespace to `en.json`, `ru.json`, `ja.json` (keys must exist in all three — parity-gated). Minimum keys: `nav_item`, `title`, `tabs.generate`, `tabs.library`, `form.anime`, `form.characters`, `form.tags`, `form.length`, `form.pov`, `form.rating`, `form.language`, `form.prompt`, `form.generate`, `length.drabble|oneshot|short`, `pov.first|third`, `rating.teen|mature|explicit`, `rating.explicitConfirm`, `lang.ru|en`, `library.empty`, `library.delete`, `reader.regenerate`, `reader.copy`, `reader.saved`, `status.generating|failed`. RU is the primary copy; EN mirrors; JA may reuse EN strings where no translation exists but every key must be present.

- [ ] **Step 11: Run FE gates**

Run:
```bash
cd /data/ae-fanfic-engine/frontend/web
bunx vitest run src/components/fanfic/ src/api/__tests__/fanfic.spec.ts
bunx tsc --noEmit
```
Expected: tests PASS, no type errors. Then invoke `/frontend-verify` (DS-lint, i18n en/ru/ja parity, real `bun run build`).

- [ ] **Step 12: Commit**

```bash
cd /data/ae-fanfic-engine
git add frontend/web/src/views/FanficsView.vue frontend/web/src/components/fanfic/ frontend/web/src/router/index.ts frontend/web/src/components/layout/Navbar.vue frontend/web/src/locales/
git commit -m "feat(fanfic-web): admin-gated /fanfics view — generate form, live reader, library, i18n

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Phase 4 — Deploy & verify

### Task 11: Push, deploy, real-key smoke, after-update

- [ ] **Step 1: Full backend test sweep**

Run: `cd /data/ae-fanfic-engine/services/fanfic && go test ./... -count=1` and `cd ../gateway && go build ./...`
Expected: green.

- [ ] **Step 2: Push the branch** (per git workflow — push before deploy)

```bash
cd /data/ae-fanfic-engine
git pull --rebase origin main
git push -u origin feat/fanfic-engine
```

- [ ] **Step 3: Deploy** — ensure `docker/.env` carries `FANFIC_GROQ_API_KEY`, then:

```bash
cd /data/ae-fanfic-engine
docker compose -f docker/docker-compose.yml up -d --build fanfic
make redeploy-gateway
make redeploy-web
make health
```
Expected: `fanfic` healthy; gateway/web redeployed.

- [ ] **Step 4: Real-key smoke** — as an admin JWT, verify the SSE end-to-end (RU mature + EN teen), the library round-trip, and that a saved row persists.

```bash
# Health
curl -s http://localhost:8097/health
# (SSE requires an admin JWT; drive it from the browser at /fanfics, or:)
curl -sN -X POST http://localhost:8000/api/fanfic/generate \
  -H "Authorization: Bearer <ADMIN_JWT>" -H "Content-Type: application/json" \
  -d '{"anime":{"title":"Frieren","japanese":"葬送のフリーレン"},"characters":[{"name":"Frieren"}],"tags":["slow-burn"],"length":"drabble","pov":"third","rating":"mature","language":"ru","prompt":"тихий вечер у костра"}' | head -40
```
Expected: `event: meta` → `event: delta` (streaming) → `event: done`; `GET /api/fanfic` then lists the saved fanfic.

- [ ] **Step 5: Manual browser smoke** — open `/fanfics` as admin: pick an anime, characters populate, generate streams live into the reader, it appears in the library, delete works. Confirm a non-admin does NOT see the nav item or the route (redirects home).

- [ ] **Step 6: After-update** — invoke `/animeenigma-after-update` (runs `/simplify` over the diff, lint/build, redeploys changed services, writes the Russian Trump-mode changelog entry, commits + pushes). Then triage the feedback report:

```bash
cd /data/ae-fanfic-engine
bin/feedback-status 2026-07-04T07-29-22_tNeymik_manual ai_done
```

- [ ] **Step 7: Clean up the worktree** (only after after-update is green)

```bash
cd /data/animeenigma
git worktree remove /data/ae-fanfic-engine
git worktree prune
```

---

## Self-Review

**Spec coverage:** §2 service → Tasks 1–7. §3 data model → Task 1/4. §4 API (SSE + list/get/delete/tags) → Tasks 5/6, gateway Task 8. §5 pipeline/prompt → Tasks 3/5. §6 Groq client → Task 2. §7 frontend → Tasks 9/10. §8 gateway (incl. SSE flush) → Task 8. §9 config/secrets → Tasks 1/7/8. §10 quota → Task 5. §11 observability → `metrics.NewCollector` (Task 6 router) + structured logs (custom `fanfic_*` counters are a nice-to-have; add in Task 6 if time permits, otherwise the standard `http_*` histograms satisfy the spec's minimum). §12 v2 backlog → already filed. §13 testing → each task's TDD steps. §14 file manifest → matches. All spec sections map to a task.

**Placeholder scan:** no TBD/TODO; every code step carries real code. Two explicit "implementer note" verification points (testcontainers helper name in Task 4; auth-token getter + envelope unwrap in Task 9; `authz.Claims` field in Task 6) are grounded checks, not placeholders.

**Type consistency:** `Emit = func(event string, data any) error` is consistent across generator (Task 5) and handler (Task 6). `streamer.Stream(...)` signature matches `groq.Client.Stream(...)` (Task 2). `fanficStore`/`libraryStore` method sets match `repo.Repository` (Task 4). `GenerateInput`/`GenerateRequest` field names align (snake_case JSON on both sides). `parseSSEBuffer`/`handleSSEEvent` names match between client (Task 9) and test.

---

## Effort metrics (per `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — net-new admin creative surface; existing flows untouched (dark-shipped).
- **CDI = 0.06 * 34** — moderate spread (new service + gateway + FE + compose), low shift (additive, mirrors anidle/gacha), Effort 34.
- **MVQ = Griffin 85%/80%** — proven composite (service scaffold + LLM egress + SSE) from a well-worn template; low slop risk.
