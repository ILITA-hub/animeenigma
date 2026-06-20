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

import logging
from contextlib import asynccontextmanager
from typing import Any

from fastapi import FastAPI, Query
from fastapi.responses import JSONResponse, Response
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from pydantic import BaseModel, Field

from .config import Config
from .engine import CamoufoxEngine, SessionGone
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

    def params(self) -> dict[str, Any]:
        return self.model_dump(exclude={"provider"}, exclude_none=True)


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


@app.get("/metrics")
async def metrics() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


@app.post("/resolve")
async def resolve(req: ResolveRequest) -> JSONResponse:
    engine: CamoufoxEngine = app.state.engine
    try:
        session = await engine.resolve(req.provider, req.params())
        return JSONResponse({"success": True, "data": session})
    except NotFoundError as exc:
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "not_found"}, status_code=404
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


@app.get("/hls")
async def hls(
    sid: str = Query(..., min_length=8, max_length=64),
    url: str = Query(..., max_length=4096),
) -> Response:
    """Mandatory stream proxy: fetch the playlist/segment through the resolving
    session's clearance-bearing browser context (same exit IP + cookies + TLS).
    Playlists are rewritten so child URIs route back here."""
    engine: CamoufoxEngine = app.state.engine
    try:
        out = await engine.proxy_fetch(sid, url)
    except SessionGone as exc:
        return Response(str(exc), status_code=410, media_type="text/plain")
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
