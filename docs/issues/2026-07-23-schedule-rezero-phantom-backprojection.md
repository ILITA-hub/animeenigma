# ISS-034 — Re:Zero S4 shows phantom weekly episodes on the schedule (frontend back-projection)

- **Date:** 2026-07-23
- **Status:** ✅ Resolved 2026-07-23
- **Severity:** Medium — user-facing wrong airing dates for ongoing anime on hiatus. No data loss.
- **Reported as:** "Despite 5 fix sessions, Re:Zero 4 is still shown as scheduled now, but should be only in August."
- **Canonical case:** Re:Zero kara Hajimeru Isekai Seikatsu 4th Season — `shikimori_id=61316`, uuid `0d23c011-b1b0-4a84-ba7b-f743fcc1a910`

## Symptom

The schedule page renders Re:Zero S4 as airing in the **current** week and the two weeks after it, with fabricated episode numbers:

| Week viewed | Rendered | Reality |
|---|---|---|
| Jul 20 – Jul 26 (current) | **ep 9 on Wed Jul 22** | nothing airs; ep 9 aired ~Jun 3 |
| Jul 27 – Aug 2 | **ep 10 on Wed Jul 29** | nothing airs |
| Aug 3 – Aug 9 | **ep 11 on Wed Aug 5** | nothing airs; ep 11 aired ~Jun 17 |
| Aug 10 – Aug 16 | ep 12 on Wed Aug 12 | ✅ correct — the only real entry |

Reproduced by running the deployed `projectOccurrences` against the live API payload
(`next_episode_at=2026-08-12T13:00:00Z`, `episodes_aired=11`, `episodes_count=19`).
In timezones ≥ UTC+12 the anchor shifts a day and the phantom lands on **today** (Jul 23) instead of Jul 22.

## Primary root cause — frontend back-projection

`frontend/web/src/composables/schedule/projection.ts` (current `main`, commit `40a6b581`) reconstructs *past*
occurrences by stepping **backward in 7-day strides from the future anchor**:

```ts
const kFrom = Math.floor((startMs - anchorMs) / WEEK_MS) - 1
const kTo   = Math.min(0, Math.ceil((endMs - anchorMs) / WEEK_MS) + 1)
for (let k = kFrom; k <= kTo; k++) {
  const ms = anchorMs + k * WEEK_MS          // date  = anchor + k weeks
  const episode = aired + 1 + k              // ep    = episodes_aired + 1 + k
  ...
}
```

This assumes the series aired **every week without interruption up to the anchor**. That assumption is exactly
what `next_episode_at` violates for a hiatus title: AniList corroboration deliberately pushes the anchor past the
gap (see [`project_anilist_airing_corroboration`](../../docs/superpowers/specs/2026-06-25-anilist-airing-corroboration-design.md)),
so every 7-day step backward from it walks **through the hiatus** and invents an airing per week.

Arithmetic for Re:Zero: `Aug 12 − 3 weeks = Jul 22`, `episode = 11 + 1 − 3 = 9`. The `episode >= 1` guard does not
fire (9 ≥ 1), and the `episode <= episodes_count` guard does not fire (9 ≤ 19), so nothing stops it.

The defect is **structural, not off-by-one**: even for episodes that genuinely aired, back-projection produces
the wrong date *and* the wrong number. Real ep 11 aired on Jun 17; this code claims it airs Aug 5. Correct past
reconstruction requires provider-confirmed occurrence history. A weekly reconstruction from a season start date
is also unsafe because it silently fabricates dates across any earlier delay or skipped week.

## Secondary defect — backend anchor provenance was not durable on every path

The first live check showed the expected AniList anchor
(`next_episode_at=2026-08-12T13:00:00Z`, `next_episode_source=anilist`), but a later check during implementation
found it had regressed to the Shikimori date (`2026-07-29`) with no source provenance.

Two metadata paths could still overwrite a defended AniList anchor:

- the generic external-anime upsert used by Shikimori-backed flows;
- the direct single-anime refresh path.

Both bypassed `defendAniListNextEpisode`, and `AnimeMetadataEqual` did not compare `next_episode_source`, so a
provenance-only correction could be treated as no change. The incident therefore had two active layers: the
browser fabricated occurrences from any future anchor, and some backend paths could still degrade that anchor.

## Why five fix sessions did not settle it

| # | Date | Commit | Change | Effect on this symptom |
|---|---|---|---|---|
| 1 | Jul 16 | `0972d1de` | catalog: add `next_episode_source` to the `animeMetadataColumns` `Select()` allowlist — without it every AniList correction was silently dropped and clobbered back within 24 h | Necessary; fixed the **anchor**. Frontend still projects around it |
| 2 | Jul 18 | `ad6e6c48` | catalog: widen `GetSchedule` to `NOW() - 7 days` so stale-anchor ongoing anime stop vanishing (AUTO-632) | Opposite symptom (missing rows), unrelated |
| 3 | Jul 20 | `5d2fefef` | catalog: respect AniList rate limit in the reconciler | Corroboration coverage, unrelated |
| 4 | Jul 21 | `6094cd6e` | **frontend: `kFrom = Math.max(0, …)` — no back-projection.** Added regression test `keeps a hiatus title out of weeks before its confirmed next airing` (anchor Aug 12, window Jul 20–27, expects `[]`) | **This actually fixed it** |
| 5a | Jul 22 12:41 | `8c7d7a07` | frontend: anchor-only — return the single confirmed airing, drop forward projection too | Still correct for Re:Zero |
| 5b | Jul 22 12:44 | `40a6b581` | frontend: "retain entries on past dates" — **re-added backward projection and deleted the hiatus regression test** from 5a/#4, flipping its sibling to assert episodes `[8, 9, 10]` are visible before the anchor | **Reintroduced the phantom, 3 minutes after it was fixed** |

Commits 3–5b were Codex-co-authored. The web container was rebuilt 2026-07-22 16:15 CEST, so `40a6b581` — the
regressed version — is what is live.

The two fix directions were oscillating between two real requirements:

- **A.** Navigating to an earlier week must still show episodes that already aired (drove 5b).
- **B.** A hiatus must not manufacture weekly episodes in the gap (drove #4/5a).

Each session optimised one and broke the other. 5b additionally removed the guard test protecting B, so the
suite went green on the regression.

## Resolution

1. **The frontend no longer projects in either direction.** `next_episode_at` contributes exactly one confirmed
   upcoming occurrence. Past dates come only from confirmed history returned by the catalog.
2. **Catalog stores durable, correction-safe occurrences.** AniList `airingSchedule` nodes are upserted by
   `(anime_id, episode)` with timestamp and source provenance. When `episodes_aired` advances, the previously
   confirmed next anchor is also captured as history, so daily refreshes preserve new airings between full AniList
   reconciliations.
3. **A bounded historical API feeds every schedule view.** `GET /api/anime/schedule/occurrences?from=…&to=…`
   returns exact occurrences and their anime metadata for at most 70 days. The frontend refreshes this range as the
   user navigates month, week, or table views and merges it with the exact upcoming anchor.
4. **All metadata update paths preserve anchor provenance.** Generic upsert, direct refresh, batch refresh, and
   calendar reconciliation now run the same AniList defense. Metadata equality includes `next_episode_source`.
   `GetSchedule` returns only genuinely upcoming anchors instead of retaining stale anchors for client projection.
5. **Both requirements are locked by tests.** The restored Re:Zero fixture proves the July hiatus weeks stay empty;
   its companion proves episode 11 appears on Jun 17 from confirmed history. Additional tests cover historical
   released anime, malformed/unrelated occurrence filtering, correction-safe persistence, hidden-anime exclusion,
   AniList history parsing, and provenance comparison.

## Process lesson

A regression test was deleted to make a suite pass, three minutes after the commit that added it. Any session that
removes a test whose name describes a previously-fixed bug should treat that as a blocking signal, not a cleanup —
and re-derive whether the two requirements can both hold before choosing one.

## References

- `frontend/web/src/composables/schedule/projection.ts` — exact occurrence merge; no projection
- `frontend/web/src/composables/useScheduleCalendar.ts` — month, week, and table consumers
- `frontend/web/src/composables/schedule/__tests__/projection.spec.ts` — hiatus and confirmed-history regressions
- `services/catalog/internal/domain/anime_airing_occurrence.go` — durable occurrence model
- `services/catalog/internal/repo/anime_airing_occurrence.go` — correction-safe persistence and bounded reads
- `services/catalog/internal/service/calendar_anilist.go` — AniList history ingestion and anchor capture
- `services/catalog/internal/handler/anime.go` — bounded historical schedule endpoint
- `libs/idmapping/client.go` — AniList airing-history query
- `docs/superpowers/specs/2026-06-25-anilist-airing-corroboration-design.md` — why the anchor jumps the hiatus
