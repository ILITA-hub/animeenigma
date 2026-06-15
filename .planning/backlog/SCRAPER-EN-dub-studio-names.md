---
id: SCRAPER-EN-dub-studio-names
title: Extract / crowd-source EN dub-studio (translation team) names
captured_at: 2026-06-15
captured_during: feedback triage of 2026-06-13T02-19-47_tNeymik_telegram (scraper capability-metadata brainstorm)
deferred_from: scraper capability-metadata layer (2026-06-15) — provider-as-rankable-unit shipped instead; real EN team names carved out
status: backlog
---

# EN dub-studio / translation-team names for the OurEnglish scraper providers

## Context

The 2026-06-13 owner TODO («придумать как надёжно разобраться со всратыми
провайдерами … не понятно у кого есть сабы у кого дабы») asked, among other things,
to **get dub team names like Kodik does** and **rank EN dub providers and teams**.

The capability-metadata brainstorm that followed established a hard constraint
(verified by reading all eight EN provider clients):

> **None of the EN scraper providers expose a translation-team / fansub / dub-studio
> name.** Every `Server.Name` is a host/CDN/source label only — `kwik`, `1anime`,
> `tserver`, `HD-1` / `HD-2` / `StreamHG`, `S-mp4` / `Yt-mp4` / `Default`,
> `kiwi` / `dune` / `hop` / `bee`, `mp4upload` / `turbovid`.

This is structural: EN aggregators serve the *official* licensed sub/dub, so there is
no competing-team concept the way Russian Kodik/AniLib have (NekoSama, Anilibria,
Dream Cast, …). Those RU parsers already model real teams via
`Translation.Title` (`services/catalog/internal/parser/kodik/client.go`) and
`Team.Name` (`services/catalog/internal/parser/animelib/client.go`); the gap is purely
on the EN side, where the upstream data does not exist.

The shipped capability layer therefore treats the **provider** (gogoanime / animepahe /
allanime / …) as the rankable unit for EN and ranks per `(anime, category)` by
health / playability / quality. This backlog item is the deferred, larger effort to
recover **real EN team/studio names**.

## The idea (what to build if/when picked up)

Recover a human-meaningful "translated by" label for EN streams, so the (future)
player can show "English dub — Funimation/Crunchyroll" instead of just "gogoanime".

Candidate data routes, roughly easiest → hardest:

1. **Licensor mapping via metadata APIs.** AniList / MAL / Shikimori expose
   `studios`, `licensors`, and external streaming links per title. Most official
   English dubs map 1:1 to a licensor (Crunchyroll/Funimation, Sentai/HIDIVE,
   Netflix, etc.). Build an `(anime → en_dub_studio)` mapping from these fields.
   Coverage: decent for popular/licensed titles, near-zero for the long tail.
2. **Curated overrides file.** Operator-editable YAML (mirror `scraper-providers.yaml`)
   mapping anime → dub studio for the cases the API route gets wrong or misses.
3. **Crowd-sourcing.** Let users submit/confirm the dub studio per title; persist +
   moderate. Highest coverage ceiling, highest build + moderation cost.

Wire whichever route(s) into the capability model's **already-reserved optional `team`
field** (see the capability spec) so this slots in without a schema change.

## Why deferred

- **Low yield for the effort.** The upstream EN providers give us nothing; every route
  above is net-new infrastructure (metadata joins, a mapping store, possibly a
  moderation surface) for a label that is "official dub" ~all the time.
- The owner-confirmed near-term win is **declutter + reliable provider ranking**, which
  the capability layer delivers without any team database.
- The capability model reserves an optional `team` field, so adopting this later is
  additive — no rework of the shipped layer.

## Cost estimate

| Component | Effort (Fib) | Risk |
|---|---|---|
| Licensor mapping from AniList/MAL (route 1) | 8 | Medium — coverage gaps, stale licensor data |
| Curated overrides YAML (route 2) | 3 | Low |
| Crowd-source submit + moderate (route 3) | 21 | High — UX + abuse/moderation |
| Wire into reserved `team` field | 2 | Low (field pre-reserved) |

## Cross-references

- Brainstorm / capability spec: `docs/superpowers/specs/2026-06-15-scraper-capability-api-design.md` (forthcoming)
- RU team models to mirror: `services/catalog/internal/parser/kodik/client.go` (`Translation.Title`),
  `services/catalog/internal/parser/animelib/client.go` (`Team.Name`)
- Provider metadata precedent: `services/scraper/internal/config/providers.go` + `scraper-providers.yaml`
- Source feedback: `2026-06-13T02-19-47_tNeymik_telegram`
