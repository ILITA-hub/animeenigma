import { hlsProxyUrl, maskedStreamUrl } from '@/utils/streaming'

/** Wrap a subtitle file URL in the signed HLS proxy (CORS + provenance).
 *  SubtitleOverlay fetches any `/`-prefixed url directly, so a pre-signed
 *  proxy url loads an un-allowlisted scraper subtitle CDN without a 502.
 *  When the backend minted a Track A masked path (opaque token — no url=
 *  shape for filter lists), prefer it outright. */
export function buildSubtitleProxyUrl(file: string, exp?: string, sig?: string, masked?: string): string {
  if (masked) return maskedStreamUrl(masked)
  const params = new URLSearchParams()
  params.set('url', file)
  if (exp && sig) {
    params.set('exp', exp)
    params.set('sig', sig)
  }
  return hlsProxyUrl(params.toString())
}

export function detectSubFormat(
  format: string | undefined,
  url: string,
): 'ass' | 'srt' | 'vtt' | null {
  const ext = (format || url.split('?')[0].split('.').pop() || '').toLowerCase()
  return ext === 'ass' || ext === 'srt' || ext === 'vtt' ? ext : null
}

/** Best-effort language code from a track label / filename. Defaults to 'ja'
 *  (the provider's burned-in-less tracks are overwhelmingly JP soft-subs). */
export function langFromTrack(label: string | undefined, url: string): string {
  const hay = `${label ?? ''} ${url}`.toLowerCase()
  if (/\b(en|eng|english)\b/.test(hay)) return 'en'
  if (/\b(ru|rus|russian)\b/.test(hay) || /[Ѐ-ӿ]/.test(label ?? '')) return 'ru'
  if (/\b(ja|jp|jpn|japanese)\b/.test(hay) || /[぀-ヿ一-鿿]/.test(label ?? '')) return 'ja'
  return 'ja'
}
