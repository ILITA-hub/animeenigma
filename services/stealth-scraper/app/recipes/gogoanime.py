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

    async def _resolve_player_url(self, rc: RecipeContext, episode_url: str) -> str:
        await self._goto(rc, episode_url)
        # The active server's embed lives in data-video (or the play iframe src).
        embed = await rc.page.evaluate(
            """() => {
                const el = document.querySelector('[data-video]');
                if (el) return el.getAttribute('data-video');
                const f = document.querySelector('.play-video iframe, iframe#playerframe, iframe');
                return f ? f.getAttribute('src') : null;
            }"""
        )
        if not embed:
            raise RecipeError("gogoanime: no data-video / player iframe on episode page")
        embed = _normalize_url(embed, episode_url)

        # A gogoanime.me.uk wrapper nests the real megaplay/vidwish iframe.
        if host_of(embed) and any(h in host_of(embed) for h in _MEGAPLAY_PLAYER_HOSTS):
            return embed
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
        return _normalize_url(nested, embed)

    async def resolve(self, rc: RecipeContext) -> dict[str, Any]:
        episode_url = await self._resolve_episode_url(rc)
        player_url = await self._resolve_player_url(rc, episode_url)

        await self._goto(rc, player_url)
        player_origin = _origin(player_url)
        referer = player_origin + "/"

        # data-id is the canonical id megaplay feeds to getSources (NOT the path
        # id — reusing the path id silently returns the wrong episode).
        data_id = await rc.page.evaluate(
            """() => {
                const el = document.querySelector('[data-id]');
                return el ? el.getAttribute('data-id') : null;
            }"""
        )
        if not data_id:
            raise RecipeError("megaplay: no data-id on player page")

        sources_url = f"{player_origin}/stream/getSources?id={quote(str(data_id))}"
        # Fetch getSources FROM the player page context, so it carries the
        # browser's cookies + Origin + fingerprint (and any clearance).
        raw = await rc.page.evaluate(
            """async (url) => {
                const r = await fetch(url, {
                    headers: { 'X-Requested-With': 'XMLHttpRequest' },
                    credentials: 'include',
                });
                return { status: r.status, body: await r.text() };
            }""",
            sources_url,
        )
        if looks_like_challenge(raw.get("status"), raw.get("body")):
            raise ChallengeError(
                f"challenge on getSources (status={raw.get('status')})",
                host=host_of(sources_url),
                kind="getsources",
            )
        if raw.get("status") != 200:
            raise RecipeError(f"megaplay getSources status {raw.get('status')}")
        try:
            payload = json.loads(raw["body"])
        except (ValueError, KeyError) as exc:
            raise RecipeError(f"megaplay getSources: bad JSON: {exc}") from exc

        session = parse_getsources(payload)
        session["referer"] = referer

        # Probe the CDN master.m3u8 from the player origin (mirrors hls.js) to
        # trigger/solve the CDN's Cloudflare challenge against THIS browser+IP,
        # so the clearance cookie lands in the context jar for harvest.
        cdn_probe = await rc.page.evaluate(
            """async (url) => {
                try {
                    const r = await fetch(url, { credentials: 'include' });
                    let head = '';
                    try { head = (await r.text()).slice(0, 256); } catch (e) {}
                    return { status: r.status, head };
                } catch (e) {
                    return { status: 0, head: String(e) };
                }
            }""",
            session["master_url"],
        )
        status = cdn_probe.get("status")
        session["cdn_probe_status"] = status
        if looks_like_challenge(status, cdn_probe.get("head")):
            raise ChallengeError(
                f"challenge on CDN master.m3u8 (status={status})",
                host=host_of(session["master_url"]),
                kind="cdn",
            )
        return session


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
