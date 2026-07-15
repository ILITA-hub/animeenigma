// Pure formatter for the streaming proxy's X-AE-Edge-Trail header.
// Render the raw trail ("p13:timeout:45003,p12:ok:210") as the hacker-mode
// logic+metrics line: "p13 45.0s✗ → p12 0.21s✓". A trailing ✓ marks the edge
// that served (outcome "ok"); every other outcome is a ✗ with its latency, so
// a cold-start wait or a burned timeout is legible at a glance.
export function formatEdgeTrail(raw: string): string {
  if (!raw) return ''
  return raw
    .split(',')
    .map((part) => {
      const [edge, outcome, ms] = part.split(':')
      const secs = Number(ms) / 1000
      const t = Number.isFinite(secs) ? (secs >= 10 ? secs.toFixed(1) : secs.toFixed(2)) : '?'
      return `${edge} ${t}s${outcome === 'ok' ? '✓' : '✗'}`
    })
    .join(' → ')
}
