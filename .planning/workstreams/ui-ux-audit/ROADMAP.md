# Roadmap — `ui-ux-audit` workstream

**Workstream:** ui-ux-audit
**Active milestone:** None — v0.1 shipped 2026-05-13

## Shipped milestones

- **v0.1 — UX Reassessment Remediation** (2026-05-13) — All 20 phases complete, 36/36 requirements closed. Closes 2026-05-12 audit findings (catastrophic security + a11y + UX) + Tier E strategic features (Continue-Watching, multi-axis filter sidebar, broadcast schedule, editorial collections, Skip-Intro). See [`milestones/v0.1-ROADMAP.md`](milestones/v0.1-ROADMAP.md) + [`v0.1-MILESTONE-AUDIT.md`](v0.1-MILESTONE-AUDIT.md).

## Next milestone

To start a new milestone in this workstream, run:

```
/gsd-new-milestone --ws ui-ux-audit
```

Suggested follow-ups for v0.2 (from deferred items + tech debt aggregated in v0.1-MILESTONE-AUDIT.md):

- True "Because you watched X" rec chip with backend seed-tracking
- Eager backfill of provider booleans (HasKodik/HasAnimeLib/HasHiAnime/HasConsumet) + HasDub
- Tier-E strategic phase follow-up mini-audits (UX-NF-04)
- AnimeLib + Kodik Skip-Intro integration (requires player abstraction work in root milestone)
- Editorial Collections — drag-and-drop reorder + MinIO cover upload
- `libs/streamprobe` Dockerfile COPY cleanup across 5 services
