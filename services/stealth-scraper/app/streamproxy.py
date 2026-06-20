"""In-sidecar HLS restream proxy.

Proxying the stream is MANDATORY: a megaplay master.m3u8 + its `.ts` segments
sit behind the same Cloudflare clearance that the browser solved, and that
clearance is bound to (exit IP, User-Agent). So the playlist + every segment
MUST be fetched through the SAME browser context (same proxy exit, same cookie
jar, same TLS fingerprint) that resolved the session — anything else re-triggers
the challenge. This module owns that: it rewrites playlists so every child URI
routes back through this sidecar, and the engine fetches each hop via the
session's Playwright APIRequestContext.

``rewrite_playlist`` is pure and unit-tested. The async fetch lives in the
engine (it needs the live browser context).
"""

from __future__ import annotations

import re
from typing import Callable
from urllib.parse import urljoin

# Attributes that carry a nested URI inside an #EXT tag.
_URI_ATTR = re.compile(r'(URI=")([^"]+)(")')


def looks_like_m3u8(body: str, content_type: str | None = None) -> bool:
    if content_type and "mpegurl" in content_type.lower():
        return True
    head = body.lstrip()[:64].upper()
    return head.startswith("#EXTM3U")


def rewrite_playlist(body: str, playlist_url: str, wrap: Callable[[str], str]) -> str:
    """Rewrite every child URI (variant playlists, segments, keys, maps, media
    renditions) so it is absolute then passed through ``wrap`` — which returns
    the sidecar proxy URL that re-fetches it via the same session.

    ``wrap`` receives an absolute URL and returns the replacement string.
    """
    out: list[str] = []
    for raw in body.splitlines():
        line = raw.rstrip("\r")
        if not line:
            out.append(line)
            continue
        if line.startswith("#"):
            # Rewrite a URI="..." attribute if present (EXT-X-KEY / -MAP /
            # -MEDIA / -I-FRAME-STREAM-INF / -PART / -PRELOAD-HINT / ...).
            if "URI=" in line:
                def _sub(m: re.Match) -> str:
                    abs_url = urljoin(playlist_url, m.group(2))
                    return f"{m.group(1)}{wrap(abs_url)}{m.group(3)}"

                line = _URI_ATTR.sub(_sub, line)
            out.append(line)
            continue
        # Bare line = a media-segment or variant-playlist URI.
        abs_url = urljoin(playlist_url, line)
        out.append(wrap(abs_url))
    return "\n".join(out)


def make_wrap(sid: str, builder: Callable[[str, str], str]) -> Callable[[str], str]:
    """Build a ``wrap`` closure binding the session id; ``builder(sid, url)``
    formats the actual proxy URL (kept injectable so the path/prefix is decided
    by the caller, not hardcoded here)."""

    def _wrap(abs_url: str) -> str:
        return builder(sid, abs_url)

    return _wrap
