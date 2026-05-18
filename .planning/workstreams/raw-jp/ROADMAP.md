# Roadmap: AnimeEnigma `raw-jp` workstream

**Workstream:** raw-jp (parallel to root v3.0 Universal Anime Scraper)
**Active milestone:** None (between milestones)
**Phase numbering:** Workstream-local — restarts at 1 inside each milestone (`v0.1-phases/01-*`, `v0.2-phases/01-*`).
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`

## Milestones

- ✅ **v0.1 Raw Provider MVP** — Shipped 2026-05-18 — see [`milestones/v0.1-SUMMARY.md`](milestones/v0.1-SUMMARY.md)
- ✅ **v0.2 Self-Hosted Library** — Shipped 2026-05-18 — see [`milestones/v0.2-SUMMARY.md`](milestones/v0.2-SUMMARY.md) (audit: [`v0.2-MILESTONE-AUDIT.md`](v0.2-MILESTONE-AUDIT.md))
- ⏳ **v0.3 Auto-Download Watched Ongoings** (planned) — RSS poller + admin oversight

## v0.2 phases (shipped — kept for reference)

All six v0.2 phases shipped 2026-05-18. Detail: [`milestones/v0.2-ROADMAP.md`](milestones/v0.2-ROADMAP.md) + [`milestones/v0.2-SUMMARY.md`](milestones/v0.2-SUMMARY.md).

| Phase | Title                                       | Status |
|-------|---------------------------------------------|--------|
| 1     | Library Service Scaffold                    | Done ✓ |
| 2     | Nyaa + AnimeTosho Search Clients            | Done ✓ |
| 3     | Torrent Client + Job Queue + Metrics        | Done ✓ |
| 4     | ffmpeg HLS Transcoder + MinIO Writer        | Done ✓ |
| 5     | RawLibrary.vue Admin UI                     | Done ✓ |
| 6     | Hybrid Resolver                             | Done ✓ |

## v0.1 phases (shipped — kept for reference)

All four v0.1 phases shipped 2026-05-18. Detail: [`milestones/v0.1-ROADMAP.md`](milestones/v0.1-ROADMAP.md) + [`milestones/v0.1-SUMMARY.md`](milestones/v0.1-SUMMARY.md).

## Next

When ready to plan v0.3:

```
/gsd-new-milestone --ws raw-jp
```
