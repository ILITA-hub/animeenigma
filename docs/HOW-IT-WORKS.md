# How AnimeEnigma Works

A self-hosted anime streaming platform that combines Shikimori catalog data with Kodik, the EN scraper roster, AnimeJoy, adult sources, and first-party storage through one Vue frontend with personalized recommendations, multilingual subtitles, and closed-loop watch progress.

This document is the high-level walkthrough — for architecture deep dives see [`CLAUDE.md`](../CLAUDE.md), and for the v2.0 recommendations engine design spec see [`docs/superpowers/specs/2026-05-03-rec-engine-design.md`](superpowers/specs/2026-05-03-rec-engine-design.md).

---

## 1. The Catalog Is Built On Demand

There is no pre-populated database. When you search for an anime:

1. The frontend hits the catalog service.
2. The catalog queries Shikimori's GraphQL API.
3. Results are mapped by **original Japanese name** as the primary key (so the same show resolves consistently across providers) and stored in Postgres.
4. The next search hits the local DB first; Shikimori is only queried when something new is asked for.

This means the catalog grows organically with what users actually look for — no nightly imports, no millions of unwatched rows.

Shikimori provides metadata: titles in three languages, descriptions, posters, genres, episode counts, scores, airing status, kind (TV / Movie / OVA / etc.), rating (G / PG / R+ / etc.), source material (manga / light novel / original / etc.), and studios.

For tags, the catalog cross-references each anime to AniList via the [ARM mapping service](https://arm.haglund.dev/api/v2) and pulls AniList's per-anime tag list. ARM also lets watch progress carry across MyAnimeList and AniList for users who import their lists.

---

## 2. One Unified Player (aePlayer)

There used to be a separate Vue component per source. Now there is **one** player — `aePlayer/AePlayer.vue` — and every source plays through it. Inside it, an in-player **Source** panel lets you switch between source *families*:

| Source family | Language | Tech | Japanese subs |
|--------|----------|------|---------------|
| **Kodik** | RU dub | Kodik HLS | No |
| **EN scraper chain** | EN sub | HTML5 `<video>` + hls.js / MP4 | Yes |
| **Raw** | JP (original) | HTML5 `<video>` + hls.js / MP4 | Yes (Jimaku + others) |
| **Hanime / 18anime** | 18+ | HTML5 `<video>` + hls.js | No |
| **ae** | EN/RU/JP | HTML5 `<video>` + hls.js (self-hosted) | Yes |

You pick a **combo**: a top slider chooses **RAW** (original audio — EN-sub, RU-sub, and pure-JP sources all in one list) or **DUB** (localized audio; a second slider picks EN or RU — there is no Japanese dub). The player then shows the providers the backend says are healthy for *this* title and auto-picks the best one; you can override it. The legacy "Classic Kodik" iframe is still available as a one-click fallback.

When a source ships an HLS or MP4 stream, the backend proxies (and signs) it for CORS — the browser talks to AnimeEnigma's gateway, not directly to the streaming CDN. The gateway also routes Japanese subtitle files from [jimaku.cc](https://jimaku.cc).

Subtitles are **off by default** — EN/RU sources usually have subs burned into the video, and for raw-JP you opt into a Jimaku/OpenSubtitles overlay via the CC menu. The `SubtitleOverlay.vue` component renders ASS / SRT / VTT tracks as selectable text on top of the video — useful for copy-pasting Japanese lines into a dictionary while you watch.

The "Watch Preference Resolver" remembers the audio/language/source combo you used per-anime and auto-picks it next time, never crossing language boundaries.

> **For the full, code-verified player model** (the combo state machine, provider groups, capability feed, subtitle behavior, URL deep-linking), see [`docs/aeplayer-reference.md`](aeplayer-reference.md).

---

## 3. Recommendations (v2.0 — shipped 2026-05-07)

The home page has two recommendation rows:

- **Trending now** — shown to anonymous (logged-out) users. Backed by population-wide signals (S3: last-30-day watch count, S4: ongoing/recent boost). Refreshed every 60 minutes by a cron job; cached in Redis for 6 hours.
- **Up Next for you** — shown to logged-in users. Backed by the full ensemble:

  ```
  final_score = 0.30·S1 + 0.20·S2 + 0.20·S3 + 0.10·S4 + 0.20·S5
  ```

  Where:
  - **S1 — Score-cluster k-NN.** Pearson correlation over your `anime_list` scores vs other users' scores, predicts your score for unwatched anime. Cold-start at &lt; 3 scored anime → contributes 0.
  - **S2 — Item-item metadata overlap.** Jaccard similarity over genres (and tags / studios when AniList tags are populated for that anime) between your top-scored anime and each candidate.
  - **S3 — Population trending.** Same as the anonymous flow.
  - **S4 — Recency boost.** 1.0 for currently-airing anime, 0.7 for anime that aired in the last 90 days, 0 otherwise.
  - **S5 — TF-IDF attribute affinity.** Time-weighted TF-IDF across six dimensions: tags (0.30), studios+producers (0.25), genres (0.15), demographic (0.10), source (0.10), kind (0.10). The Classic Kodik iframe cannot expose reliable duration and falls back to episode count; aePlayer's HTML5 sources record real watched duration.

Each signal goes through a **per-pool min-max normalizer** before the weighted sum, so weights are coherent across signals at very different raw scales.

The user-signal precompute runs every 6 hours on a cron. It also fires within ~5 minutes of any new `watch_history` row via a Redis-debounced trigger, so the row updates as you watch instead of waiting until the next cron tick.

### S6 — "Because you finished X" pin

When you mark an anime complete with a score &ge; 7, a pinned tile appears at the top of your "Up Next for you" row labeled "Because you finished {anime name}". The pin lasts 7 days or until you complete a newer high-scored anime.

The pin candidate is found by a **cascade**:

1. **Local co-occurrence** — query `rec_completion_co_occurrence` (a materialized table of seed→candidate pairs where two users completed both with score &ge; 7), sort by overlap count.
2. **Shikimori `/similar` fallback** — if the local pool yields fewer than 5 candidates after the user's filter, hit `https://shikimori.io/api/animes/:id/similar` for community "viewers also liked" data.
3. **Score-5 fallback** — if both pools are empty after filtering, retry the local query with the threshold dropped to score &ge; 5. Never goes lower than 5 (the spec explicitly forbids score &gt; 0 because it could surface "more like the thing they hated").

The seed update is **synchronous** during `MarkEpisodeWatched` so the pin appears the next time you load the home page — no waiting on a cron.

### What's filtered out

S11 (the candidate filter) excludes:
- Anime where your `anime_list.status` is `completed` or `dropped`
- Anime where the admin has set `hidden = true`

### Auditability

Admins can visit `/admin/recs/:user_id` to see the full breakdown for any user — a top-50 table with each signal's contribution per row, the S5 TF-IDF term breakdown for any row (e.g. "studio: Madhouse, tf 0.41, idf 2.30"), the S6 cascade source ("local" / "shikimori_similar" / "score_5_fallback"), and a separate panel listing every anime S11 removed and why.

There's also a "Force recompute" button that busts the user's Redis cache and re-runs the user precompute synchronously (returns in ~10ms). Useful for debugging or for refreshing recs after a long idle period.

---

## 4. Watch Progress Is The Source Of Truth

The platform tracks watch progress across aePlayer's HTML5 sources. The separate Classic Kodik iframe has limited event access and uses its compatibility path.

- `watch_progress` table — per-episode position. `completed = true` is the **single source of truth** for "I finished this episode" across all surfaces (the v1.0 milestone refactored away three competing definitions).
- `watch_history` — append-only log of session starts and ends. Carries player, language, watch type, translation ID, duration watched.
- `anime_list` — your manual list (status, score, completed_at). Driven by you, but `episodes` count auto-bumps from `watch_progress` completions.

The episode picker in every player follows a state machine:
- **Watching** — return to your in-progress episode.
- **Finished** — offer the next unaired episode (or rewatch if it's a finished show).
- **Not yet aired** — show the airing schedule.

Anonymous users get a localStorage equivalent so the experience is the same shape — your progress just doesn't sync across devices until you sign in.

---

## 5. Privacy, Self-Hosting, And Telemetry

- **Self-hosted by default.** No CDN, no analytics-as-a-service. The whole stack runs on a single VPS or homelab box; the architecture is sized for small groups, not millions of users.
- **No tracking pixels.** The frontend emits internal `rec_click` and `rec_watched` events to the backend (`/api/events/rec`) so the recommendation engine can compute per-signal click-through-rates in Grafana — but those events never leave the server.
- **API keys for automation.** Generate an `ak_…` key in Profile → Settings → API Key. The gateway resolves API keys to a JWT internally so downstream services don't need to know about the prefix.
- **MAL / AniList import.** Bring your existing watchlists in via the import endpoints. ARM resolves Shikimori → MAL → AniList IDs without needing manual mapping.
- **Admin endpoints are role-gated.** `role = admin` JWT claim required at the middleware layer, plus a frontend route guard for UX. Defense-in-depth — the route guard is decorative; the middleware is the actual security boundary.

---

## 6. Tech Stack

- **Backend:** Go microservices under `services/` with GORM, Postgres, Redis, and shared libraries under `libs/`. Each service owns its `go.mod`.
- **Frontend:** Vue 3 + Vite + Tailwind + Pinia + vue-i18n (EN / RU / JA). Built with `bun`.
- **Infrastructure:** Docker Compose for local dev, Kubernetes manifests in `deploy/kustomize/` for production. Prometheus metrics on every service's `/metrics`; Grafana dashboards in `infra/grafana/dashboards/`.
- **External APIs:** Shikimori (catalog), AniList (tags via ARM mapping), MyAnimeList (optional sync), Kodik and the DB-backed scraper/catalog provider roster (video), and Jimaku/OpenSubtitles (subtitle files).

---

## 7. Where Things Live

| Concern | Path |
|---------|------|
| Catalog & Shikimori parser | `services/catalog/` |
| Recommendations engine | `services/recs/` |
| Admin debug page | `frontend/web/src/views/admin/AdminRecs.vue` + `services/recs/internal/handler/admin_recs.go` |
| Subtitle overlay | `frontend/web/src/components/player/SubtitleOverlay.vue` |
| Watch preference resolver (v1.0) | `services/player/internal/service/preference/` |
| Design specs | `docs/superpowers/specs/` |
| Milestone history | `.planning/milestones/` (v1.0 + v2.0 archives) |

---
