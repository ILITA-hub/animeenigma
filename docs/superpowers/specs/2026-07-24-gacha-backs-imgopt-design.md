# Gacha: массовая рубашка + оптимизация картинок карточек

**Date:** 2026-07-24 · **Status:** approved (owner: «Да» to combined scope)
**Metrics:** UXΔ = +3 (Better) · CDI = 0.03 * 8 · MVQ = Sprite 88%/85%

## Part 1 — Bulk card-back («рубашка»)

- `service.BulkCardSet` (+ `repo.CardBulkSet`) gains `BackPath *string`
  (`json:"back_path"`). Empty string is ALLOWED and means "reset to the
  default branded back ◆" (frontend already falls back when `back_path` is
  empty). No other validation. Wire format/endpoints unchanged
  (`PATCH /api/gacha/admin/cards/bulk`).
- FE bulk actions bar gains a **«Рубашка»** button (visible with selection):
  opens a small dialog with the same file-or-URL upload slot used by the card
  dialog (existing `uploadFile`/`uploadUrl`, kind `cards`) + preview +
  «Применить (N)» → `bulkUpdateCards(ids, { back_path })` + refetch, and
  «Сбросить на дефолт» → `bulkUpdateCards(ids, { back_path: '' })`.

## Part 2 — Image optimization

### 2a. Recompress on ingest (services/gacha)

Both ingest paths (`IngestUpload`, `IngestFromURL`) converge on `(buf, ct)`
before `makeKey`+`Upload`. Insert a shared `optimize(buf, ct)` step:

- `image/gif` (animation) and `image/webp` (no stdlib encoder, already
  efficient) → passthrough unchanged.
- Decode png/jpeg; on decode failure → passthrough (never fail an upload).
- If the longest side > 2048px → downscale to fit 2048 (x/image/draw
  CatmullRom, same as the streaming proxy).
- Fully opaque image → re-encode JPEG q85 (`.jpg`, `image/jpeg`).
- Has transparency → re-encode PNG only when downscaled, else passthrough
  (JPEG would destroy the alpha channel).
- If the re-encoded result is NOT smaller than the (possibly downscaled)
  original encoding candidate → keep the smaller bytes.
- New dep in `services/gacha`: `golang.org/x/image`.

Existing objects are not migrated; recompression applies to new uploads
(including the bulk-upload dialog, which uses the same endpoint).

### 2b. Thumbnails through the existing streaming image-proxy

- `services/streaming` image-proxy accepts **relative gacha URLs**: if
  `url` matches `^/api/gacha/images/(cards|banners)/[A-Za-z0-9._-]+$`, it is
  rewritten to `<GACHA_INTERNAL_URL>/api/gacha/images/…` (env, default
  `http://gacha:8093`, Docker DNS — no compose change) and flows through the
  existing cache/resize pipeline. SSRF-safe: fixed internal base + strict key
  regex. Absolute URLs keep the existing domain allowlist. MAL fallback is
  naturally skipped (`extractAnimeID` finds nothing).
- FE `useImageProxy.ts`: `isProxyableUrl` (and thus `cardPosterUrl`) treats
  `/api/gacha/images/…` relative URLs as proxyable.
- Surfaces switch to `cardPosterUrl(cardImageUrl(path), w)`:
  AdminGacha table + GachaCardPicker w=128; DropsModal, GachaCollection,
  CardCollectionBlock w=256; PullSummary, GachaPromoCard (spotlight) w=384;
  GachaSlider backdrop w=640. **CardViewer3D stays on originals** (front and
  back) — fullscreen 3D deserves full resolution.

### 2c. Cache headers

`GET /api/gacha/images/*` → `Cache-Control: public, max-age=31536000,
immutable` (keys are UUIDs; an object's content never changes — re-upload
mints a new key). Proxied thumbnails additionally enjoy the existing host-
nginx 30-day cache on `/api/streaming/image-proxy`.

## Testing

- gacha: optimize() unit tests (opaque png→jpeg smaller; alpha png stays png;
  gif/webp passthrough; >2048 downscale; garbage bytes passthrough);
  BulkCardSet.back_path apply/reset test.
- streaming: rewrite-match tests (valid key → rewritten, traversal/бэд ключ →
  rejected, absolute URLs → old allowlist path).
- FE: «Рубашка» dialog spec (apply + reset call shapes); cardPosterUrl gacha
  branch unit test; touched-surface specs stay green.

## Out of scope

Migration of the 22 existing originals; WebP encoding (needs cgo); host-nginx
config changes.
