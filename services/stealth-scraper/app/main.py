"""FastAPI entrypoint for the stealth-scraper sidecar.

Routes (internal Docker network only — never gateway-exposed):
  GET  /healthz   liveness + pool/proxy snapshot
  GET  /metrics   Prometheus
  POST /resolve   {provider, ...params} -> resolved stream session

The Go ``scraper`` service is the only caller (mirrors how it calls
animepahe-resolver). Input is schema-validated; recipes enforce a per-provider
host allowlist (SSRF guard).
"""

from __future__ import annotations

import base64
import logging
from contextlib import asynccontextmanager
from typing import Any

from fastapi import FastAPI, Query
from fastapi.responses import JSONResponse, Response
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from pydantic import BaseModel, Field

from .config import Config
from .engine import (
    CamoufoxEngine,
    CapacityExceeded,
    FetchTimeout,
    PoolExhausted,
    ProviderWedged,
    SessionGone,
    UserQuotaExceeded,
)
from .recipes import ChallengeError, NotFoundError, RecipeError

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("stealth-scraper")


class ResolveRequest(BaseModel):
    provider: str = Field(..., min_length=1, max_length=32)
    # gogoanime accepts either a title+episode (full in-browser chain) or a
    # pre-resolved episode_url / embed shortcut.
    title: str | None = Field(default=None, max_length=200)
    keyword: str | None = Field(default=None, max_length=200)
    episode: int | None = Field(default=None, ge=0, le=10_000)
    category: str = Field(default="sub", pattern="^(sub|dub)$")
    episode_url: str | None = Field(default=None, max_length=2048)
    # embed_url is a known server/wrapper URL the Go scraper already discovered
    # via its curl ListServers — the sidecar resolves it straight to the player
    # (skips search/episode discovery).
    embed_url: str | None = Field(default=None, max_length=2048)
    # base_url is the provider mirror from the DB roster (scraper_providers.base_url),
    # passed by the Go scraper — the sidecar holds no provider URL consts/envs.
    base_url: str | None = Field(default=None, max_length=256)
    proxy_type: str | None = Field(default=None, max_length=16)
    # Opaque quota key (authed user id, or a salted client-IP hash for anon),
    # set by the catalog→scraper hop. Used only for per-user concurrency
    # accounting; never persisted or logged in clear.
    user_key: str | None = Field(default=None, max_length=128)

    def params(self) -> dict[str, Any]:
        return self.model_dump(exclude={"provider", "user_key"}, exclude_none=True)


class FetchRequest(BaseModel):
    provider: str = Field(..., min_length=1, max_length=32)
    url: str = Field(..., max_length=8192)
    # GET-only for now (nineanime discovery). POST is wired for future allanime.
    method: str = Field(default="GET", pattern="^(GET|POST)$")
    user_key: str | None = Field(default=None, max_length=128)


@asynccontextmanager
async def lifespan(app: FastAPI):
    cfg = Config.from_env()
    engine = CamoufoxEngine(cfg)
    engine.set_logger(log)
    await engine.start()
    app.state.cfg = cfg
    app.state.engine = engine
    log.info("stealth-scraper started (pool=%d)", cfg.pool_size)
    try:
        yield
    finally:
        await engine.stop()
        log.info("stealth-scraper stopped")


app = FastAPI(title="stealth-scraper", version="0.1.0", lifespan=lifespan)


@app.get("/healthz")
async def healthz() -> dict:
    engine: CamoufoxEngine = app.state.engine
    return engine.health()


@app.get("/readyz")
async def readyz() -> JSONResponse:
    """Readiness (observability only — D8: does NOT drive a Docker/k8s restart).
    503 when the pool has been saturated continuously past the configured
    window; 200 otherwise. /healthz stays 200 for process liveness so in-flight
    streams keep playing while the pool self-heals."""
    engine: CamoufoxEngine = app.state.engine
    if engine.is_ready():
        return JSONResponse({"ready": True}, status_code=200)
    body = {"ready": False}
    body.update(engine.health())
    return JSONResponse(body, status_code=503)


@app.get("/metrics")
async def metrics() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


@app.post("/resolve")
async def resolve(req: ResolveRequest) -> JSONResponse:
    engine: CamoufoxEngine = app.state.engine
    try:
        session = await engine.resolve(req.provider, req.params(), user_key=req.user_key)
        return JSONResponse({"success": True, "data": session})
    except NotFoundError as exc:
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "not_found"}, status_code=404
        )
    except CapacityExceeded as exc:
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "capacity"}, status_code=503
        )
    except UserQuotaExceeded as exc:
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "user_quota"}, status_code=503
        )
    except ProviderWedged as exc:
        # Warm session poisoned (>= poison_max crashes) — 503 (retryable) so the
        # Go orchestrator fails over; the Go breaker reads kind=provider_wedged.
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "provider_wedged"},
            status_code=503,
        )
    except PoolExhausted as exc:
        # Pool saturated — 503 (retryable) so the Go orchestrator fails over.
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "pool_exhausted"}, status_code=503
        )
    except ChallengeError as exc:
        # All exits challenged — surface as 502 so the Go side fails over.
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "challenge"}, status_code=502
        )
    except RecipeError as exc:
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "error"}, status_code=502
        )
    except Exception as exc:  # noqa: BLE001
        log.exception("resolve crashed")
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "internal"}, status_code=500
        )


@app.post("/fetch")
async def fetch(req: FetchRequest) -> JSONResponse:
    """Discovery fetch: GET an allowlisted provider URL through a warm,
    challenge-solved browser session and return the raw body (base64). The Go
    scraper keeps its parsers and only swaps transport when engine=browser."""
    engine: CamoufoxEngine = app.state.engine
    try:
        out = await engine.browser_fetch(req.provider, req.url, user_key=req.user_key)
        return JSONResponse({
            "success": True,
            "status": out["status"],
            "content_type": out["content_type"],
            "body": base64.b64encode(out["body"]).decode(),
        })
    except NotFoundError as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "not_found"}, status_code=404)
    except CapacityExceeded as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "capacity"}, status_code=503)
    except UserQuotaExceeded as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "user_quota"}, status_code=503)
    except ProviderWedged as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "provider_wedged"}, status_code=503)
    except PoolExhausted as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "pool_exhausted"}, status_code=503)
    except ChallengeError as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "challenge"}, status_code=502)
    except RecipeError as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "error"}, status_code=502)
    except Exception as exc:  # noqa: BLE001
        log.exception("fetch crashed")
        return JSONResponse({"success": False, "error": str(exc), "kind": "internal"}, status_code=500)


@app.get("/hls")
async def hls(
    sid: str = Query(..., min_length=8, max_length=64),
    url: str = Query(..., max_length=8192),  # signed CDN segment URLs can be long
) -> Response:
    """Mandatory stream proxy: fetch the playlist/segment through the resolving
    session's clearance-bearing browser context (same exit IP + cookies + TLS).
    Playlists are rewritten so child URIs route back here."""
    engine: CamoufoxEngine = app.state.engine
    try:
        out = await engine.proxy_fetch(sid, url)
    except SessionGone as exc:
        return Response(str(exc), status_code=410, media_type="text/plain")
    except FetchTimeout as exc:
        # Hung upstream fetch; the session was torn down. 504 so the client/Go
        # side can re-resolve rather than retry a dead session.
        return Response(str(exc), status_code=504, media_type="text/plain")
    except RecipeError as exc:
        return Response(str(exc), status_code=400, media_type="text/plain")
    except Exception as exc:  # noqa: BLE001
        log.exception("hls proxy crashed")
        return Response("upstream proxy error", status_code=502, media_type="text/plain")
    headers = {"Cache-Control": "no-store", "Access-Control-Allow-Origin": "*"}
    return Response(
        content=out["body"],
        status_code=out["status"],
        media_type=out["content_type"],
        headers=headers,
    )


@app.delete("/session/{sid}")
async def close_session(sid: str) -> JSONResponse:
    engine: CamoufoxEngine = app.state.engine
    return JSONResponse({"closed": await engine.aclose_session(sid)})
