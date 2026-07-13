---
id: INFRA-ipv6-streaming-origin
title: Add IPv6 (AAAA) support to the streaming origin
captured_at: 2026-07-13
captured_during: admin TODO request (@tNeymik, Telegram) replying to the gerahertz repeat-report escalation
deferred_from: 2026-07-10 streaming QUIC/HTTP-3 mitigation pass (sysctls + BBR + HTTP/3 shipped; AAAA explicitly left TODO)
status: backlog
---

# Add IPv6 (AAAA record) to the streaming origin

## Context

Our streaming origin (Netcup, Germany, `152.53.160.135`) is **IPv4-only** — no `AAAA`
record. Repeat reporter `@gerahertz` (SoftBank/ODN fixed-line, Tokyo, JP) hits a
network-layer return-path failure on large transfers: server serves 200/206 fast
(181MB in 9.1s, 231MB in 3.0s) but his client sees status 0 and hangs 5-164s. Full
diagnosis in `[[project_gerahertz_jp_longhaul_path_diagnosis]]` (memory).

Leading fingerprint is PMTU black-holing / JP PPPoE IPv4 peak-hour congestion on his
fixed line. **JP IPv6 IPoE is uncongested** relative to the PPPoE IPv4 leg most JP
fixed-line ISPs (SoftBank/ODN included) still route through — so an IPv6 path to our
origin would very likely route around the broken leg entirely for IPv6-capable JP
clients.

The 2026-07-10 mitigation pass shipped host sysctls (`mtu_probing=1`, BBR, `fq`) +
HTTP/3/QUIC on the streaming edge (`[[project_streaming_quic_http3_nginx130]]`), which
helps but doesn't address IPv4-only-origin PMTU black-holing at the source. AAAA/IPv6
was explicitly called out as "still TODO" in that diagnosis and is now escalated again
by the same reporter, prompting this backlog capture.

## The idea (what to build if/when picked up)

1. Provision an IPv6 address on the Netcup origin host (or via whatever edge/CDN-less
   ingress is in front of `streaming`/`gateway` — this platform has no CDN by design,
   so this is host-level, not a CDN toggle).
2. Add the `AAAA` record alongside the existing `A` record for the streaming domain(s).
3. Confirm nginx/gateway listen on `[::]` and the QUIC/HTTP-3 listener
   (`[[project_streaming_quic_http3_nginx130]]`) is dual-stack.
4. Verify with a JP-vantage-point test (or ask `@gerahertz` to retest) that IPv6-capable
   clients route over IPoE and no longer hit the PMTU-black-hole symptom.

## Why deferred (not done inline)

- Host/DNS-level provisioning (new address, AAAA record, dual-stack nginx config,
  firewall/ufw considerations — note `[[project_host_ddos_protection]]`: **ufw is
  TABOO** on this host) is infra work outside the maintenance bot's auto-fix/button-fix
  scope — no code diff, no service redeploy, needs the host owner.
- Affects a shared production host directly (network interface / DNS), higher blast
  radius than any of the bot's authorized operations.
- Single confirmed repeat-reporter so far; worth doing but not urgent enough to
  interrupt other work — this capture exists so it isn't lost.

## Cost estimate

| Component | Effort (Fib) | Risk |
|---|---|---|
| Provision IPv6 on origin host + AAAA DNS record | 3 | Low |
| Dual-stack nginx/QUIC listener config + verify | 2 | Low |
| JP-vantage retest / confirm with reporter | 1 | Low |

## Cross-references

- Diagnosis: `[[project_gerahertz_jp_longhaul_path_diagnosis]]` (memory)
- Prior mitigation: `[[project_streaming_quic_http3_nginx130]]` (memory)
- Host networking constraint: `[[project_host_ddos_protection]]` (memory) — ufw taboo
- Source: admin message from `@tNeymik`, Telegram, replying to escalation for feedback
  entry `2026-07-13T08-41-32_tNeymik_telegram`
