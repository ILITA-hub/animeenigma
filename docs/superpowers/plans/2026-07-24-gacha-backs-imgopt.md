# Gacha Bulk Card-Back + Image Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bulk-set the card back («рубашка») from the bulk actions bar, and make card images cheap: recompress on ingest, serve grid thumbnails through the existing streaming image-proxy, immutable caching.

**Architecture:** Spec `docs/superpowers/specs/2026-07-24-gacha-backs-imgopt-design.md`. Three services touched: gacha (BulkCardSet.back_path, ingest recompression, cache header), streaming (image-proxy accepts relative gacha URLs), web (Рубашка bulk dialog, proxied thumbnails).

**Tech Stack:** Go (image/jpeg, image/png, x/image/draw), Vue 3 + vitest.

## Global Constraints

- Worktree: `/tmp/ae-gacha-backs` (branch `gacha-backs-imgopt`). ALL edits there.
- Commits: conventional, explicit `git add <paths>` for new files, pathspec commits, three co-author trailers (Claude Code <noreply@anthropic.com>, 0neymik0 <0neymik0@gmail.com>, NANDIorg <super.egor.mamonov@yandex.ru>).
- i18n: every new key in en.json + ru.json + ja.json.
- Frontend: bun/bunx only; DS-lint hook must stay green; Input/Select/Checkbox primitives, no bare form controls.
- Never fail an upload because optimization failed — decode errors passthrough original bytes.
- `back_path: ""` is valid (reset to default back); never strip it from a request.

---

### Task 1: gacha BE — BulkCardSet.back_path

**Files:** Modify `services/gacha/internal/repo/card.go`, `services/gacha/internal/service/content.go`; test `services/gacha/internal/service/content_test.go`.

**Interfaces:** Produces `repo.CardBulkSet.BackPath *string`, `service.BulkCardSet.BackPath *string` (`json:"back_path"`). Consumed by Task 4 FE.

- [ ] Step 1 (test first): extend `TestBulkUpdateCards_ValidationAndApply` in content_test.go — after the existing valid-update block append:

```go
	// back_path: set on both, then reset to "" (empty = default branded back).
	back := "cards/back.webp"
	if _, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{
		IDs: []string{c1.ID}, Set: BulkCardSet{BackPath: &back},
	}); err != nil {
		t.Fatalf("set back_path: %v", err)
	}
	got, err = svc.GetCard(ctx, c1.ID)
	if err != nil {
		t.Fatalf("get c1 after back_path: %v", err)
	}
	if got.BackPath != back {
		t.Errorf("back_path not applied: %q", got.BackPath)
	}
	blank2 := ""
	if _, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{
		IDs: []string{c1.ID}, Set: BulkCardSet{BackPath: &blank2},
	}); err != nil {
		t.Fatalf("reset back_path: %v", err)
	}
	got, err = svc.GetCard(ctx, c1.ID)
	if err != nil {
		t.Fatalf("get c1 after reset: %v", err)
	}
	if got.BackPath != "" {
		t.Errorf("back_path must reset to empty, got %q", got.BackPath)
	}
```
(Adjust `got, err =` vs `:=` to fit surrounding code.)

- [ ] Step 2: run `go test ./internal/service/ -run TestBulkUpdateCards -v` → compile FAIL (BackPath undefined).
- [ ] Step 3: add `BackPath *string` to `repo.CardBulkSet`; in `BulkUpdateCards` add `if set.BackPath != nil { updates["back_path"] = *set.BackPath }`. Add `BackPath *string \`json:"back_path"\`` to `service.BulkCardSet`; include in the "at least one field" emptiness check and in the repo.CardBulkSet literal. NO validation on value (empty allowed, like SourceTitle).
- [ ] Step 4: `go test ./...` → all ok.
- [ ] Step 5: pathspec commit `feat(gacha): bulk back_path (card back) in bulk update`.

---

### Task 2: gacha BE — ingest recompression + immutable cache header

**Files:** Modify `services/gacha/internal/service/images.go`, `services/gacha/internal/handler/images.go`, `services/gacha/go.mod` (+go.sum); test `services/gacha/internal/service/images_test.go` (append).

**Interfaces:** internal only. Both `IngestFromURL` and `IngestUpload` call the new `optimize` right before `makeKey`+`Upload`.

- [ ] Step 1 (tests first) — append to images_test.go (mirror its existing fake-store setup; generate test images in-code):

```go
// helper: encode an in-memory image
func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestOptimize_OpaquePNGBecomesJPEG(t *testing.T) {
	// 100x100 solid color, fully opaque
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{200, 30, 30, 255}}, image.Point{}, draw.Src)
	out, ct, ext := optimize(encodePNG(t, img), "image/png")
	if ct != "image/jpeg" || ext != ".jpg" {
		t.Errorf("opaque png must become jpeg, got ct=%q ext=%q", ct, ext)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("output not decodable jpeg: %v", err)
	}
}

func TestOptimize_AlphaPNGStaysPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 50, 50)) // zero-value = transparent
	in := encodePNG(t, img)
	out, ct, ext := optimize(in, "image/png")
	if ct != "image/png" || ext != ".png" {
		t.Errorf("alpha png must stay png, got ct=%q ext=%q", ct, ext)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("small alpha png must pass through unchanged")
	}
}

func TestOptimize_DownscalesOversized(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3000, 1500))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{10, 20, 30, 255}}, image.Point{}, draw.Src)
	out, ct, _ := optimize(encodePNG(t, img), "image/png")
	if ct != "image/jpeg" {
		t.Fatalf("expected jpeg, got %q", ct)
	}
	dec, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if w := dec.Bounds().Dx(); w != 2048 {
		t.Errorf("longest side must be 2048, got %d", w)
	}
}

func TestOptimize_PassthroughGifWebpGarbage(t *testing.T) {
	for _, tc := range []struct{ ct string }{{"image/gif"}, {"image/webp"}} {
		in := []byte("not-an-image")
		out, ct, ext := optimize(in, tc.ct)
		if !bytes.Equal(out, in) || ct != tc.ct || ext == ".jpg" {
			t.Errorf("%s must pass through untouched", tc.ct)
		}
	}
	in := []byte{0x89, 0x50, 0x4E, 0x47, 0xDE, 0xAD} // broken png
	out, ct, _ := optimize(in, "image/png")
	if !bytes.Equal(out, in) || ct != "image/png" {
		t.Errorf("undecodable bytes must pass through")
	}
}
```
Imports needed: `image`, `image/color`, `image/jpeg`, `image/png`, `draw "image/draw"` (alias stdlib draw; the production code uses x/image/draw separately).

- [ ] Step 2: run → compile FAIL (`optimize` undefined).
- [ ] Step 3: `cd services/gacha && go get golang.org/x/image@latest && go mod tidy`. Implement in images.go:

```go
const (
	maxDimension = 2048 // longest side after ingest; larger uploads are downscaled
	jpegQuality  = 85
)

// optimize recompresses ingested art: oversized images are downscaled to fit
// maxDimension, fully-opaque images become JPEG q85 (anime art PNGs are
// typically 8-10x smaller as JPEG), transparent ones stay PNG (JPEG has no
// alpha). GIF (animation) and WebP (no stdlib encoder) pass through, as does
// anything that fails to decode — optimization must never fail an upload.
// Returns (bytes, contentType, ext).
func optimize(buf []byte, ct string) ([]byte, string, string) {
	if ct != "image/png" && ct != "image/jpeg" {
		return buf, ct, extForCT(ct)
	}
	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return buf, ct, extForCT(ct)
	}

	b := img.Bounds()
	longest := max(b.Dx(), b.Dy())
	scaled := false
	if longest > maxDimension {
		scale := float64(maxDimension) / float64(longest)
		dst := image.NewRGBA(image.Rect(0, 0,
			int(float64(b.Dx())*scale+0.5), int(float64(b.Dy())*scale+0.5)))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
		img = dst
		scaled = true
	}

	if isOpaque(img) {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality}); err == nil &&
			(scaled || out.Len() < len(buf)) {
			return out.Bytes(), "image/jpeg", ".jpg"
		}
		return buf, ct, extForCT(ct)
	}

	// Transparency: only worth re-encoding when we actually downscaled.
	if scaled {
		var out bytes.Buffer
		if err := png.Encode(&out, img); err == nil && out.Len() < len(buf) {
			return out.Bytes(), "image/png", ".png"
		}
	}
	return buf, ct, extForCT(ct)
}

// isOpaque reports whether every pixel has full alpha.
func isOpaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return false
			}
		}
	}
	return true
}

func extForCT(ct string) string {
	if ext, ok := allowedContentTypes[ct]; ok {
		return ext
	}
	return ""
}
```
Imports: `image`, `image/jpeg`, `image/png`, `_ "image/gif"` not needed (gif passes through before decode), `xdraw "golang.org/x/image/draw"`. Note `image.NewRGBA` produces a type whose `Opaque()` exists — the interface assertion covers the common decoded types (RGBA/NRGBA/YCbCr).

Hook it in BOTH paths, replacing the direct upload of `buf`/`ct`:
- `IngestFromURL`: after the size check → `buf2, ct2, ext2 := optimize(buf, ct)` where `ct` is the normalized MIME (strip params first via resolveExt's ct — use the same trimmed value the ext came from); use `makeKey(kind, ext2)` + upload `buf2` with `ct2`. Keep the old `ext` derivation for the fallback path inside optimize (extForCT).
- `IngestUpload`: same, after the ct-normalisation block.

- [ ] Step 4: handler `images.go` Serve — change `Cache-Control` to `public, max-age=31536000, immutable`.
- [ ] Step 5: `go test ./...` → all ok. Confirm `go.mod` gained `golang.org/x/image` (direct).
- [ ] Step 6: pathspec commit (incl. go.mod go.sum) `feat(gacha): recompress card art on ingest + immutable image cache`.

---

### Task 3: streaming BE — image-proxy accepts relative gacha URLs

**Files:** Modify `services/streaming/internal/service/image_proxy.go`, `services/streaming/internal/config/config.go` (+ its Config struct + DI in cmd/main where the service is constructed); test alongside existing image-proxy tests.

**Interfaces:** Produces: `GET /api/streaming/image-proxy?url=/api/gacha/images/cards/<key>&w=<n>` works. Consumed by Task 5 FE.

- [ ] Step 1 (test first) — in the image-proxy test file (find it; if none exists create `image_proxy_gacha_test.go` in the service package):

```go
func TestRewriteGachaURL(t *testing.T) {
	s := &ImageProxyService{gachaBaseURL: "http://gacha:8093"}
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"/api/gacha/images/cards/ab-1.png", "http://gacha:8093/api/gacha/images/cards/ab-1.png", true},
		{"/api/gacha/images/banners/x.webp", "http://gacha:8093/api/gacha/images/banners/x.webp", true},
		{"/api/gacha/images/cards/../secret", "", false},
		{"/api/gacha/images/other/x.png", "", false},
		{"https://shikimori.one/x.png", "", false},
		{"/api/streaming/whatever", "", false},
	}
	for _, c := range cases {
		got, ok := s.rewriteGachaURL(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("rewriteGachaURL(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
```

- [ ] Step 2: run → FAIL. Step 3: implement:

```go
// gachaImagePattern matches the relative gacha art URLs the frontend sends for
// resizing. Mirrors the key validation in services/gacha handler/images.go.
var gachaImagePattern = regexp.MustCompile(`^/api/gacha/images/((?:cards|banners)/[A-Za-z0-9._-]+)$`)

// rewriteGachaURL maps a relative gacha image URL onto the internal gacha
// service base. SSRF-safe: fixed base + strict key regex (no traversal chars).
func (s *ImageProxyService) rewriteGachaURL(rawURL string) (string, bool) {
	m := gachaImagePattern.FindStringSubmatch(rawURL)
	if m == nil {
		return "", false
	}
	return s.gachaBaseURL + "/api/gacha/images/" + m[1], true
}
```

Struct: add `gachaBaseURL string` field; constructor `NewImageProxyService` gains a `gachaBaseURL string` param (update the call site in cmd/streaming-api or wherever DI happens). Config: `GACHA_INTERNAL_URL` default `http://gacha:8093` (follow existing getEnv pattern; thread through to the constructor).

In `GetImage`, replace the allowlist gate:
```go
	if rewritten, ok := s.rewriteGachaURL(rawURL); ok {
		rawURL = rewritten
	} else if !s.isDomainAllowed(rawURL) {
		return nil, fmt.Errorf("domain not allowed")
	}
```
(Cache key derives from the REWRITTEN rawURL — stable since base is fixed config.)

- [ ] Step 4: `cd services/streaming && go test ./...` → ok (existing proxy tests must not regress; if the constructor signature change breaks tests, update those call sites too).
- [ ] Step 5: pathspec commit `feat(streaming): image-proxy resizes gacha card art (relative /api/gacha/images URLs)`.

---

### Task 4: FE — «Рубашка» bulk dialog

**Files:** Modify `frontend/web/src/api/gacha.ts` (BulkCardSet + back_path), `frontend/web/src/views/admin/AdminGacha.vue`, locales en/ru/ja; test `frontend/web/src/views/admin/AdminGacha.spec.ts`.

**Interfaces:** Consumes `bulkUpdateCards`. Produces: bulk bar button + dialog.

- [ ] Step 1: `gacha.ts` — add `back_path?: string` to `BulkCardSet`.
- [ ] Step 2: i18n keys (en/ru/ja, under gacha.admin):
  - en: `"bulk_back_btn": "Card back"`, `"bulk_back_title": "Bulk card back"`, `"bulk_back_hint": "Upload one back image — it will be applied to all selected cards. Reset returns the branded default."`, `"bulk_back_apply": "Apply to {n}"`, `"bulk_back_reset": "Reset to default"`
  - ru: `"bulk_back_btn": "Рубашка"`, `"bulk_back_title": "Массовая рубашка"`, `"bulk_back_hint": "Загрузите одну рубашку — она применится ко всем выбранным карточкам. Сброс вернёт фирменную ◆."`, `"bulk_back_apply": "Применить ({n})"`, `"bulk_back_reset": "Сбросить на дефолт"`
  - ja: `"bulk_back_btn": "カード裏面"`, `"bulk_back_title": "裏面の一括設定"`, `"bulk_back_hint": "裏面画像を1枚アップロードすると、選択したすべてのカードに適用されます。リセットでデフォルトの◆に戻ります。"`, `"bulk_back_apply": "適用 ({n})"`, `"bulk_back_reset": "デフォルトに戻す"`
- [ ] Step 3: AdminGacha.vue — bulk bar gains (before the delete button):
```html
            <Button size="sm" variant="outline" :disabled="bulkBusy" data-testid="bulk-back-btn" @click="showBulkBack = true">
              {{ $t('gacha.admin.bulk_back_btn') }}
            </Button>
```
New Modal at template end (next to the bulk-upload dialog), mirroring the card dialog's back-upload slot idiom (file input + URL input + preview):
```html
  <Modal v-model="showBulkBack" :title="$t('gacha.admin.bulk_back_title')" closable>
    <p class="text-white/70 text-sm mb-3">{{ $t('gacha.admin.bulk_back_hint') }}</p>
    <div class="flex items-start gap-3">
      <img v-if="bulkBackPreview" :src="bulkBackPreview" alt="" class="w-16 h-20 object-cover rounded border border-white/20" />
      <div class="flex-1 space-y-2">
        <input ref="bulkBackFileInput" type="file" accept="image/*" class="hidden" @change="onBulkBackFile" />
        <Button size="sm" variant="outline" :disabled="bulkBackUploading" @click="bulkBackFileInput?.click()">
          <Upload class="size-4 mr-1" /> {{ $t('gacha.admin.card_image_or') }}
        </Button>
        <Input v-model="bulkBackUrl" :placeholder="$t('gacha.admin.card_image_url_placeholder')" @blur="onBulkBackUrl" />
        <Alert v-if="bulkBackError" variant="destructive">{{ $t('gacha.admin.bulk_back_title') }}</Alert>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <Button variant="outline" :disabled="bulkBusy" data-testid="bulk-back-reset" @click="applyBulkBack('')">
          {{ $t('gacha.admin.bulk_back_reset') }}
        </Button>
        <Button :disabled="!bulkBackPath || bulkBackUploading || bulkBusy" data-testid="bulk-back-apply" @click="applyBulkBack(bulkBackPath)">
          {{ $t('gacha.admin.bulk_back_apply', { n: selectedIds.size }) }}
        </Button>
      </div>
    </template>
  </Modal>
```
Script (mirror onBackFileChange/onBackUrlBlur idiom already in the file):
```ts
// ── Bulk card-back dialog ─────────────────────────────────────────────────────
const showBulkBack = ref(false)
const bulkBackPath = ref('')
const bulkBackUrl = ref('')
const bulkBackPreview = ref<string | null>(null)
const bulkBackUploading = ref(false)
const bulkBackError = ref(false)
const bulkBackFileInput = ref<HTMLInputElement | null>(null)

watch(showBulkBack, open => {
  if (open) {
    bulkBackPath.value = ''
    bulkBackUrl.value = ''
    bulkBackPreview.value = null
    bulkBackError.value = false
  }
})

async function onBulkBackFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  bulkBackUploading.value = true
  bulkBackError.value = false
  try {
    const res = await gachaAdminApi.uploadFile(file, 'cards')
    const path = res.data.data?.image_path ?? ''
    bulkBackPath.value = path
    bulkBackPreview.value = path ? cardImageUrl(path) : null
  } catch {
    bulkBackError.value = true
  } finally {
    bulkBackUploading.value = false
  }
}

async function onBulkBackUrl() {
  const url = bulkBackUrl.value.trim()
  if (!url) return
  bulkBackUploading.value = true
  bulkBackError.value = false
  try {
    const res = await gachaAdminApi.uploadUrl(url, 'cards')
    const path = res.data.data?.image_path ?? ''
    bulkBackPath.value = path
    bulkBackPreview.value = path ? cardImageUrl(path) : url
  } catch {
    bulkBackError.value = true
  } finally {
    bulkBackUploading.value = false
  }
}

async function applyBulkBack(path: string) {
  await applyBulk({ back_path: path })
  showBulkBack.value = false
}
```
(Match the upload-response access style actually present in the file after the simplify pass — check first.)
- [ ] Step 4 (spec): add test — select a row, open dialog via `bulk-back-btn`, mock uploadFile, call `onBulkBackFile`-equivalent through the file input or vm, click `bulk-back-apply`, assert `bulkUpdateCards(['c1'], { back_path: 'cards/back.png' })`; second test: `bulk-back-reset` → `bulkUpdateCards(['c1'], { back_path: '' })`. Mirror existing mount/stub idioms.
- [ ] Step 5: `bunx vitest run src/views/admin/AdminGacha.spec.ts` + `bunx vue-tsc --noEmit` → green.
- [ ] Step 6: pathspec commit `feat(web): bulk card-back dialog in gacha admin`.

---

### Task 5: FE — proxied thumbnails on gacha surfaces

**Files:** Modify `frontend/web/src/composables/useImageProxy.ts`; surfaces: `src/views/admin/AdminGacha.vue`, `src/components/admin/GachaCardPicker.vue`, `src/components/gacha/DropsModal.vue`, `src/components/profile/GachaCollection.vue`, `src/components/profile/showcase/blocks/CardCollectionBlock.vue`, `src/components/gacha/PullSummary.vue`, `src/components/home/spotlight/cards/GachaPromoCard.vue`, `src/components/gacha/GachaSlider.vue`. NOT `CardViewer3D.vue` (originals stay). Tests: `src/composables/__tests__/useImageProxy.spec.ts` (or create), touched-surface specs stay green.

**Interfaces:** Consumes Task 3 proxy behavior + existing `cardPosterUrl`.

- [ ] Step 1 (unit test first) for the helper:
```ts
it('routes relative gacha image URLs through the proxy', () => {
  expect(cardPosterUrl('/api/gacha/images/cards/x.png', 128))
    .toBe('/api/streaming/image-proxy?url=' + encodeURIComponent('/api/gacha/images/cards/x.png') + '&w=128')
})
it('passes non-proxyable absolute URLs through', () => {
  expect(cardPosterUrl('https://example.com/a.png', 128)).toBe('https://example.com/a.png')
})
```
- [ ] Step 2: `isProxyableUrl` in useImageProxy.ts — add before the `try`:
```ts
  if (url.startsWith('/api/gacha/images/')) return true
```
- [ ] Step 3: switch surfaces — each currently binds `cardImageUrl(...)` (or a local equivalent) into an `<img :src>`; wrap with `cardPosterUrl(..., w)`:
  AdminGacha table thumb + GachaCardPicker → w=128; DropsModal, GachaCollection, CardCollectionBlock → w=256; PullSummary, GachaPromoCard → w=384; GachaSlider backdrop → w=640. Import `cardPosterUrl` from `@/composables/useImageProxy` in each. Inspect each file for the exact binding — some may use computed helpers; keep changes minimal (wrap at the binding site or the helper's return).
- [ ] Step 4: `bunx vitest run` on the touched spec files (all surfaces that have specs) + `bunx vue-tsc --noEmit` → green.
- [ ] Step 5: pathspec commit `perf(web): gacha art thumbnails via streaming image-proxy`.

---

### Task 6: gates + ship

- [ ] `cd services/gacha && go test ./...`; `cd services/streaming && go test ./...`
- [ ] `frontend/web`: `bunx vitest run` (full), `bunx vue-tsc --noEmit`, DS-lint, i18n-lint, `bun run build`
- [ ] Live smoke after deploy: `curl -sI 'https://animeenigma.org/api/streaming/image-proxy?url=%2Fapi%2Fgacha%2Fimages%2Fcards%2F<real-key>&w=256'` → 200 image/jpeg, small; original still 200 with immutable header.
- [ ] Merge (pull-rebase-push), `/animeenigma-after-update` (redeploy gacha, streaming, web), worktree cleanup.
