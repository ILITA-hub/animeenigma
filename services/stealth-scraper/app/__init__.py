"""stealth-scraper — Camoufox-based browser scraping sidecar for AnimeEnigma.

A standalone, internal-only HTTP service that resolves provider stream sources
through a real (anti-detect Firefox / Camoufox) browser, so we can pass the
JS/fingerprint/clearance challenges that a plain Go ``net/http`` client (curl-
class) cannot. Phase 1 target: gogoanime → megaplay (``cdn.mewstream.buzz``).

See ``docs/superpowers/plans/2026-06-20-stealth-browser-scraper-framework.md``.
"""

__version__ = "0.1.0"
