/**
 * Diagnostics capture utility for error reporting.
 * Intercepts console logs and network requests to provide
 * comprehensive diagnostic data when users report issues.
 */

interface ConsoleEntry {
  time: string
  level: string
  message: string
}

interface NetworkEntry {
  time: string
  method: string
  url: string
  status: number
  duration: number
  size: number
  error?: string
}

export interface PlayerContext {
  playerType: string
  animeId: string
  animeName: string
  episodeNumber?: number | null
  serverName?: string | null
  streamUrl?: string | null
  errorMessage?: string | null
}

export interface DiagnosticReport {
  timestamp: string
  url: string
  user_agent: string
  screen_size: string
  language: string
  user_id: string | null
  username: string | null
  player_type: string
  anime_id: string
  anime_name: string
  episode_number: number | null
  server_name: string | null
  stream_url: string | null
  error_message: string | null
  console_logs: string
  network_logs: string
  page_html: string
  description: string
}

const MAX_CONSOLE_ENTRIES = 100
const MAX_NETWORK_ENTRIES = 50
const MAX_HTML_SIZE = 500 * 1024 // 500KB

const consoleLogs: ConsoleEntry[] = []
const networkLogs: NetworkEntry[] = []
let initialized = false

function addConsoleEntry(level: string, args: unknown[]) {
  const message = args
    .map((a) => {
      try {
        return typeof a === 'object' ? JSON.stringify(a) : String(a)
      } catch {
        return String(a)
      }
    })
    .join(' ')

  consoleLogs.push({
    time: new Date().toISOString(),
    level,
    message: message.slice(0, 2000),
  })

  if (consoleLogs.length > MAX_CONSOLE_ENTRIES) {
    consoleLogs.shift()
  }
}

function addNetworkEntry(entry: NetworkEntry) {
  networkLogs.push(entry)
  if (networkLogs.length > MAX_NETWORK_ENTRIES) {
    networkLogs.shift()
  }
}

/**
 * Initialize diagnostic capture. Call once early in the app lifecycle.
 * Intercepts console methods and tracks network requests.
 */
export function initDiagnostics() {
  if (initialized) return
  initialized = true

  // Intercept console methods
  const origLog = console.log
  const origWarn = console.warn
  const origError = console.error

  console.log = (...args: unknown[]) => {
    addConsoleEntry('log', args)
    origLog.apply(console, args)
  }
  console.warn = (...args: unknown[]) => {
    addConsoleEntry('warn', args)
    origWarn.apply(console, args)
  }
  console.error = (...args: unknown[]) => {
    addConsoleEntry('error', args)
    origError.apply(console, args)
  }

  // Track network requests via PerformanceObserver
  if (typeof PerformanceObserver !== 'undefined') {
    try {
      const observer = new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) {
          const res = entry as PerformanceResourceTiming
          addNetworkEntry({
            time: new Date(performance.timeOrigin + res.startTime).toISOString(),
            method: 'GET',
            url: res.name.slice(0, 500),
            status: 0, // not available from PerformanceObserver
            duration: Math.round(res.duration),
            size: res.transferSize || 0,
          })
        }
      })
      observer.observe({ type: 'resource', buffered: false })
    } catch {
      // PerformanceObserver not supported
    }
  }
}

/**
 * Hook into an Axios instance to capture API request/response logs.
 * Call after creating the Axios instance.
 */
export function hookAxiosDiagnostics(axiosInstance: {
  interceptors: {
    response: {
      use: (
        onFulfilled: (response: any) => any,
        onRejected: (error: any) => any,
      ) => void
    }
  }
}) {
  axiosInstance.interceptors.response.use(
    (response: any) => {
      addNetworkEntry({
        time: new Date().toISOString(),
        method: response.config?.method?.toUpperCase() || 'GET',
        url: (response.config?.url || '').slice(0, 500),
        status: response.status || 0,
        duration: 0,
        size: 0,
      })
      return response
    },
    (error: any) => {
      addNetworkEntry({
        time: new Date().toISOString(),
        method: error.config?.method?.toUpperCase() || 'GET',
        url: (error.config?.url || '').slice(0, 500),
        status: error.response?.status || 0,
        duration: 0,
        size: 0,
        error: error.message?.slice(0, 500),
      })
      return Promise.reject(error)
    },
  )
}

/**
 * Collect all diagnostic data for an error report.
 */
export function collectDiagnostics(
  context: PlayerContext,
  userDescription: string,
  userId: string | null,
  username: string | null,
): DiagnosticReport {
  let pageHtml = ''
  try {
    pageHtml = document.documentElement.outerHTML
    if (pageHtml.length > MAX_HTML_SIZE) {
      pageHtml = pageHtml.slice(0, MAX_HTML_SIZE) + '...[truncated]'
    }
  } catch {
    pageHtml = '[could not capture]'
  }

  return {
    timestamp: new Date().toISOString(),
    url: window.location.href,
    user_agent: navigator.userAgent,
    screen_size: `${window.screen.width}x${window.screen.height}`,
    language: navigator.language,
    user_id: userId,
    username,
    player_type: context.playerType,
    anime_id: context.animeId,
    anime_name: context.animeName,
    episode_number: context.episodeNumber ?? null,
    server_name: context.serverName ?? null,
    stream_url: context.streamUrl ?? null,
    error_message: context.errorMessage ?? null,
    console_logs: JSON.stringify(consoleLogs),
    network_logs: JSON.stringify(networkLogs),
    page_html: pageHtml,
    description: userDescription,
  }
}
