# Phase 09 — Deferred / Out-of-Scope Items

## 09-03 (player Logic B)

- **Pre-existing, out-of-scope:** `TestMALExportHandler_GetUserExports_Authorized`
  (`services/player/internal/handler/mal_export_test.go:118`) fails in the
  sandbox with a 500 `EXTERNAL_API_ERROR "scheduler service"`. The test issues a
  real network call to the scheduler service, which is unreachable in this
  isolated environment. NOT caused by 09-03 changes (Logic B touches
  service/repo/config/main only; no MAL-export or scheduler-client code). The
  scoped verification packages (`internal/service`, `internal/repo`) are green.
  Left untouched per the SCOPE BOUNDARY rule.
