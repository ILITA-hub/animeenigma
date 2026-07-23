# ISS-034 — Re:Zero S4 shows phantom weekly episodes on the schedule (frontend back-projection)

- **Date:** 2026-07-23
- **Status:** 🔬 Researched, **NO FIX APPLIED** (owner asked for research only)
- **Severity:** Medium — user-facing wrong airing dates for every ongoing anime on hiatus. No data loss; backend is correct.
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

## Root cause — frontend only

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

The defect is **structural, not off-by-one**: even for the episodes that genuinely aired, back-projection produces
the wrong date *and* the wrong number. Real ep 11 aired around Jun 17; this code claims it airs Aug 5. Correct past
reconstruction requires real aired history (e.g. `aired_on` + weekly-from-ep-1, or a `last_episode_aired_at` field
added to the schedule feed) — it cannot be derived from a future anchor.

## What is NOT broken (verified live 2026-07-23)

The entire backend chain is correct and needs no change:

| Layer | Evidence |
|---|---|
| DB row | `next_episode_at = 2026-08-12 13:00:00+00`, `next_episode_source = anilist`, refreshed 2026-07-22 13:32 — the AniList value survives the nightly Shikimori refresh, so the `0972d1de` allowlist fix holds |
| `calendar_sync` | `job_successes.last_success_at = 2026-07-20 04:00:54+00` (weekly Mon 04:00 cron ran) |
| `GetSchedule` | `services/catalog/internal/repo/anime.go:543` — ongoing + `next_episode_at > NOW() - INTERVAL '7 days'`; Re:Zero passes |
| `GET /api/anime/schedule` | returns `"next_episode_at":"2026-08-12T13:00:00Z"`, `"next_episode_source":"anilist"` for shikimori_id 61316 |

The wrong dates are created in the browser, after a correct payload arrives.

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

## Constraints for a real fix

1. **Never derive past occurrences from the future anchor.** Past entries need actual aired history — either
   `aired_on` + weekly-from-episode-1 (bounded by `episodes_aired`), or a new last-aired timestamp on the schedule
   payload. Anything that steps backward from `next_episode_at` reintroduces this bug.
2. **Keep forward projection off.** `next_episode_at` confirms exactly one airing; projecting past it re-creates the
   original AUTO-632-era phantom-future problem.
3. **Restore the deleted regression test verbatim** (`keeps a hiatus title out of weeks before its confirmed next
   airing`) plus a companion asserting requirement A, so the next session cannot satisfy one by deleting the other.
4. Re:Zero S4 (anchor Aug 12, `episodes_aired=11`, `episodes_count=19`) is the standing fixture for both.

## Process lesson

A regression test was deleted to make a suite pass, three minutes after the commit that added it. Any session that
removes a test whose name describes a previously-fixed bug should treat that as a blocking signal, not a cleanup —
and re-derive whether the two requirements can both hold before choosing one.

## References

- `frontend/web/src/composables/schedule/projection.ts` — defect site
- `frontend/web/src/composables/useScheduleCalendar.ts:53,72,88` — the three consumers (month, week, table views)
- `frontend/web/src/composables/schedule/__tests__/projection.spec.ts` — test file the guard was removed from
- `services/catalog/internal/repo/anime.go:543` — `GetSchedule` (verified correct)
- `services/catalog/internal/service/calendar_anilist.go` — AniList corroboration (verified correct)
- `docs/superpowers/specs/2026-06-25-anilist-airing-corroboration-design.md` — why the anchor jumps the hiatus
