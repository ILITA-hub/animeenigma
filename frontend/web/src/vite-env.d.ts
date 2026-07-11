/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  // eslint-disable-next-line @typescript-eslint/ban-types, @typescript-eslint/no-explicit-any
  const component: DefineComponent<{}, {}, any>
  export default component
}

interface ImportMetaEnv {
  readonly VITE_API_URL: string
  readonly VITE_SOCKET_URL: string
  readonly VITE_APP_NAME: string
  readonly VITE_APP_VERSION: string
  readonly VITE_HLS_PROXY_BASE?: string
  readonly VITE_HLS_PROXY_TIERS?: string
  /** Short git commit hash of the deployed build (baked at build time, shown in footer). */
  readonly VITE_GIT_COMMIT?: string
  readonly VITE_OFFLINE_DOWNLOADS_ENABLED?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
