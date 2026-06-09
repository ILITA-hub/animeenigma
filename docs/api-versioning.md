# API Versioning Strategy

Status: **documented baseline** (notebook point 12:00:45). This file records the
strategy that already governs the API in practice — it does not change any route.

## Context

AnimeEnigma is a **self-hosted, single-tenant** platform. The Vue frontend and the
Go backend are built from the **same monorepo commit** and deployed together
(`make redeploy-web` ships the only first-party client; backends ship via
`make redeploy-<service>`). There is no fleet of independently-versioned external
consumers to keep in lockstep, so the cost/benefit of a URL version prefix is very
different from a public multi-tenant API.

The one programmatic non-browser consumer is the **API-key path** (e.g. MAL import),
which targets the same `/api/*` surface.

## Current scheme: unversioned `/api/*`, gateway-routed

All requests enter through the gateway and are routed by path prefix to a service
(see `CLAUDE.md` → Gateway Routing):

```
/api/auth/*          → auth:8080
/api/anime/*         → catalog:8081
/api/streaming/*     → streaming:8082
/api/users/*         → player:8083
/api/rooms/*, /api/game/* → rooms:8084
/api/themes/*        → themes:8086
/api/notifications/* → notifications:8090
/api/watch-together/* → watch-together:8091
…
```

There is intentionally **no `/api/v1/` prefix**. The contract is pinned by:

- **OpenAPI specs** in `api/openapi/*.yaml` (`auth, catalog, common, player, rooms,
  streaming, themes`), each carrying an `info.version` (currently `1.0.0`).
- **GraphQL schema** in `api/graphql/schema.graphql`.
- **Protobuf** in `api/proto/*.proto`.
- **Internal-only** endpoints under `/internal/*` are **never** gateway-proxied
  (Docker-network only) and are exempt from this policy.

The `info.version` in the OpenAPI specs is the **semantic version of the contract**,
bumped per the compatibility rules below. The HTTP surface stays unversioned.

## Compatibility rules (additive-by-default)

Because client and server ship together, the rule is **"don't break a deploy that
straddles the rollout window."** During a redeploy, an old web bundle may talk to a
new backend (and briefly vice-versa). Therefore:

**Backward-compatible (allowed without a version bump beyond a MINOR/PATCH of the
spec `info.version`):**
- Adding a new endpoint.
- Adding an **optional** request field (server must default it).
- Adding a field to a response (clients must ignore unknown fields — the frontend
  axios layer and typed models already tolerate extra keys).
- Widening an enum where clients have a default branch (e.g. the feedback
  `status` enum — the `statusLabel`/`statusClass` helpers fall through to a
  default, so adding `ai_done` was non-breaking).
- Relaxing a validation constraint.

**Breaking (requires the migration path below + a MAJOR bump of the spec
`info.version`):**
- Removing or renaming an endpoint, field, or enum value a client still reads.
- Making an optional request field required, or tightening validation.
- Changing a field's type or units, or the meaning of a value.
- Changing status-code or error-shape semantics a client branches on.

## Migration path for an unavoidable breaking change

1. **Add, don't mutate.** Introduce the new endpoint/field alongside the old one
   (e.g. `name_v2`, or a new route). Ship backend first.
2. **Migrate the frontend** in a later commit to consume the new shape.
3. **Deprecate** the old surface: mark it `deprecated: true` in the OpenAPI spec,
   note it in the changelog, and keep it for at least one release cycle.
4. **Remove** the old surface only after the frontend no longer references it and a
   redeploy can no longer straddle the change.

### When a URL prefix *is* warranted

Introduce a parallel `/api/v2/<service>/*` prefix (additively, leaving `/api/*`
serving v1) **only** if a breaking change cannot be expressed additively on the same
path *and* a long dual-running window is needed — for example a third-party
integration we don't control. This has not been needed to date; prefer the additive
migration above.

## Checklist for any API change

- [ ] Update the relevant `api/openapi/*.yaml` (and bump `info.version`: PATCH/MINOR
      for additive, MAJOR for breaking).
- [ ] Confirm it's additive; if breaking, follow the migration path and add a
      changelog entry.
- [ ] Keep `/internal/*` endpoints off the gateway.
