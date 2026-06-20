"""gogoanime → megaplay resolution recipe.

Chain (mirrors services/scraper/internal/providers/gogoanime + embeds/megaplay):

  gogoanimes.fi/search.html?keyword=<kw>   → /category/<slug>
    → /<slug>-episode-<N>                   (episode page)
    → data-video = gogoanime.me.uk/newplayer.php  (wrapper)
    → nested <iframe src="https://megaplay.buzz/stream/...">  (player)
    → player page exposes data-id="<realId>"
    → GET <player-origin>/stream/getSources?id=<realId>  (X-Requested-With)
    → {"sources":{"file":"<master.m3u8>"},"tracks":[…],"intro":…,"outro":…}

The master.m3u8 lives on cdn.mewstream.buzz (Cloudflare-fronted). The whole
point of running this in a real browser is that the page's own JS executes the
challenge and accumulates the clearance cookie, which the engine then harvests
for the CDN host (plan §3.6).

Pure helpers (search_keywords / parse_getsources / build_*_url) are exported and
unit-tested without a browser. The async ``resolve`` is duck-typed against a
Playwright Page.
"""

from __future__ import annotations

import asyncio
import json
import re
from typing import Any
from urllib.parse import quote, urljoin

from .base import (
    ChallengeError,
    NotFoundError,
    Recipe,
    RecipeContext,
    RecipeError,
    host_allowed,
    host_of,
    looks_like_challenge,
)

GOGOANIME_ALLOWED_HOSTS = {
    "gogoanimes.fi",
    "gogoanime.me.uk",
    "megaplay.buzz",
    "vidwish.live",
    "mewstream.buzz",
    "lostproject.club",
}

_MEGAPLAY_PLAYER_HOSTS = ("megaplay.buzz", "vidwish.live")

# In-page liveness probe: fetch the candidate master through the real Firefox
# network stack and return "status|first16chars". A single STRING arg + no
# typed-array work (keeps Camoufox's xray wrapper happy).
_PROBE_MASTER_JS = """async (url) => {
  try { const r = await fetch(url); const t = await r.text(); return r.status + '|' + t.slice(0, 16); }
  catch (e) { return '0|'; }
}"""
_WORD_RUN = re.compile(r"[0-9A-Za-z]+(?: [0-9A-Za-z]+)*")
_LEAD_WORD = re.compile(r"^[0-9A-Za-z]+")


# --------------------------------------------------------------------------- #
# Pure helpers (unit-tested)
# --------------------------------------------------------------------------- #
def search_keywords(title: str) -> list[str]:
    """Emit search keyword candidates, mirroring the Go FindID.searchKeywords:
    the leading clean phrase up to the first punctuation, plus a first-word
    fallback. gogoanimes.fi matches keywords as a LITERAL substring and 404s on
    apostrophes, so the full title often misses.

        "Frieren: Beyond Journey's End" -> ["Frieren"]
        "Re:Zero kara Hajimeru..."       -> ["Re"]
        "One Piece"                       -> ["One Piece", "One"]
    """
    title = (title or "").strip()
    out: list[str] = []
    m = _WORD_RUN.match(title)
    if m and m.group(0).strip():
        out.append(m.group(0).strip())
    if title:
        fw = _LEAD_WORD.match(title)
        if fw and fw.group(0) and fw.group(0) not in out:
            out.append(fw.group(0))
    return out


def build_search_url(base_url: str, keyword: str) -> str:
    return f"{base_url.rstrip('/')}/search.html?keyword={quote(keyword)}"


def build_episode_url(base_url: str, slug: str, episode: int, dub: bool = False) -> str:
    suffix = "-dub" if dub else ""
    return f"{base_url.rstrip('/')}/{slug}{suffix}-episode-{episode}"


def is_absolute_http(url: Any) -> bool:
    return isinstance(url, str) and (
        url.startswith("http://") or url.startswith("https://")
    )


def parse_getsources(data: dict) -> dict:
    """Parse a megaplay getSources payload into a normalized partial session.

    ``sources`` is an object ``{"file": ...}`` (megaplay); a bare list is also
    tolerated for resilience against shape drift.
    """
    src = data.get("sources")
    master: str | None = None
    if isinstance(src, dict):
        master = src.get("file")
    elif isinstance(src, list) and src and isinstance(src[0], dict):
        master = src[0].get("file")

    if not is_absolute_http(master):
        raise RecipeError(f"megaplay getSources: non-absolute sources.file {master!r}")

    subs: list[dict] = []
    for t in data.get("tracks") or []:
        if not isinstance(t, dict):
            continue
        if t.get("kind") in ("captions", "subtitles") and t.get("file"):
            subs.append(
                {
                    "url": t["file"],
                    "label": t.get("label"),
                    "default": bool(t.get("default")),
                }
            )

    def _interval(v: Any) -> dict | None:
        if isinstance(v, dict) and ("start" in v or "end" in v):
            return {"start": int(v.get("start", 0)), "end": int(v.get("end", 0))}
        return None

    return {
        "master_url": master,
        "subtitles": subs,
        "intro": _interval(data.get("intro")),
        "outro": _interval(data.get("outro")),
    }


# --------------------------------------------------------------------------- #
# Async recipe
# --------------------------------------------------------------------------- #
class GogoanimeRecipe(Recipe):
    name = "gogoanime"
    allowed_hosts = GOGOANIME_ALLOWED_HOSTS

    async def _goto(self, rc: RecipeContext, url: str) -> Any:
        """Navigate with host-allowlist + challenge detection."""
        host = host_of(url)
        if not host_allowed(host, self.allowed_hosts):
            raise RecipeError(f"navigation blocked (host not allowed): {host}")
        resp = await rc.page.goto(
            url, wait_until="domcontentloaded", timeout=rc.cfg.nav_timeout_ms
        )
        status = resp.status if resp else None
        # Cheap challenge sniff on the landing document.
        try:
            title = await rc.page.title()
        except Exception:  # noqa: BLE001 - title() best-effort
            title = ""
        if looks_like_challenge(status, title):
            raise ChallengeError(
                f"challenge on {host} (status={status}, title={title!r})",
                host=host,
                kind="navigation",
            )
        return resp

    async def _resolve_episode_url(self, rc: RecipeContext) -> str:
        p = rc.params
        if p.get("episode_url"):
            return p["episode_url"]

        # base_url is DB-sourced (scraper_providers.base_url), passed in the
        # request — the sidecar holds no provider URL consts/envs.
        base = p.get("base_url")
        if not base:
            raise RecipeError("gogoanime: base_url required (DB roster) when no episode_url")
        dub = (p.get("category") or "sub").lower() == "dub"
        episode = int(p["episode"])
        title = p.get("keyword") or p.get("title") or ""

        for kw in search_keywords(title):
            await self._goto(rc, build_search_url(base, kw))
            # Classic gogo markup: result anchors under p.name -> /category/<slug>
            href = await rc.page.evaluate(
                """() => {
                    const a = document.querySelector('p.name a[href*="/category/"]')
                            || document.querySelector('a[href*="/category/"]');
                    return a ? a.getAttribute('href') : null;
                }"""
            )
            if not href:
                continue
            m = re.search(r"/category/([^/?#]+)", href)
            if not m:
                continue
            slug = m.group(1)
            return build_episode_url(base, slug, episode, dub=dub)

        raise NotFoundError(f"gogoanime: no /category match for {title!r}")

    async def _resolve_player_url(self, rc: RecipeContext, episode_url: str) -> tuple[str, str]:
        """Return (player_url, goto_referer). The megaplay/vidwish player only
        serves its real page when navigated WITH the embedding wrapper's referer
        (otherwise it returns a "file not found" error page) — so we also return
        the referer the goto must carry."""
        await self._goto(rc, episode_url)
        # The active server's embed lives in data-video (or the play iframe src).
        # Prefer an HD/megaplay server over decoy embeds (filemoon/vidmoly).
        embed = await rc.page.evaluate(
            """(hosts) => {
                const vids = Array.from(document.querySelectorAll('[data-video]'))
                    .map(el => el.getAttribute('data-video')).filter(Boolean);
                const pick = vids.find(v => hosts.some(h => v.includes(h)))
                          || vids.find(v => v.includes('gogoanime.me.uk'))
                          || vids[0];
                if (pick) return pick;
                const f = document.querySelector('.play-video iframe, iframe#playerframe, iframe');
                return f ? f.getAttribute('src') : null;
            }""",
            list(_MEGAPLAY_PLAYER_HOSTS),
        )
        if not embed:
            raise RecipeError("gogoanime: no data-video / player iframe on episode page")
        embed = _normalize_url(embed, episode_url)
        return await self._embed_to_player(rc, embed, _origin(episode_url))

    async def _embed_to_player(self, rc: RecipeContext, embed: str, site_origin: str) -> tuple[str, str]:
        """Resolve an embed/wrapper URL to (player_url, goto_referer). The
        megaplay/vidwish player only serves its real page when navigated WITH the
        embedding wrapper's referer — else it returns a "file not found" page."""
        embed = _normalize_url(embed, site_origin)
        # Already a megaplay/vidwish player → referer is the episode site origin.
        if host_of(embed) and any(h in host_of(embed) for h in _MEGAPLAY_PLAYER_HOSTS):
            return embed, site_origin + "/"

        # A gogoanime.me.uk wrapper nests the real megaplay/vidwish iframe. The
        # wrapper origin is the referer the player expects.
        await self._goto(rc, embed)
        nested = await rc.page.evaluate(
            """(hosts) => {
                for (const f of document.querySelectorAll('iframe')) {
                    const src = f.getAttribute('src') || '';
                    if (hosts.some(h => src.includes(h))) return src;
                }
                return null;
            }""",
            list(_MEGAPLAY_PLAYER_HOSTS),
        )
        if not nested:
            raise RecipeError("gogoanime: no nested megaplay/vidwish iframe in wrapper")
        return _normalize_url(nested, embed), _origin(embed) + "/"

    async def resolve(self, rc: RecipeContext) -> dict[str, Any]:
        # The Go scraper passes embed_url = a known server/wrapper URL (its own
        # curl-based ListServers already found it); fall back to full search →
        # episode → embed discovery when only a title/episode is given.
        embed_url = rc.params.get("embed_url")
        if embed_url:
            player_url, goto_referer = await self._embed_to_player(
                rc, embed_url, _origin(embed_url)
            )
        else:
            episode_url = await self._resolve_episode_url(rc)
            player_url, goto_referer = await self._resolve_player_url(rc, episode_url)
        player_origin = _origin(player_url)
        # The CDN enforces the megaplay player origin as Referer on the
        # master/segment fetches (verified live 2026-06-20).
        referer = player_origin + "/"

        # The stream id + CDN host are built at RUNTIME by the player JS and are
        # NOT in the static HTML, and the CDN host rotates (mewstream.buzz →
        # cinewave2.site → …). So we INTERCEPT the requests the player actually
        # fires rather than parsing the DOM. on('response') sees subframes too.
        captured: dict[str, Any] = {"getsources": None, "masters": [], "m3u8s": []}

        def _on_response(resp: Any) -> None:
            u = resp.url
            if "getSources" in u and captured["getsources"] is None:
                captured["getsources"] = u
            elif ".m3u8" in u:
                captured["m3u8s"].append(u)
                if "master" in u.lower():
                    captured["masters"].append(u)

        rc.page.on("response", _on_response)
        try:
            resp = await rc.page.goto(
                player_url, referer=goto_referer,
                wait_until="domcontentloaded", timeout=rc.cfg.nav_timeout_ms,
            )
        except Exception as exc:  # noqa: BLE001
            raise RecipeError(f"megaplay player nav failed: {exc}") from exc
        title = await _safe_title(rc.page)
        if looks_like_challenge(resp.status if resp else None, title):
            raise ChallengeError(
                f"challenge on megaplay player (status={resp.status if resp else None})",
                host=host_of(player_url), kind="player",
            )

        session: dict[str, Any] = {
            "master_url": None,
            "referer": referer,
            "subtitles": [],
            "intro": None,
            "outro": None,
            "cdn_probe_status": None,
        }

        # PRIMARY: derive the master from the getSources RESPONSE. getSources
        # fires on player init (reliable) and its sources.file IS the live CDN
        # master — so we don't depend on autoplay actually starting playback
        # (a cold profile blocks autoplay, so the .m3u8 is never fetched). We
        # re-fetch getSources via APIRequestContext (no browser CORS) and parse.
        gs_master: str | None = None
        gs_url = await self._await_getsources(captured, rc.cfg)
        if gs_url:
            try:
                r = await rc.context.request.get(
                    gs_url,
                    headers={"X-Requested-With": "XMLHttpRequest", "Referer": referer},
                )
                if r.status == 200:
                    gs = parse_getsources(json.loads(await r.text()))
                    gs_master = gs["master_url"]
                    session["subtitles"] = gs["subtitles"]
                    session["intro"] = gs["intro"]
                    session["outro"] = gs["outro"]
            except RecipeError:
                raise
            except Exception:  # noqa: BLE001 - fall through to interception
                pass

        # FALLBACK candidate(s): if getSources gave no master, wait for an
        # intercepted .m3u8 (requires the player to have actually started
        # playback). When getSources DID give a master we skip this wait — a
        # cold profile won't fire playback anyway, so polling only adds latency.
        if gs_master is None:
            await self._await_m3u8(captured, rc.cfg)

        # Build an ordered, de-duped candidate list (getSources master first,
        # then any intercepted master/.m3u8) and probe each: megaplay pins some
        # streams to a CDN that hard-blocks our exit IP at the Cloudflare WAF
        # actually-dead (HTTP 404 / removed) vs. live. The probe MUST use an
        # IN-PAGE fetch (real Firefox network stack): these CDNs are Cloudflare-
        # fingerprint-gated, so a non-browser fetch (APIRequestContext / curl /
        # Go) gets a 403 even when the stream is perfectly playable in-browser —
        # probing that way would FALSELY reject every Cloudflare-fronted stream.
        # Only a master that the browser itself can fetch is accepted; otherwise
        # we raise so the Go orchestrator fails over to the next provider.
        candidates: list[str] = []
        for c in [gs_master, *captured["masters"], *captured["m3u8s"]]:
            if c and c not in candidates:
                candidates.append(c)
        if not candidates:
            raise RecipeError("megaplay: no master from getSources or intercepted .m3u8")

        last_status: int | None = None
        for cand in candidates:
            status, ok = await self._probe_master(rc, cand, referer)
            last_status = status
            if ok:
                session["master_url"] = cand
                session["cdn_probe_status"] = status
                return session

        session["cdn_probe_status"] = last_status
        raise RecipeError(
            f"megaplay: all {len(candidates)} CDN candidate(s) unfetchable in-browser "
            f"(last status={last_status}) — stream removed/expired ({host_of(candidates[0])})"
        )

    async def _probe_master(
        self, rc: RecipeContext, url: str, referer: str
    ) -> tuple[int | None, bool]:
        """Liveness probe via the page's IN-PAGE ``fetch()`` (real Firefox network
        stack — the only client that passes the CDN's Cloudflare fingerprint gate).
        Live iff 200 AND the body is an actual HLS playlist (#EXT…). The recipe's
        page is on the megaplay player origin at this point, so the fetch carries
        the right Origin/Referer the CDN expects."""
        try:
            raw = await rc.page.evaluate(_PROBE_MASTER_JS, url)
            status_s, head = raw.split("|", 1)
            status = int(status_s)
            return (status or None), (status == 200 and head.lstrip().startswith("#EXT"))
        except Exception:  # noqa: BLE001 - eval/network error == not live
            return None, False

    async def _await_getsources(self, captured: dict, cfg: Any) -> str | None:
        """Poll for the getSources request the player fires on init."""
        attempts = getattr(cfg, "capture_attempts", 40)
        delay = getattr(cfg, "capture_delay", 0.5)
        for _ in range(attempts):
            if captured["getsources"]:
                return captured["getsources"]
            await asyncio.sleep(delay)
        return captured["getsources"]

    async def _await_m3u8(self, captured: dict, cfg: Any) -> str | None:
        """Poll for an intercepted playlist (fallback when getSources missed).
        Prefer a 'master' playlist; fall back to the first .m3u8."""
        attempts = getattr(cfg, "capture_attempts", 40)
        delay = getattr(cfg, "capture_delay", 0.5)
        for _ in range(attempts):
            if captured["masters"] or captured["m3u8s"]:
                break
            await asyncio.sleep(delay)
        if captured["masters"]:
            return captured["masters"][0]
        if captured["m3u8s"]:
            return captured["m3u8s"][0]
        return None


def _normalize_url(url: str, base: str) -> str:
    url = url.strip()
    if url.startswith("//"):
        return "https:" + url
    if url.startswith("http://") or url.startswith("https://"):
        return url
    return urljoin(base, url)


def _origin(url: str) -> str:
    from urllib.parse import urlsplit

    s = urlsplit(url)
    return f"{s.scheme}://{s.netloc}"


async def _safe_title(page: Any) -> str:
    try:
        return await page.title()
    except Exception:  # noqa: BLE001
        return ""
