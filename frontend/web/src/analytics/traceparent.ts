// W3C Trace Context generator. The browser has no real spans (RUM is out of
// scope for v1); it mints a synthetic traceparent per API call so the backend
// trace roots with a trace_id the frontend also knows, which lets us stamp
// that trace_id onto the analytics click event that triggered the call.
function randomHex(bytes: number): string {
  const buf = new Uint8Array(bytes)
  crypto.getRandomValues(buf)
  let out = ''
  for (const b of buf) out += b.toString(16).padStart(2, '0')
  return out
}

export function newTraceparent(): { header: string; traceId: string } {
  const traceId = randomHex(16) // 128-bit
  const spanId = randomHex(8) // 64-bit
  // version 00, trace-flags 01 (sampled — the collector tail-samples centrally)
  return { header: `00-${traceId}-${spanId}-01`, traceId }
}
