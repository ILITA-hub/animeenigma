# Deferred Items — Phase 03

## From plan 03-04 (cache effects)

- **Pre-existing: `services/catalog` GOWORK=off build reports `missing go.sum entry for github.com/klauspost/compress@v1.18.0 go.mod`.**
  - Discovered while validating the libs/cache→tracing dependency edge with `GOWORK=off go build` in the catalog consumer.
  - NOT caused by 03-04: catalog's go.mod/go.sum were reverted to their pre-sync state, and the cache→tracing/authz edge adds nothing to catalog's module closure (catalog already requires tracing+authz). The in-workspace `go build ./...` for catalog is green; Docker builds run `go mod download` which resolves this transitively.
  - Out of scope for this plan (touches catalog's module graph, owned elsewhere). Resolve with `cd services/catalog && go mod download github.com/klauspost/compress && go mod tidy` if a future plan needs catalog to build under GOWORK=off without a download step.
