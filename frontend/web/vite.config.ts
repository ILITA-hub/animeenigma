import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import compression from 'vite-plugin-compression'
import { VitePWA } from 'vite-plugin-pwa'
import { fileURLToPath, URL } from 'node:url'

// RU static-edge (Maskanya) asset routing — dark-shipped. When
// VITE_MSK_ASSET_HOST is set at build time, JS chunk URLs are emitted as a
// runtime window.__assetHost() call so a per-user probe (utils/assetEdge.ts)
// can serve dynamic-import chunks from the geographically-closer edge. Empty
// (the default) => byte-identical to today: origin-relative URLs, no indirection.
const MSK_ASSET_HOST = process.env.VITE_MSK_ASSET_HOST || ''

// Per-route <link rel="modulepreload"> (first-load waterfall, 2026-07-06 trace).
// Lazy route chunks (e.g. Anime.vue) can't be requested until the entry bundle
// has downloaded AND executed and the router resolves the URL — a serial second
// hop (~1 RTT) after the ~3s entry download on a modest self-hosted link. This
// build-time plugin reads the emitted bundle (so filenames stay hash-safe),
// maps each top route's path-prefix to its chunk URLs, and injects a tiny inline
// <head> script that preloads ONLY the chunks for the route the visitor actually
// landed on — so that chunk streams in parallel with the entry bundle instead of
// after it. Per-route match = no over-preloading other routes. The browser
// dedupes the href against the later dynamic import(), so no double fetch, and
// against the entry's own modulepreloads, so listing shared vendors is a no-op.
// Disabled under MSK edge-routing (VITE_MSK_ASSET_HOST) where chunks are served
// from the edge via window.__assetHost(), not these origin-relative URLs.
function aeRouteModulePreload(): Plugin {
  // Most-specific first is not required (the injected script picks the longest
  // matching prefix); '/' matches the Home route by exact path only.
  const ROUTES = [
    { view: 'src/views/Anime.vue', match: '/anime/' },
    { view: 'src/views/Browse.vue', match: '/browse' },
    { view: 'src/views/Home.vue', match: '/' },
  ]
  return {
    name: 'ae-route-modulepreload',
    apply: 'build',
    transformIndexHtml: {
      order: 'post',
      handler(html, ctx) {
        const bundle = ctx.bundle
        if (!bundle) return html
        const map: Record<string, string[]> = {}
        for (const r of ROUTES) {
          const chunk = Object.values(bundle).find(
            (c) =>
              c.type === 'chunk' &&
              !!c.facadeModuleId &&
              c.facadeModuleId.split('?')[0].replace(/\\/g, '/').endsWith(r.view)
          ) as import('rollup').OutputChunk | undefined
          if (!chunk) continue
          // Facade chunk ONLY — deliberately NOT its import tree. On a saturated
          // link these preloads are High-priority and sit in <head> before the
          // entry <script>, so preloading the whole subtree (shared vendors +
          // leaves like ass-compiler that aren't needed for first render) would
          // contend with and delay the entry bundle. Shared vendors are already
          // Vite-preloaded as entry deps; the facade's own deps still stream via
          // the runtime __vitePreload when the route executes. Removing the ONE
          // serial hop for the route chunk is the safe, net-positive win.
          map[r.match] = ['/' + chunk.fileName]
        }
        if (!Object.keys(map).length) return html
        // Longest-prefix match; keys longer than 1 char match as a path prefix,
        // '/' matches the exact root only (so it never shadows /anime//browse).
        const script =
          `(function(){var M=${JSON.stringify(map)},p=location.pathname,b=null,n=-1;` +
          `for(var k in M){var m=k.length>1?(p===k||p.indexOf(k)===0):p===k;` +
          `if(m&&k.length>n){b=k;n=k.length}}` +
          `if(b){var u=M[b];for(var i=0;i<u.length;i++){var l=document.createElement('link');` +
          `l.rel='modulepreload';l.href=u[i];document.head.appendChild(l)}}})();`
        return {
          html,
          tags: [{ tag: 'script', attrs: {}, children: script, injectTo: 'head' as const }],
        }
      },
    },
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  // Runtime host only for JS chunk refs (dynamic imports). The index.html
  // bootstrap (entry + preloaded vendors) and CSS-referenced assets stay
  // origin-relative, so first paint is unchanged.
  ...(MSK_ASSET_HOST
    ? {
        experimental: {
          renderBuiltUrl(filename: string, { hostType }: { hostType: string }) {
            if (hostType === 'js') {
              return { runtime: `window.__assetHost(${JSON.stringify(filename)})` }
            }
            return undefined
          },
        },
      }
    : {}),
  plugins: [
    vue(),
    // Per-route modulepreload — off under MSK edge-routing (chunks come from the
    // edge, not the origin-relative URLs this plugin emits).
    ...(MSK_ASSET_HOST ? [] : [aeRouteModulePreload()]),
    compression({
      algorithm: 'gzip',
      threshold: 1024,
    }),
    // Pre-compressed .br twins served by nginx `brotli_static on` (zero
    // runtime CPU, same model as gzip_static) — ~15-20% smaller than gzip
    // on JS/CSS. Page-load optimization 2026-06-11.
    compression({
      algorithm: 'brotliCompress',
      ext: '.br',
      threshold: 1024,
    }),
    VitePWA({
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.ts',
      // We self-manage registration (kill-switch + playback-aware reload in
      // src/pwa/registerPwa.ts) — the plugin only builds sw.js + manifest.
      injectRegister: false,
      manifest: {
        name: 'AnimeEnigma',
        short_name: 'AnimeEnigma',
        description: 'Anime streaming platform',
        lang: 'ru',
        start_url: '/',
        scope: '/',
        display: 'standalone',
        theme_color: '#08080f',
        background_color: '#08080f',
        icons: [
          { src: '/android-chrome-192x192.png', sizes: '192x192', type: 'image/png' },
          { src: '/android-chrome-512x512.png', sizes: '512x512', type: 'image/png' },
          // Same art declared maskable — acceptable v1 (logo sits centered);
          // dedicated safe-zone art can replace it later without code changes.
          { src: '/android-chrome-512x512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      injectManifest: {
        globPatterns: ['**/*.{js,css,html,woff2,svg,png,ico,webmanifest}'],
        // .gz/.br twins are nginx-only; changelog.json is fetched fresh every
        // page load by design; branding/ is heavy static art.
        globIgnores: ['**/*.{gz,br}', 'changelog.json', 'branding/**'],
        maximumFileSizeToCacheInBytes: 3 * 1024 * 1024,
      },
      devOptions: { enabled: false },
    }),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  server: {
    port: 3000,
    host: true,
    proxy: {
      '/api': {
        target: process.env.VITE_API_URL || 'http://localhost:8000',
        changeOrigin: true
      },
      '/socket.io': {
        target: process.env.VITE_SOCKET_URL || 'http://localhost:8000',
        changeOrigin: true,
        ws: true
      }
    }
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    rollupOptions: {
      output: {
        // Page-load optimization 2026-06-11: the default per-module splitting
        // produced 90+ chunks, 35 of them under 2KB (every lucide icon was its
        // own request) — at a ~300ms RTT the request COUNT, not bytes,
        // dominated page-load time. Group the stable vendor trees into a few
        // immutable-cached chunks and let rollup merge leftover crumbs.
        manualChunks(id: string) {
          // Lazy locales (see src/i18n.ts) get pinned chunks — without this,
          // rollup merged ja.json into an unrelated route chunk, making every
          // visitor of that route download 70KB of Japanese messages.
          if (id.includes('/src/locales/en.json')) return 'locale-en'
          if (id.includes('/src/locales/ja.json')) return 'locale-ja'
          if (!id.includes('node_modules')) return undefined
          if (id.includes('hls.js')) return 'hls-vendor'
          if (id.includes('socket.io') || id.includes('engine.io')) return 'socket-vendor'
          // All lucide icons in one cached chunk instead of one request each.
          if (id.includes('vuedraggable') || id.includes('sortablejs')) return 'showcase-editor'
          if (id.includes('lucide-vue-next')) return 'icons'
          // reka-ui (+ its floating-ui positioning dep) — shared headless-UI
          // primitives used by nearly every view.
          if (id.includes('reka-ui') || id.includes('@floating-ui')) return 'ui-vendor'
          // Core framework. The [\\/] guards keep e.g. vue-i18n's own deps
          // matched explicitly, not by substring accident.
          if (/[\\/]node_modules[\\/](vue|@vue|vue-router|pinia|vue-i18n|@intlify)[\\/]/.test(id)) {
            return 'vue-vendor'
          }
          return undefined
        },
        // Merge side-effect-free micro-chunks (sub-10KB shared component/
        // composable slivers) into their importers where rollup can prove it
        // safe — kills most of the remaining <2KB requests.
        experimentalMinChunkSize: 10240,
      }
    }
  }
})
