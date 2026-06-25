# Upscaler Dynamic Server-Provisioned Models — Implementation Plan

> **For agentic workers:** executed via superpowers:subagent-driven-development on branch `feat/upscaler-service`. Tasks T25–T30.

**Goal:** Make the worker model-agnostic. It boots with only the built-in `mock` model (plus any `PREINSTALLED_MODELS`); the server provisions any other model at runtime. Remove the `MODEL` env var; add `PREINSTALLED_MODELS`.

**Architecture (owner-approved 2026-06-25):** Pull-on-demand. The job names the model; the lease grant carries it to the worker; if the worker lacks that model it fetches the weights from the server (capability-signed, MinIO-backed, checksum-verified), installs + registers + caches them, then runs. Models are uploaded to the server by an admin (weights → MinIO + `upscale_models` row). This REVERSES the Phase-1 CD-13 "baked-in models" decision toward dynamic provisioning (CD-13's deferred `/worker/models/*` half).

**Tech stack:** Go (services/upscaler + worker module), gorilla/websocket control plane, HMAC capability handles, MinIO artifacts, realesrgan-ncnn-vulkan runtime (weight-sets = models), `mock` = built-in pure-Go passthrough.

## Global Constraints
- The `worker/` module is separate (`github.com/ILITA-hub/animeenigma/worker`, NOT in root go.work); build/test with `GOWORK=off GOTOOLCHAIN=go1.25.0`. Wire structs in `worker/internal/wire/wire.go` MUST stay BYTE-IDENTICAL (field names + json tags) to `services/upscaler/internal/controlplane/protocol.go`.
- GORM string defaults single-quoted. Capability HMAC: do NOT weaken the shared `capability` package; model handles reuse `capability.MintJobHandle`/`VerifyJobHandle` with a new operation string. Fail-closed, constant-time.
- `mock` is ALWAYS available (built-in Go); it needs no weights, no GPU, no fetch. A worker with no `PREINSTALLED_MODELS` and no provisioned model can still run `mock` jobs.
- Capability/data-plane discipline from existing tasks: server stamps identity (worker_id from authenticated conn), never trust worker-supplied paths; checksum-verify artifacts; generic error bodies on `/worker/*`.
- Checksum = SHA-256 hex of the artifact (matches `UpscaleModel.Checksum`).

---

## T25: Protocol — lease grant carries model + scale + a model-fetch capability
**Files:** `worker/internal/wire/wire.go`, `services/upscaler/internal/controlplane/protocol.go` (byte-identical), `services/upscaler/internal/controlplane/hub.go` (lease grant construction), `services/upscaler/internal/service/leaser.go` + repos (thread job Model/Scale to the grant). Tests: wire parity + grant population.
- Add to `LeaseGrantPayload`: `Model string json:"model"`, `Scale int json:"scale"`, and `ModelHandle *LeaseHandle json:"model_handle,omitempty"` (a capability handle for `GET /worker/models/{name}`; nil when Model=="mock" or the worker is known to have it — for Phase-1 simplicity, always populate it for non-mock models). The existing `LeaseHandles`/`LeaseHandle` shape is the template.
- Server: when building the grant (hub.go), look up the job for the leased segment (`JobRepository.Get(jobID)`), set Model+Scale from the job; for non-mock models mint a model capability handle via `capability.MintJobHandle(name, "model", 0, ttl)` and put the signed URL/exp/sig in `ModelHandle`.
- Worker: `LeaseGrantPayload` mirrors the new fields. (No behavior change yet — consumed in T29.)

## T26: Server model store — admin upload + registry
**Files:** `services/upscaler/internal/handler/admin.go` (or a new `model_admin.go`) + `transport/router.go` (admin routes), `services/upscaler/internal/repo/model.go` (UpscaleModel repo: Upsert/Get/List), `services/upscaler/internal/minio/` (PutObject already exists). Tests.
- `POST /api/upscale/models` (admin, behind X-Gateway-Internal): accepts a model name + version + scale + one or more weight files (multipart, e.g. `{name}.param` + `{name}.bin` for realesrgan). Streams them to MinIO under a deterministic `ObjectPath` (e.g. `models/{name}/{version}/...` or a single tar), computes SHA-256 → `Checksum`, upserts the `upscale_models` row (Builtin=false).
- `GET /api/upscale/models` (list), `GET /api/upscale/models/{name}` (metadata), `DELETE /api/upscale/models/{name}` (optional).
- A model artifact = a single archive (tar) of the realesrgan weight files for that model name, so serving + checksum + install are one object. Document the layout.

## T27: Server model serving — capability-signed worker data-plane
**Files:** `services/upscaler/internal/handler/` (new `models.go` worker-facing handler) + `transport/router.go` (`/worker/models/{name}` under the worker routes), `services/gateway/internal/handler/external_api.go` (ext edge route for `/worker/models/*`). Tests (capability-gated 401, checksum header, traversal 400).
- `GET /worker/models/{name}`: verify the model capability handle (`capability.VerifyJobHandle(name, "model", 0, exp, sig, now)`, fail-closed); stream the artifact from MinIO (`ObjectPath`); set an `X-Model-Checksum` (SHA-256) + `X-Model-Version` header. Generic error bodies. Reuse the segment-GET streaming/auth discipline.
- Gateway: add `/worker/models/*` to the internet-facing worker edge (API-key gated, same as `/worker/segments/*`; strip inbound internal headers).

## T28: Worker model manager (remove MODEL, add PREINSTALLED_MODELS)
**Files:** `worker/internal/agent/config.go` (remove `Model`, add `PreinstalledModels []string` from env `PREINSTALLED_MODELS` comma-sep), `worker/internal/upscale/` (a `Manager`: registry of available models + dynamic install), `worker/internal/agent/client.go` (RegisterPayload.ModelsAvailable from the manager; drop the startup single PipelineProcessor pinned to cfg.Model). Tests (`-race`).
- Manager: thread-safe map of name→Model. Always register `mock`. At boot, for each `PREINSTALLED_MODELS` name, register a realesrgan model pointing at the baked `/models` weights (assume the image placed them). `Available() []string`. `Get(name) (Model, bool)`. `Install(name, version string, artifact io.Reader, checksum string) error` — verify SHA-256, extract the weight archive into `/models` (or WORK_DIR/models), register a realesrgan model for `name`, idempotent + concurrency-safe (one install per name in flight).
- `mock` is always present and never fetched.
- RegisterPayload.ModelsAvailable = `manager.Available()`.

## T29: Worker per-job model selection + pull-on-demand
**Files:** `worker/internal/agent/leaseloop.go` (processSegment), `worker/internal/agent/client.go` (model fetch via ModelHandle). Tests (`-race`): mock path unchanged; missing-model triggers fetch→install→run; fetch failure handled.
- `processSegment` uses `grant.Model` (default to `mock` if empty for safety): `m, ok := manager.Get(grant.Model)`; if `!ok`, fetch from the server using `grant.ModelHandle` (capability-signed GET `/worker/models/{name}`), verify checksum, `manager.Install(...)`, then `manager.Get` again. Build the per-job processor for that model + `grant.Scale`. Cache (Install registers it for reuse). On fetch/install failure: fail the segment cleanly (server re-leases) with a clear log; do NOT crash the worker.
- Remove the single startup `PipelineProcessor` from `cfg.Model`; the processor is now selected per-segment from the manager. Keep the `processorFn` test seam.

## T30: E2E (pull-on-demand) + Dockerfile + handover refresh
**Files:** `services/upscaler/internal/e2e/mock_e2e_test.go` (add a pull-on-demand scenario), `worker/Dockerfile` (PREINSTALLED_MODELS handling — bake an optional weight set; default none → only mock), `/data/upscaler-handover/README.md` regen, rebuild + re-`docker save` the worker image. 
- E2E: existing mock job still green; ADD: register a fake "model" on the server (a tiny tar that the worker's install path accepts, with a stub processor so no real GPU needed — or reuse a second mock-like model name served from MinIO) → submit a job naming it → assert the worker FETCHES it (server logs a `/worker/models/{name}` GET) → installs → completes. Keep it GPU-free.
- Dockerfile: document `PREINSTALLED_MODELS` (the operator can bake weight-sets into `/models` and list them; default empty). Remove any `MODEL` references.
- Handover README: remove `MODEL`, document `PREINSTALLED_MODELS` + the admin model-upload flow + that real models are now provisioned from the server (no rebuild needed to add a model).
- Re-validate the container end-to-end (isolated stack, mock + one provisioned model) and re-package the deliverable.

## T31: Post-review hardening (realesrgan `-m`, artifact hardening, immutable-name doc)
**Files:** `worker/internal/upscale/realesrgan.go`, `worker/internal/upscale/manager.go`, `services/upscaler/internal/handler/model_admin.go`, `docs/superpowers/plans/2026-06-25-upscaler-dynamic-models.md`. Tests updated and new.

### I-A — realesrgan `-m modelsDir` (IMPORTANT)
`Upscale` now passes `-m {modelsDir}` to `realesrgan-ncnn-vulkan` when `modelsDir` is non-empty. `newRealesrgan` gains a `modelsDir string` parameter. Both `NewManager` (preinstalled path) and `Install` (pull-on-demand registration) pass `cfg.ModelsDir` / the manager's `modelsDir`. The built-in `init()` models (global registry) pass an empty string so the runtime uses its default search path — these are image-baked weights that live alongside the binary. The fix ensures that if `MODELS_DIR` is set to a non-default path, the runtime actually finds the weight files that `Install` extracted there.

### I-B — immutable model name convention (DOC)
**Model names are immutable content addresses.** A running worker caches a model by name for its lifetime via the Manager registry. `Install` is idempotent (no-op if the name is already registered). Changing a model's weights requires registering them under a **new name** (e.g. `realesrgan-x4plus-anime-v2`) and restarting workers. There is no in-place update path. A comment to this effect lives in `manager.go` near the `Install` registration step, and this plan section documents it for operators.

### Hardening batch
- **Name traversal guard (worker):** `Install` rejects names containing `/`, `\`, or `..` before any disk or path operation. The server also validates on upload, but the worker must not trust the lease grant (defence in depth).
- **Artifact size cap (worker):** `io.LimitReader(artifact, maxModelArtifactBytes+1)` (2 GiB cap) wraps the artifact read. Reading one byte beyond the cap detects overflow and returns a clear error; truncation never occurs. Const is documented.
- **TAR non-regular entry rejection:** `extractTAR` now rejects any entry whose `Typeflag != tar.TypeReg` with a clear error. Symlinks, hardlinks, character devices, FIFOs — all rejected. The existing absolute-path and `..` guards are retained.
- **Both weight files required:** After extraction, `Install` checks that both `{name}.param` and `{name}.bin` exist on disk. A TAR that silently omits either file would produce a broken model at runtime. On failure, partial files are cleaned up and nothing is registered.
- **Admin upload cap (server):** `UploadModel` wraps the body in `http.MaxBytesReader(w, r.Body, maxUploadBodyBytes)` (2 GiB) before `ParseMultipartForm`. A `*http.MaxBytesError` is distinguished and returns 413 with a clear message.

## Self-review
- Coverage: remove MODEL (T28) ✓; PREINSTALLED_MODELS (T28) ✓; mock-only boot (T28) ✓; server sends any model pull-on-demand (T25 grant model + T27 serving + T29 fetch) ✓; admin upload→MinIO (T26) ✓.
- Type consistency: `LeaseGrantPayload.{Model,Scale,ModelHandle}` defined T25, consumed T29; `Manager.{Available,Get,Install}` defined T28, consumed T29; `UpscaleModel` repo defined T26, consumed T27.
- Reverses CD-13 — note it in the spec doc (`docs/superpowers/specs/2026-06-23-anime-upscaler-design.md`) as part of T25.
