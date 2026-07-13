import { reactive } from 'vue'

// Module-scoped Shikimori-id → display-title cache for the File Manager's
// aeProvider/<id> folder transcripts. It lives in this .ts module (not in a
// FileManager.vue <script> block) for two reasons: it survives component
// remounts (true ES-module scope), and its reset hook stays a plain named
// export — a named export from a `.vue` file trips TS2614 against the `*.vue`
// module shim, which the clean production build enforces.
// Value '' = resolved-but-untitled/404 (cached so it is not refetched).
export const titleCache = reactive<Record<string, string>>({})

// Test-only helper: clear the cache so specs don't bleed resolved titles
// across cases (the cache otherwise survives remounts within the session).
export function resetTitleCache(): void {
  for (const k of Object.keys(titleCache)) delete titleCache[k]
}
