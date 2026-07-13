// Shared helpers for the raw-library admin views (TorrentClient + FileManager).
// formatBytes humanizes a byte count; unwrap peels the {success,data} httputil
// envelope every /api/library/* + /api/anime/* response is wrapped in.

export function formatBytes(n: number): string {
  if (!n || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let val = n
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024
    i++
  }
  return val.toFixed(val >= 100 ? 0 : 1) + ' ' + units[i]
}

export function unwrap<T>(resp: { data?: { data?: T } | T }): T | undefined {
  const body = resp.data as { data?: T } | T | undefined
  if (body && typeof body === 'object' && 'data' in (body as Record<string, unknown>)) {
    return (body as { data: T }).data
  }
  return body as T | undefined
}
