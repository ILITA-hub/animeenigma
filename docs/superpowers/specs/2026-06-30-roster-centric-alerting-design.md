# Roster-Centric Alerting — Design

**Date:** 2026-06-30
**Status:** Approved design, pre-implementation
**Author:** Claude Code (with project owner)
**Topic:** Cut garbage alerts + idle maintenance-bot runs by making the provider
lifecycle (`provider_state` gauge) the single source of truth for provider
alerting, and by deferring high-noise transient streaming/gateway error-rate
alerts without losing their data.

---

## 1. Problem

Two-week maintenance-bot review (2026-06-16 → 06-30) found:

1. **Garbage alerts.** ~14 of 27 Grafana alerts in the window were the *same*
   transient class — `High Error Rate` on `streaming`/`gateway` driven by the
   HLS proxy returning `502` when an *upstream CDN* 403s/410s or a signed
   session expires (AUTO-495/503/505/507/508/509/510/511/512 …). They are not
   our service failing, they self-resolve, and they recur nightly.

2. **Idle bot runs.** Every firing alert that survives dedup/cooldown spawns a
   full `claude` analysis process. The transient streaming/gateway alerts almost
   always end `info_only`/`resolved` with no action — wasted compute.

3. **Two independent deciders of "is a provider in trouble."** Grafana evaluates
   per-provider trouble from *raw* metrics (`parser_requests_total`,
   `playability_canary_runs_total`, `parser_ad_decoy_total`, …) **and** the
   catalog runs a provider lifecycle state machine (`policy`×`health`). They
   disagree, which is the root of nearly every false positive we have patched
   (NaN parser-failure-rate AUTO-487, AES-128 `decode_failed` miruro,
   degraded-provider re-alerts AUTO-486/488). The maintenance bot then
   *re-derives* provider state at runtime (`shouldSuppressForProvider`) to
   reconcile — a third place the same truth lives.

## 2. Goals / Non-goals

**Goals**

- G1. Defer `High Error Rate` on `streaming` + `gateway` from notifying / from
  spawning a bot run, **without losing any data** (it stays queryable for
  analysis).
- G2. Emit **one** actionable alert when a provider group's auto-failover
  collapses (nothing works) or several providers fail *together* — "requires
  real action." Gradual one-at-a-time degrade stays silent (the daily
  provider-recovery operator owns that).
- G3. Make the alerting **roster-centric**: the catalog lifecycle gauge
  (`provider_state`) is the only provider-trouble decider; Grafana and the
  roster dashboard panel read the *same* series.
- G4. **Simplify**: remove redundant per-provider pagers and the bot's
  state-reconciliation code.

**Non-goals**

- No new storage for deferred-alert data (YAGNI — Prometheus + ClickHouse
  already hold it; see §5).
- No change to the provider lifecycle state machine itself (policy/health
  transitions, probe cadence) — we only *read* its output.
- No EN-specific special-casing — the mechanism is uniform across all groups
  (§4.2).

## 3. Background — the existing pieces we build on

- **`provider_state{provider}` gauge** — emitted *solely* by catalog
  (`services/catalog/internal/service/scraperprovider/roster_metrics.go`,
  `EmitProviderStates`), re-emitted on every policy transition
  (`internal/handler/internal_provider_policy.go`). Numeric encoding from
  `domain/scraper_provider.go` (`DerivedState`/`StateCode`):

  | Code | State | Meaning |
  |------|-------|---------|
  | 4 | `UP` | auto + up — in auto-failover, healthy |
  | 3 | `Recovering` | health recovering (any policy) |
  | 2 | `Degraded` | **manual** — registered, out of auto-failover (last-resort) |
  | 1 | `Down` | **auto + down — actively failing in the chain** |
  | 0 | `Disabled` | policy disabled |

  **Key property:** the lifecycle already separates *sudden* from *gradual*. A
  provider still in the auto chain that is failing reads `Down(1)`. A provider
  that aged out gradually was auto-demoted to **manual → `Degraded(2)`** (the
  24h demote rule). So `Down(1)` inherently means "recently/actively failing,"
  and gradual attrition never shows up as `Down`. We get simultaneity detection
  for free from the state encoding — no snapshots needed.

- **Maintenance bot** (`services/maintenance/cmd/maintenance/main.go`):
  - Grafana alerts arrive **primarily via webhook** → `processBatch` (dispatch
    loop ~line 582+). A reconcile **poller** (goroutine 2, ~line 306) is a
    safety net for missed webhooks.
  - `isSuppressed(alertKey)` (env `SUPPRESSED_ALERTS`, keyed `alertName:service`)
    is currently checked **only in the poller** (line 334), **not** in the
    webhook path. So suppression does not work for the normal delivery path
    today.
  - `shouldSuppressForProvider(provider)` (line 1087) calls catalog
    `/internal/scraper/providers` to skip escalation for managed (`policy!=auto`)
    providers — runtime re-derivation of state.
  - `scraperProviderFaultLine()` (line 1153) formats a one-line "⚠️ Unhealthy:
    …" summary from the same roster endpoint.
  - `escalateBatch()` (line 1663) fires once when ≥3 services have active alerts,
    throttled 24h via the `escalate-outage` cooldown.

- **Grafana** routes *all* alerts through one notification policy
  (`docker/grafana/provisioning/alerting/policies.yml`) → `maintenance-webhook`
  contact point → bot. Provider/scraper-related rules in `rules.yml`:
  `Parser Failure Rate` (for 30m), `Scraper Provider Stream-Segment Down`
  (for 4h), `ScraperPlayabilityRegression` (for 0s), `ScraperAdDecoySurge`
  (for 5m), `ScraperUnplayableSpike` (for 5m), `Kodik Player Unavailable`
  (for 30m).

## 4. Design

Delivered in two phases. Phase 1 is additive and reversible; Phase 2 is the
cleanup, applied after Phase 1 has soaked a few days.

### 4.1 Phase 1a — Defer streaming + gateway `High Error Rate` (bot)

- Add an `isSuppressed(key)` check to the **webhook dispatch loop**
  (`processBatch`, beside the existing dedup at ~line 598):

  ```go
  if msg.Type == domain.MessageAlertFiring && len(msg.Alerts) > 0 {
      key := msg.Alerts[0].Name + ":" + msg.Alerts[0].Service
      if s.isSuppressed(key) {
          continue // deferred: no Telegram, no Claude run, no issue
      }
  }
  ```

- Set, in `docker/.env`:

  ```
  SUPPRESSED_ALERTS=High Error Rate:streaming,High Error Rate:gateway
  ```

- The Grafana `High Error Rate` rule is **untouched** — it still evaluates,
  still shows in Grafana, and its alert state-history is intact. "Deferred,"
  not "deleted." Catalog's `High Error Rate` and every other service keep
  paging normally (suppression is keyed `alertName:service`).

### 4.2 Phase 1b — Roster-centric fleet rules (catalog + Grafana)

**Catalog change:** add a `group` label to the `provider_state` gauge. The
catalog already knows each provider's group (`ScraperProvider.Group`). Touch
points:

- `libs/metrics` — `ProviderState` gauge gains a `group` label.
- `roster_metrics.go` `EmitProviderStates` — `WithLabelValues(r.Name, r.Group)`.
- `internal_provider_policy.go` live-transition emit — same label set.
- Update `roster_metrics_test.go` + any `provider_state` assertions.

The `group` label is an **aggregation dimension, not a filter** — there is no
EN special-case. The same two rules cover every group (`en`, `ru`, `jp`,
`adult`, `firstparty`) uniformly.

**Two new Grafana rules** (`rules.yml`), `for: 30m`, Prometheus datasource:

- **`ProviderFleetNoAutoPlayable`** — a group has registered providers but none
  auto-playable (best provider is `Down`/`Degraded`, not `UP`/`Recovering`):

  ```promql
  (max by (group) (provider_state) >= 1) and (max by (group) (provider_state) < 3)
  ```

  > ⚠️ Two correctness details, both deliberate:
  > - **Use `max by (group)`, not `count(... >= 3) == 0`.** The naive form makes
  >   a fully-collapsed group **vanish** from the result (the `>= 3` filter drops
  >   all members → no series → no alert). `max by (group)(provider_state)`
  >   evaluates over the *unfiltered* vector, so every group always yields a
  >   value and `< 3` is a real comparison.
  > - **The `>= 1` floor excludes all-`Disabled` groups.** A group whose only
  >   providers are intentionally `Disabled(0)` is a steady state, not an outage,
  >   and must not page. `>= 1` means "at least one registered provider exists"
  >   (Down/Degraded/Recovering/UP), so the rule fires only when a group that
  >   *should* be serving has regressed below auto-playable.

- **`ProviderFleetCorrelatedDown`** — ≥2 providers in a group actively failing
  at once:

  ```promql
  count by (group) (provider_state == 1) >= 2
  ```

  Because gradual attrition becomes `Degraded(2)` (manual), only *actively
  failing in-auto-chain* providers (`Down(1)`) count — so this fires on a shared
  failure event (e.g. Camoufox pool crash taking 3 browser providers together),
  not on slow one-at-a-time aging. The empty-set vanish behaviour is *correct*
  here: a group with zero `Down` providers produces no series → no fire.

  > **⚠ Correction (2026-07-11, AUTO-577).** The premise above — that gradual
  > attrition demotes to `Degraded(2)` so `Down(1)` implies *recently* failing —
  > **does not hold** for providers that keep actively failing probes: they stay
  > pinned at `Down(1)` indefinitely (allanime-okru since 07-07, animepahe CF,
  > gogoanime fake-content all sat at `Down(1)` for days). `count(==1) >= 2` then
  > fired on three *independent, long-standing* EN outages that merely overlapped,
  > wrongly paging "correlated outage (shared cause)." The rule is now
  > **edge-triggered** — it counts only providers that transitioned INTO `Down`
  > within the last 15m (`(provider_state == 1) unless (provider_state offset 15m
  > == 1)`), with `for: 5m` sitting inside that pulse. So simultaneity is now
  > detected from the transition *edge*, not inferred from the state encoding, and
  > gradual one-at-a-time decay can never reach the threshold. See the live rule
  > in `docker/grafana/provisioning/alerting/rules.yml`.

Both rules carry a `group` label and `severity: page-fleet` so the bot can
route them (§4.3). `for: 30m` is the persistence hold (skip transient blips),
consistent with the existing error-rate rules. (Grafana condition wiring —
refId A/B/C reduce+threshold — mirrors the existing rules; the PromQL above is
the intent.)

Single-provider groups (e.g. `ru`=Kodik, `firstparty`=ae) behave correctly:
`ProviderFleetCorrelatedDown` can never fire (only one member), but
`ProviderFleetNoAutoPlayable` fires when that single provider drops below
auto-playable — **provided the group's steady-state is `UP`** (see R5).

### 4.3 Phase 1c — Bot handling for fleet alerts

The two fleet alerts are **real, actionable** outages, so they flow through the
**normal Claude analysis + Telegram + escalate path** — like any other
`escalate`-tier alert. No special "skip Claude" handling. (Idle-run savings come
from deferring the garbage streaming/gateway alerts (§4.1) and from Phase 2
demoting the per-provider pagers (§4.4) — not from short-circuiting a genuine
fleet outage.)

The only roster-specific behaviour:

- **Aggregation is already done upstream:** the Grafana rule is `by (group)`, so
  the bot receives **one** alert per affected group instead of N per-provider
  alerts. The per-provider fault detail comes from Claude's analysis (it reads
  the live roster), so `scraperProviderFaultLine` can simply be generalized to a
  `group` argument for the firing-message annotation if useful.
- **Standard active-alert dedup applies** (`GetActiveAlert`) so Grafana's 4h
  `repeat_interval` re-sends are processed once, not re-analyzed on every repeat.
- **No periodic re-alert.** The escalation fires once per occurrence (after the
  30m hold); it is **not** re-sent on a 24h timer. Auto-clear is handled by the
  existing `checkResolvedAlerts` path — when Grafana stops firing, the
  active-alert entry clears, so a *new* later collapse escalates again as a fresh
  occurrence.

### 4.4 Phase 2 — Simplification cleanup (after soak)

- **Demote redundant per-provider pagers to dashboard-only.** Keep the rules /
  panels / data, but remove them from the notification route (e.g. label
  `severity: diagnostic` + a notification-policy matcher that drops
  `severity=diagnostic`, or route them to a no-op contact point):
  `Parser Failure Rate`, `Scraper Provider Stream-Segment Down`,
  `ScraperPlayabilityRegression`, `ScraperAdDecoySurge`, `ScraperUnplayableSpike`.
  Rationale: these raw signals already feed the lifecycle via probe verdicts;
  they do not need to page independently. The fleet rules (§4.2) cover the
  actionable aggregate; the daily recovery operator + 6h probe cover
  per-provider recovery.
- **Delete `shouldSuppressForProvider()`** and its dispatch-loop gate. It exists
  only to suppress per-provider alerts for managed providers — but once those
  rules no longer page, the function is dead. Managed/disabled providers are
  simply absent from the `>= 3` / `== 1` counts, so the fleet rules are
  inherently lifecycle-aware with no runtime reconciliation.
- Trim `scraperProviderFaultLine` usage to its remaining caller (the fleet
  message); drop the now-unused firing-message append path if it has no other
  consumer.

## 5. Data preservation (G1)

Deferring the streaming/gateway alerts loses **no** data:

- **Prometheus / Grafana** keep the exact series the alert is computed from
  (`http_requests_total{status=~"5.."}`) plus the alert's own state-history
  (the rule still evaluates).
- **ClickHouse `events`** already stores per-egress-effect rows — `host`,
  `provider`, `status` (incl. upstream `403`/`404`/`410`), `bytes`,
  `duration_ms` — via the analytics `/internal/effects` pipeline
  (`services/analytics/internal/handler/effects.go`,
  `internal/repo/clickhouse_schema.go` `events` table). This is the granular,
  queryable record of the upstream failures behind the noise.

Implementation must include a one-time **verification** that streaming egress
effects are landing in ClickHouse `events` (read-only check, no code). If a gap
is found, note it — do not build new storage in this work.

## 6. Components & boundaries

| Unit | Responsibility | Depends on | Testable in isolation |
|------|----------------|------------|-----------------------|
| `provider_state` gauge (catalog) | Publish derived lifecycle state per `(provider, group)` | DB roster rows | yes — emit test asserts code+labels |
| Grafana fleet rules | Evaluate fleet collapse / correlated-down per group | `provider_state` series | yes — PromQL unit reasoning + manual/synthetic check |
| Bot webhook suppress gate | Drop deferred alert keys silently | `SUPPRESSED_ALERTS` cfg | yes — table test |
| Bot fleet-alert handling | Route the aggregated per-group alert through the normal Claude analysis + escalate path | alert name/labels, active-alert dedup | yes — handler test with fakes |

## 7. Testing

- **Bot**:
  - Webhook-path suppression: suppressed `High Error Rate:streaming` dropped
    silently (no Telegram, no issue, no Claude); non-suppressed proceeds.
  - Fleet alert: routed through the normal Claude analysis + escalate path (a
    Claude run **is** expected); active-alert dedup means a repeated Grafana
    delivery is processed once, not re-analyzed; resolve clears the active-alert
    entry; no periodic re-alert is scheduled. Handwritten fakes (project
    convention — no testify/mock).
- **Catalog**:
  - `provider_state` emits with `(provider, group)` labels at boot seed and on a
    simulated policy transition.
- **Grafana** (correctness-critical):
  - `max by (group)` zero-preservation: a fully-`Down` group yields a value that
    trips `< 3` (alert), and an all-`Disabled` group is excluded by `>= 1` (no
    alert). Verify with a synthetic metric set (promtool / manual rule eval),
    since jsdom/unit tests cannot exercise PromQL.
  - Per-group steady-state baseline check (R5): confirm every group normally
    sits at `max(provider_state) >= 3` so the rule does not page in steady state.
- **Data**: confirm streaming egress rows present in ClickHouse `events`
  (read-only).

## 8. Rollout / verification

1. Phase 1 in one worktree → `go test ./...` for catalog + maintenance,
   `make redeploy-catalog` + rebuild `bin/maintenance`, set `SUPPRESSED_ALERTS`
   in `docker/.env`.
2. **Baseline gate (R5):** after catalog redeploy emits the `group`-labelled
   gauge, query `max by (group)(provider_state)` for every group and confirm
   each reads `>= 3` in steady state. Resolve any group that does not (fix seed
   policy/health, or add a label-matcher exclusion) **before** `make
   restart-grafana` loads the fleet rules.
3. Confirm: streaming/gateway `High Error Rate` no longer reaches Telegram;
   Grafana still shows the rule; `events` still records egress.
4. Synthetically drive a group to all-`Down` (or wait for a real transition) →
   confirm exactly one fleet Telegram + one escalate issue (Claude analysis
   runs); a repeated Grafana delivery is deduped, not re-analyzed.
5. Soak a few days; confirm gradual single-provider degrade stays silent.
6. Phase 2: demote per-provider pagers, delete `shouldSuppressForProvider`,
   redeploy + restart-grafana; confirm dashboards still populate and no provider
   pager noise returns.

## 9. Risks & mitigations

- **R1 — losing fast per-provider detection** when Phase 2 demotes raw-signal
  pagers. *Mitigation:* the fleet rules cover the actionable aggregate; the 6h
  probe + daily recovery operator + dashboards retain per-provider visibility;
  Phase 2 is deferred until Phase 1 proves out and is reversible (re-add the
  notification matcher).
- **R2 — `group` label cardinality / emit-site drift.** *Mitigation:* group is
  low-cardinality (≤6 values); the two existing emit sites are covered by tests
  asserting the label set.
- **R3 — PromQL zero-row gotcha** (§4.2). *Mitigation:* `max by (group)` form
  (never an empty result) + explicit synthetic test.
- **R4 — single-provider groups.** Handled by design (§4.2): `NoAutoPlayable`
  covers them; `CorrelatedDown` correctly cannot fire.
- **R5 — uniform rule false-fires for a group whose steady-state is not `UP`.**
  "Same mechanism for everyone" (§4.2) evaluates every group, so a group that
  normally sits at `Degraded`/`Down` (e.g. if Kodik in `ru`, or ae in
  `firstparty`, is seeded `policy=manual` rather than `auto+up`) would page
  constantly — re-introducing the noise class this work removes. *Mitigation:*
  Phase 1 implementation **must** read the live `provider_state` for every group
  first and confirm each normally reads `>= 3`. If a group's legitimate
  steady-state is below auto-playable, either correct its seed policy/health so
  the lifecycle reflects reality, or exclude that group from the rule via a
  label matcher (documented inline). This baseline check is a hard gate before
  the rules go live (see §7, §8 step 2). The `>= 1` floor already covers the
  all-`Disabled` case; R5 is specifically about `Degraded`/`Down` baselines.

## 10. Tunable parameters (defaults)

| Param | Default | Where |
|-------|---------|-------|
| Deferred alert keys | `High Error Rate:streaming,High Error Rate:gateway` | `SUPPRESSED_ALERTS` (docker/.env) |
| Auto-playable threshold | `>= 3` (UP/Recovering) | fleet rule PromQL |
| Correlated-down count | `>= 2` | fleet rule PromQL |
| Persistence hold | `30m` | rule `for:` |
| Re-alert | none — fire once per occurrence, auto-clear on resolve | bot active-alert dedup |

## 11. Metrics (project conventions — `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — the owner gets dramatically fewer junk pages and one
  clear "act now" signal; end-users unaffected (playback behavior unchanged).
- **CDI = 0.04 * 13** — Spread: touches catalog metric + Grafana rules + bot
  dispatch (moderate, 3 areas). Shift: behavior-preserving for users, changes
  operator notification surface. Effort_Fib: 13 (two-phase, cross-service, one
  PromQL gotcha, test surface). DO NOT pre-multiply.
- **MVQ = Griffin 85% / 80%** — disciplined consolidation onto an existing
  source of truth (Griffin: vigilant, removes more than it adds); high
  slop-resistance because it deletes the duplicate-decider class of bugs rather
  than patching symptoms.

## 12. Open items deferred to the implementation plan

- Exact `libs/metrics.ProviderState` signature change + every call site.
- Whether Phase 2's demotion uses a `severity` matcher in `policies.yml` vs a
  no-op contact point (decide during planning by reading the current policy).
- Generalizing `scraperProviderFaultLine(group string)` signature + callers.
