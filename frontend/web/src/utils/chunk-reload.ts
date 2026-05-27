// Recover from stale-chunk errors after a deploy by reloading the page.
//
// Vite emits several distinct error messages depending on whether the failed
// resource is a JS dynamic import or a CSS modulepreload:
//   - "Failed to fetch dynamically imported module"  (JS, fetch path)
//   - "error loading dynamically imported module"     (JS, alt phrasing)
//   - "Unable to preload CSS for /assets/Foo.css"     (CSS modulepreload)
// Webpack-era phrasings ("Loading chunk N failed", "Loading CSS chunk ...
// failed") are kept in the pattern for safety even though this app uses Vite.
const CHUNK_ERROR_RE =
  /Loading (CSS )?chunk [\w-]+ failed|Failed to fetch dynamically imported module|error loading dynamically imported module|Unable to preload CSS/i

const RELOAD_KEY = 'chunkReloadAt'
const RELOAD_COOLDOWN_MS = 10_000

function extractMessage(err: unknown): string {
  if (!err) return ''
  if (typeof err === 'string') return err
  if (err instanceof Error) return err.message
  if (typeof err === 'object' && 'message' in err && typeof (err as { message: unknown }).message === 'string') {
    return (err as { message: string }).message
  }
  return String(err)
}

export function isChunkLoadError(err: unknown): boolean {
  return CHUNK_ERROR_RE.test(extractMessage(err))
}

// Recover from a stale-chunk error with a full page load (which fetches the
// fresh index.html + new hashed asset names), AND only if we haven't already
// recovered for the same reason in the last few seconds. The cooldown breaks
// the infinite-reload loop that would otherwise happen if the asset is
// genuinely missing server-side (vs. just an old hash from a previous deploy
// that has since been replaced).
//
// `targetUrl` is the route the user was navigating TO when the chunk failed.
// When present we navigate there with a full load instead of reloading the
// current URL — otherwise a failed lazy-route import during navigation (which
// vue-router aborts WITHOUT committing the URL) would reload the *origin*
// route, dumping the user back on the page they started from (usually "/")
// instead of the one they clicked. The unhandledrejection path (component
// async failures with no target route) omits it and falls back to a reload.
//
// Returns true if a recovery navigation was triggered (caller can suppress
// further error logging in that case).
export function tryReloadOnChunkError(err: unknown, targetUrl?: string): boolean {
  if (!isChunkLoadError(err)) return false

  try {
    const last = Number(sessionStorage.getItem(RELOAD_KEY) || 0)
    if (last && Date.now() - last < RELOAD_COOLDOWN_MS) {
      // Already reloaded recently — the asset is truly gone. Stop looping.
      console.error('[chunk-reload] asset still missing after reload, giving up:', extractMessage(err))
      return false
    }
    sessionStorage.setItem(RELOAD_KEY, String(Date.now()))
  } catch {
    // sessionStorage unavailable (private mode, etc.) — fall through and reload anyway.
  }

  if (targetUrl) {
    window.location.assign(targetUrl)
  } else {
    window.location.reload()
  }
  return true
}
